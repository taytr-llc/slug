//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

const dailyActualProcessLimit = 16 << 20

type dailyActualHandoff struct {
	Version    int    `json:"version"`
	ParentPID  int    `json:"parent_pid"`
	ParentPGID int    `json:"parent_pgid"`
	DeadlineNS int64  `json:"scenario_deadline_ns"`
	Root       string `json:"root"`
	Reference  string `json:"reference"`
	Output     string `json:"output"`
	RelayURL   string `json:"relay_url"`
	NodeAURL   string `json:"node_a_url"`
	NodeBURL   string `json:"node_b_url"`
	DayBNS     int64  `json:"day_b_ns"`
	PhaseASHA  string `json:"phase_a_sha256"`
	PhaseBSHA  string `json:"phase_b_sha256"`
}

type dailyActualRecoveryResult struct {
	Version             int      `json:"version"`
	PID                 int      `json:"pid"`
	ParentPID           int      `json:"parent_pid"`
	PGID                int      `json:"pgid"`
	DeadlineNS          int64    `json:"scenario_deadline_ns"`
	HandoffSHA          string   `json:"handoff_sha256"`
	Pass                bool     `json:"pass"`
	Completed           bool     `json:"completed"`
	OwnersClosed        bool     `json:"owners_closed"`
	Reopens             int      `json:"reopens"`
	Failure             string   `json:"failure,omitempty"`
	Phase               string   `json:"phase"`
	CleanupErrors       []string `json:"cleanup_errors"`
	CompleteSHA         string   `json:"complete_sha256"`
	ColdSourceSeconds   float64  `json:"cold_source_seconds"`
	FinalInspectSeconds float64  `json:"final_change_inspect_seconds"`
}

type dailyActualComplete struct {
	Snapshot    poolsettle.DailyActualState `json:"snapshot"`
	Report      poolsettle.Report           `json:"report"`
	Height      uint32                      `json:"height"`
	BBlock      string                      `json:"B_block"`
	SourceID    string                      `json:"source_id"`
	ConfigID    string                      `json:"config_id"`
	GuardDigest string                      `json:"guard_digest"`
}

// These are bounded public fixture files, never a private database/key reader.
func dailyActualPublicBytes(path string, limit int) []byte {
	require(filepath.IsAbs(path) && filepath.Clean(path) == path, "absolute public evidence path")
	st, e := os.Lstat(path)
	check(e, "public evidence lstat")
	require(st.Mode().IsRegular() && st.Size() > 0 && st.Size() <= int64(limit), "bounded regular public evidence")
	f, e := os.Open(path)
	check(e, "public evidence open")
	opened, se := f.Stat()
	b, re := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	ce := f.Close()
	check(errors.Join(se, re, ce), "public evidence read/close")
	require(os.SameFile(st, opened) && len(b) <= limit && int64(len(b)) == st.Size(), "same bounded public evidence")
	return b
}

func dailyActualDecodePublic(path, hash string, limit int, out any) {
	b := dailyActualPublicBytes(path, limit)
	require(len(hash) == 64 && hx(digest(b)) == hash, "original public evidence hash")
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	check(d.Decode(out), "public evidence decode")
	require(d.Decode(new(any)) == io.EOF, "one public evidence object")
}

func dailyActualReadComplete(output, hash string) dailyActualComplete {
	var c dailyActualComplete
	dailyActualDecodePublic(filepath.Join(output, "03-complete.json"), hash, dailyActualProcessLimit, &c)
	require(c.Height == 924 && c.Snapshot.Credits == 721 && len(c.Snapshot.Batches) == 2 && c.Snapshot.Pending == 0 && len(c.Snapshot.Frontier) == 1 && len(c.Snapshot.Frontier[0].Claims) == 721, "complete child financial shape")
	return c
}

type dailyActualRecoveryProcess struct {
	cmd        *exec.Cmd
	done       chan error
	log        *os.File
	startError error
	started    bool
	joined     bool
	forced     bool
	closeTried bool
	closeError bool
	waitError  string
	pid        int
	parentPID  int
	pgid       int
	exitCode   int
	deadlineNS int64
	handoffSHA string
	resultSHA  string
	child      dailyActualRecoveryResult
}

