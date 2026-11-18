//go:build desktopintegration && slugtestclock && !slugtestpow

// Set48 actual composition is opt-in and consumes a separately marked cold copy.
// It preserves the old Set47 tests and their original bounds.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func resultActualRoot(t *testing.T) (string, string) {
	t.Helper()
	root, node := os.Getenv("SLUG_RESULT_ACTUAL_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit Set48 owned cold copy and normal node required")
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		t.Fatal("canonical owned root", e)
	}
	b, e := os.ReadFile(filepath.Join(root, "SET48-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(b, &m) != nil || m.Version != 1 || m.Scope != "Set48 owned cold copy" || m.SourceRoot == root {
		t.Fatal("explicit Set48 marker", e)
	}
	return root, node
}
func resultActualAdvance(t *testing.T, h *cardClockHarness, o *boutCoreSeed, target uint32) {
	t.Helper()
	if target >= 512 {
		t.Fatal("Set48 chain header bound")
	}
	for n := 0; n < 512; n++ {
		o.assertAdmissions(t)
		var height uint32
		if e := o.rpc.call(h.ctx, "getblockcount", nil, &height); e != nil {
			t.Fatal(e)
		}
		if height >= target {
			return
		}
		seedClockMine(t, h.ctx, o.rpc, bytes.Repeat([]byte{0x45}, 20))
	}
	t.Fatal("bounded result source advance exhausted")
}
func resultActualExportSourceChain(t *testing.T, ctx context.Context, rpc *seedClockRPC, root string, tip gymseed.Tip) {
	t.Helper()
	if tip.Height >= 512 {
		t.Fatal("Set48 source header bound")
	}
	type row struct {
		Height   uint32 `json:"height"`
		Hash     string `json:"hash"`
		BlockHex string `json:"block_hex"`
	}
	rows := make([]row, tip.Height+1)
	hash := tip.Hash
	total := 0
	for h := int(tip.Height); h >= 0; h-- {
		var body string
		if e := rpc.call(ctx, "getblock", []any{hash, 0}, &body); e != nil {
			t.Fatal(e)
		}
		raw, e := hex.DecodeString(body)
		if e != nil || len(raw) < 81 || len(raw) > 4000000 || hex.EncodeToString(raw) != body {
			t.Fatal("bounded canonical body")
		}
		total += len(raw)
		if total > 24<<20 {
			t.Fatal("source branch byte bound")
		}
		rows[h] = row{uint32(h), hash, body}
		previous := append([]byte(nil), raw[4:36]...)
		for i, j := 0, len(previous)-1; i < j; i, j = i+1, j-1 {
			previous[i], previous[j] = previous[j], previous[i]
		}
		hash = hex.EncodeToString(previous)
	}
	if hash != strings.Repeat("0", 64) {
		t.Fatal("genesis ancestry")
	}
	boutClockSave(t, root, "public-result-source-chain.json", rows)
}

