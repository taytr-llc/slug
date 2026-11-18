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
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

type boutAuthority struct {
	pin             gymbout.Pin
	storeID         string
	createdMS       int64
	sourceCreatedMS int64
}
type boutJobExpectation struct{ cardID, saturday, boutID, kind, state, terminalDigest string }
type boutCorrelation struct {
	view         pilot.ManifestViewPin
	jobs         map[string]boutJobExpectation
	snapshotJobs map[string]bool
}

func LaunchBouts(helper, dataDir, node string) (*StableBridge, error) {
	b, e := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--data-dir", dataDir, "--node-binary", node))
	if e == nil {
		b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled, b.boutsEnabled = true, true, true, true, true
	}
	return b, e
}
func boutFailure(id uint64, code string) pilot.BoutResponse {
	if id == 0 {
		id = 1
	}
	return pilot.BoutResponse{ID: id, Error: code}
}
func (b *StableBridge) clearBoutView() {
	b.mu.Lock()
	b.bout = nil
	b.boutPresentation = nil
	b.mu.Unlock()
}
func (b *StableBridge) boutContext() (boutAuthority, pilot.ManifestViewPin, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.boutPin == nil || b.bout == nil || b.view == nil || b.bout.view != *b.view {
		return boutAuthority{}, pilot.ManifestViewPin{}, false
	}
	return *b.boutPin, b.bout.view, true
}
func (b *StableBridge) boutExpectation(id string) (boutJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bout == nil {
		return boutJobExpectation{}, false, false
	}
	e, ok := b.bout.jobs[id]
	return e, ok, b.bout.snapshotJobs[id]
}
func (b *StableBridge) canTrackBoutJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bout == nil {
		return false
	}
	for _, e := range b.bout.jobs {
		if !eventTerminal(e.state) {
			return false
		}
	}
	return true
}
func boutBegin(command string) bool {
	return command == "bout_run"
}

func (b *StableBridge) BoutRequest(ctx context.Context, req pilot.BoutRequest) pilot.BoutResponse {
	if !b.boutsEnabled {
		return boutFailure(0, "bout_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, e := pilot.MarshalBoutRequest(pre); e != nil {
		return boutFailure(0, "invalid_request")
	}
	if req.Command == "bout_snapshot" {
		// CardSnapshot establishes every prior authority before Bout discovery.
		e := b.CardRequest(ctx, pilot.CardRequest{Command: "card_snapshot"})
		if !e.OK {
			if e.Error == "busy" {
				return boutFailure(0, "busy")
			}
			return boutFailure(0, "bout_unavailable")
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return boutFailure(0, "busy")
		}
		return boutFailure(0, "bout_unavailable")
	}
	defer func() { <-b.gate }()
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.boutRequestHeld(c, id, req)
}

func (b *StableBridge) boutRequestHeld(ctx context.Context, id uint64, req pilot.BoutRequest) pilot.BoutResponse {
	req.ID = id
	raw, e := pilot.MarshalBoutRequest(req)
	if e != nil {
		return boutFailure(id, "invalid_request")
	}
	a, view, ready := b.boutContext()
	if req.Command != "bout_snapshot" && (!ready || view.ViewID != req.ViewID) {
		return boutFailure(id, "stale_view")
	}
	var expected boutJobExpectation
	discoveredJob := false
	if boutBegin(req.Command) {
		if !b.canTrackBoutJob() {
			return boutFailure(id, "busy")
		}
		event, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin, req.Saturday)
		if e != nil {
			return boutFailure(id, "bout_calendar")
		}
		key, e := gymcard.CardID(a.pin.SourcePin, event)
		if e != nil {
			return boutFailure(id, "invalid_request")
		}
		expected = boutJobExpectation{cardID: key, saturday: req.Saturday, boutID: req.BoutID, kind: strings.TrimPrefix(req.Command, "bout_")}
	}
	if oneOf(req.Command, "bout_job", "bout_cancel") {
		var known, discovered bool
		expected, known, discovered = b.boutExpectation(req.JobID)
		discoveredJob = !known && discovered
		if !known && !discovered {
			return boutFailure(id, "bout_not_found")
		}
		if !known && req.Command == "bout_cancel" {
			found := b.boutRequestHeld(ctx, id, pilot.BoutRequest{Command: "bout_job", ViewID: req.ViewID, JobID: req.JobID})
			if !found.OK {
				return found
			}
			expected, known, _ = b.boutExpectation(req.JobID)
			if !known {
				return boutFailure(id, "outcome_unknown")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return boutFailure(0, "outcome_unknown")
			}
			req.ID = id
			raw, e = pilot.MarshalBoutRequest(req)
			if e != nil {
				return boutFailure(id, "invalid_request")
			}
		}
	}
	unknown := boutBegin(req.Command) || req.Command == "bout_cancel" || req.Command == "bout_job" && discoveredJob || oneOf(req.Command, "bout_status", "bout_record", "bout_card_status")
	if ctx.Err() != nil {
		return boutFailure(id, "bout_cancelled")
	}
	frame, e := b.exchangeBoutFamily(ctx, id, false, false, false, false, false, false, true, raw)
	if e != nil || frame.bout == nil {
		b.clearBoutView()
		if unknown {
			return boutFailure(id, "outcome_unknown")
		}
		return boutFailure(id, "bout_unavailable")
	}
	r := *frame.bout
	if !r.OK {
		// Optional context on a refusal is still authority-bearing. A stale
		// or substituted error envelope must not become a definite refusal.
		if (r.View != nil && *r.View != view) || (r.StoreID != "" && (!ready || r.StoreID != a.storeID)) {
			b.fail(ErrUnavailable)
			if unknown {
				return boutFailure(id, "outcome_unknown")
			}
			return boutFailure(id, "bout_unavailable")
		}
		if oneOf(r.Error, "unauthorized", "forbidden", "stale_view", "outcome_unknown", "bout_unavailable", "bout_not_found", "bout_source_incomplete", "bout_source_changed", "bout_corrupt", "bout_clock") {
			b.clearBoutPresentation()
		}
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		// A well-formed definite refusal is evidence, not transport uncertainty.
		// Only a lost/malformed dispatched exchange uses the rule above.
		return r
	}
	valid := false
	if req.Command == "bout_snapshot" {
		valid = b.rememberBoutSnapshot(r)
	} else {
		valid = b.correlateBoutEnvelope(req, r, a, view)
		if valid && r.Job != nil {
			j := *r.Job
			if expected.cardID == "" {
				expected = boutJobExpectation{cardID: j.CardID, saturday: j.Saturday, boutID: j.BoutID, kind: j.Kind}
			}
			valid = b.correlateBoutJob(req, j, expected, a, view) && b.rememberBoutJob(j, expected, boutBegin(req.Command))
		}
	}
	if !valid {
		b.fail(ErrUnavailable)
		if unknown {
			return boutFailure(id, "outcome_unknown")
		}
		return boutFailure(id, "bout_unavailable")
	}
	if boutBegin(req.Command) {
		b.clearBoutPresentation()
	}
	if r.Record != nil {
		if !b.selectBoutRecord(view, r.Record.Record) {
			b.fail(ErrUnavailable)
			return boutFailure(id, "bout_unavailable")
		}
	}
	if r.Job != nil && r.Job.Result != nil {
		if !b.selectBoutRecord(view, r.Job.Result.Record, true) {
			b.fail(ErrUnavailable)
			return boutFailure(id, "bout_unavailable")
		}
	}
	return r
}