func dailyActualLaunchRecovery(ctx context.Context, root, reference, output, relay, nodeA, nodeB string, dayB time.Time) *dailyActualRecoveryProcess {
	deadline, ok := ctx.Deadline()
	require(ok && ctx.Err() == nil && time.Until(deadline) > 0, "remaining original scenario deadline")
	pgid, e := syscall.Getpgid(0)
	check(e, "parent process group")
	h := dailyActualHandoff{Version: 1, ParentPID: os.Getpid(), ParentPGID: pgid, DeadlineNS: deadline.UnixNano(), Root: root, Reference: reference, Output: output, RelayURL: relay, NodeAURL: nodeA, NodeBURL: nodeB, DayBNS: dayB.UnixNano(), PhaseASHA: hx(digest(dailyActualPublicBytes(filepath.Join(output, "01-A.json"), dailyActualProcessLimit))), PhaseBSHA: hx(digest(dailyActualPublicBytes(filepath.Join(output, "02-B-pending.json"), dailyActualProcessLimit)))}
	data := encode(h)
	handoff := filepath.Join(output, "recovery-handoff.json")
	f, e := os.OpenFile(handoff, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(e, "unique public child handoff")
	_, we := f.Write(data)
	check(errors.Join(we, f.Close()), "public child handoff close")
	executable, e := os.Executable()
	check(e, "same compiled test binary")
	p := &dailyActualRecoveryProcess{done: make(chan error, 1), parentPID: os.Getpid(), pgid: pgid, deadlineNS: deadline.UnixNano(), handoffSHA: hx(digest(data)), exitCode: -1}
	p.log, p.startError = os.OpenFile(filepath.Join(output, "recovery-child.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if p.startError != nil {
		return p
	}
	p.cmd = exec.Command(executable, "-test.run=^TestDailyActualReopenChild$", "-test.count=1", "-test.v", "-test.timeout=35m")
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "SLUG_DAILY_") {
			p.cmd.Env = append(p.cmd.Env, item)
		}
	}
	p.cmd.Env = append(p.cmd.Env, "SLUG_DAILY_ACTUAL=1", "SLUG_DAILY_REOPEN_CHILD=1", "SLUG_DAILY_ROOT="+root, "SLUG_DAILY_REFERENCE="+reference, "SLUG_DAILY_OUTPUT="+output, "SLUG_DAILY_HANDOFF="+handoff, "SLUG_DAILY_HANDOFF_SHA="+p.handoffSHA)
	// No SysProcAttr: child stays inside the original outer-owned process group.
	p.cmd.Stdout = &cappedLog{f: p.log}
	p.cmd.Stderr = p.cmd.Stdout
	p.startError = p.cmd.Start()
	if p.startError != nil {
		return p
	}
	p.started, p.pid = true, p.cmd.Process.Pid
	go func() { p.done <- p.cmd.Wait() }()
	return p
}

func (p *dailyActualRecoveryProcess) finish(e error) {
	p.joined = true
	p.exitCode = p.cmd.ProcessState.ExitCode()
	if e != nil {
		p.waitError = fmt.Sprintf("%.256s", e)
	}
	p.closeLog()
}
func (p *dailyActualRecoveryProcess) closeLog() {
	if p.log != nil && !p.closeTried {
		p.closeTried = true
		p.closeError = p.log.Close() != nil
	}
}
func (p *dailyActualRecoveryProcess) wait(ctx context.Context) {
	select {
	case e := <-p.done:
		p.finish(e)
	case <-ctx.Done():
		p.stop()
	}
}
func (p *dailyActualRecoveryProcess) stop() {
	if !p.started {
		p.closeLog()
		return
	}
	if p.joined || p.forced {
		return
	}
	select {
	case e := <-p.done:
		p.finish(e)
		return
	default:
	}
	p.forced = true
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case e := <-p.done:
		p.finish(e)
	case <-time.After(12 * time.Second):
		_ = p.cmd.Process.Kill()
		select {
		case e := <-p.done:
			p.finish(e)
		case <-time.After(3 * time.Second):
			// Keep the failed owner/log reachable for the outer group guard.
		}
	}
}
func (p *dailyActualRecoveryProcess) clean() bool {
	return p.started && p.joined && !p.forced && p.exitCode == 0 && p.waitError == "" && p.closeTried && !p.closeError
}
func (p *dailyActualRecoveryProcess) metadata() map[string]any {
	return map[string]any{"parent_pid": p.parentPID, "launched_child_pid": p.pid, "child_reported_pid": p.child.PID, "child_reported_parent_pid": p.child.ParentPID, "parent_pgid": p.pgid, "child_reported_pgid": p.child.PGID, "exit_code": p.exitCode, "joined": p.joined, "forced": p.forced, "wait_error": p.waitError, "log_close_error": p.closeError, "scenario_deadline_ns": p.deadlineNS, "handoff_sha256": p.handoffSHA, "child_report_sha256": p.resultSHA, "child_completed": p.child.Completed, "child_owners_closed": p.child.OwnersClosed, "child_cleanup_errors": p.child.CleanupErrors, "clean": p.clean()}
}