func TestResultActualPrepareLaterSource(t *testing.T) {
	root, node := resultActualRoot(t)
	old := boutClockLoad(t, root, "public-result-original-card.json")
	if !reflect.DeepEqual(old.Read.Record, old.Build.Record) {
		t.Fatal("original card must remain historical and conflicted")
	}
	h := boutClockOpen(t, root, node, old.ClockMS+1)
	h.login(t, "zero256")
	w := newBoutClockWire(t, h.ctx, h.c)
	v := h.c.Snapshot().View.ViewID
	prior := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: old.Saturday})
	if !prior.OK || prior.Record.Status.Usable || !reflect.DeepEqual(prior.Record.Record, old.Build.Record) {
		t.Fatal("copied original evidence", prior.Error)
	}
	older := boutClockLoad(t, filepath.Join(root, "original-set47-public"), "public-card-result.json")
	olderRead := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: older.Saturday})
	if !olderRead.OK || olderRead.Record.Status.Usable || !reflect.DeepEqual(olderRead.Record.Record, older.Build.Record) {
		t.Fatal("copied earlier conflicted card", olderRead.Error)
	}
	boutClockSave(t, root, "public-result-original-cards-before.json", []any{olderRead.Record, prior.Record})
	h.logout(t)
	// The old event's calendar deadline is exactly the next Saturday noon.
	h.cutoff = old.Build.Record.Source.SourceReceipt.Input.Event.IncidentDeadlineMS
	h.now.Store(h.cutoff - 60000)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		h.login(t, fixture)
		v = h.c.Snapshot().View.ViewID
		u, err := h.c.EntryUpcoming(h.ctx, v)
		if err != nil || u.Event.CutoffMS != h.cutoff || u.Event.Saturday == old.Saturday {
			t.Fatal("later Saturday", err)
		}
		h.date = u.Event.Saturday
		ids := []string{}
		for _, participant := range old.Build.Record.Source.SourceReceipt.Input.Participants {
			if participant.Owner.AccountID == h.c.Snapshot().View.AccountID {
				for _, m := range participant.Members {
					ids = append(ids, m.Assignment.ProspectID)
				}
			}
		}
		if len(ids) == 0 {
			t.Fatal("no retained genuine entrants")
		}
		sort.Strings(ids)
		st := h.c.StableSnapshot()
		j := h.entryJob(t, func() (pilot.EntryJob, error) {
			return h.c.BeginEntryPrepare(h.ctx, v, gymentry.Request{Version: 1, EventID: u.Event.EventID, Saturday: h.date, Action: "replace", ExpectedRosterRevision: st.Status.RosterRevision, ProspectIDs: ids})
		})
		h.entryJob(t, func() (pilot.EntryJob, error) { return h.c.BeginEntryApply(h.ctx, v, j.OperationID) })
		h.logout(t)
	}
	h.now.Store(h.cutoff + 1000)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		h.login(t, fixture)
		v = h.c.Snapshot().View.ViewID
		j, err := h.c.BeginEventCapture(h.ctx, v, h.date)
		if err != nil {
			t.Fatal(err)
		}
		seedClockWait(t, h.ctx, func() bool {
			j, err = h.c.EventJob(h.ctx, v, j.JobID)
			if err != nil {
				t.Fatal(err)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "completed" {
			t.Fatal("later snapshot", j.Error)
		}
		h.logout(t)
	}
	h.login(t, "zero256")
	v = h.c.Snapshot().View.ViewID
	prepared := w.job(t, h.ctx, v, h.date, "seed_prepare")
	if prepared.State != "completed" || prepared.Result.Publication != nil {
		t.Fatal("fresh later publication intent", prepared.Error)
	}
	acceptedBefore := h.c.Snapshot().AcceptedCount
	seedPin := *h.c.SeedSnapshot().Pin
	w.close(t)
	if err := h.c.Close(); err != nil {
		t.Fatal(err)
	}
	h.c = nil
	o := boutCoreOpen(t, h, seedPin, "result-source", acceptedBefore)
	fundingHash160, _ := hex.DecodeString("06afd46bcdfd22ef94ac122aa11f241244a37ecc")
	fundingHeight, fundingHash := seedClockMine(t, h.ctx, o.rpc, fundingHash160)
	// Funding scans run against a stationary owned chain. Do not call Publish
	// until its full retained funding prefix equals the actual current tip.
	complete := false
	for n := 0; n < 120; n++ {
		_ = o.action(t, "reconcile", h.date)
		info, e := o.seeds.Info(h.ctx)
		if e != nil {
			t.Fatal(e)
		}
		var tip uint32
		var hash string
		if e = o.rpc.call(h.ctx, "getblockcount", nil, &tip); e != nil {
			t.Fatal(e)
		}
		if e = o.rpc.call(h.ctx, "getblockhash", []any{tip}, &hash); e != nil {
			t.Fatal(e)
		}
		t.Logf("core-only funding height=%d complete=%t tip=%d", info.FundingHeight, info.FundingComplete, tip)
		if info.FundingComplete && info.FundingHeight == int32(tip) && info.FundingHash == hash {
			complete = true
			break
		}
	}
	if !complete {
		t.Fatal("bounded stationary funding scan exhausted")
	}
	publication := o.action(t, "publish", h.date)
	if publication.Publication == nil {
		t.Fatal("missing definite publication")
	}
	var tip uint32
	if err := o.rpc.call(h.ctx, "getblockcount", nil, &tip); err != nil {
		t.Fatal(err)
	}
	resultActualAdvance(t, h, o, tip+3)
	var observed gymseed.Record
	for n := 0; n < 120; n++ {
		observed = o.action(t, "reconcile", h.date)
		if observed.Target != nil {
			if observed.Observation.TargetDescendants >= 60 {
				break
			}
			resultActualAdvance(t, h, o, observed.Target.Height+60)
		}
	}
	if observed.Target == nil || observed.Observation.TargetDescendants < 60 {
		t.Fatal("bounded fixed target/depth wait")
	}
	attest := o.action(t, "attest", h.date)
	if attest.Finality == nil || attest.Publication == nil || attest.Publication.RawTx != publication.Publication.RawTx {
		t.Fatal("later finality")
	}
	resultActualExportSourceChain(t, h.ctx, o.rpc, root, attest.Finality.ObservedTip)
	// One key2 coinbase; all remaining fixture blocks use the public sink.
	resultActualAdvance(t, h, o, fundingHeight+99)
	var fundingBody string
	if e := o.rpc.call(h.ctx, "getblock", []any{fundingHash, 0}, &fundingBody); e != nil {
		t.Fatal(e)
	}
	boutClockSave(t, root, "public-result-funding.json", map[string]any{"height": fundingHeight, "hash": fundingHash, "block_hex": fundingBody, "script": "001406afd46bcdfd22ef94ac122aa11f241244a37ecc"})
	for name, hash := range map[string]string{"result-source-anchor": attest.Anchor.Hash, "result-source-target": attest.Target.Hash} {
		var body string
		if err := o.rpc.call(h.ctx, "getblock", []any{hash, 0}, &body); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "public-"+name+"-block.hex"), []byte(body+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	o.assertAdmissions(t)
	o.close(t)
	if t.Failed() {
		t.Fatal("core-only owner did not cleanly close before parent reopen")
	}
	boutClockResume(t, h)
	v = h.c.Snapshot().View.ViewID
	if h.c.Snapshot().AcceptedCount != acceptedBefore {
		t.Fatal("core preparation admitted a candidate")
	}
	w = newBoutClockWire(t, h.ctx, h.c)
	built := w.cardJob(t, h.ctx, v, h.date)
	if built.State != "completed" || built.Result == nil || built.Result.Replayed || !built.Result.Status.Usable {
		t.Fatal("later usable card", built.Error)
	}
	plan := built.Result.Record.Plan
	if plan.BoutCount != 1 || plan.Counts.TotalCount != old.Build.Record.Plan.Counts.TotalCount || plan.Counts.PairedCount != 2 || plan.Counts.TotalCount != plan.Counts.PairedCount+plan.Counts.UnmatchedCount {
		t.Fatal("later C1 accounting", plan.Counts)
	}
	// Retained assignments and birth stay exact; only the new cutoff projection changes.
	for _, p := range built.Result.Record.Source.SourceReceipt.Input.Participants {
		for _, m := range p.Members {
			found := false
			for _, op := range old.Build.Record.Source.SourceReceipt.Input.Participants {
				for _, om := range op.Members {
					if om.Assignment.ProspectID == m.Assignment.ProspectID {
						found = reflect.DeepEqual(om.Assignment, m.Assignment)
					}
				}
			}
			if !found {
				t.Fatal("original assignment/birth changed")
			}
		}
	}
	prior = w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: old.Saturday})
	if !prior.OK || prior.Record.Status.Usable || !reflect.DeepEqual(prior.Record.Record, old.Build.Record) {
		t.Fatal("new event changed original conflict/evidence", prior.Error)
	}
	read := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: h.date})
	if !read.OK || !read.Record.Status.Usable {
		t.Fatal("later saved card", read.Error)
	}
	seed, err := json.Marshal(attest)
	if err != nil {
		t.Fatal(err)
	}
	boutClockSave(t, root, "public-result-prepared-card.json", boutClockPublic{1, true, true, h.now.Load(), h.date, h.c.CardSnapshot(), *built.Result, *read.Record, seed})
	if h.c.Snapshot().AcceptedCount != acceptedBefore {
		t.Fatal("additional admission cap exceeded")
	}
	boutClockSave(t, root, "public-result-preparation-counts.json", map[string]any{"preparation_mode": "authenticated_core_seed_with_owned_normal_nodes_no_admission_worker", "automatic_admissions": 0, "core_account_id": o.accountID, "core_account_generation": o.generation, "core_account_hold_until_ms": o.holdUntilMS, "accepted_before": acceptedBefore, "accepted_after": h.c.Snapshot().AcceptedCount, "maximum_additional_per_gym": 0, "maximum_preparation_additional": 0, "card_counts": plan.Counts, "entrants": plan.Entrants})
	// The later conflict-stage harness has its own login observations. Preserve
	// this stage's current View evidence before that reusable log is replaced.
	boutClockSave(t, root, "public-result-prepared-views.json", h.views)
	t.Logf("later genuine card entered=%d paired=%d unmatched=%d bouts=%d", plan.Counts.TotalCount, plan.Counts.PairedCount, plan.Counts.UnmatchedCount, plan.BoutCount)
	w.close(t)
	h.c = nil
}
