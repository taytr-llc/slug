// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

type seedAuthority struct {
	pin       gymseed.Pin
	storeID   string
	createdMS int64
}
type seedJobExpectation struct{ eventKey, saturday, kind, state, terminalDigest string }
type seedCorrelation struct {
	view         pilot.ManifestViewPin
	jobs         map[string]seedJobExpectation
	snapshotJobs map[string]bool
}

func LaunchSeeds(helper, dataDir, node string) (*StableBridge, error) {
	b, e := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--data-dir", dataDir, "--node-binary", node))
	if e == nil {
		b.entriesEnabled, b.eventsEnabled, b.seedsEnabled = true, true, true
	}
	return b, e
}
func seedFailure(id uint64, code string) pilot.SeedResponse {
	if id == 0 {
		id = 1
	}
	return pilot.SeedResponse{ID: id, Error: code}
}
func (b *StableBridge) clearSeedView() { b.mu.Lock(); b.seed = nil; b.mu.Unlock() }
func (b *StableBridge) seedContext() (seedAuthority, pilot.ManifestViewPin, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.seedPin == nil || b.seed == nil || b.view == nil || b.seed.view != *b.view {
		return seedAuthority{}, pilot.ManifestViewPin{}, false
	}
	return *b.seedPin, b.seed.view, true
}
func (b *StableBridge) seedExpectation(id string) (seedJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seed == nil {
		return seedJobExpectation{}, false, false
	}
	e, ok := b.seed.jobs[id]
	return e, ok, b.seed.snapshotJobs[id]
}
func (b *StableBridge) canTrackSeedJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seed == nil {
		return false
	}
	for _, e := range b.seed.jobs {
		if !eventTerminal(e.state) {
			return false
		}
	}
	return true
}
func seedBegin(command string) bool {
	return oneOf(command, "seed_prepare", "seed_publish", "seed_reconcile", "seed_attest")
}

func (b *StableBridge) SeedRequest(ctx context.Context, req pilot.SeedRequest) pilot.SeedResponse {
	if !b.seedsEnabled {
		return seedFailure(0, "seed_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, e := pilot.MarshalSeedRequest(pre); e != nil {
		return seedFailure(0, "invalid_request")
	}
	if req.Command == "seed_snapshot" {
		// EventSnapshot establishes Manifest→Stable→Entry before the Event pin.
		e := b.EventRequest(ctx, pilot.EventRequest{Command: "event_snapshot"})
		if !e.OK {
			if e.Error == "busy" {
				return seedFailure(0, "busy")
			}
			return seedFailure(0, "seed_unavailable")
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return seedFailure(0, "busy")
		}
		return seedFailure(0, "seed_unavailable")
	}
	defer func() { <-b.gate }()
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.seedRequestHeld(c, id, req)
}

func (b *StableBridge) seedRequestHeld(ctx context.Context, id uint64, req pilot.SeedRequest) pilot.SeedResponse {
	req.ID = id
	raw, e := pilot.MarshalSeedRequest(req)
	if e != nil {
		return seedFailure(id, "invalid_request")
	}
	a, view, ready := b.seedContext()
	if req.Command != "seed_snapshot" && (!ready || view.ViewID != req.ViewID) {
		return seedFailure(id, "stale_view")
	}
	var expected seedJobExpectation
	if seedBegin(req.Command) {
		if !b.canTrackSeedJob() {
			return seedFailure(id, "busy")
		}
		event, e := gymevent.EventForDate(a.pin.SourcePin, req.Saturday)
		if e != nil {
			return seedFailure(id, "seed_calendar")
		}
		key, e := gymseed.EventKey(a.pin, event)
		if e != nil {
			return seedFailure(id, "invalid_request")
		}
		expected = seedJobExpectation{eventKey: key, saturday: req.Saturday, kind: strings.TrimPrefix(req.Command, "seed_")}
	}
	if oneOf(req.Command, "seed_job", "seed_cancel") {
		var known, discovered bool
		expected, known, discovered = b.seedExpectation(req.JobID)
		if !known && !discovered {
			return seedFailure(id, "seed_not_found")
		}
		if !known && req.Command == "seed_cancel" {
			found := b.seedRequestHeld(ctx, id, pilot.SeedRequest{Command: "seed_job", ViewID: req.ViewID, JobID: req.JobID})
			if !found.OK {
				return found
			}
			expected, known, _ = b.seedExpectation(req.JobID)
			if !known {
				return seedFailure(id, "outcome_unknown")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return seedFailure(0, "outcome_unknown")
			}
			req.ID = id
			raw, e = pilot.MarshalSeedRequest(req)
			if e != nil {
				return seedFailure(id, "invalid_request")
			}
		}
	}
	unknown := seedBegin(req.Command) || oneOf(req.Command, "seed_job", "seed_cancel")
	if ctx.Err() != nil {
		return seedFailure(id, "seed_cancelled")
	}
	frame, e := b.exchangeSeedFamily(ctx, id, false, false, false, false, true, raw)
	if e != nil || frame.seed == nil {
		b.clearSeedView()
		if unknown {
			return seedFailure(id, "outcome_unknown")
		}
		return seedFailure(id, "seed_unavailable")
	}
	r := *frame.seed
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if unknown && oneOf(r.Error, "unauthorized", "stale_view", "seed_unavailable", "seed_not_found") {
			return seedFailure(id, "outcome_unknown")
		}
		return r
	}
	valid := false
	if req.Command == "seed_snapshot" {
		valid = b.rememberSeedSnapshot(r)
	} else {
		valid = b.correlateSeedEnvelope(req, r, a, view)
		if valid && r.Job != nil {
			j := *r.Job
			if expected.eventKey == "" {
				expected = seedJobExpectation{eventKey: j.EventKey, saturday: j.Saturday, kind: j.Kind}
			}
			valid = b.correlateSeedJob(req, j, expected, a, view) && b.rememberSeedJob(j, expected, seedBegin(req.Command))
		}
	}
	if !valid {
		b.fail(ErrUnavailable)
		if unknown {
			return seedFailure(id, "outcome_unknown")
		}
		return seedFailure(id, "seed_unavailable")
	}
	return r
}

