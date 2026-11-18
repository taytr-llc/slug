//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
// Opt-in actual candidate career exercise on a runner-declared owned cold copy.
// The unchanged Set48 mining helpers retain normal PoW and explicit RPC scope.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerActualOwnedRoot(root string) error {
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		return fmt.Errorf("canonical career root required")
	}
	b, e := os.ReadFile(filepath.Join(root, "SET49-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(b, &m) != nil || m.Version != 1 || m.Scope != "Set49 owned cold copy" || m.SourceRoot == root || !filepath.IsAbs(m.SourceRoot) {
		return fmt.Errorf("Set49 copy declaration")
	}
	return nil
}

func TestCareerActualNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_CAREER_ACTUAL_CHILD")
	if root == "" {
		return
	}
	if e := careerActualOwnedRoot(root); e != nil {
		t.Fatal(e)
	}
	fail := func(e error) {
		_ = os.WriteFile(filepath.Join(root, "career-child-error.txt"), []byte(e.Error()+"\n"), 0600)
		os.Exit(2)
	}
	var p boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &p); e != nil || !p.NormalPoW || !p.TaggedClock || p.Saturday != "2026-12-05" {
		fail(fmt.Errorf("career source declaration: %v", e))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	c, e := accountpilot.NewCareersForTestClock(ctx, pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(p.ClockMS + 1) })
	if e != nil {
		fail(e)
	}
	e = pilot.ServeCareers(ctx, c, os.Stdin, os.Stdout)
	ce := c.Close()
	if e != nil && e != context.Canceled {
		fail(e)
	}
	if ce != nil {
		fail(ce)
	}
	os.Exit(0)
}

type careerActualNative struct {
	*resultActualNative
	activation      careerstate.Activation
	careerRows      []map[string]any
	lastCareerJobID string
}