func (b *StableBridge) rememberBoutSnapshot(r pilot.BoutResponse) bool {
	s := r.Snapshot
	if s == nil {
		return false
	}
	if s.State == "unavailable" {
		if s.View != nil || r.View != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" {
			return false
		}
		b.clearBoutView()
		return true
	}
	source, _, ok := b.cardContext()
	if s.State == "setup_incomplete" {
		// A source can still be incomplete. The established Manifest view is
		// required, but this response cannot establish any Bout authority.
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view || b.boutPin != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" || s.CreatedAtMS != 0 {
			return false
		}
		b.bout = nil
		b.boutPresentation = nil
		return true
	}
	if !ok || s.Pin == nil || s.CreatedAtMS <= 0 {
		return false
	}
	expected, e := gymbout.CandidatePin(source.pin)
	if e != nil {
		return false
	}
	pin, e := gymbout.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	want, e := gymbout.MarshalPin(expected)
	if e != nil {
		return false
	}
	got, e := gymbout.MarshalPin(pin)
	if e != nil || string(got) != string(want) {
		return false
	}
	id, e := gymbout.ID(expected)
	if e != nil || id != s.StoreID || r.StoreID != id || s.SourceCreatedAtMS != source.createdMS || s.CreatedAtMS < source.createdMS {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view {
		return false
	}
	if b.boutPin == nil {
		b.boutPin = &boutAuthority{pin: pin, storeID: id, createdMS: s.CreatedAtMS, sourceCreatedMS: s.SourceCreatedAtMS}
	} else if b.boutPin.storeID != id || b.boutPin.createdMS != s.CreatedAtMS || b.boutPin.sourceCreatedMS != s.SourceCreatedAtMS {
		return false
	}
	if b.bout == nil || b.bout.view != *s.View {
		b.bout = &boutCorrelation{view: *s.View, jobs: map[string]boutJobExpectation{}}
	}
	b.bout.snapshotJobs = map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.bout.snapshotJobs[id] = true
		}
	}
	for id := range b.bout.jobs {
		if !b.bout.snapshotJobs[id] {
			delete(b.bout.jobs, id)
		}
	}
	return true
}