func (b *StableBridge) rememberSeedSnapshot(r pilot.SeedResponse) bool {
	s := r.Snapshot
	if s == nil {
		return false
	}
	if s.State == "unavailable" {
		if s.View != nil || r.View != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" {
			return false
		}
		b.clearSeedView()
		return true
	}
	event, _, ok := b.eventContext()
	if s.State == "setup_incomplete" {
		// A source can still be incomplete. The established Manifest view is
		// required, but this response cannot establish any Seed authority.
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view || b.seedPin != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" || s.CreatedAtMS != 0 {
			return false
		}
		b.seed = nil
		return true
	}
	if !ok || s.Pin == nil || s.CreatedAtMS <= 0 {
		return false
	}
	expected, e := gymseed.CandidatePin(event.pin)
	if e != nil {
		return false
	}
	pin, e := gymseed.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	want, e := gymseed.MarshalPin(expected)
	if e != nil {
		return false
	}
	got, e := gymseed.MarshalPin(pin)
	if e != nil || string(got) != string(want) {
		return false
	}
	id, e := gymseed.ID(expected)
	if e != nil || id != s.StoreID || r.StoreID != id || s.PublisherAccountID != expected.PublisherAccountID {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view {
		return false
	}
	if b.seedPin == nil {
		b.seedPin = &seedAuthority{pin: pin, storeID: id, createdMS: s.CreatedAtMS}
	} else if b.seedPin.storeID != id || b.seedPin.createdMS != s.CreatedAtMS {
		return false
	}
	if b.seed == nil || b.seed.view != *s.View {
		b.seed = &seedCorrelation{view: *s.View, jobs: map[string]seedJobExpectation{}}
	}
	b.seed.snapshotJobs = map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.seed.snapshotJobs[id] = true
		}
	}
	for id := range b.seed.jobs {
		if !b.seed.snapshotJobs[id] {
			delete(b.seed.jobs, id)
		}
	}
	return true
}

