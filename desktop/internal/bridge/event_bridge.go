// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
)

// This is installed from the owned helper's validated root after correlating
// the current participant with the existing manifest/stable/entry authority.
// It survives a session change and is never inferred from a record response.
type eventAuthority struct {
	pin       gymevent.Pin
	storeID   string
	createdMS int64
}
type eventJobExpectation struct {
	eventID, saturday, participantID, state, terminalDigest string
}
type eventCorrelation struct {
	view          pilot.ManifestViewPin
	participantID string
	jobs          map[string]eventJobExpectation
	snapshotJobs  map[string]bool
}

func LaunchEvents(helper, dataDir, node string) (*StableBridge, error) {
	b, err := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--data-dir", dataDir, "--node-binary", node))
	if err == nil {
		b.entriesEnabled, b.eventsEnabled = true, true
	}
	return b, err
}
func eventFailure(id uint64, code string) pilot.EventResponse {
	if id == 0 {
		id = 1
	}
	return pilot.EventResponse{ID: id, Error: code}
}
func (b *StableBridge) clearEventView() {
	b.mu.Lock()
	b.event = nil
	b.mu.Unlock()
}
func cloneEventPin(p gymevent.Pin) gymevent.Pin {
	p.Participants = append([]gymentry.Pin{}, p.Participants...)
	return p
}
func equalEventPin(a, z gymevent.Pin) bool {
	return a.Version == z.Version && a.CalendarID == z.CalendarID && a.RulesID == z.RulesID && slices.Equal(a.Participants, z.Participants)
}
func eventTerminal(s string) bool { return oneOf(s, "completed", "failed", "cancelled") }

// Snapshots establish the existing three source contexts before requesting
// the shared coordinator. Each exchange uses the same nonblocking pipe gate;
// exact view comparison below refuses an intervening account transition.
func (b *StableBridge) prepareEventContext(ctx context.Context) string {
	m, err := b.Request(ctx, "snapshot", 0)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return "busy"
		}
		return "event_unavailable"
	}
	if m.Snapshot == nil {
		return "event_unavailable"
	}
	if m.Snapshot.View == nil {
		return ""
	}
	s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !s.OK {
		if s.Error == "busy" {
			return "busy"
		}
		return "event_unavailable"
	}
	e := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_snapshot"})
	if !e.OK {
		if e.Error == "busy" {
			return "busy"
		}
		return "event_unavailable"
	}
	return ""
}

func (b *StableBridge) EventRequest(ctx context.Context, req pilot.EventRequest) pilot.EventResponse {
	if !b.eventsEnabled {
		return eventFailure(0, "event_unavailable")
	}
	preflight := req
	preflight.ID = 1
	if _, err := pilot.MarshalEventRequest(preflight); err != nil {
		return eventFailure(0, "invalid_request")
	}
	if req.Command == "event_snapshot" {
		if code := b.prepareEventContext(ctx); code != "" {
			return eventFailure(0, code)
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return eventFailure(0, "busy")
		}
		return eventFailure(0, "event_unavailable")
	}
	defer func() { <-b.gate }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.eventRequestHeld(ctx, id, req)
}

func (b *StableBridge) eventContext() (eventAuthority, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.eventPin == nil || b.event == nil || b.view == nil || b.event.view != *b.view {
		return eventAuthority{}, "", false
	}
	a := *b.eventPin
	a.pin = cloneEventPin(a.pin)
	return a, b.event.participantID, true
}
func (b *StableBridge) eventExpectation(id string) (eventJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.event == nil {
		return eventJobExpectation{}, false, false
	}
	e, known := b.event.jobs[id]
	return e, known, b.event.snapshotJobs[id]
}
func (b *StableBridge) canTrackEventJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.event == nil {
		return false
	}
	for _, e := range b.event.jobs {
		if !eventTerminal(e.state) {
			return false
		}
	}
	return true
}

