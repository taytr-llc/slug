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
)

type entryJobExpectation struct {
	kind, eventID, saturday, operationID, state string
	request                                     *gymentry.Request
	intent                                      *gymentry.Intent
	terminalDigest                              string
}

// The pipe gate serializes requests. mu also protects this small cache because
// a reader failure clears it immediately, independently of the pipe owner.
type entryCorrelation struct {
	view         pilot.ManifestViewPin
	pin          gymentry.Pin
	storeID      string
	jobs         map[string]entryJobExpectation
	snapshotJobs map[string]bool
}

func LaunchEntries(helper, dataDir, node string) (*StableBridge, error) {
	b, err := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--data-dir", dataDir, "--node-binary", node))
	if err == nil {
		b.entriesEnabled = true
	}
	return b, err
}

func entryFailure(id uint64, code string) pilot.EntryResponse {
	if id == 0 {
		id = 1
	}
	return pilot.EntryResponse{ID: id, Error: code}
}
func (b *StableBridge) clearEntryView() {
	b.mu.Lock()
	b.entry = nil
	b.mu.Unlock()
}
func (b *StableBridge) entryPin() (gymentry.Pin, string, bool) {
	b.mu.Lock()
	healthy := b.failure == nil
	b.mu.Unlock()
	if !healthy || !b.entriesEnabled || !b.previewPinned() {
		return gymentry.Pin{}, "", false
	}
	p := *b.stablePin
	namespace, err := gymentry.NamespaceID(p.Source.Network, p.Source.Genesis, p.AccountAuthorityID)
	if err != nil {
		return gymentry.Pin{}, "", false
	}
	pin, err := gymentry.ValidatePin(gymentry.Pin{
		Version: 1, NamespaceID: namespace, CalendarID: gymentry.CalendarID(),
		RulesID: gymentry.RulesID(), StableStoreID: b.storeID, StablePin: p,
	})
	if err != nil {
		return gymentry.Pin{}, "", false
	}
	id, err := gymentry.ID(pin)
	return pin, id, err == nil
}
func (b *StableBridge) entryReady(pin gymentry.Pin, store string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failure == nil && b.entry != nil && b.view != nil &&
		b.entry.view == *b.view && b.entry.pin == pin && b.entry.storeID == store
}
func entryStartsJob(command string) bool {
	return oneOf(command, "entry_prepare", "entry_apply", "entry_check")
}
func entryMutation(command string) bool {
	return oneOf(command, "entry_prepare", "entry_apply", "entry_abandon")
}
func entryTerminal(state string) bool {
	return oneOf(state, "complete", "failed", "cancelled")
}
func cloneEntryRequest(r gymentry.Request) *gymentry.Request {
	r.ProspectIDs = append([]string{}, r.ProspectIDs...)
	return &r
}
func cloneEntryIntent(i gymentry.Intent) *gymentry.Intent {
	i.Request = *cloneEntryRequest(i.Request)
	return &i
}
func equalEntryRequest(a, z gymentry.Request) bool {
	return a.Version == z.Version && a.EventID == z.EventID && a.Saturday == z.Saturday &&
		a.Action == z.Action && a.ExpectedRevision == z.ExpectedRevision &&
		a.ExpectedRosterRevision == z.ExpectedRosterRevision && slices.Equal(a.ProspectIDs, z.ProspectIDs)
}
func (b *StableBridge) entryExpectation(id string) (entryJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entry == nil {
		return entryJobExpectation{}, false, false
	}
	e, ok := b.entry.jobs[id]
	return e, ok, b.entry.snapshotJobs[id]
}
func (b *StableBridge) canTrackEntryJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entry == nil {
		return false
	}
	if len(b.entry.jobs) < 3 {
		return true
	}
	for _, e := range b.entry.jobs {
		if entryTerminal(e.state) {
			return true
		}
	}
	return false
}
func (b *StableBridge) nextEntryIDHeld() uint64 {
	b.nextID++
	if b.nextID > manifestSafeInteger {
		b.fail(ErrUnavailable)
		return 0
	}
	return b.nextID
}

