//go:build desktopintegration && slugtestclock && !slugtestpow

package accountpilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

// This fixture owns its authority clock and normal-PoW nodes. It does not add a
// shipped clock, funding, chain-control, or candidate-selection entry point.
type cardClockHarness struct {
	c      *StableController
	ctx    context.Context
	now    atomic.Int64
	cfg    pilot.Config
	date   string
	cutoff int64
	root   string
	views  []pilot.ManifestSnapshot
}

func (h *cardClockHarness) login(t *testing.T, f string) {
	t.Helper()
	if e := h.c.LoginFixture(f); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, h.ctx, func() bool {
		s := h.c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("login", f, s.Account.LastError)
		}
		return s.Account.State == "signed_in" && s.View != nil
	})
	h.views = append(h.views, h.c.Snapshot())
	b, _ := json.MarshalIndent(h.views, "", "  ")
	if e := os.WriteFile(filepath.Join(h.root, "observed-views.json"), append(b, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}
func (h *cardClockHarness) logout(t *testing.T) {
	t.Helper()
	if e := h.c.Logout(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, h.ctx, func() bool { return h.c.Snapshot().Account.State == "signed_out" })
}
func (h *cardClockHarness) stable(t *testing.T, j pilot.StableJob, e error) pilot.StableJob {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, h.ctx, func() bool {
		j, e = h.c.StableJob(h.ctx, j.View.ViewID, j.JobID)
		if e != nil {
			t.Fatal(e)
		}
		return j.State != "queued" && j.State != "running"
	})
	if j.State != "done" {
		t.Fatal("stable", j.Kind, j.Error)
	}
	return j
}
func (h *cardClockHarness) entry(t *testing.T, j pilot.EntryJob, e error) pilot.EntryJob {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, h.ctx, func() bool {
		j, e = h.c.EntryJob(h.ctx, j.View.ViewID, j.JobID)
		if e != nil {
			t.Fatal(e)
		}
		return j.State != "queued" && j.State != "running"
	})
	if j.State != "complete" {
		t.Fatal("entry", j.Error)
	}
	return j
}
func (h *cardClockHarness) stableJob(t *testing.T, fn func() (pilot.StableJob, error)) pilot.StableJob {
	t.Helper()
	var j pilot.StableJob
	var e error
	seedClockWait(t, h.ctx, func() bool { j, e = fn(); return e == nil || e.Error() != "busy" })
	return h.stable(t, j, e)
}
func (h *cardClockHarness) entryJob(t *testing.T, fn func() (pilot.EntryJob, error)) pilot.EntryJob {
	t.Helper()
	var j pilot.EntryJob
	var e error
	seedClockWait(t, h.ctx, func() bool { j, e = fn(); return e == nil || e.Error() != "busy" })
	return h.entry(t, j, e)
}
func cardClockSetup(t *testing.T, root, node string) *cardClockHarness {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	t.Cleanup(cancel)
	h := &cardClockHarness{ctx: ctx, root: root, cfg: pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}}
	h.now.Store(time.Now().UnixMilli())
	var e error
	h.c, e = NewCardsForTestClock(h.cfg, func() time.Time { return time.UnixMilli(h.now.Load()) })
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if h.c != nil {
			_ = h.c.Close()
		}
	})
	h.login(t, "zero256")
	u, e := h.c.EntryUpcoming(ctx, h.c.Snapshot().View.ViewID)
	if e != nil {
		t.Fatal(e)
	}
	h.date, h.cutoff = u.Event.Saturday, u.Event.CutoffMS
	if e = h.c.Close(); e != nil {
		t.Fatal(e)
	}
	h.c = nil
	h.now.Store(h.cutoff - 60000)
	h.c, e = NewCardsForTestClock(h.cfg, func() time.Time { return time.UnixMilli(h.now.Load()) })
	if e != nil {
		t.Fatal(e)
	}
	all := map[string][]gymstable.Item{}
	selected := map[string][]gymstable.Item{}
	fixtures := []string{"zero256", "incrementing256"}
	// At most 32 genuine retained candidates per fixture, in batches of four.
	// Selection is test setup only; the frozen core computes the actual card.
	for count := uint64(4); count <= 32 && len(selected) == 0; count += 4 {
		for _, f := range fixtures {
			h.login(t, f)
			if e = h.c.Start(); e != nil {
				t.Fatal(e)
			}
			seedClockWait(t, ctx, func() bool {
				s := h.c.Snapshot()
				if s.AcceptedCount > 32 {
					t.Fatal("genuine collection exceeded 32 retained candidates")
				}
				if s.State == "failed" {
					t.Fatal("mining", s.LastError)
				}
				return s.AcceptedCount >= count
			})
			h.c.Stop()
			v := h.c.Snapshot().View.ViewID
			for {
				j := h.stableJob(t, func() (pilot.StableJob, error) { return h.c.BeginStableSync(ctx, v) })
				if j.Sync.Status.CaughtUp {
					break
				}
			}
			all[f] = nil
			for after := uint64(0); ; {
				j := h.stableJob(t, func() (pilot.StableJob, error) { return h.c.BeginStableInventory(ctx, v, after, 8) })
				all[f] = append(all[f], j.Page.Items...)
				if !j.Page.HasMore {
					break
				}
				after = j.Page.NextAfter
			}
			if len(all[f]) > 32 {
				t.Fatal("retained collection exceeded fixed cap")
			}
			t.Logf("genuine candidate inventory fixture=%s count=%d", f, len(all[f]))
			h.logout(t)
		}
		for _, a := range all[fixtures[0]] {
			for _, b := range all[fixtures[1]] {
				if a.Manifest.Physical.Class != b.Manifest.Physical.Class {
					continue
				}
				x, y := a.Condition.RatingHundredths, b.Condition.RatingHundredths
				if 2*min(x, y) < max(x, y) {
					continue
				}
				selected[fixtures[0]] = []gymstable.Item{a}
				selected[fixtures[1]] = []gymstable.Item{b}
				break
			}
			if len(selected) > 0 {
				break
			}
		}
	}
	if len(selected) != 2 {
		t.Fatal("bounded genuine same-class in-band search exhausted")
	}
	// Prefer one additional class from the first gym so the same actual card
	// proves an unmatched reason as well as a booked pair.
	class := selected[fixtures[0]][0].Manifest.Physical.Class
	for _, a := range all[fixtures[0]] {
		if a.Manifest.Physical.Class != class {
			selected[fixtures[0]] = append(selected[fixtures[0]], a)
			break
		}
	}
	public, _ := json.MarshalIndent(struct {
		Inventories map[string][]gymstable.Item `json:"inventories"`
		Selected    map[string][]gymstable.Item `json:"selected"`
	}{all, selected}, "", "  ")
	if e = os.WriteFile(filepath.Join(root, "public-candidate-selection.json"), append(public, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	for _, f := range fixtures {
		h.login(t, f)
		v := h.c.Snapshot().View.ViewID
		ids := []string{}
		for i, item := range selected[f] {
			st := h.c.StableSnapshot()
			p := h.stableJob(t, func() (pilot.StableJob, error) {
				return h.c.BeginStablePrepare(ctx, v, gymstable.SlotRequest{ExpectedRevision: st.Status.RosterRevision, Slot: uint32(i + 1), ProspectID: item.Assignment.ProspectID})
			})
			h.stableJob(t, func() (pilot.StableJob, error) { return h.c.BeginStableApply(ctx, v, p.Intent.OperationID) })
			ids = append(ids, item.Assignment.ProspectID)
		}
		sort.Strings(ids)
		st := h.c.StableSnapshot()
		u, e := h.c.EntryUpcoming(ctx, v)
		if e != nil || u.Event.Saturday != h.date {
			t.Fatal("entry date", e)
		}
		p := h.entryJob(t, func() (pilot.EntryJob, error) {
			return h.c.BeginEntryPrepare(ctx, v, gymentry.Request{Version: 1, EventID: u.Event.EventID, Saturday: h.date, Action: "replace", ExpectedRosterRevision: st.Status.RosterRevision, ProspectIDs: ids})
		})
		h.entryJob(t, func() (pilot.EntryJob, error) { return h.c.BeginEntryApply(ctx, v, p.OperationID) })
		h.logout(t)
	}
	h.now.Store(h.cutoff + 1000)
	for _, f := range fixtures {
		h.login(t, f)
		v := h.c.Snapshot().View.ViewID
		j, e := h.c.BeginEventCapture(ctx, v, h.date)
		if e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool {
			j, e = h.c.EventJob(ctx, v, j.JobID)
			if e != nil {
				t.Fatal(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "completed" {
			t.Fatal("capture", j.Error)
		}
		h.logout(t)
	}
	h.login(t, "zero256")
	return h
}
func newCardClockWire(t *testing.T, ctx context.Context, c *StableController) *seedClockWire {
	t.Helper()
	in, writer := io.Pipe()
	reader, out := io.Pipe()
	w := &seedClockWire{reader: reader, in: writer, scan: bufio.NewScanner(reader), done: make(chan error, 1)}
	w.scan.Buffer(make([]byte, 4096), pilot.MaxCardResponseSize)
	go func() { e := pilot.ServeCards(ctx, c, in, out); _ = out.CloseWithError(e); _ = in.Close(); w.done <- e }()
	t.Cleanup(func() { w.close(t) })
	return w
}
func (w *seedClockWire) cardCall(t *testing.T, r pilot.CardRequest) pilot.CardResponse {
	t.Helper()
	w.next++
	r.ID = w.next
	b, e := pilot.MarshalCardRequest(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.in.Write(append(b, '\n')); e != nil {
		t.Fatal(e)
	}
	if !w.scan.Scan() {
		t.Fatal("card pipe", w.scan.Err())
	}
	v, e := pilot.ParseCardResponse(w.scan.Bytes())
	if e != nil || v.ID != r.ID {
		t.Fatalf("card frame %d: %v %s", r.ID, e, w.scan.Text())
	}
	return v
}
func (w *seedClockWire) cardJob(t *testing.T, ctx context.Context, v, date string) pilot.CardJob {
	t.Helper()
	var r pilot.CardResponse
	seedClockWait(t, ctx, func() bool {
		r = w.cardCall(t, pilot.CardRequest{Command: "card_build", ViewID: v, Saturday: date})
		return r.Error != "busy"
	})
	if !r.OK || r.Job == nil || r.Job.State != "queued" {
		t.Fatal("card ACK", r.Error)
	}
	j := *r.Job
	seedClockWait(t, ctx, func() bool {
		r = w.cardCall(t, pilot.CardRequest{Command: "card_job", ViewID: v, JobID: j.JobID})
		if r.Error == "busy" {
			return false
		}
		if !r.OK || r.Job == nil {
			t.Fatal("card poll", r.Error)
		}
		j = *r.Job
		return j.State != "queued" && j.State != "running"
	})
	return j
}
func (w *seedClockWire) cardRead(t *testing.T, ctx context.Context, r pilot.CardRequest) pilot.CardResponse {
	t.Helper()
	var out pilot.CardResponse
	seedClockWait(t, ctx, func() bool { out = w.cardCall(t, r); return out.Error != "busy" })
	return out
}

func TestCardClockActualPositivePair(t *testing.T) {
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute node required")
	}
	base, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "card-testclock-")
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("owned evidence", root)
	h := cardClockSetup(t, root, node)
	ctx := h.ctx
	w := newCardClockWire(t, ctx, h.c)
	v := h.c.Snapshot().View.ViewID
	snap := w.cardCall(t, pilot.CardRequest{Command: "card_snapshot"})
	if !snap.OK || snap.Snapshot.State != "ready" {
		t.Fatal("card setup", snap.Error)
	}
	before := w.cardJob(t, ctx, v, h.date)
	if before.Error != "card_source_incomplete" {
		t.Fatal("unfinalized build", before.Error)
	}
	prepared := w.job(t, ctx, v, h.date, "seed_prepare")
	if prepared.State != "completed" || prepared.Result.Publication != nil {
		t.Fatal("prepare", prepared.Error)
	}
	if e = h.c.Start(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool { return h.c.SeedSnapshot().NodeState == "ready" })
	home := filepath.Join(h.cfg.DataDir, "gyms", h.c.Snapshot().View.AccountID)
	rpc := seedClockOwnedRPC(t, home)
	keyHash, _ := hex.DecodeString("751e76e8199196d454941c45d1b3a323f1433bd6")
	fundHeight, fundHash := seedClockMine(t, ctx, rpc, keyHash)
	seedClockAdvance(t, ctx, rpc, fundHeight+100)
	var publication pilot.SeedJob
	for n := 0; n < 100; n++ {
		j := w.job(t, ctx, v, h.date, "seed_reconcile")
		if j.State != "completed" {
			t.Fatal("funding reconcile", j.Error)
		}
		publication = w.job(t, ctx, v, h.date, "seed_publish")
		if publication.State == "completed" && publication.Result.Publication != nil {
			break
		}
		if publication.Error != "seed_scan_incomplete" && publication.Error != "seed_waiting_for_funds" {
			t.Fatal("publication", publication.Error)
		}
	}
	if publication.Result == nil || publication.Result.Publication == nil {
		t.Fatal("bounded funding scan exhausted")
	}
	rawTx := publication.Result.Publication.RawTx
	var tip uint32
	if e = rpc.call(ctx, "getblockcount", nil, &tip); e != nil {
		t.Fatal(e)
	}
	seedClockAdvance(t, ctx, rpc, tip+3)
	var final pilot.SeedJob
	cancelled := 0
	for n := 0; n < 120; n++ {
		final = w.job(t, ctx, v, h.date, "seed_reconcile")
		if final.State == "cancelled" && final.Error == "seed_cancelled" && ctx.Err() == nil && cancelled < 2 {
			cancelled++
			t.Log("explicit read-only retry after retained cancellation", final.JobID)
			continue
		}
		if final.State != "completed" {
			t.Fatal("finality reconcile", final.Error)
		}
		if final.Result.Target != nil {
			if final.Result.Observation.TargetDescendants >= 60 {
				break
			}
			seedClockAdvance(t, ctx, rpc, final.Result.Target.Height+60)
		}
	}
	if final.Result == nil || final.Result.Target == nil || final.Result.Observation.TargetDescendants < 60 {
		t.Fatal("bounded target/depth wait")
	}
	attest := w.job(t, ctx, v, h.date, "seed_attest")
	if attest.State != "completed" || attest.Result.Finality == nil || attest.Result.Publication.RawTx != rawTx {
		t.Fatal("attest", attest.Error)
	}
	seedClockExportChain(t, ctx, rpc, root, attest.Result.Finality.ObservedTip)
	for name, hash := range map[string]string{"funding": fundHash, "anchor": attest.Result.Anchor.Hash, "target": attest.Result.Target.Hash} {
		var body string
		if e = rpc.call(ctx, "getblock", []any{hash, 0}, &body); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(root, "public-"+name+"-block.hex"), []byte(body+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	built := w.cardJob(t, ctx, v, h.date)
	if built.State != "completed" || built.Result == nil || built.Result.Replayed || !built.Result.Status.Usable {
		t.Fatal("positive card", built.Error)
	}
	plan := built.Result.Record.Plan
	if plan.BoutCount != 1 || plan.Counts.PairedCount != 2 || plan.Counts.TotalCount != uint32(len(plan.Entrants)) || plan.Counts.TotalCount != plan.Counts.PairedCount+plan.Counts.UnmatchedCount {
		t.Fatal("real booked pair counts", plan.Counts, plan.BoutCount)
	}
	for _, row := range plan.Entrants {
		if row.State == "unmatched" && row.Reason != "no_opponent_in_class" {
			t.Fatal("unexpected setup unmatched reason", row.Reason)
		}
	}
	t.Logf("genuine card total=%d paired=%d unmatched=%d bouts=%d", plan.Counts.TotalCount, plan.Counts.PairedCount, plan.Counts.UnmatchedCount, plan.BoutCount)
	recordBytes, _ := json.Marshal(built.Result.Record)
	replay := w.cardJob(t, ctx, v, h.date)
	if replay.State != "completed" || !replay.Result.Replayed || !reflect.DeepEqual(replay.Result.Record, built.Result.Record) {
		t.Fatal("immutable card replay", replay.Error)
	}
	terminalBytes, _ := json.Marshal(replay)
	// A metadata refresh must never alter a retained terminal result. The current
	// conflict overlay comes from explicit Status/Read, not the old job's status.
	peer := seedClockOwnedNamedRPC(t, home, "peer")
	for _, r := range []*seedClockRPC{rpc, peer} {
		var out any
		if e = r.call(ctx, "invalidateblock", []any{attest.Result.Target.Hash}, &out); e != nil {
			t.Fatal(e)
		}
	}
	conflict := w.job(t, ctx, v, h.date, "seed_reconcile")
	if conflict.State != "completed" || len(conflict.Result.Conflicts) == 0 {
		t.Fatal("retained source conflict", conflict.Error)
	}
	status := w.cardRead(t, ctx, pilot.CardRequest{Command: "card_status", ViewID: v, Saturday: h.date})
	if !status.OK || !status.Status.Exists || status.Status.Usable || status.Status.Source.Status.ConflictCount == 0 {
		t.Fatal("fresh conflict overlay", status.Error)
	}
	old := w.cardCall(t, pilot.CardRequest{Command: "card_job", ViewID: v, JobID: replay.JobID})
	oldBytes, _ := json.Marshal(old.Job)
	if !old.OK || !bytes.Equal(terminalBytes, oldBytes) {
		t.Fatal("terminal changed after overlay", old.Error)
	}
	read := w.cardRead(t, ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: h.date})
	if !read.OK || read.Record.Status.Usable || !reflect.DeepEqual(read.Record.Record, built.Result.Record) {
		t.Fatal("historical conflict read", read.Error)
	}
	conflictReplay := w.cardJob(t, ctx, v, h.date)
	if conflictReplay.State != "completed" || !conflictReplay.Result.Replayed || conflictReplay.Result.Status.Usable || !reflect.DeepEqual(conflictReplay.Result.Record, built.Result.Record) {
		t.Fatal("conflict replay rematched", conflictReplay.Error)
	}
	h.c.Stop()
	h.logout(t)
	if w.cardCall(t, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: h.date}).OK {
		t.Fatal("revoked view disclosed")
	}
	h.login(t, "incrementing256")
	iv := h.c.Snapshot().View.ViewID
	foreign := w.cardRead(t, ctx, pilot.CardRequest{Command: "card_record", ViewID: iv, Saturday: h.date})
	if !foreign.OK || !reflect.DeepEqual(foreign.Record.Record, built.Result.Record) {
		t.Fatal("shared second fixture record", foreign.Error)
	}
	h.logout(t)
	h.login(t, "zero256")
	oldView := h.c.Snapshot().View.ViewID
	gen := h.c.Snapshot().Account.Generation
	if e = h.c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool {
		s := h.c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("reset", s.Account.LastError)
		}
		return s.Account.State == "signed_in" && s.Account.Generation == gen+1
	})
	held := h.c.Snapshot()
	if !held.Account.Held || held.View.ViewID == oldView {
		t.Fatal("held reset identity")
	}
	heldReplay := w.cardJob(t, ctx, held.View.ViewID, h.date)
	if heldReplay.State != "completed" || !heldReplay.Result.Replayed || !reflect.DeepEqual(heldReplay.Result.Record, built.Result.Record) {
		t.Fatal("held card replay", heldReplay.Error)
	}
	lastID := heldReplay.JobID
	w.close(t)
	h.c = nil
	h.c, e = NewCardsForTestClock(h.cfg, func() time.Time { return time.UnixMilli(h.now.Load()) })
	if e != nil {
		t.Fatal(e)
	}
	h.login(t, "zero256")
	w = newCardClockWire(t, ctx, h.c)
	nv := h.c.Snapshot().View.ViewID
	reopened := w.cardRead(t, ctx, pilot.CardRequest{Command: "card_record", ViewID: nv, Saturday: h.date})
	if !reopened.OK || !reflect.DeepEqual(reopened.Record.Record, built.Result.Record) || reopened.Record.Status.Usable {
		t.Fatal("restart card", reopened.Error)
	}
	if j := w.cardCall(t, pilot.CardRequest{Command: "card_job", ViewID: nv, JobID: lastID}); j.OK || j.Error != "card_not_found" {
		t.Fatal("restart retained terminal", j.Error)
	}
	history := w.cardRead(t, ctx, pilot.CardRequest{Command: "card_history", ViewID: nv, Limit: 8})
	if !history.OK || len(history.History.Cards) != 1 || history.History.Cards[0].Usable {
		t.Fatal("history", history.Error)
	}
	if got, _ := json.Marshal(reopened.Record.Record); !bytes.Equal(recordBytes, got) {
		t.Fatal("record bytes changed")
	}
	public := struct {
		Version     uint32              `json:"version"`
		TaggedClock bool                `json:"tagged_clock"`
		NormalPoW   bool                `json:"normal_pow"`
		ClockMS     int64               `json:"clock_ms"`
		Saturday    string              `json:"saturday"`
		Snapshot    pilot.CardSnapshot  `json:"snapshot"`
		Build       gymcard.BuildResult `json:"build"`
		Read        gymcard.ReadResult  `json:"read"`
		Seed        any                 `json:"seed"`
	}{1, true, true, h.now.Load(), h.date, h.c.CardSnapshot(), *built.Result, *reopened.Record, *attest.Result}
	b, _ := json.MarshalIndent(public, "", "  ")
	if e = os.WriteFile(filepath.Join(root, "public-card-result.json"), append(b, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	w.close(t)
	h.c = nil
}