func (b *StableBridge) eventRequestHeld(ctx context.Context, id uint64, req pilot.EventRequest) pilot.EventResponse {
	req.ID = id
	raw, err := pilot.MarshalEventRequest(req)
	if err != nil {
		return eventFailure(id, "invalid_request")
	}
	a, participant, ready := b.eventContext()
	if req.Command != "event_snapshot" && (!ready || b.view.ViewID != req.ViewID) {
		return eventFailure(id, "stale_view")
	}
	var expected eventJobExpectation
	if req.Command == "event_capture" {
		if !b.canTrackEventJob() {
			return eventFailure(id, "busy")
		}
		event, err := gymevent.EventForDate(a.pin, req.Saturday)
		if err != nil {
			return eventFailure(id, "event_calendar")
		}
		expected = eventJobExpectation{eventID: event.EventID, saturday: req.Saturday, participantID: participant}
	}
	if oneOf(req.Command, "event_job", "event_cancel") {
		var known, discovered bool
		expected, known, discovered = b.eventExpectation(req.JobID)
		if !known && !discovered {
			return eventFailure(id, "event_not_found")
		}
		if !known && req.Command == "event_cancel" {
			fetched := b.eventRequestHeld(ctx, id, pilot.EventRequest{Command: "event_job", ViewID: req.ViewID, JobID: req.JobID})
			if !fetched.OK {
				return fetched
			}
			expected, known, _ = b.eventExpectation(req.JobID)
			if !known {
				return eventFailure(id, "outcome_unknown")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return eventFailure(0, "outcome_unknown")
			}
			req.ID = id
			raw, err = pilot.MarshalEventRequest(req)
			if err != nil {
				return eventFailure(id, "invalid_request")
			}
		}
	}
	unknown := oneOf(req.Command, "event_capture", "event_job", "event_cancel")
	if ctx.Err() != nil {
		return eventFailure(id, "event_cancelled")
	}
	frame, err := b.exchangeEventFamily(ctx, id, false, false, false, true, raw)
	if err != nil || frame.event == nil {
		b.clearEventView()
		if unknown {
			return eventFailure(id, "outcome_unknown")
		}
		return eventFailure(id, "event_unavailable")
	}
	r := *frame.event
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if unknown && oneOf(r.Error, "unauthorized", "stale_view", "event_unavailable", "event_not_found") {
			return eventFailure(id, "outcome_unknown")
		}
		return r
	}
	valid := false
	if req.Command == "event_snapshot" {
		valid = r.Snapshot != nil && b.rememberEventSnapshot(r)
	} else {
		valid = b.correlateEventEnvelope(req, r, a, participant)
		if valid && r.Job != nil {
			j := *r.Job
			if expected.eventID == "" {
				expected = eventJobExpectation{eventID: j.EventID, saturday: j.Saturday, participantID: participant}
			}
			valid = b.correlateEventJob(req, j, expected, a, participant) && b.rememberEventJob(j, expected, req.Command == "event_capture")
		}
	}
	if !valid {
		b.fail(ErrUnavailable)
		if unknown {
			return eventFailure(id, "outcome_unknown")
		}
		return eventFailure(id, "event_unavailable")
	}
	return r
}

func (b *StableBridge) rememberEventSnapshot(r pilot.EventResponse) bool {
	s := r.Snapshot
	if s == nil {
		return false
	}
	if s.State == "unavailable" {
		if s.View != nil || r.View != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" {
			return false
		}
		b.clearEventView()
		return true
	}
	entry, store, ok := b.entryPin()
	if !ok || !b.entryReady(entry, store) || b.view == nil || s.View == nil || r.View == nil ||
		*s.View != *b.view || *r.View != *b.view || r.StoreID != s.StoreID || s.ParticipantID != entry.StablePin.Identity.GymID {
		return false
	}
	if s.State == "setup_incomplete" {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.event = nil
		return b.failure == nil && b.eventPin == nil && s.Pin == nil && s.StoreID == "" && s.CoordinatorCreatedAtMS == 0
	}
	if s.Pin == nil || s.CoordinatorCreatedAtMS <= 0 {
		return false
	}
	pin, err := gymevent.ValidatePin(*s.Pin)
	if err != nil {
		return false
	}
	id, err := gymevent.ID(pin)
	if err != nil || id != s.StoreID {
		return false
	}
	matched := false
	for _, p := range pin.Participants {
		if p == entry {
			matched = true
		}
	}
	if !matched {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil {
		return false
	}
	if b.eventPin == nil {
		b.eventPin = &eventAuthority{pin: cloneEventPin(pin), storeID: id, createdMS: s.CoordinatorCreatedAtMS}
	} else if b.eventPin.storeID != id || b.eventPin.createdMS != s.CoordinatorCreatedAtMS || !equalEventPin(b.eventPin.pin, pin) {
		return false
	}
	if b.event == nil || b.event.view != *s.View {
		b.event = &eventCorrelation{view: *s.View, participantID: s.ParticipantID, jobs: make(map[string]eventJobExpectation)}
	}
	b.event.snapshotJobs = make(map[string]bool, 3)
	for _, job := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if job != "" {
			b.event.snapshotJobs[job] = true
		}
	}
	for job := range b.event.jobs {
		if !b.event.snapshotJobs[job] {
			delete(b.event.jobs, job)
		}
	}
	return true
}