// EntryRequest never returns a plain thrown error for a possibly dispatched
// mutation. The caller retains its durable operation or Pending query route.
func (b *StableBridge) EntryRequest(ctx context.Context, req pilot.EntryRequest) pilot.EntryResponse {
	if !b.entriesEnabled {
		return entryFailure(0, "entry_unavailable")
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return entryFailure(0, "busy")
		}
		return entryFailure(0, "entry_unavailable")
	}
	defer func() { <-b.gate }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.entryRequestHeld(ctx, id, req)
}

// Only this private already-gate-held path is used for Apply's metadata read
// and a snapshot-discovered job. It cannot recursively acquire the pipe gate.
func (b *StableBridge) entryRequestHeld(ctx context.Context, id uint64, req pilot.EntryRequest) pilot.EntryResponse {
	req.ID = id
	raw, err := pilot.MarshalEntryRequest(req)
	if err != nil {
		return entryFailure(id, "invalid_request")
	}
	// Own every slice/pointer used as correlation, independent of the caller.
	req, err = pilot.ParseEntryRequest(raw)
	if err != nil {
		return entryFailure(id, "invalid_request")
	}
	pin, store, ok := b.entryPin()
	if !ok {
		return entryFailure(id, "entry_unavailable")
	}
	if req.Command != "entry_snapshot" && (b.view.ViewID != req.ViewID || !b.entryReady(pin, store)) {
		return entryFailure(id, "stale_view")
	}
	if entryStartsJob(req.Command) && !b.canTrackEntryJob() {
		return entryFailure(id, "busy")
	}
	var expected entryJobExpectation
	if req.Command == "entry_apply" {
		read := b.entryRequestHeld(ctx, id, pilot.EntryRequest{Command: "entry_operation", ViewID: req.ViewID, OperationID: req.OperationID})
		if !read.OK {
			return read // No Apply was dispatched.
		}
		if read.Operation == nil || read.Operation.Operation == nil {
			return entryFailure(id, "entry_not_found")
		}
		intent := read.Operation.Operation.Intent
		if intent.Generation != b.view.Generation || intent.ViewID != req.ViewID {
			return entryFailure(id, "stale_view")
		}
		expected = entryJobExpectation{kind: "apply", eventID: intent.Request.EventID, saturday: intent.Request.Saturday,
			operationID: req.OperationID, request: cloneEntryRequest(intent.Request), intent: cloneEntryIntent(intent)}
		id = b.nextEntryIDHeld()
		if id == 0 {
			return entryFailure(0, "entry_unavailable")
		}
		req.ID = id
		raw, err = pilot.MarshalEntryRequest(req)
		if err != nil {
			return entryFailure(id, "invalid_request")
		}
	} else if req.Command == "entry_prepare" {
		expected = entryJobExpectation{kind: "prepare", eventID: req.Request.EventID,
			saturday: req.Request.Saturday, request: cloneEntryRequest(*req.Request)}
	} else if req.Command == "entry_check" {
		event, err := gymentry.EventForDate(pin, req.Saturday)
		if err != nil {
			return entryFailure(id, "entry_calendar")
		}
		expected = entryJobExpectation{kind: "check", eventID: event.EventID, saturday: req.Saturday}
	}
	discoveredJob := false
	if oneOf(req.Command, "entry_job", "entry_cancel") {
		var known, discovered bool
		expected, known, discovered = b.entryExpectation(req.JobID)
		discoveredJob = !known && discovered
		if !known && !discovered {
			return entryFailure(id, "entry_not_found")
		}
		if !known && req.Command == "entry_cancel" {
			fetched := b.entryRequestHeld(ctx, id, pilot.EntryRequest{Command: "entry_job", ViewID: req.ViewID, JobID: req.JobID})
			if !fetched.OK {
				return fetched
			}
			expected, known, _ = b.entryExpectation(req.JobID)
			if !known {
				return entryFailure(id, "busy")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return entryFailure(0, "entry_unavailable")
			}
			req.ID = id
			raw, err = pilot.MarshalEntryRequest(req)
			if err != nil {
				return entryFailure(id, "invalid_request")
			}
		}
	}
	unknown := entryMutation(req.Command) ||
		oneOf(req.Command, "entry_job", "entry_cancel") && (discoveredJob || oneOf(expected.kind, "prepare", "apply"))
	// Metadata prefetches may have succeeded after cancellation. Do not send
	// the next request once cancellation is observed. This only describes
	// dispatch of this request, never the outcome of an existing entry job.
	if ctx.Err() != nil {
		return entryFailure(id, "entry_cancelled")
	}
	frame, err := b.exchangeEntryFamily(ctx, id, false, false, true, raw)
	if err != nil || frame.entry == nil {
		b.clearEntryView()
		if unknown {
			return entryFailure(id, "outcome_unknown")
		}
		return entryFailure(id, "entry_unavailable")
	}
	response := *frame.entry
	if !response.OK {
		if oneOf(response.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if unknown && oneOf(response.Error, "unauthorized", "stale_view", "entry_unavailable", "entry_not_found") {
			return entryFailure(id, "outcome_unknown")
		}
		return response
	}
	if !b.correlateEntryEnvelope(req, response, pin, store) {
		b.fail(ErrUnavailable)
		if unknown {
			return entryFailure(id, "outcome_unknown")
		}
		return entryFailure(id, "entry_unavailable")
	}
	if response.Snapshot != nil {
		if !b.rememberEntrySnapshot(*response.Snapshot, pin, store) {
			return entryFailure(id, "entry_unavailable")
		}
		return response
	}
	if response.Job != nil {
		j := *response.Job
		if expected.kind == "" {
			// A snapshot only discovers an ID. Establish the complete tuple
			// from checked metadata before accepting a terminal result/action.
			expected = entryJobExpectation{kind: j.Kind, eventID: j.EventID, saturday: j.Saturday, operationID: j.OperationID}
			switch j.Kind {
			case "prepare":
				if j.State != "complete" || j.Intent == nil {
					if entryTerminal(j.State) {
						return entryFailure(id, "outcome_unknown")
					}
					return entryFailure(id, "busy")
				}
				expected.request = cloneEntryRequest(j.Intent.Request)
			case "apply":
				next := b.nextEntryIDHeld()
				if next == 0 {
					return entryFailure(id, "outcome_unknown")
				}
				read := b.entryRequestHeld(ctx, next, pilot.EntryRequest{Command: "entry_operation", ViewID: req.ViewID, OperationID: j.OperationID})
				if !read.OK {
					if read.Error == "busy" {
						return entryFailure(id, "busy")
					}
					// The discovered Apply may already have committed. Failure
					// to recover its exact intent is not a definite rejection.
					return entryFailure(id, "outcome_unknown")
				}
				if read.Operation == nil || read.Operation.Operation == nil {
					return entryFailure(id, "outcome_unknown")
				}
				i := read.Operation.Operation.Intent
				expected.request, expected.intent = cloneEntryRequest(i.Request), cloneEntryIntent(i)
			}
		}
		if !b.correlateEntryJob(req, j, expected, pin, store) ||
			!b.rememberEntryJob(j, expected, entryStartsJob(req.Command)) {
			b.fail(ErrUnavailable)
			if unknown || oneOf(j.Kind, "prepare", "apply") {
				return entryFailure(id, "outcome_unknown")
			}
			return entryFailure(id, "entry_unavailable")
		}
	}
	return response
}

func boundEntryEvent(pin gymentry.Pin, event gymentry.Event) bool {
	want, err := gymentry.EventForDate(pin, event.Saturday)
	return err == nil && event == want
}
func boundEntryStatus(pin gymentry.Pin, store string, status gymentry.EventStatus) bool {
	id, err := gymentry.EntryID(store, status.Event.EventID, pin.StablePin.Identity.GymID)
	return err == nil && status.StoreID == store && status.EntryID == id && boundEntryEvent(pin, status.Event)
}
func boundEntryReceipt(pin gymentry.Pin, store string, receipt gymentry.Receipt) bool {
	id, err := gymentry.EntryID(store, receipt.Event.EventID, pin.StablePin.Identity.GymID)
	if err != nil || receipt.StoreID != store || receipt.EntryID != id || !boundEntryEvent(pin, receipt.Event) {
		return false
	}
	for _, m := range receipt.Members {
		if m.Assignment.GymID != pin.StablePin.Identity.GymID || m.Assignment.DescriptorID != pin.StablePin.Source.DescriptorID {
			return false
		}
	}
	return true
}
func (b *StableBridge) correlateEntryEnvelope(req pilot.EntryRequest, r pilot.EntryResponse, pin gymentry.Pin, store string) bool {
	if req.Command == "entry_snapshot" && r.Snapshot != nil && r.Snapshot.State == "unavailable" {
		return r.View == nil && r.StoreID == "" && r.Snapshot.View == nil && r.Snapshot.StoreID == ""
	}
	if b.view == nil || r.View == nil || *r.View != *b.view || r.StoreID != store {
		return false
	}
	switch req.Command {
	case "entry_snapshot":
		return r.Snapshot != nil && r.Snapshot.View != nil && *r.Snapshot.View == *r.View && r.Snapshot.StoreID == store
	case "entry_status", "entry_upcoming":
		if r.Status == nil || !boundEntryStatus(pin, store, *r.Status) {
			return false
		}
		if req.Command == "entry_status" {
			return r.Status.Event.Saturday == req.Saturday
		}
		if r.Status.Event.CutoffMS <= r.Status.ObservedAtMS {
			return false
		}
		day, err := time.Parse("2006-01-02", r.Status.Event.Saturday)
		if err != nil {
			return false
		}
		previous := day.AddDate(0, 0, -7)
		if previous.Year() < 2026 {
			return true
		}
		event, err := gymentry.EventForDate(pin, previous.Format("2006-01-02"))
		return err == nil && event.CutoffMS <= r.Status.ObservedAtMS
	case "entry_history":
		h := r.History
		if h == nil || h.StoreID != store || h.AfterEventID != req.AfterEventID || len(h.Events) > int(req.Limit) {
			return false
		}
		for n, status := range h.Events {
			if !boundEntryStatus(pin, store, status) || status.Event.EventID == req.AfterEventID ||
				n > 0 && status.Event.CutoffMS <= h.Events[n-1].Event.CutoffMS {
				return false
			}
		}
		return true
	case "entry_operation", "entry_pending", "entry_abandon":
		o := r.Operation
		if o == nil || o.StoreID != store {
			return false
		}
		if o.Operation == nil {
			return req.Command == "entry_pending"
		}
		i := o.Operation.Intent
		if req.Command == "entry_pending" && o.Operation.State != "prepared" ||
			req.Command == "entry_abandon" && o.Operation.State == "prepared" {
			return false
		}
		if req.Command != "entry_pending" && i.OperationID != req.OperationID {
			return false
		}
		event, err := gymentry.EventForDate(pin, i.Request.Saturday)
		if err != nil || event.EventID != i.Request.EventID {
			return false
		}
		// Historical author views are allowed for queries and abandonment.
		return o.Operation.Receipt == nil || boundEntryReceipt(pin, store, *o.Operation.Receipt)
	default:
		j := r.Job
		return j != nil && j.View == *r.View && j.StoreID == store &&
			(!oneOf(req.Command, "entry_job", "entry_cancel") || j.JobID == req.JobID)
	}
}

func (b *StableBridge) correlateEntryJob(req pilot.EntryRequest, j pilot.EntryJob, e entryJobExpectation, pin gymentry.Pin, store string) bool {
	event, err := gymentry.EventForDate(pin, j.Saturday)
	if err != nil || event.EventID != j.EventID || j.Kind != e.kind || j.EventID != e.eventID ||
		j.Saturday != e.saturday || b.view == nil || j.View != *b.view || j.StoreID != store {
		return false
	}
	if entryStartsJob(req.Command) && j.State != "queued" {
		return false
	}
	if e.request != nil && (e.request.EventID != j.EventID || e.request.Saturday != j.Saturday) {
		return false
	}
	if entryTerminal(e.state) && (j.State != e.state || entryJobDigest(j) != e.terminalDigest) ||
		e.state == "running" && j.State == "queued" {
		return false
	}
	switch j.Kind {
	case "prepare":
		if e.request == nil {
			return false
		}
		if j.Intent == nil {
			return j.OperationID == ""
		}
		i := j.Intent
		return i.OperationID == j.OperationID && equalEntryRequest(i.Request, *e.request) &&
			i.Generation == b.view.Generation && i.ViewID == b.view.ViewID
	case "apply":
		if j.OperationID != e.operationID || e.intent == nil || e.request == nil ||
			e.intent.Generation != b.view.Generation || e.intent.ViewID != b.view.ViewID ||
			e.intent.OperationID != j.OperationID {
			return false
		}
		if j.Mutation == nil {
			return true
		}
		r := j.Mutation.Receipt
		if !boundEntryReceipt(pin, store, r) || r.OperationID != e.operationID ||
			r.OperationSequence != e.intent.Sequence || r.Generation != b.view.Generation ||
			r.ViewID != b.view.ViewID || r.Action != e.request.Action ||
			r.PreviousRevision != e.request.ExpectedRevision || r.Revision != r.PreviousRevision+1 ||
			r.CommittedAtMS < e.intent.PreparedAtMS || r.Event != event || len(r.Members) != len(e.request.ProspectIDs) {
			return false
		}
		for n, m := range r.Members {
			if m.Assignment.ProspectID != e.request.ProspectIDs[n] {
				return false
			}
		}
		return r.RosterRevision == e.request.ExpectedRosterRevision
	case "check":
		if j.OperationID != "" {
			return false
		}
		if j.Check == nil {
			return true
		}
		c := j.Check
		if !boundEntryStatus(pin, store, c.Status) || c.Status.Event != event || len(c.Members) != len(c.Status.SavedIDs) {
			return false
		}
		for n, m := range c.Members {
			a := m.Item.Assignment
			if a.ProspectID != c.Status.SavedIDs[n] || a.GymID != pin.StablePin.Identity.GymID ||
				a.DescriptorID != pin.StablePin.Source.DescriptorID || m.Item.Condition.AsOfMS != c.AsOfMS {
				return false
			}
		}
		return true
	}
	return false
}
func entryJobDigest(j pilot.EntryJob) string {
	raw, err := json.Marshal(j)
	if err != nil || len(raw) > pilot.MaxEntryResponseSize {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
func (b *StableBridge) rememberEntrySnapshot(s pilot.EntrySnapshot, pin gymentry.Pin, store string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil {
		return false
	}
	if s.View == nil {
		b.entry = nil
		return true
	}
	if b.entry == nil || b.entry.view != *s.View || b.entry.storeID != store || b.entry.pin != pin {
		b.entry = &entryCorrelation{view: *s.View, pin: pin, storeID: store, jobs: make(map[string]entryJobExpectation)}
	}
	b.entry.snapshotJobs = make(map[string]bool, 3)
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.entry.snapshotJobs[id] = true
		}
	}
	for id := range b.entry.jobs {
		if !b.entry.snapshotJobs[id] {
			delete(b.entry.jobs, id)
		}
	}
	return true
}
func (b *StableBridge) rememberEntryJob(j pilot.EntryJob, e entryJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.entry == nil || j.View != b.entry.view || j.StoreID != b.entry.storeID {
		return false
	}
	_, known := b.entry.jobs[j.JobID]
	if began && (known || b.entry.snapshotJobs[j.JobID]) {
		return false
	}
	if began || entryTerminal(j.State) {
		for id, old := range b.entry.jobs {
			if id != j.JobID && entryTerminal(old.state) {
				delete(b.entry.jobs, id)
			}
		}
	}
	if began {
		b.entry.snapshotJobs = nil
	}
	if !known && len(b.entry.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if j.Intent != nil {
		e.intent = cloneEntryIntent(*j.Intent)
		e.operationID = j.OperationID
	}
	if entryTerminal(j.State) {
		e.terminalDigest = entryJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.entry.jobs[j.JobID] = e
	return true
}