func (h *careerActualNative) open() {
	h.t.Helper()
	if e := careerActualOwnedRoot(h.root); e != nil {
		h.t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerActualNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_CAREER_ACTUAL_CHILD="+h.root)
	var e error
	h.b, e = startStable(cmd)
	if e != nil {
		h.t.Fatal(e)
	}
	h.b.entriesEnabled, h.b.eventsEnabled, h.b.seedsEnabled, h.b.cardsEnabled, h.b.boutsEnabled, h.b.resultsEnabled, h.b.careersEnabled = true, true, true, true, true, true, true
	h.processes = newManifestProcessInventory(filepath.Join(h.root, "data"), h.node, cmd.Process.Pid)
}
func (h *careerActualNative) careerRequest(q pilot.CareerRequest) (pilot.CareerResponse, pilot.StableJob) {
	h.t.Helper()
	view := *h.view.View
	pause := func() {
		select {
		case <-h.ctx.Done():
			h.t.Fatal(h.ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	command, kind := "stable_inventory", "inventory"
	if q.Command == "career_roster" {
		command, kind = "stable_roster", "roster"
	}
	request := pilot.StableRequest{Command: command, ViewID: view.ViewID, After: q.After, FirstSlot: q.FirstSlot, Limit: q.Limit}
	deadline := time.Now().Add(45 * time.Second)
	var j pilot.StableJob
	for time.Now().Before(deadline) {
		r := h.b.StableRequest(h.ctx, request)
		if r.OK && r.Job != nil {
			j = *r.Job
			break
		}
		if r.Error != "busy" {
			h.t.Fatal("career source job admission", command, r.Error)
		}
		pause()
	}
	if j.JobID == "" || j.State != "queued" || j.View != view || j.Kind != kind {
		h.t.Fatal("career source job ACK", command, j.State)
	}
	id, store := j.JobID, j.StoreID
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			h.t.Fatal("career source job timeout", command)
		}
		pause()
		r := h.b.StableRequest(h.ctx, pilot.StableRequest{Command: "stable_job", ViewID: view.ViewID, JobID: id})
		if !r.OK || r.Job == nil || r.Job.JobID != id || r.Job.View != view || r.Job.Kind != kind || r.Job.StoreID != store {
			h.t.Fatal("career source job correlation", r.Error)
		}
		j = *r.Job
	}
	if j.State != "done" || j.Error != "" {
		h.t.Fatal("career source job failed", kind, j.State, j.Error)
	}
	q.ViewID, q.JobID = view.ViewID, id
	if kind == "inventory" {
		if j.Page == nil || j.Roster != nil || j.Page.After != q.After {
			h.t.Fatal("career inventory source tuple")
		}
		q.After = j.Page.After
	} else if j.Roster == nil || j.Page != nil || j.Roster.FirstSlot != q.FirstSlot {
		h.t.Fatal("career roster source tuple")
	}
	var r pilot.CareerResponse
	metadataDeadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(metadataDeadline) {
		r = h.b.CareerRequest(h.ctx, q)
		if r.Error != "busy" {
			break
		}
		pause()
	}
	if !r.OK || r.JobID != id || r.View == nil || *r.View != view || r.StoreID != store {
		h.t.Fatal("career completed source job binding", r.Error)
	}
	if kind == "inventory" {
		if r.Page == nil || !reflect.DeepEqual(r.Page.Source, *j.Page) {
			h.t.Fatal("career inventory source changed")
		}
	} else if r.Roster == nil || !reflect.DeepEqual(r.Roster.Source, *j.Roster) {
		h.t.Fatal("career roster source changed")
	}
	h.lastCareerJobID = id
	return r, j
}
func (h *careerActualNative) ownCorner(saved gymbout.Record) gymcard.BoutCorner {
	h.t.Helper()
	for _, c := range saved.Source.Corners {
		if c.Owner.GymID == h.view.GymID {
			return c
		}
	}
	h.t.Fatal("current original gym not in booked bout")
	return gymcard.BoutCorner{}
}

// Expected totals follow the published one-standard-bout rule directly, without
// calling the award fold under test or rerunning the fight engine.
func careerActualExpected(saved gymbout.Record, id string, awarded bool) fightercareer.Totals {
	if !awarded {
		return fightercareer.Totals{}
	}
	if saved.Result.Winner == "" {
		return fightercareer.Totals{Draws: 1, XP: 1, BasePoints: 4}
	}
	if saved.Result.Winner != id {
		return fightercareer.Totals{Losses: 1, XP: 1}
	}
	points := uint32(14)
	if saved.Result.Method == "decision" {
		points = 10
	}
	return fightercareer.Totals{Wins: 1, XP: 1, BasePoints: points}
}
func (h *careerActualNative) captureCareer(stage string, saved gymbout.Record, awarded bool) {
	h.t.Helper()
	corner := h.ownCorner(saved)
	id := corner.Member.Assignment.ProspectID
	expected := careerActualExpected(saved, id, awarded)
	inventory, roster := []pilot.CareerResponse{}, []pilot.CareerResponse{}
	inventoryJobs, rosterJobs := []pilot.StableJob{}, []pilot.StableJob{}
	invFound, rosterFound := 0, 0
	validate := func(a careerstate.Activation, source []gymstable.Item, items []gymstable.CareerItem, found *int) {
		if h.activation.ActivationID == "" {
			h.activation = a
		}
		if a != h.activation || len(source) != len(items) {
			h.t.Fatal(stage, "career activation or parallel page changed")
		}
		for n, x := range items {
			old := source[n]
			if old.Condition.ProXP != 0 || old.Assignment.ProspectID != x.ProspectID {
				h.t.Fatal(stage, "legacy item changed its XP/identity")
			}
			want := fightercareer.Totals{}
			if x.ProspectID == id {
				*found++
				want = expected
				if old.Assignment != corner.Member.Assignment {
					h.t.Fatal(stage, "career source assignment changed")
				}
			}
			if x.Totals != want || x.Condition.ProXP != int(want.XP) {
				h.t.Fatalf("%s career count/XP mismatch for %s: got %+v want %+v", stage, x.ProspectID, x.Totals, want)
			}
		}
	}
	after := uint64(0)
	for n := 0; n < 4; n++ {
		r, job := h.careerRequest(pilot.CareerRequest{Command: "career_inventory", After: after, Limit: 8})
		if r.Page == nil || r.Roster != nil {
			h.t.Fatal(stage, "inventory family")
		}
		p := r.Page
		validate(p.Activation, p.Source.Items, p.Items, &invFound)
		inventory = append(inventory, r)
		inventoryJobs = append(inventoryJobs, job)
		if !p.Source.HasMore {
			break
		}
		if p.Source.NextAfter <= after || n == 3 {
			h.t.Fatal(stage, "bounded real inventory pagination")
		}
		after = p.Source.NextAfter
	}
	first := uint32(1)
	for n := 0; n < 2; n++ {
		r, job := h.careerRequest(pilot.CareerRequest{Command: "career_roster", FirstSlot: first, Limit: 8})
		if r.Roster == nil || r.Page != nil {
			h.t.Fatal(stage, "roster family")
		}
		p := r.Roster
		source := make([]gymstable.Item, len(p.Source.Items))
		for i, x := range p.Source.Items {
			source[i] = x.Item
		}
		validate(p.Activation, source, p.Items, &rosterFound)
		roster = append(roster, r)
		rosterJobs = append(rosterJobs, job)
		if p.Source.NextSlot == 0 {
			break
		}
		if p.Source.NextSlot <= first || n == 1 {
			h.t.Fatal(stage, "bounded real roster pagination")
		}
		first = p.Source.NextSlot
	}
	if invFound != 1 || rosterFound != 1 {
		h.t.Fatal(stage, "booked fighter must occur exactly once in both current views", invFound, rosterFound)
	}
	h.careerRows = append(h.careerRows, map[string]any{"stage": stage, "view": h.view.View, "held": h.view.Account.Held, "prospect_id": id, "awarded": awarded, "expected_totals": expected, "inventory": inventory, "roster": roster, "inventory_source_jobs": inventoryJobs, "roster_source_jobs": rosterJobs})
}
func (h *careerActualNative) entryRequest(q pilot.EntryRequest) pilot.EntryResponse {
	h.t.Helper()
	if q.Command != "entry_snapshot" {
		q.ViewID = h.view.View.ViewID
	}
	var r pilot.EntryResponse
	seedNativeClockWait(h.t, h.ctx, func() bool { r = h.b.EntryRequest(h.ctx, q); return r.Error != "busy" })
	return r
}
func (h *careerActualNative) entryJob(q pilot.EntryRequest) pilot.EntryJob {
	h.t.Helper()
	r := h.entryRequest(q)
	if !r.OK || r.Job == nil || r.Job.State != "queued" {
		h.t.Fatal("career entry ACK", q.Command, r.Error)
	}
	j := *r.Job
	seedNativeClockWait(h.t, h.ctx, func() bool {
		r = h.entryRequest(pilot.EntryRequest{Command: "entry_job", JobID: j.JobID})
		if !r.OK || r.Job == nil {
			h.t.Fatal("career entry poll", r.Error)
		}
		j = *r.Job
		return j.State != "queued" && j.State != "running"
	})
	return j
}
func (h *careerActualNative) refuseNextEntry(saved gymbout.Record) map[string]any {
	h.t.Helper()
	if h.view.Account.Held {
		h.t.Fatal("C1 refusal must exercise unheld original authority")
	}
	id := h.ownCorner(saved).Member.Assignment.ProspectID
	boot := h.entryRequest(pilot.EntryRequest{Command: "entry_snapshot"})
	if !boot.OK || boot.Snapshot == nil || boot.Snapshot.State != "idle" || boot.Snapshot.Error != "" || boot.View == nil || *boot.View != *h.view.View || boot.Snapshot.View == nil || *boot.Snapshot.View != *h.view.View || boot.StoreID == "" || boot.Snapshot.StoreID != boot.StoreID {
		state := ""
		if boot.Snapshot != nil {
			state = boot.Snapshot.State
		}
		h.t.Fatalf("original next-entry bootstrap: ok=%t state=%q error=%q", boot.OK, state, boot.Error)
	}
	before := h.entryRequest(pilot.EntryRequest{Command: "entry_status", Saturday: "2026-12-12"})
	if !before.OK || before.Status == nil || before.Status.Revision != 0 || len(before.Status.SavedIDs) != 0 || before.Status.PendingOperationID != "" {
		h.t.Fatal("empty next C1 event", before.Error)
	}
	request := gymentry.Request{Version: 1, EventID: before.Status.Event.EventID, Saturday: before.Status.Event.Saturday, Action: "replace", ExpectedRevision: before.Status.Revision, ExpectedRosterRevision: before.Status.CurrentRosterRevision, ProspectIDs: []string{id}}
	prepared := h.entryJob(pilot.EntryRequest{Command: "entry_prepare", Request: &request})
	rejected := prepared
	var applied *pilot.EntryJob
	if prepared.State == "complete" {
		if prepared.Intent == nil || prepared.OperationID == "" {
			h.t.Fatal("prepared C1 intent missing")
		}
		j := h.entryJob(pilot.EntryRequest{Command: "entry_apply", OperationID: prepared.OperationID})
		applied = &j
		rejected = j
	}
	if rejected.State != "failed" || rejected.Error != "career_candidate_ineligible" || !reflect.DeepEqual(rejected.IneligibleProspectIDs, []string{id}) || rejected.Mutation != nil {
		h.t.Fatal("awarded fighter admitted to zero-record C1", rejected.State, rejected.Error, rejected.IneligibleProspectIDs)
	}
	after := h.entryRequest(pilot.EntryRequest{Command: "entry_status", Saturday: "2026-12-12"})
	if !after.OK || after.Status == nil || after.Status.Revision != before.Status.Revision || len(after.Status.SavedIDs) != 0 || after.Status.LatestOperationID != "" {
		h.t.Fatal("declined C1 Apply changed entry revision/receipt", after.Error)
	}
	var abandoned *pilot.EntryResponse
	if after.Status.PendingOperationID != "" {
		if after.Status.PendingOperationID != prepared.OperationID {
			h.t.Fatal("foreign pending C1 operation")
		}
		r := h.entryRequest(pilot.EntryRequest{Command: "entry_abandon", OperationID: prepared.OperationID})
		if !r.OK || r.Operation == nil || r.Operation.Operation == nil || r.Operation.Operation.Receipt != nil || r.Operation.Operation.State != "abandoned" {
			h.t.Fatal("explicit C1 pending cleanup", r.Error)
		}
		abandoned = &r
	}
	final := h.entryRequest(pilot.EntryRequest{Command: "entry_status", Saturday: "2026-12-12"})
	if !final.OK || final.Status == nil || final.Status.Revision != 0 || len(final.Status.SavedIDs) != 0 || final.Status.PendingOperationID != "" {
		h.t.Fatal("C1 cleanup changed absent enrollment", final.Error)
	}
	return map[string]any{"before": before, "prepare": prepared, "apply": applied, "rejected": rejected, "after": after, "abandon": abandoned, "final": final, "prospect_id": id, "scope": "Concrete unheld next-Saturday entry guard refuses a nonzero career record before enrollment; no later card or general matchmaking policy is claimed."}
}

func TestCareerActualNativePublication(t *testing.T) {
	root, node := os.Getenv("SLUG_CAREER_ACTUAL_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit Set49 prepared owned copy")
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		t.Fatal("canonical root")
	}
	marker, e := os.ReadFile(filepath.Join(root, "SET49-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(marker, &m) != nil || m.Version != 1 || m.Scope != "Set49 owned cold copy" || m.SourceRoot == root {
		t.Fatal("Set49 copy declaration")
	}
	var prepared boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &prepared); e != nil || prepared.Build.Record.Plan.BoutCount != 1 || !prepared.Build.Status.Usable || prepared.Saturday != "2026-12-05" {
		t.Fatal("genuine later source", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	h := &careerActualNative{resultActualNative: &resultActualNative{t: t, ctx: ctx, root: root, node: node, date: prepared.Saturday, bid: prepared.Build.Record.Plan.Bouts[0].BoutID, baseline: map[string]pilot.ManifestSnapshot{}}}
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
	h.captureCareer("saved-provisional", saved, false)
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
	h.captureCareer("included-provisional", saved, false)
	// Reopen the actual parent/helper and reconnect through an explicit Check.
	firstRecord := h.read().Record
	h.close()
	h.open()
	h.login("zero256")
	if !reflect.DeepEqual(h.read().Record, firstRecord) {
		t.Fatal("reopen changed retained first publication")
	}
	h.captureCareer("reopened-provisional", saved, false)
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
	h.captureCareer("depth59-provisional", saved, false)
	h.mineTo(anchor.Height+60, 0x45)
	h.checkUntil("depth60", func(r gymresult.ActionResult) bool { return r.Status.Descendants == 60 })
	finalizedJob := h.job(pilot.ResultRequest{Command: "result_attest"})
	if finalizedJob.State != "completed" || finalizedJob.Result == nil || finalizedJob.Result.Record.Finality == nil || !finalizedJob.Result.Status.CurrentFinality {
		t.Fatal("result's own finality", finalizedJob.Error)
	}
	finalized := *finalizedJob.Result
	if finalized.Record.Finality.ObservedAtMS < h.activation.ActivatedAtMS {
		t.Fatal("award predates explicit career activation")
	}
	h.captureCareer("finalized60", saved, true)
	h.export("finalized60")
	h.mineTo(anchor.Height+61, 0x45)
	at61 := h.checkUntil("depth61", func(r gymresult.ActionResult) bool { return r.Status.Descendants == 61 })
	if !reflect.DeepEqual(at61.Record.Finality, finalized.Record.Finality) || !reflect.DeepEqual(at61.Record.Checkpoint, finalized.Record.Checkpoint) {
		t.Fatal("first result certificate changed at61")
	}
	h.export("finalized61")
	h.captureCareer("finalized61", saved, true)
	replayed := h.job(pilot.ResultRequest{Command: "result_attest"})
	if replayed.State != "completed" || replayed.Result == nil || !reflect.DeepEqual(replayed.Result.Record.Finality, finalized.Record.Finality) || !reflect.DeepEqual(replayed.Result.Record.Checkpoint, finalized.Record.Checkpoint) {
		t.Fatal("repeat attestation changed first finality", replayed.Error)
	}
	h.captureCareer("repeat-attestation", saved, true)
	// Report-only crossing conflict retains original certificate/inclusion forever.
	crossingRecord := h.read().Record
	h.invalidate(anchor.Hash)
	h.close()
	h.open()
	h.login("zero256")
	if !reflect.DeepEqual(h.read().Record, crossingRecord) {
		t.Fatal("checkpoint crossing restart changed retained record")
	}
	h.captureCareer("reopened-after-finality", saved, true)
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
	h.captureCareer("retained-after-conflict", saved, true)
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
	if r := h.b.CareerRequest(ctx, pilot.CareerRequest{Command: "career_inventory", ViewID: oldView, JobID: h.lastCareerJobID, Limit: 8}); r.OK || r.Error != "stale_view" || r.JobID != "" || r.Page != nil || r.Roster != nil {
		t.Fatal("revoked original career View disclosed")
	}
	h.captureCareer("held-after-reset", saved, true)
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
	h.captureCareer("other-gym", saved, true)
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
	resultActualSave(t, root, "public-result-native-evidence.json", map[string]any{"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": true, "automatic_admissions": 0, "baseline": h.baseline, "rows": h.rows, "first_inclusion": firstInclusion, "reincluded": reincluded, "finalized": finalized, "at61": at61, "conflict": conflict, "conflict_proof": conflictProof, "other_view": other, "saved_bout": saved, "original_cards": oldCards, "scope": "Actual native private-pipe result API and owned node-only processes; original publication evidence remains awards:none. Candidate career evidence is separately recorded in public-career-native-evidence.json. No chooser, GUI or production account claim."})
	// Additional career assertions follow the complete original publication evidence;
	// they do not change its login sequence, six exports or historical source rows.
	c1 := h.refuseNextEntry(saved)
	h.captureCareer("after-c1-refusal", saved, true)
	resultActualSave(t, root, "public-career-native-evidence.json", map[string]any{
		"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": true,
		"automatic_admissions": 0, "activation": h.activation, "rows": h.careerRows,
		"result_key": finalized.Record.ResultKey, "finality": finalized.Record.Finality,
		"saved_bout": saved, "c1_refusal": c1,
		"scope": "Actual native candidate CareerInventory/Roster from the concrete activated Result reader. One standard award per fighter at first own finality; permanent through replay, restart and reported conflict. Original Item XP remains zero. No standings, title allocation, GUI or production authority claim.",
	})
	h.close()
}
