//go:build desktopintegration && slugtestclock && !slugtestpow

// Real owned-node publication exercise, opt-in under a separately frozen runner.
// No native chooser or visual rendering is claimed by the trusted test path.
package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type resultActualRPC struct {
	config prospectnode.RPCConfig
	client *http.Client
}

func resultActualOwnedRPC(t *testing.T, home string) *resultActualRPC {
	return resultActualNamedRPC(t, home, "primary")
}
func resultActualNamedRPC(t *testing.T, home, name string) *resultActualRPC {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(home, name+".conf"))
	if e != nil {
		t.Fatal(e)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			values[k] = v
		}
	}
	if !strings.HasPrefix(values["rpclisten"], "127.0.0.1:") || values["rpcuser"] == "" || values["rpcpass"] == "" {
		t.Fatal("not owned loopback config")
	}
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	t.Cleanup(tr.CloseIdleConnections)
	return &resultActualRPC{prospectnode.RPCConfig{Version: 1, URL: "http://" + values["rpclisten"], Username: values["rpcuser"], Password: values["rpcpass"]}, &http.Client{Transport: tr, Timeout: 15 * time.Second}}
}
func (r *resultActualRPC) call(ctx context.Context, method string, params []any, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	q, e := http.NewRequestWithContext(ctx, http.MethodPost, r.config.URL, bytes.NewReader(body))
	if e != nil {
		return e
	}
	q.SetBasicAuth(r.config.Username, r.config.Password)
	q.Header.Set("Content-Type", "application/json")
	res, e := r.client.Do(q)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if e != nil {
		return e
	}
	var v struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &v) != nil || res.StatusCode != 200 {
		return fmt.Errorf("owned RPC envelope")
	}
	if string(v.Error) != "null" && len(v.Error) != 0 {
		return fmt.Errorf("owned RPC %s refused", method)
	}
	return json.Unmarshal(v.Result, out)
}

// Only fixed local classes are logged; RPC text, endpoints and credentials are
// never included in the bounded mining diagnostics.
func resultActualMineErrorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	var submission *prospectnode.SubmissionError
	if errors.As(err, &submission) {
		switch submission.Code {
		case "unsupported_profile", "invalid_request", "unavailable", "rejected", "outcome_unknown":
			return submission.Code
		default:
			return "submission_other"
		}
	}
	switch err.Error() {
	case "node RPC reported an error":
		return "rpc_error"
	case "node template parent is inconsistent":
		return "template_parent"
	case "node tip changed during context resolution":
		return "tip_changed"
	case "node template time interval is inconsistent":
		return "template_time"
	case "invalid complete node block template":
		return "template_invalid"
	default:
		return "other"
	}
}

// Explicit test-only funding on the actual retained zero256 chain. The
// application never adds a mining address or generates an operator-fund block.
func resultActualMine(t *testing.T, ctx context.Context, rpc *resultActualRPC, payoutHash []byte) (uint32, string) {
	t.Helper()
	client, e := prospectnode.NewClient(rpc.config)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	addr, e := address.NewAddressWitnessPubKeyHash(payoutHash, &chaincfg.MainNetParams)
	if e != nil {
		t.Fatal(e)
	}
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	failures := map[string]int{}
	for attempt := 0; attempt < 64; attempt++ {
		template, e := client.ResolveTemplate(ctx, policy)
		if e != nil {
			failures["resolve_template/"+resultActualMineErrorClass(e)]++
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			time.Sleep(30 * time.Millisecond)
			continue
		}
		job, e := prospectjob.New(template.Context, "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", strings.Repeat("45", 32), time.Now().UnixMilli())
		if e != nil {
			t.Fatal(e)
		}
		candidate, e := prospectwork.Build(job, template, addr.EncodeAddress(), uint64(attempt))
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := hex.DecodeString(candidate.BlockHex)
		var block wire.MsgBlock
		if block.Deserialize(bytes.NewReader(raw)) != nil {
			t.Fatal("candidate decode")
		}
		target := blockchain.CompactToBig(block.Header.Bits)
		won := false
		var hash string
		for nonce := uint32(0); nonce < 64; nonce++ {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			block.Header.Nonce = nonce
			work := block.Header.BlockHash()
			if blockchain.HashToBig(&work).Cmp(target) <= 0 {
				won = true
				hash = work.String()
				break
			}
		}
		if !won {
			failures["proof/no_winner"]++
			continue
		}
		var encoded bytes.Buffer
		if block.Serialize(&encoded) != nil {
			t.Fatal("encode")
		}
		if e := client.SubmitBlock(ctx, template.Context, hex.EncodeToString(encoded.Bytes())); e != nil {
			failures["submit_block/"+resultActualMineErrorClass(e)]++
			continue
		}
		height := template.Context.ParentHeight + 1
		var canonical string
		if e := rpc.call(ctx, "getblockhash", []any{height}, &canonical); e != nil {
			failures["canonical/"+resultActualMineErrorClass(e)]++
		} else if canonical == hash {
			t.Logf("explicit normal-PoW fixture block height=%d hash=%s", height, hash)
			return height, hash
		} else {
			failures["canonical/hash_mismatch"]++
		}
	}
	t.Fatalf("bounded explicit funding retries exhausted; stage_error_counts=%v", failures)
	return 0, ""
}

func TestResultActualNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_RESULT_ACTUAL_CHILD")
	if root == "" {
		return
	}
	fail := func(e error) {
		_ = os.WriteFile(filepath.Join(root, "result-child-error.txt"), []byte(e.Error()+"\n"), 0600)
		os.Exit(2)
	}
	var p boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &p); e != nil || !p.NormalPoW || !p.TaggedClock || p.Saturday != "2026-12-05" {
		fail(fmt.Errorf("result source declaration: %v", e))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	c, e := accountpilot.NewResultsForTestClock(ctx, pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(p.ClockMS + 1) })
	if e != nil {
		fail(e)
	}
	e = pilot.ServeResults(ctx, c, os.Stdin, os.Stdout)
	ce := c.Close()
	if e != nil && e != context.Canceled {
		fail(e)
	}
	if ce != nil {
		fail(ce)
	}
	os.Exit(0)
}

type resultActualNative struct {
	t                     *testing.T
	ctx                   context.Context
	root, node, date, bid string
	b                     *StableBridge
	processes             *manifestProcessInventory
	view                  pilot.ManifestSnapshot
	baseline              map[string]pilot.ManifestSnapshot
	rows                  []map[string]any
	rpc, peer             *resultActualRPC
}

func (h *resultActualNative) open() {
	h.t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestResultActualNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_RESULT_ACTUAL_CHILD="+h.root)
	var e error
	h.b, e = startStable(cmd)
	if e != nil {
		h.t.Fatal(e)
	}
	h.b.entriesEnabled, h.b.eventsEnabled, h.b.seedsEnabled, h.b.cardsEnabled, h.b.boutsEnabled, h.b.resultsEnabled = true, true, true, true, true, true
	h.processes = newManifestProcessInventory(filepath.Join(h.root, "data"), h.node, cmd.Process.Pid)
}
func (h *resultActualNative) close() {
	h.t.Helper()
	if h.b != nil {
		e := h.b.Close()
		h.b = nil
		if e != nil {
			h.t.Error("helper close", e)
		}
	}
	if h.processes != nil {
		h.processes.checkClosed(h.t)
		h.processes = nil
	}
}
func (h *resultActualNative) account(state string) pilot.ManifestSnapshot {
	h.t.Helper()
	var s pilot.ManifestSnapshot
	seedNativeClockWait(h.t, h.ctx, func() bool {
		r, e := h.b.Request(h.ctx, "snapshot", 0)
		if e != nil || r.Snapshot == nil {
			h.t.Fatal("actual account", e)
		}
		s = *r.Snapshot
		if s.Account.State == "failed" {
			h.t.Fatal("actual account failed", s.Account.LastError)
		}
		return s.Account.State == state && (state != "signed_in" || s.View != nil)
	})
	return s
}
func (h *resultActualNative) login(fixture string) {
	h.t.Helper()
	if _, e := h.b.RequestFixture(h.ctx, fixture); e != nil {
		h.t.Fatal(e)
	}
	h.view = h.account("signed_in")
	if h.view.State != "stopped" || h.view.WalletState != "closed" {
		h.t.Fatal("publication must begin without mining or wallet")
	}
	if before, ok := h.baseline[fixture]; ok {
		h.unchanged(before, h.view)
	} else {
		h.baseline[fixture] = h.view
	}
	r := h.request(pilot.ResultRequest{Command: "result_snapshot"})
	if !r.OK || r.Snapshot == nil || r.Snapshot.State != "ready" || r.Snapshot.NodeState != "offline" || r.Snapshot.NodeOwner != "none" {
		h.t.Fatal("result initial stopped bootstrap", r.Error)
	}
	h.rows = append(h.rows, map[string]any{"stage": "login", "fixture": fixture, "account": h.view, "snapshot": r.Snapshot})
	h.processes.capture(h.t, fixture, "result-before-connect", h.view.View.AccountID, h.view.View.AuthorityOrigin, false)
}
func (h *resultActualNative) unchanged(a, z pilot.ManifestSnapshot) {
	h.t.Helper()
	if a.Attempts != z.Attempts || a.AcceptedCount != z.AcceptedCount || a.Pending != z.Pending || a.PayoutAddress != z.PayoutAddress || a.GymID != z.GymID || a.SpendableAtoms != z.SpendableAtoms || a.ImmatureAtoms != z.ImmatureAtoms || z.State != "stopped" || z.WalletState != "closed" {
		h.t.Fatal("publication changed mining or wallet counters")
	}
}
func (h *resultActualNative) request(q pilot.ResultRequest) pilot.ResultResponse {
	h.t.Helper()
	var r pilot.ResultResponse
	seedNativeClockWait(h.t, h.ctx, func() bool { r = h.b.ResultRequest(h.ctx, q); return r.Error != "busy" })
	return r
}
func (h *resultActualNative) job(q pilot.ResultRequest) pilot.ResultJob {
	h.t.Helper()
	q.ViewID = h.view.View.ViewID
	q.Saturday = h.date
	q.BoutID = h.bid
	var r pilot.ResultResponse
	if q.Command == "result_export" {
		r = h.b.ExportResultProof(h.ctx, q.ViewID, q.Saturday, q.BoutID, q.Path)
	} else {
		r = h.request(q)
	}
	if !r.OK || r.Job == nil || r.Job.State != "queued" {
		h.t.Fatal("actual result ACK", q.Command, r.Error)
	}
	j := *r.Job
	seedNativeClockWait(h.t, h.ctx, func() bool {
		r = h.request(pilot.ResultRequest{Command: "result_job", ViewID: q.ViewID, JobID: j.JobID})
		if !r.OK || r.Job == nil {
			h.t.Fatal("actual result poll", q.Command, r.Error)
		}
		j = *r.Job
		return j.State != "queued" && j.State != "running"
	})
	h.rows = append(h.rows, map[string]any{"stage": q.Command, "job": j})
	return j
}
func (h *resultActualNative) read() gymresult.ReadResult {
	h.t.Helper()
	r := h.request(pilot.ResultRequest{Command: "result_record", ViewID: h.view.View.ViewID, Saturday: h.date, BoutID: h.bid})
	if !r.OK || r.Record == nil {
		h.t.Fatal("actual retained read", r.Error)
	}
	return *r.Record
}
func (h *resultActualNative) checkUntil(label string, accept func(gymresult.ActionResult) bool) gymresult.ActionResult {
	h.t.Helper()
	for n := 0; n < 128; n++ {
		j := h.job(pilot.ResultRequest{Command: "result_reconcile"})
		if j.State != "completed" || j.Result == nil {
			h.t.Fatal(label, "check", j.State, j.Error)
		}
		if accept(*j.Result) {
			return *j.Result
		}
	}
	h.t.Fatal(label, "bounded check exhausted")
	return gymresult.ActionResult{}
}
func (h *resultActualNative) connectEvidence(label string) {
	h.t.Helper()
	r := h.request(pilot.ResultRequest{Command: "result_snapshot"})
	if !r.OK || r.Snapshot == nil || r.Snapshot.NodeState != "online" || r.Snapshot.NodeOwner != "publication" {
		h.t.Fatal(label, "node-only owner", r.Error)
	}
	s := h.account("signed_in")
	h.unchanged(h.baseline["zero256"], s)
	h.processes.capture(h.t, "zero256", label, s.View.AccountID, s.View.AuthorityOrigin, true)
	home := filepath.Join(h.root, "data", "gyms", s.View.AccountID)
	h.rpc = resultActualNamedRPC(h.t, home, "primary")
	h.peer = resultActualNamedRPC(h.t, home, "peer")
	h.rows = append(h.rows, map[string]any{"stage": label, "snapshot": r.Snapshot, "account": s, "owned_nodes": h.processes.nodes})
}
func (h *resultActualNative) mineTo(target uint32, payout byte) {
	h.t.Helper()
	if target >= 512 {
		h.t.Fatal("bounded actual chain")
	}
	for n := 0; n < 512; n++ {
		var height uint32
		if e := h.rpc.call(h.ctx, "getblockcount", nil, &height); e != nil {
			h.t.Fatal(e)
		}
		if height >= target {
			return
		}
		resultActualMine(h.t, h.ctx, h.rpc, bytes.Repeat([]byte{payout}, 20))
	}
	h.t.Fatal("bounded mine exhausted")
}
func (h *resultActualNative) invalidate(hash string) {
	h.t.Helper()
	for _, r := range []*resultActualRPC{h.rpc, h.peer} {
		var out any
		if e := r.call(h.ctx, "invalidateblock", []any{hash}, &out); e != nil {
			h.t.Fatal("owned isolated invalidation", e)
		}
	}
}
func (h *resultActualNative) export(label string) gymresult.ProofSummary {
	h.t.Helper()
	path := filepath.Join(h.root, "public-result-proof-"+label+".json")
	j := h.job(pilot.ResultRequest{Command: "result_export", Path: path})
	if j.State != "completed" || j.Export == nil {
		h.t.Fatal(label, "native proof export", j.Error)
	}
	b, e := os.ReadFile(path)
	info, se := os.Lstat(path)
	digest := sha256.Sum256(b)
	if e != nil || se != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || len(b) > 24<<20 || len(b) == 0 || b[len(b)-1] != '\n' || uint32(len(b)) != j.Export.Bytes || hex.EncodeToString(digest[:]) != j.Export.SHA256 {
		h.t.Fatal("native exclusive bounded proof", e, se)
	}
	return *j.Export
}
func (h *resultActualNative) savedBout() gymbout.Record {
	h.t.Helper()
	var r pilot.BoutResponse
	seedNativeClockWait(h.t, h.ctx, func() bool {
		r = h.b.BoutRequest(h.ctx, pilot.BoutRequest{Command: "bout_record", ViewID: h.view.View.ViewID, Saturday: h.date, BoutID: h.bid})
		return r.Error != "busy"
	})
	if !r.OK || r.Record == nil {
		h.t.Fatal("saved actual bout", r.Error)
	}
	return r.Record.Record
}
func TestResultActualNativePublication(t *testing.T) {
	root, node := os.Getenv("SLUG_RESULT_ACTUAL_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit Set48 prepared owned copy")
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		t.Fatal("canonical root")
	}
	marker, e := os.ReadFile(filepath.Join(root, "SET48-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(marker, &m) != nil || m.Version != 1 || m.Scope != "Set48 owned cold copy" || m.SourceRoot == root {
		t.Fatal("Set48 copy declaration")
	}
	var prepared boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &prepared); e != nil || prepared.Build.Record.Plan.BoutCount != 1 || !prepared.Build.Status.Usable || prepared.Saturday != "2026-12-05" {
		t.Fatal("genuine later source", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	h := &resultActualNative{t: t, ctx: ctx, root: root, node: node, date: prepared.Saturday, bid: prepared.Build.Record.Plan.Bouts[0].BoutID, baseline: map[string]pilot.ManifestSnapshot{}}
	defer h.close()
	h.open()
	h.login("zero256")
	// First save uses the existing shipped Bout API, with no synthetic transcript.
	br := h.b.BoutRequest(ctx, pilot.BoutRequest{Command: "bout_run", ViewID: h.view.View.ViewID, Saturday: h.date, BoutID: h.bid})
	if !br.OK || br.Job == nil {
		t.Fatal("actual first Run", br.Error)
	}
	bj := *br.Job
	seedNativeClockWait(t, ctx, func() bool {
		br = h.b.BoutRequest(ctx, pilot.BoutRequest{Command: "bout_job", ViewID: h.view.View.ViewID, JobID: bj.JobID})
		if !br.OK || br.Job == nil {
			t.Fatal("actual bout poll", br.Error)
		}
		bj = *br.Job
		return bj.State != "queued" && bj.State != "running"
	})
	if bj.State != "completed" || bj.Result == nil || bj.Result.Replayed || !bj.Result.Status.Saved.Usable {
		t.Fatal("new actual saved bout", bj.Error)
	}
	saved := bj.Result.Record
	boutNativeSave(t, root, "public-result-saved-bout.json", map[string]any{"normal_pow": true, "tagged_clock": true, "record": saved, "job": bj, "view": h.view.View})
	initial := h.request(pilot.ResultRequest{Command: "result_status", ViewID: h.view.View.ViewID, Saturday: h.date, BoutID: h.bid})
	if !initial.OK || initial.Status == nil || initial.Status.Exists {
		t.Fatal("initial absent result", initial.Error)
	}
	first := h.job(pilot.ResultRequest{Command: "result_publish"})
	if first.State != "failed" || first.Error != "result_scan_incomplete" {
		t.Fatal("first Publish preserves preparation and definite scan refusal", first.State, first.Error)
	}
	h.connectEvidence("first-publication-node-only")
	preparedRead := h.read()
	if preparedRead.Record.Publication != nil || preparedRead.Status.Phase != "prepared" {
		t.Fatal("signed preparation before any dispatch")
	}
	h.checkUntil("funding", func(r gymresult.ActionResult) bool { return r.Status.FundingState == "available" })
	published := h.job(pilot.ResultRequest{Command: "result_publish"})
	if published.State != "completed" || published.Result == nil || published.Result.Record.Publication == nil || published.Result.Record.Publication.State != "submitted" {
		t.Fatal("native submitted result", published.Error)
	}
	rawTx := published.Result.Record.Publication.RawTx
	txid := published.Result.Record.Publication.TxID
	var tip uint32
	if e := h.rpc.call(ctx, "getblockcount", nil, &tip); e != nil {
		t.Fatal(e)
	}
	h.mineTo(tip+1, 0x45)
	firstInclusion := h.checkUntil("initial inclusion", func(r gymresult.ActionResult) bool { return r.Record.Inclusion != nil })
	h.export("first-inclusion")
	// Reopen the actual parent/helper and reconnect through an explicit Check.
	firstRecord := h.read().Record
	h.close()
	h.open()
	h.login("zero256")
	if !reflect.DeepEqual(h.read().Record, firstRecord) {
		t.Fatal("reopen changed retained first publication")
	}
	h.checkUntil("reopen", func(r gymresult.ActionResult) bool { return r.Record.Inclusion != nil })
	h.connectEvidence("reopened-node-only")
	h.invalidate(firstInclusion.Record.Inclusion.Anchor.Hash)
	orphan := h.checkUntil("pre-checkpoint orphan", func(r gymresult.ActionResult) bool {
		return r.Record.Inclusion == nil && len(r.Record.PriorInclusions) == 1
	})
	if orphan.Record.Checkpoint != nil || orphan.Record.Publication.RawTx != rawTx {
		t.Fatal("pre-checkpoint orphan mutated identity")
	}
	// Forced invalidation leaves the live header/peer-height observations ahead
	// of the active chain. Exercise owned restart recovery, not uninterrupted
	// mining after this test-only RPC; keep the durable orphan history exact.
	h.close()
	h.open()
	h.login("zero256")
	if !reflect.DeepEqual(h.read().Record, orphan.Record) {
		t.Fatal("orphan restart changed retained record")
	}
	h.checkUntil("orphan restart", func(r gymresult.ActionResult) bool {
		return r.Record.Inclusion == nil && len(r.Record.PriorInclusions) == 1
	})
	h.connectEvidence("orphan-reopened-node-only")
	var pool []string
	if e := h.rpc.call(ctx, "getrawmempool", nil, &pool); e != nil {
		t.Fatal(e)
	}
	inPool := false
	for _, v := range pool {
		inPool = inPool || v == txid
	}
	if !inPool {
		retry := h.job(pilot.ResultRequest{Command: "result_publish"})
		if retry.State != "completed" || retry.Result == nil || retry.Result.Record.Publication.RawTx != rawTx {
			t.Fatal("exact retained retry", retry.Error)
		}
	}
	h.mineTo(firstInclusion.Record.Inclusion.Anchor.Height, 0x46)
	reincluded := h.checkUntil("exact re-inclusion", func(r gymresult.ActionResult) bool { return r.Record.Inclusion != nil })
	anchor := reincluded.Record.Inclusion.Anchor
	if anchor.Hash == firstInclusion.Record.Inclusion.Anchor.Hash || reincluded.Record.Publication.RawTx != rawTx || len(reincluded.Record.PriorInclusions) != 1 || reincluded.Record.Inclusion.Generation != 1 {
		t.Fatal("pre-final re-inclusion history")
	}
	h.export("reincluded")
	h.mineTo(anchor.Height+59, 0x45)
	at59 := h.checkUntil("depth59", func(r gymresult.ActionResult) bool { return r.Status.Descendants == 59 })
	denied := h.job(pilot.ResultRequest{Command: "result_attest"})
	if denied.State != "failed" || denied.Error != "result_not_mature" || at59.Record.Finality != nil {
		t.Fatal("depth59 attestation refusal", denied.Error)
	}
	h.mineTo(anchor.Height+60, 0x45)
	h.checkUntil("depth60", func(r gymresult.ActionResult) bool { return r.Status.Descendants == 60 })
	finalizedJob := h.job(pilot.ResultRequest{Command: "result_attest"})
	if finalizedJob.State != "completed" || finalizedJob.Result == nil || finalizedJob.Result.Record.Finality == nil || !finalizedJob.Result.Status.CurrentFinality {
		t.Fatal("result's own finality", finalizedJob.Error)
	}
	finalized := *finalizedJob.Result
	h.export("finalized60")
	h.mineTo(anchor.Height+61, 0x45)
	at61 := h.checkUntil("depth61", func(r gymresult.ActionResult) bool { return r.Status.Descendants == 61 })
	if !reflect.DeepEqual(at61.Record.Finality, finalized.Record.Finality) || !reflect.DeepEqual(at61.Record.Checkpoint, finalized.Record.Checkpoint) {
		t.Fatal("first result certificate changed at61")
	}
	h.export("finalized61")
	// Report-only crossing conflict retains original certificate/inclusion forever.
	crossingRecord := h.read().Record
	h.invalidate(anchor.Hash)
	h.close()
	h.open()
	h.login("zero256")
	if !reflect.DeepEqual(h.read().Record, crossingRecord) {
		t.Fatal("checkpoint crossing restart changed retained record")
	}
	h.checkUntil("checkpoint crossing restart", func(r gymresult.ActionResult) bool {
		return r.Status.Phase == "conflict"
	})
	h.connectEvidence("checkpoint-crossing-reopened-node-only")
	h.mineTo(anchor.Height, 0x47)
	conflict := h.checkUntil("checkpoint crossing", func(r gymresult.ActionResult) bool { return r.Status.Phase == "conflict" })
	if conflict.Status.CurrentFinality || !reflect.DeepEqual(conflict.Record.Finality, finalized.Record.Finality) || !reflect.DeepEqual(conflict.Record.Checkpoint, finalized.Record.Checkpoint) || !reflect.DeepEqual(conflict.Record.Inclusion, finalized.Record.Inclusion) || conflict.Record.Publication.RawTx != rawTx {
		t.Fatal("crossing conflict rewrote history")
	}
	conflictProof := h.export("conflict")
	if !reflect.DeepEqual(h.savedBout(), saved) {
		t.Fatal("publication changed saved transcript")
	}
	h.connectEvidence("conflict-node-only")
	// Reset revokes the old View and drains node ownership. Held reads/checks remain
	// possible while signing/publication is refused before reconnection.
	oldView := h.view.View.ViewID
	if _, e := h.b.Request(ctx, "reset_fixture", 0); e != nil {
		t.Fatal(e)
	}
	seedNativeClockWait(t, ctx, func() bool {
		h.view = h.account("signed_in")
		return h.view.Account.Held && h.view.View.ViewID != oldView
	})
	if r := h.request(pilot.ResultRequest{Command: "result_record", ViewID: oldView, Saturday: h.date, BoutID: h.bid}); r.OK {
		t.Fatal("revoked original View disclosed")
	}
	boot := h.request(pilot.ResultRequest{Command: "result_snapshot"})
	if !boot.OK || boot.Snapshot == nil || boot.Snapshot.NodeOwner != "none" || boot.Snapshot.NodeState != "offline" {
		t.Fatal("reset did not drain owner", boot.Error)
	}
	for _, command := range []string{"result_publish", "result_attest"} {
		j := h.job(pilot.ResultRequest{Command: command})
		if j.State != "failed" || j.Error != "forbidden" {
			t.Fatal("held strong action", command, j.Error)
		}
	}
	h.checkUntil("held read-only check", func(r gymresult.ActionResult) bool { return r.Status.Phase == "conflict" })
	h.connectEvidence("held-check-node-only")
	h.export("held-conflict")
	if _, e := h.b.Request(ctx, "logout", 0); e != nil {
		t.Fatal(e)
	}
	h.account("signed_out")
	h.login("incrementing256")
	other := h.read()
	if !reflect.DeepEqual(other.Record.Finality, finalized.Record.Finality) || other.Record.Publication.RawTx != rawTx || !reflect.DeepEqual(h.savedBout(), saved) {
		t.Fatal("other participant lost immutable evidence")
	}
	for _, command := range []string{"result_publish", "result_reconcile", "result_attest"} {
		j := h.job(pilot.ResultRequest{Command: command})
		if j.State != "failed" || j.Error != "forbidden" {
			t.Fatal("other participant connected publisher", command, j.Error)
		}
	}
	h.unchanged(h.baseline["incrementing256"], h.account("signed_in"))
	oldCards := []gymcard.ReadResult{}
	for _, name := range []string{"public-card-result.json", "public-bout-prepared-card.json"} {
		var old boutNativePublicCard
		if e := boutNativeLoad(filepath.Join(root, "original-set47-public"), name, &old); e != nil {
			t.Fatal(e)
		}
		var reply pilot.CardResponse
		seedNativeClockWait(t, ctx, func() bool {
			reply = h.b.CardRequest(ctx, pilot.CardRequest{Command: "card_record", ViewID: h.view.View.ViewID, Saturday: old.Saturday})
			return reply.Error != "busy"
		})
		if !reply.OK || reply.Record == nil || reply.Record.Status.Usable || !reflect.DeepEqual(reply.Record.Record, old.Build.Record) {
			t.Fatal("original conflicted card changed", old.Saturday, reply.Error)
		}
		oldCards = append(oldCards, *reply.Record)
	}
	resultActualSave(t, root, "public-result-native-evidence.json", map[string]any{"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": true, "automatic_admissions": 0, "baseline": h.baseline, "rows": h.rows, "first_inclusion": firstInclusion, "reincluded": reincluded, "finalized": finalized, "at61": at61, "conflict": conflict, "conflict_proof": conflictProof, "other_view": other, "saved_bout": saved, "original_cards": oldCards, "scope": "Actual native private-pipe result API and owned node-only processes; no chooser, GUI, production account or award authority."})
	h.close()
}

// This external diagnostic archive is separate from native transport/cache
// bounds. It retains every complete finite job response, including scan steps.
func resultActualSave(t *testing.T, root, name string, value any) {
	t.Helper()
	b, e := json.Marshal(value)
	if e != nil || len(b)+1 > 24<<20 {
		t.Fatal("bounded result evidence archive", e, len(b))
	}
	f, e := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, we := f.Write(append(b, '\n'))
	ce := f.Close()
	if we != nil || ce != nil {
		t.Fatal(we, ce)
	}
}