func dailyActualReadRecoveryResult(p *dailyActualRecoveryProcess, output string) dailyActualRecoveryResult {
	path := filepath.Join(output, "recovery-child-report.json")
	p.resultSHA = hx(digest(dailyActualPublicBytes(path, 65536)))
	dailyActualDecodePublic(path, p.resultSHA, 65536, &p.child)
	c := p.child
	require(c.Version == 1 && c.PID == p.pid && c.PID != p.parentPID && c.ParentPID == p.parentPID && c.PGID == p.pgid && c.DeadlineNS == p.deadlineNS && c.HandoffSHA == p.handoffSHA, "exact original fresh child correlation")
	require(c.Pass && c.Completed && c.OwnersClosed && len(c.CleanupErrors) == 0 && c.Reopens == 1, "child completed with original owners joined")
	return c
}

func dailyActualChildRPC(endpoint string) *node {
	u, e := url.Parse(endpoint)
	check(e, "original loopback endpoint")
	host, port, e := net.SplitHostPort(u.Host)
	check(e, "original explicit node port")
	n, e := strconv.Atoi(port)
	require(e == nil && n > 0 && n <= 65535 && host == "127.0.0.1" && u.Scheme == "http" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "", "fixture-only original loopback endpoint")
	return &node{rpc: &rpc{endpoint, &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}}
}

func TestDailyActualReopenChild(t *testing.T) {
	if os.Getenv("SLUG_DAILY_ACTUAL") != "1" || os.Getenv("SLUG_DAILY_REOPEN_CHILD") != "1" {
		t.Skip("only the original actual parent may select this recovery child")
	}
	var h dailyActualHandoff
	dailyActualDecodePublic(os.Getenv("SLUG_DAILY_HANDOFF"), os.Getenv("SLUG_DAILY_HANDOFF_SHA"), 16384, &h)
	pgid, e := syscall.Getpgid(0)
	check(e, "child inherited process group")
	require(h.Version == 1 && h.ParentPID == os.Getppid() && h.ParentPID != os.Getpid() && h.ParentPGID == pgid && h.Root == os.Getenv("SLUG_DAILY_ROOT") && h.Reference == os.Getenv("SLUG_DAILY_REFERENCE") && h.Output == os.Getenv("SLUG_DAILY_OUTPUT"), "original child selection")
	for _, path := range []string{h.Root, h.Output} {
		actual, e := filepath.EvalSymlinks(path)
		require(e == nil && filepath.IsAbs(path) && filepath.Clean(path) == path && actual == path, "original canonical existing fixture path")
	}
	deadline := time.Unix(0, h.DeadlineNS)
	require(time.Until(deadline) > 0 && time.Until(deadline) <= 30*time.Minute, "original remaining scenario time")
	life, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ctx, cancel := context.WithDeadline(life, deadline)
	defer cancel()
	result := dailyActualRecoverChild(ctx, h, os.Getenv("SLUG_DAILY_HANDOFF_SHA"))
	if !result.Pass {
		t.Fatalf("child recovery failed: phase=%s failure=%s cleanup=%v", result.Phase, result.Failure, result.CleanupErrors)
	}
}