func (b *StableBridge) correlateSeedEnvelope(req pilot.SeedRequest, r pilot.SeedResponse, a seedAuthority, view pilot.ManifestViewPin) bool {
	if r.View == nil || *r.View != view || r.StoreID != a.storeID {
		return false
	}
	_, current, ok := b.seedContext()
	if !ok || current != view {
		return false
	}
	switch req.Command {
	case "seed_status":
		if r.Status == nil || r.Status.Event.Saturday != req.Saturday || r.Status.Exists && r.Status.Event.CutoffMS <= a.createdMS {
			return false
		}
		_, e := gymseed.ValidateStatus(a.pin, *r.Status)
		return e == nil
	case "seed_record":
		return r.Record != nil && r.Record.SourceReceipt.Input.Event.Saturday == req.Saturday && b.validSeedRecord(a, *r.Record)
	case "seed_history":
		if r.History == nil || r.History.AfterEventKey != req.AfterEventKey || len(r.History.Events) > int(req.Limit) {
			return false
		}
		if _, e := gymseed.ValidateHistoryPage(a.pin, *r.History); e != nil {
			return false
		}
		for _, s := range r.History.Events {
			if s.Event.CutoffMS <= a.createdMS {
				return false
			}
		}
		return true
	case "seed_prepare", "seed_publish", "seed_reconcile", "seed_attest", "seed_job", "seed_cancel":
		return r.Job != nil && r.Job.View == view && r.Job.StoreID == a.storeID && (seedBegin(req.Command) || r.Job.JobID == req.JobID)
	}
	return false
}
func (b *StableBridge) validSeedRecord(a seedAuthority, r gymseed.Record) bool {
	if r.SourceReceipt.Input.Event.CutoffMS <= a.createdMS || r.PreparedAtMS < a.createdMS {
		return false
	}
	// ParseSeedResponse already checked closed transport shape, signatures,
	// source receipt digests and all local consistency constraints. Bind that
	// value to this established authority without projecting fighters on polls.
	event, err := gymevent.EventForDate(a.pin.SourcePin, r.SourceReceipt.Input.Event.Saturday)
	if err != nil || event != r.SourceReceipt.Input.Event || r.StoreID != a.storeID {
		return false
	}
	key, err := gymseed.EventKey(a.pin, event)
	d := r.Declaration.Declaration
	if err != nil || key != r.EventKey || d.EventKey != key || d.StoreID != a.storeID || d.SourceStoreID != a.pin.SourceStoreID || d.PublisherChainID != a.pin.PublisherChainID || d.Network != a.pin.Network || d.Genesis != a.pin.Genesis || d.FloorHeight != a.pin.FloorHeight || d.FloorHash != a.pin.FloorHash || d.RulesID != a.pin.RulesID || d.PublisherPublicKey != a.pin.PublisherPublicKey || d.CheckpointPublicKey != a.pin.CheckpointPublicKey {
		return false
	}
	participants := r.SourceReceipt.Input.Participants
	if len(participants) != len(a.pin.SourcePin.Participants) {
		return false
	}
	for i, p := range a.pin.SourcePin.Participants {
		v := participants[i]
		sid, err := gymentry.ID(p)
		if err != nil {
			return false
		}
		inner, err := gymentry.EventForDate(p, event.Saturday)
		owner := gymevent.AssignmentOwner{OwnershipBasis: "candidate_assignment", AccountAuthorityID: p.StablePin.AccountAuthorityID, AccountID: p.StablePin.AccountID, GymID: p.StablePin.Identity.GymID}
		if err != nil || v.ParticipantID != owner.GymID || v.Owner != owner || v.EntryStoreID != sid || v.Entry.StoreID != sid || v.Entry.Event != inner || v.Roster.StoreID != p.StableStoreID {
			return false
		}
		for _, m := range v.Members {
			if m.Assignment.GymID != owner.GymID || m.Assignment.DescriptorID != p.StablePin.Source.DescriptorID {
				return false
			}
		}
	}
	return true
}
func seedJobDigest(j pilot.SeedJob) string {
	raw, e := json.Marshal(j)
	if e != nil || len(raw) > pilot.MaxSeedJobSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (b *StableBridge) correlateSeedJob(req pilot.SeedRequest, j pilot.SeedJob, e seedJobExpectation, a seedAuthority, view pilot.ManifestViewPin) bool {
	event, err := gymevent.EventForDate(a.pin.SourcePin, j.Saturday)
	if err != nil {
		return false
	}
	key, err := gymseed.EventKey(a.pin, event)
	if err != nil || j.EventKey != key || j.EventKey != e.eventKey || j.Saturday != e.saturday || j.Kind != e.kind || j.StoreID != a.storeID || j.View != view {
		return false
	}
	if seedBegin(req.Command) && j.State != "queued" || e.state == "running" && j.State == "queued" {
		return false
	}
	if eventTerminal(e.state) {
		return j.State == e.state && seedJobDigest(j) == e.terminalDigest
	}
	if j.Result != nil {
		return j.Result.EventKey == key && b.validSeedRecord(a, *j.Result)
	}
	return j.State != "completed"
}
func (b *StableBridge) rememberSeedJob(j pilot.SeedJob, e seedJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.seed == nil || b.seedPin == nil || j.View != b.seed.view || j.StoreID != b.seedPin.storeID {
		return false
	}
	if began || eventTerminal(j.State) {
		for id, old := range b.seed.jobs {
			if id != j.JobID && eventTerminal(old.state) {
				delete(b.seed.jobs, id)
			}
		}
	}
	if _, known := b.seed.jobs[j.JobID]; !known && len(b.seed.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if eventTerminal(j.State) {
		e.terminalDigest = seedJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.seed.jobs[j.JobID] = e
	return true
}
