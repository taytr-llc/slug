//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

// No clock selector is carried into the new process. Day is a refusal fence,
// compared with the actual wall clock, never installed into an owner.
type dailyAdmissionHandoff struct {
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
	Day        string `json:"day"`
	PhaseASHA  string `json:"phase_a_sha256"`
	PhaseBSHA  string `json:"phase_b_sha256"`
}

type dailyAdmissionComplete struct {
	Snapshot        poolsettle.DailyActualState             `json:"snapshot"`
	Report          poolsettle.Report                       `json:"report"`
	Height          uint32                                  `json:"height"`
	SourceID        string                                  `json:"source_id"`
	ConfigID        string                                  `json:"config_id"`
	GuardDigest     string                                  `json:"guard_digest"`
	Receipt         poolshare.Submission                    `json:"receipt"`
	Status          poolshare.OperatorAdmissionStatus       `json:"status"`
	Evidence        poolsettle.DailyAdmissionActualEvidence `json:"evidence"`
	PendingReopened poolsettle.DailyActualState             `json:"pending_reopened"`
	Recovered       poolsettle.DailyActualState             `json:"recovered"`
	RecoveredReport poolsettle.Report                       `json:"recovered_report"`
	Round59         poolround.Record                        `json:"round59"`
	Report59        poolsettle.Report                       `json:"report59"`
	Snapshot59      poolsettle.DailyActualState             `json:"snapshot59"`
	Round60         poolround.Record                        `json:"round60"`
	Funding         []poolsettle.Funding                    `json:"funding"`
}