func (b *StableBridge) correlateEventEnvelope(req pilot.EventRequest, r pilot.EventResponse, a eventAuthority, participant string) bool {
	if b.view == nil || r.View == nil || *r.View != *b.view || r.StoreID != a.storeID {
		return false
	}
	switch req.Command {
	case "event_status":
		if r.Status == nil || r.Status.Event.Saturday != req.Saturday || r.Status.CoordinatorCreatedAtMS != a.createdMS {
			return false
		}
		_, err := gymevent.ValidateEventStatus(a.pin, *r.Status)
		return err == nil
	case "event_record":
		if r.Record == nil || r.Record.Input.Event.Saturday != req.Saturday || r.Record.Input.Event.CutoffMS <= a.createdMS {
			return false
		}
		_, err := gymevent.ValidateSnapshotReceipt(a.pin, *r.Record)
		return err == nil
	case "event_history":
		if r.History == nil || r.History.AfterEventID != req.AfterEventID || len(r.History.Events) > int(req.Limit) {
			return false
		}
		if _, err := gymevent.ValidateHistoryPage(a.pin, *r.History); err != nil {
			return false
		}
		for _, e := range r.History.Events {
			if e.CoordinatorCreatedAtMS != a.createdMS {
				return false
			}
		}
		return true
	case "event_capture", "event_job", "event_cancel":
		j := r.Job
		return j != nil && j.View == *b.view && j.StoreID == a.storeID && j.ParticipantID == participant &&
			(req.Command == "event_capture" || j.JobID == req.JobID)
	}
	return false
}

func eventJobDigest(j pilot.EventJob) string {
	raw, err := json.Marshal(j)
	if err != nil || len(raw) > pilot.MaxEventResponseSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (b *StableBridge) correlateEventJob(req pilot.EventRequest, j pilot.EventJob, e eventJobExpectation, a eventAuthority, participant string) bool {
	event, err := gymevent.EventForDate(a.pin, j.Saturday)
	if err != nil || j.Kind != "capture" || j.EventID != event.EventID || j.EventID != e.eventID || j.Saturday != e.saturday ||
		j.ParticipantID != participant || j.ParticipantID != e.participantID || j.StoreID != a.storeID || b.view == nil || j.View != *b.view {
		return false
	}
	if req.Command == "event_capture" && j.State != "queued" || e.state == "running" && j.State == "queued" {
		return false
	}
	if eventTerminal(e.state) {
		// A checked terminal result stays byte-identical. Polling never runs
		// the manifest/condition semantics again or retains a second result.
		return j.State == e.state && eventJobDigest(j) == e.terminalDigest
	}
	if j.Result != nil {
		if j.Result.Status.Event != event || j.Result.Status.CoordinatorCreatedAtMS != a.createdMS || j.Result.Capture.Snapshot.ParticipantID != participant {
			return false
		}
		_, err := gymevent.ValidateCaptureResult(a.pin, *j.Result)
		return err == nil
	}
	return j.State != "completed"
}
func (b *StableBridge) rememberEventJob(j pilot.EventJob, e eventJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.event == nil || b.eventPin == nil || j.View != b.event.view || j.StoreID != b.eventPin.storeID || j.ParticipantID != b.event.participantID {
		return false
	}
	if began || eventTerminal(j.State) {
		for id, old := range b.event.jobs {
			if id != j.JobID && eventTerminal(old.state) {
				delete(b.event.jobs, id)
			}
		}
	}
	if _, known := b.event.jobs[j.JobID]; !known && len(b.event.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if eventTerminal(j.State) {
		e.terminalDigest = eventJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.event.jobs[j.JobID] = e
	return true
}