func dailyActualRecoverChild(ctx context.Context, h dailyActualHandoff, handoffSHA string) (out dailyActualRecoveryResult) {
	out = dailyActualRecoveryResult{Version: 1, PID: os.Getpid(), ParentPID: os.Getppid(), PGID: h.ParentPGID, DeadlineNS: h.DeadlineNS, HandoffSHA: handoffSHA, CleanupErrors: []string{}}
	owners := &dailyActualOwners{}
	defer func() {
		if v := recover(); v != nil {
			out.Failure = fmt.Sprintf("%.1024s", fmt.Sprint(v))
		}
		if e := owners.close(); e != nil {
			out.CleanupErrors = append(out.CleanupErrors, fmt.Sprintf("owners: %.256s", e))
		} else {
			out.OwnersClosed = true
		}
		out.Pass = out.Completed && out.Failure == "" && out.OwnersClosed && len(out.CleanupErrors) == 0
		write(filepath.Join(h.Output, "recovery-child-report.json"), out)
	}()
	out.Phase = "original public evidence before fresh-process source open"
	require(!wire.TestPoW, "normal PoW in independent child")
	r := dailyActualReadReference(h.Reference)
	var priorA struct {
		Batch          poolsettle.Batch  `json:"batch"`
		Reward         dailyActualReward `json:"reward"`
		CanonicalBlock string            `json:"canonical_block"`
	}
	var priorB struct {
		Batch   poolsettle.Batch    `json:"batch"`
		Rewards []dailyActualReward `json:"rewards"`
		Report  poolsettle.Report   `json:"report"`
	}
	dailyActualDecodePublic(filepath.Join(h.Output, "01-A.json"), h.PhaseASHA, dailyActualProcessLimit, &priorA)
	dailyActualDecodePublic(filepath.Join(h.Output, "02-B-pending.json"), h.PhaseBSHA, dailyActualProcessLimit, &priorB)
	require(len(priorB.Rewards) == 721 && reflect.DeepEqual(priorA.Reward, priorB.Rewards[0]), "original complete reward evidence")
	originalA, originalB := priorA.Batch, priorB.Batch
	dailyActualCheckBatch(originalA, r, priorB.Rewards, true)
	dailyActualCheckBatch(originalB, r, priorB.Rewards, false)
	a, b := dailyActualChildRPC(h.NodeAURL), dailyActualChildRPC(h.NodeBURL)
	defer a.rpc.c.CloseIdleConnections()
	defer b.rpc.c.CloseIdleConnections()
	checkedRelay := dailyActualChildRPC(h.RelayURL)
	defer checkedRelay.rpc.c.CloseIdleConnections()
	rpc := prospectnode.RPCConfig{Version: 1, URL: h.RelayURL, Username: "fixture", Password: "public-pool-actual"}
	out.Phase = "single fresh-process reopen of all original owners"
	dailyActualOpen(ctx, filepath.Join(h.Root, "owners"), r, rpc, false, owners)
	out.Reopens = 1
	out.ColdSourceSeconds = owners.coldSeconds
	check(poolsettle.DailyActualClock(ctx, owners.ledger, time.Unix(0, h.DayBNS)), "same original day B after reopen")
	snap := dailyActualSnapshot(ctx, owners)
	require(snap.Credits == 721 && len(snap.Batches) == 2 && snap.Pending == 2, "reopened exact original state")
	dailyActualSame(snap.Batches[0], originalA)
	dailyActualSame(snap.Batches[1], originalB)
	report := dailyActualRun(ctx, owners, true)
	require(report.State == "pending" && report.Reason == "payout-pending", "saved exact node B resolves lost reply")
	out.Phase = "B confirmation and one721-origin change"
	bhash := generate(ctx, a, b)
	require(hx(raw(minedTx(ctx, a, bhash, originalB.TxID))) == originalB.Raw, "B canonical exact bytes")
	report = dailyActualRun(ctx, owners, true)
	require(report.State == "complete", "B complete")
	snap = dailyActualSnapshot(ctx, owners)
	require(snap.Credits == 721 && len(snap.Batches) == 2 && snap.Pending == 0 && len(snap.Frontier) == 1, "final complete frontier")
	dailyActualSame(snap.Batches[0], originalA)
	dailyActualSame(snap.Batches[1], originalB)
	dailyActualFinalBalances(report, r)
	require(reflect.DeepEqual(snap.Frontier[0], *originalB.Change), "same original721-origin change")
	before := time.Now()
	op, done := context.WithTimeout(ctx, 30*time.Second)
	funding, e := owners.evidence.Inspect(op, snap.Frontier)
	done()
	check(e, "actual721-origin single-change node audit")
	out.FinalInspectSeconds = time.Since(before).Seconds()
	require(len(funding) == 1 && funding[0].TxID == originalB.TxID && funding[0].Index == originalB.Change.Index && funding[0].State == "ready", "complete fresh final funding")
	report = dailyActualRun(ctx, owners, false)
	require(report.State == "complete" && report.Reason == "daily-batch-complete", "same occupied day no third batch")
	require(dailyActualRun(ctx, owners, true).State == "complete", "repeated recovery idempotent")
	snap = dailyActualSnapshot(ctx, owners)
	require(len(snap.Batches) == 2 && snap.Credits == 721, "exactly two signed batches")
	dailyActualSame(snap.Batches[0], originalA)
	dailyActualSame(snap.Batches[1], originalB)
	var height uint32
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 924, "final exact block budget")
	sameTip(ctx, a, b)
	peers(ctx, a)
	peers(ctx, b)
	complete := dailyActualComplete{Snapshot: snap, Report: report, Height: height, BBlock: bhash, SourceID: r.SourceID, ConfigID: r.ConfigID, GuardDigest: r.GuardDigest}
	write(filepath.Join(h.Output, "03-complete.json"), complete)
	out.CompleteSHA = hx(digest(dailyActualPublicBytes(filepath.Join(h.Output, "03-complete.json"), dailyActualProcessLimit)))
	require(ctx.Err() == nil, "original scenario remains live at child completion")
	out.Completed = true
	return out
}