func (b *StableBridge) correlateBoutEnvelope(req pilot.BoutRequest, r pilot.BoutResponse, a boutAuthority, view pilot.ManifestViewPin) bool {
	if r.View == nil || *r.View != view || r.StoreID != a.storeID {
		return false
	}
	_, current, ok := b.boutContext()
	if !ok || current != view {
		return false
	}
	switch req.Command {
	case "bout_card_status":
		if r.CardStatus == nil || r.CardStatus.Source.Event.Saturday != req.Saturday || !validBoutState(a, r.CardStatus.Source) || r.CardStatus.StoreID != a.storeID {
			return false
		}
		for _, saved := range r.CardStatus.Bouts {
			if saved.Exists && saved.CreatedAtMS < a.createdMS {
				return false
			}
		}
		return true
	case "bout_status":
		return r.Status != nil && r.Status.Source.Event.Saturday == req.Saturday && r.Status.Saved.Bout.BoutID == req.BoutID && validBoutStatus(a, *r.Status)
	case "bout_record":
		return r.Record != nil && r.Record.Status.Source.Event.Saturday == req.Saturday && r.Record.Record.BoutID == req.BoutID && validBoutStatus(a, r.Record.Status) && b.validBoutRecord(a, r.Record.Record)
	case "bout_run", "bout_job", "bout_cancel":
		return r.Job != nil && r.Job.View == view && r.Job.StoreID == a.storeID && (boutBegin(req.Command) || r.Job.JobID == req.JobID)
	}
	return false
}
func validBoutState(a boutAuthority, s gymcard.BoutState) bool {
	ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin, s.Event.Saturday)
	if e != nil || ev != s.Event || s.CardExists && s.CardCreatedAtMS < a.sourceCreatedMS {
		return false
	}
	id, e := gymcard.CardID(a.pin.SourcePin, ev)
	return e == nil && id == s.CardID
}
func validBoutStatus(a boutAuthority, s gymbout.Status) bool {
	return s.StoreID == a.storeID && validBoutState(a, s.Source) && (!s.Saved.Exists || s.Saved.CreatedAtMS >= a.createdMS)
}
func (b *StableBridge) validBoutRecord(a boutAuthority, r gymbout.Record) bool {
	s := r.Source
	if r.StoreID != a.storeID || r.CreatedAtMS < a.createdMS || s.StoreID != a.pin.SourceStoreID || s.CardCreatedAtMS < a.sourceCreatedMS {
		return false
	}
	ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin, s.Event.Saturday)
	if e != nil || ev != s.Event {
		return false
	}
	id, e := gymcard.CardID(a.pin.SourcePin, ev)
	if e != nil || id != r.CardID {
		return false
	}
	key, e := gymseed.EventKey(a.pin.SourcePin.SourcePin, ev)
	if e != nil || key != s.SeedEventKey {
		return false
	}
	for _, corner := range s.Corners {
		matched := false
		for _, p := range a.pin.SourcePin.SourcePin.SourcePin.Participants {
			owner := gymevent.AssignmentOwner{OwnershipBasis: "candidate_assignment", AccountAuthorityID: p.StablePin.AccountAuthorityID, AccountID: p.StablePin.AccountID, GymID: p.StablePin.Identity.GymID}
			if corner.Owner == owner && corner.Member.Assignment.DescriptorID == p.StablePin.Source.DescriptorID && corner.Member.Assignment.GymID == owner.GymID {
				matched = true
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
func boutJobDigest(j pilot.BoutJob) string {
	raw, e := json.Marshal(j)
	if e != nil || len(raw) > pilot.MaxBoutJobSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (b *StableBridge) correlateBoutJob(req pilot.BoutRequest, j pilot.BoutJob, e boutJobExpectation, a boutAuthority, view pilot.ManifestViewPin) bool {
	event, err := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin, j.Saturday)
	if err != nil {
		return false
	}
	key, err := gymcard.CardID(a.pin.SourcePin, event)
	if err != nil || j.CardID != key || j.CardID != e.cardID || j.BoutID != e.boutID || j.Saturday != e.saturday || j.Kind != e.kind || j.StoreID != a.storeID || j.View != view {
		return false
	}
	if boutBegin(req.Command) && j.State != "queued" || e.state == "running" && j.State == "queued" {
		return false
	}
	if eventTerminal(e.state) {
		return j.State == e.state && boutJobDigest(j) == e.terminalDigest
	}
	if j.Result != nil {
		return j.Result.Record.CardID == key && b.validBoutRecord(a, j.Result.Record) && validBoutStatus(a, j.Result.Status)
	}
	return j.State != "completed"
}
func (b *StableBridge) rememberBoutJob(j pilot.BoutJob, e boutJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.bout == nil || b.boutPin == nil || j.View != b.bout.view || j.StoreID != b.boutPin.storeID {
		return false
	}
	if began || eventTerminal(j.State) {
		for id, old := range b.bout.jobs {
			if id != j.JobID && eventTerminal(old.state) {
				delete(b.bout.jobs, id)
			}
		}
	}
	if _, known := b.bout.jobs[j.JobID]; !known && len(b.bout.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if eventTerminal(j.State) {
		e.terminalDigest = boutJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.bout.jobs[j.JobID] = e
	return true
}