func dailyAdmissionLaunch(ctx context.Context, root, reference, output, relay, nodeA, nodeB, day string) *dailyActualRecoveryProcess {
	deadline, ok := ctx.Deadline()
	require(ok && ctx.Err() == nil && time.Until(deadline) > 0 && time.Until(deadline) <= 900*time.Second, "original remaining admission deadline")
	pgid, e := syscall.Getpgid(0)
	check(e, "parent process group")
	h := dailyAdmissionHandoff{Version: 1, ParentPID: os.Getpid(), ParentPGID: pgid, DeadlineNS: deadline.UnixNano(), Root: root, Reference: reference, Output: output, RelayURL: relay, NodeAURL: nodeA, NodeBURL: nodeB, Day: day, PhaseASHA: hx(digest(dailyActualPublicBytes(filepath.Join(output, "01-A.json"), dailyActualProcessLimit))), PhaseBSHA: hx(digest(dailyActualPublicBytes(filepath.Join(output, "02-B-pending.json"), dailyActualProcessLimit)))}
	data := encode(h)
	handoff := filepath.Join(output, "recovery-handoff.json")
	f, e := os.OpenFile(handoff, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(e, "unique public admission handoff")
	_, we := f.Write(data)
	check(errors.Join(we, f.Close()), "public admission handoff close")
	executable, e := os.Executable()
	check(e, "same compiled test binary")
	p := &dailyActualRecoveryProcess{done: make(chan error, 1), parentPID: os.Getpid(), pgid: pgid, deadlineNS: deadline.UnixNano(), handoffSHA: hx(digest(data)), exitCode: -1}
	p.log, p.startError = os.OpenFile(filepath.Join(output, "recovery-child.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if p.startError != nil {
		return p
	}
	p.cmd = exec.Command(executable, "-test.run=^TestDailyAdmissionActualReopenChild$", "-test.count=1", "-test.v", "-test.timeout=18m")
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "SLUG_DAILY_") {
			p.cmd.Env = append(p.cmd.Env, item)
		}
	}
	p.cmd.Env = append(p.cmd.Env, "SLUG_DAILY_ACTUAL=1", "SLUG_DAILY_REOPEN_CHILD=1", "SLUG_DAILY_ROOT="+root, "SLUG_DAILY_REFERENCE="+reference, "SLUG_DAILY_OUTPUT="+output, "SLUG_DAILY_HANDOFF="+handoff, "SLUG_DAILY_HANDOFF_SHA="+p.handoffSHA)
	// The existing concrete process owner joins this child and its log. No
	// SysProcAttr is installed: the outer guard owns the inherited process group.
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

func TestDailyAdmissionActualReopenChild(t *testing.T) {
	if os.Getenv("SLUG_DAILY_ACTUAL") != "1" || os.Getenv("SLUG_DAILY_REOPEN_CHILD") != "1" {
		t.Skip("only the original admission actual parent selects this child")
	}
	var h dailyAdmissionHandoff
	dailyActualDecodePublic(os.Getenv("SLUG_DAILY_HANDOFF"), os.Getenv("SLUG_DAILY_HANDOFF_SHA"), 16384, &h)
	pgid, e := syscall.Getpgid(0)
	check(e, "inherited original process group")
	require(h.Version == 1 && h.ParentPID == os.Getppid() && h.ParentPID != os.Getpid() && h.ParentPGID == pgid && h.Root == os.Getenv("SLUG_DAILY_ROOT") && h.Reference == os.Getenv("SLUG_DAILY_REFERENCE") && h.Output == os.Getenv("SLUG_DAILY_OUTPUT"), "original admission child selection")
	for _, path := range []string{h.Root, h.Output} {
		actual, e := filepath.EvalSymlinks(path)
		require(e == nil && filepath.IsAbs(path) && filepath.Clean(path) == path && actual == path, "original canonical existing fixture path")
	}
	deadline := time.Unix(0, h.DeadlineNS)
	require(time.Until(deadline) > 0 && time.Until(deadline) <= 900*time.Second, "unchanged original remaining deadline")
	life, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ctx, cancel := context.WithDeadline(life, deadline)
	defer cancel()
	result := dailyAdmissionRecoverChild(ctx, h, os.Getenv("SLUG_DAILY_HANDOFF_SHA"))
	if !result.Pass {
		t.Fatalf("admission child failed: phase=%s failure=%s cleanup=%v", result.Phase, result.Failure, result.CleanupErrors)
	}
}

func dailyAdmissionRefresh(ctx context.Context, o *dailyAdmissionOwners, hash string) poolround.Record {
	op, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	v, e := o.base.rounds.Refresh(op, hash)
	check(e, "original full round refresh")
	return v
}

func dailyAdmissionRoundB(record poolround.Record, prior dailyAdmissionPhaseB, r dailyAdmissionReference) {
	b := record.Round
	require(b.Sequence == 2 && b.BlockHash == prior.Receipt.WorkHash && b.BlockHeight == 103 && b.FirstAccepted == 2 && b.LastAccepted == 3 && b.RewardAtoms == r.Economics.RewardB && b.PoolTarget == r.Original.Binding.Source.PoolTarget && b.PayoutAddress == r.Original.Binding.Source.PayoutAddress, "original B round identity and reward")
	require(reflect.DeepEqual(b.ShareKeys, []string{prior.Ordinary.DuplicateKey, prior.Receipt.DuplicateKey}) && len(b.Allocations) == 2 && reflect.DeepEqual(b.Winner, prior.Tail.Entries[2]), "unchanged complete ordinary plus late winner window")
	for i, account := range r.Original.Accounts {
		found := false
		for _, allocation := range b.Allocations {
			if allocation.GymID == account.GymID {
				require(!found && allocation.Shares == 1 && (poolsettle.Fraction{Numerator: allocation.Numerator, Denominator: allocation.Denominator}) == r.Economics.RoundBGross[i], "independent exact gross half reward")
				found = true
			}
		}
		require(found, "both original gyms in B round")
	}
}

func dailyAdmissionOriginalA(s poolsettle.DailyActualState, a dailyAdmissionPhaseA, credits, pending uint64, state string) {
	require(s.Credits == credits && len(s.Batches) == 1 && s.Pending == pending && s.Batches[0].State == state, "exact one original signed batch and credit count")
	dailyActualSame(s.Batches[0], a.Batch)
}

func dailyAdmissionCreditB(ctx context.Context, o *dailyAdmissionOwners, nodeA *node, r dailyAdmissionReference, priorA dailyAdmissionPhaseA, priorB dailyAdmissionPhaseB, record poolround.Record, state poolsettle.DailyActualState) (poolsettle.DailyAdmissionActualEvidence, []poolsettle.Funding) {
	evidence := dailyAdmissionEvidence(ctx, o)
	require(evidence.BodySHA256 == priorA.Evidence.BodySHA256 && evidence.BodyBytes == priorA.Evidence.BodyBytes && len(evidence.Credits) == 2 && len(priorA.Evidence.Credits) == 1 && reflect.DeepEqual(evidence.Credits[0], priorA.Evidence.Credits[0]), "immutable A body and old credit; exactly one appended B credit")
	credit := evidence.Credits[1]
	c := credit.Coin
	require(credit.RoundHash == record.Round.BlockHash && len(credit.RoundDigest) == 64 && c.Coinbase && c.BlockHash == priorB.Receipt.WorkHash && c.BlockHeight == 103 && c.Atoms == r.Economics.RewardB && c.Script == r.Original.Binding.Guard.FundingScript && len(c.Claims) == 3, "B original confirmed reward and credit provenance")
	mined := block(ctx, nodeA, priorB.Receipt.WorkHash)
	require(len(mined.Transactions) == 2 && mined.Transactions[0].TxHash().String() == c.TxID && int(c.Index) < len(mined.Transactions[0].TxOut) && hx(raw(mined.Transactions[1])) == priorA.Batch.Raw, "B credited exact coinbase and unchanged A transaction")
	output := mined.Transactions[0].TxOut[c.Index]
	require(output.Value == c.Atoms && hx(output.PkScript) == c.Script, "exact B native funding output")
	want := map[string]poolsettle.Fraction{r.Original.Accounts[0].GymID: r.Economics.RoundBNet[0], r.Original.Accounts[1].GymID: r.Economics.RoundBNet[1], "": r.Economics.OperatorB}
	for _, claim := range c.Claims {
		amount, ok := want[claim.GymID]
		require(ok && claim.RoundHash == c.BlockHash && claim.RoundHeight == 103 && claim.Amount == amount, "independent original B99/1 claims")
		delete(want, claim.GymID)
	}
	require(len(want) == 0 && len(state.Frontier) == 2, "entire two-coin frontier")
	foundA, foundB := false, false
	for _, coin := range state.Frontier {
		if coin.TxID == c.TxID && coin.Index == c.Index {
			require(!foundB && reflect.DeepEqual(coin, c), "B credit remains wholly unspent")
			foundB = true
		} else {
			require(!foundA && reflect.DeepEqual(coin, *priorA.Batch.Change), "original A operator change remains wholly unspent")
			foundA = true
		}
	}
	require(foundA && foundB, "no discarded or substituted financial frontier")
	op, done := context.WithTimeout(ctx, 30*time.Second)
	funding, err := o.base.evidence.Inspect(op, state.Frontier)
	done()
	check(err, "actual original final funding inspection")
	require(len(funding) == 2, "both original funding results")
	seen := map[string]bool{}
	for _, f := range funding {
		key := fmt.Sprintf("%s:%d", f.TxID, f.Index)
		require(!seen[key], "unique funding result")
		seen[key] = true
		if f.TxID == c.TxID && f.Index == c.Index {
			require(f.State == "immature", "B60 lock does not grant100-block spendability")
		} else {
			require(f.TxID == priorA.Batch.TxID && f.Index == priorA.Batch.Change.Index && f.State == "ready", "only original A change is ready")
		}
	}
	return evidence, funding
}

func dailyAdmissionRecoverChild(ctx context.Context, h dailyAdmissionHandoff, handoffSHA string) (out dailyActualRecoveryResult) {
	out = dailyActualRecoveryResult{Version: 1, PID: os.Getpid(), ParentPID: os.Getppid(), PGID: h.ParentPGID, DeadlineNS: h.DeadlineNS, HandoffSHA: handoffSHA, CleanupErrors: []string{}}
	owners := &dailyAdmissionOwners{base: &dailyActualOwners{}}
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
	out.Phase = "original public phase evidence before fresh source open"
	require(!wire.TestPoW, "normal PoW in fresh child")
	dailyAdmissionDay(h.Day)
	r := dailyAdmissionReadReference(h.Reference)
	var priorA dailyAdmissionPhaseA
	var priorB dailyAdmissionPhaseB
	dailyActualDecodePublic(filepath.Join(h.Output, "01-A.json"), h.PhaseASHA, dailyActualProcessLimit, &priorA)
	dailyActualDecodePublic(filepath.Join(h.Output, "02-B-pending.json"), h.PhaseBSHA, dailyActualProcessLimit, &priorB)
	require(priorA.Day == h.Day && priorB.AbsentBefore && priorB.ExpiryObserved >= priorB.Work.Job.ExpiresAtMS && priorB.CanonicalObserved >= priorB.Work.Job.ExpiresAtMS && priorB.Tail.Through == 3 && len(priorB.Tail.Entries) == 3 && priorA.Evidence.BodySHA256 == priorB.Evidence.BodySHA256, "original expiry/unseen/physical evidence")
	dailyAdmissionBatchA(priorA.Batch, priorA.Reward, r)
	a, b := dailyActualChildRPC(h.NodeAURL), dailyActualChildRPC(h.NodeBURL)
	defer a.rpc.c.CloseIdleConnections()
	defer b.rpc.c.CloseIdleConnections()
	checkedRelay := dailyActualChildRPC(h.RelayURL)
	defer checkedRelay.rpc.c.CloseIdleConnections()
	rpcConfig := prospectnode.RPCConfig{Version: 1, URL: h.RelayURL, Username: "fixture", Password: "public-pool-actual"}
	out.Phase = "one fresh-process original-owner reopen and duplicate receipt"
	dailyAdmissionOpen(ctx, filepath.Join(h.Root, "owners"), r, rpcConfig, false, owners)
	out.Reopens = 1
	out.ColdSourceSeconds = owners.base.coldSeconds
	dailyAdmissionStatus(ctx, owners, 3, 3, 2)
	pending := dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(pending, priorA, 1, 1, priorA.Batch.State)
	evidence := dailyAdmissionEvidence(ctx, owners)
	require(reflect.DeepEqual(evidence, priorA.Evidence), "cold original b1 and credit1 exact before recovery")
	lookup, err := dailyAdmissionLookup(ctx, owners, r.Original.Accounts[1], priorB.Work, priorB.Receipt.Nonce)
	check(err, "cold exact receipt lookup")
	duplicate := dailyActualSubmit(ctx, owners.base, r.Original.Accounts[1], priorB.Work, priorB.Receipt.Nonce)
	require(reflect.DeepEqual(lookup, priorB.Receipt) && reflect.DeepEqual(duplicate, priorB.Receipt), "entire original late receipt across fresh process")
	tail, err := owners.base.source.AccountingPage(ctx, 0, 0, 8)
	check(err, "cold original accepted tail")
	require(reflect.DeepEqual(tail, priorB.Tail), "all original accepted rows after duplicate")
	dailyAdmissionStatus(ctx, owners, 3, 3, 2)
	var height uint32
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 103 && sameTip(ctx, a, b) == priorB.Receipt.WorkHash && hx(raw(minedTx(ctx, a, priorB.Receipt.WorkHash, priorA.Batch.TxID))) == priorA.Batch.Raw, "A confirmed in exact B before recovery")
	out.Phase = "recover original pending A by observation without another send"
	recoveredReport := dailyActualRun(ctx, owners.base, true)
	dailyAdmissionReport(recoveredReport, r, "a_recovered_before_b_credit", h.Day)
	recovered := dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(recovered, priorA, 1, 0, "confirmed")
	require(reflect.DeepEqual(dailyAdmissionEvidence(ctx, owners), priorA.Evidence), "recovery changes delivery only")
	out.Phase = "B59 remains provisional and earns no new credit"
	dailyActualMature(ctx, a, b, 162)
	round59 := dailyAdmissionRefresh(ctx, owners, priorB.Receipt.WorkHash)
	dailyAdmissionRoundB(round59, priorB, r)
	require(round59.Confirmation == nil && round59.Observation != nil && round59.Observation.Canonical && round59.Observation.Descendants == 59 && round59.Observation.TipHeight == 162 && round59.Status() == "provisional", "exact59 descendants never lock earnings")
	report59 := dailyActualRun(ctx, owners.base, false)
	dailyAdmissionReport(report59, r, "b59_same_day", h.Day)
	snap59 := dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(snap59, priorA, 1, 0, "confirmed")
	require(reflect.DeepEqual(dailyAdmissionEvidence(ctx, owners), priorA.Evidence), "no B credit before60")
	out.Phase = "B60 imports exactly once with same occupied daily slot"
	dailyActualMature(ctx, a, b, 163)
	round60 := dailyAdmissionRefresh(ctx, owners, priorB.Receipt.WorkHash)
	dailyAdmissionRoundB(round60, priorB, r)
	require(reflect.DeepEqual(round60.Round, round59.Round) && round60.Confirmation != nil && round60.Confirmation.Canonical && round60.Confirmation.Descendants == 60 && round60.Confirmation.TipHeight == 163 && round60.Status() == "confirmed", "immutable B allocation locks at exactly60")
	report := dailyActualRun(ctx, owners.base, false)
	dailyAdmissionReport(report, r, "b60_same_day", h.Day)
	snap := dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(snap, priorA, 2, 0, "confirmed")
	beforeInspect := time.Now()
	evidence, funding := dailyAdmissionCreditB(ctx, owners, a, r, priorA, priorB, round60, snap)
	out.FinalInspectSeconds = time.Since(beforeInspect).Seconds()
	// One complete repeated import and one exact receipt replay cannot create a
	// second B credit or occupy another signing slot on the same real day.
	report = dailyActualRun(ctx, owners.base, false)
	dailyAdmissionReport(report, r, "b60_same_day", h.Day)
	duplicate = dailyActualSubmit(ctx, owners.base, r.Original.Accounts[1], priorB.Work, priorB.Receipt.Nonce)
	require(reflect.DeepEqual(duplicate, priorB.Receipt), "final duplicate original receipt")
	snap = dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(snap, priorA, 2, 0, "confirmed")
	require(reflect.DeepEqual(dailyAdmissionEvidence(ctx, owners), evidence), "no duplicate credit or immutable batch mutation")
	status := dailyAdmissionStatus(ctx, owners, 3, 3, 2)
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 163, "exact final normal block budget")
	sameTip(ctx, a, b)
	peers(ctx, a)
	peers(ctx, b)
	dailyAdmissionDay(h.Day)
	complete := dailyAdmissionComplete{Snapshot: snap, Report: report, Height: height, SourceID: r.Original.SourceID, ConfigID: r.Original.ConfigID, GuardDigest: r.Original.GuardDigest, Receipt: duplicate, Status: status, Evidence: evidence, PendingReopened: pending, Recovered: recovered, RecoveredReport: recoveredReport, Round59: round59, Report59: report59, Snapshot59: snap59, Round60: round60, Funding: funding}
	write(filepath.Join(h.Output, "03-complete.json"), complete)
	out.CompleteSHA = hx(digest(dailyActualPublicBytes(filepath.Join(h.Output, "03-complete.json"), dailyActualProcessLimit)))
	require(ctx.Err() == nil, "original scenario deadline remains live")
	out.Completed = true
	return out
}
