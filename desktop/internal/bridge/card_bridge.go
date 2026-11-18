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
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

type cardAuthority struct {
	pin             gymcard.Pin
	storeID         string
	createdMS       int64
	sourceCreatedMS int64
}
type cardJobExpectation struct{ cardID, saturday, kind, state, terminalDigest string }
type cardCorrelation struct {
	view         pilot.ManifestViewPin
	jobs         map[string]cardJobExpectation
	snapshotJobs map[string]bool
}

func LaunchCards(helper, dataDir, node string) (*StableBridge, error) {
	b, e := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--data-dir", dataDir, "--node-binary", node))
	if e == nil {
		b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled = true, true, true, true
	}
	return b, e
}
func cardFailure(id uint64, code string) pilot.CardResponse {
	if id == 0 {
		id = 1
	}
	return pilot.CardResponse{ID: id, Error: code}
}
func (b *StableBridge) clearCardView() { b.mu.Lock(); b.card = nil; b.mu.Unlock() }
func (b *StableBridge) cardContext() (cardAuthority, pilot.ManifestViewPin, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.cardPin == nil || b.card == nil || b.view == nil || b.card.view != *b.view {
		return cardAuthority{}, pilot.ManifestViewPin{}, false
	}
	return *b.cardPin, b.card.view, true
}
func (b *StableBridge) cardExpectation(id string) (cardJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.card == nil {
		return cardJobExpectation{}, false, false
	}
	e, ok := b.card.jobs[id]
	return e, ok, b.card.snapshotJobs[id]
}
func (b *StableBridge) canTrackCardJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.card == nil {
		return false
	}
	for _, e := range b.card.jobs {
		if !eventTerminal(e.state) {
			return false
		}
	}
	return true
}
func cardBegin(command string) bool {
	return command == "card_build"
}

func (b *StableBridge) CardRequest(ctx context.Context, req pilot.CardRequest) pilot.CardResponse {
	if !b.cardsEnabled {
		return cardFailure(0, "card_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, e := pilot.MarshalCardRequest(pre); e != nil {
		return cardFailure(0, "invalid_request")
	}
	if req.Command == "card_snapshot" {
		// SeedSnapshot establishes every prior authority before Card discovery.
		e := b.SeedRequest(ctx, pilot.SeedRequest{Command: "seed_snapshot"})
		if !e.OK {
			if e.Error == "busy" {
				return cardFailure(0, "busy")
			}
			return cardFailure(0, "card_unavailable")
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return cardFailure(0, "busy")
		}
		return cardFailure(0, "card_unavailable")
	}
	defer func() { <-b.gate }()
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.cardRequestHeld(c, id, req)
}

func (b *StableBridge) cardRequestHeld(ctx context.Context, id uint64, req pilot.CardRequest) pilot.CardResponse {
	req.ID = id
	raw, e := pilot.MarshalCardRequest(req)
	if e != nil {
		return cardFailure(id, "invalid_request")
	}
	a, view, ready := b.cardContext()
	if req.Command != "card_snapshot" && (!ready || view.ViewID != req.ViewID) {
		return cardFailure(id, "stale_view")
	}
	var expected cardJobExpectation
	discoveredJob := false
	if cardBegin(req.Command) {
		if !b.canTrackCardJob() {
			return cardFailure(id, "busy")
		}
		event, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin, req.Saturday)
		if e != nil {
			return cardFailure(id, "card_calendar")
		}
		key, e := gymcard.CardID(a.pin, event)
		if e != nil {
			return cardFailure(id, "invalid_request")
		}
		expected = cardJobExpectation{cardID: key, saturday: req.Saturday, kind: strings.TrimPrefix(req.Command, "card_")}
	}
	if oneOf(req.Command, "card_job", "card_cancel") {
		var known, discovered bool
		expected, known, discovered = b.cardExpectation(req.JobID)
		discoveredJob = !known && discovered
		if !known && !discovered {
			return cardFailure(id, "card_not_found")
		}
		if !known && req.Command == "card_cancel" {
			found := b.cardRequestHeld(ctx, id, pilot.CardRequest{Command: "card_job", ViewID: req.ViewID, JobID: req.JobID})
			if !found.OK {
				return found
			}
			expected, known, _ = b.cardExpectation(req.JobID)
			if !known {
				return cardFailure(id, "outcome_unknown")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return cardFailure(0, "outcome_unknown")
			}
			req.ID = id
			raw, e = pilot.MarshalCardRequest(req)
			if e != nil {
				return cardFailure(id, "invalid_request")
			}
		}
	}
	unknown := cardBegin(req.Command) || req.Command == "card_cancel" || req.Command == "card_job" && discoveredJob || oneOf(req.Command, "card_status", "card_record", "card_history")
	if ctx.Err() != nil {
		return cardFailure(id, "card_cancelled")
	}
	frame, e := b.exchangeCardFamily(ctx, id, false, false, false, false, false, true, raw)
	if e != nil || frame.card == nil {
		b.clearCardView()
		if unknown {
			return cardFailure(id, "outcome_unknown")
		}
		return cardFailure(id, "card_unavailable")
	}
	r := *frame.card
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		// A well-formed definite refusal is evidence, not transport uncertainty.
		// Only a lost/malformed dispatched exchange uses the rule above.
		return r
	}
	valid := false
	if req.Command == "card_snapshot" {
		valid = b.rememberCardSnapshot(r)
	} else {
		valid = b.correlateCardEnvelope(req, r, a, view)
		if valid && r.Job != nil {
			j := *r.Job
			if expected.cardID == "" {
				expected = cardJobExpectation{cardID: j.CardID, saturday: j.Saturday, kind: j.Kind}
			}
			valid = b.correlateCardJob(req, j, expected, a, view) && b.rememberCardJob(j, expected, cardBegin(req.Command))
		}
	}
	if !valid {
		b.fail(ErrUnavailable)
		if unknown {
			return cardFailure(id, "outcome_unknown")
		}
		return cardFailure(id, "card_unavailable")
	}
	return r
}

func (b *StableBridge) rememberCardSnapshot(r pilot.CardResponse) bool {
	s := r.Snapshot
	if s == nil {
		return false
	}
	if s.State == "unavailable" {
		if s.View != nil || r.View != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" {
			return false
		}
		b.clearCardView()
		return true
	}
	source, _, ok := b.seedContext()
	if s.State == "setup_incomplete" {
		// A source can still be incomplete. The established Manifest view is
		// required, but this response cannot establish any Card authority.
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view || b.cardPin != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" || s.CreatedAtMS != 0 {
			return false
		}
		b.card = nil
		return true
	}
	if !ok || s.Pin == nil || s.CreatedAtMS <= 0 {
		return false
	}
	expected, e := gymcard.CandidatePin(source.pin)
	if e != nil {
		return false
	}
	pin, e := gymcard.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	want, e := gymcard.MarshalPin(expected)
	if e != nil {
		return false
	}
	got, e := gymcard.MarshalPin(pin)
	if e != nil || string(got) != string(want) {
		return false
	}
	id, e := gymcard.ID(expected)
	if e != nil || id != s.StoreID || r.StoreID != id || s.SourceCreatedAtMS != source.createdMS || s.CreatedAtMS < source.createdMS {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view {
		return false
	}
	if b.cardPin == nil {
		b.cardPin = &cardAuthority{pin: pin, storeID: id, createdMS: s.CreatedAtMS, sourceCreatedMS: s.SourceCreatedAtMS}
	} else if b.cardPin.storeID != id || b.cardPin.createdMS != s.CreatedAtMS || b.cardPin.sourceCreatedMS != s.SourceCreatedAtMS {
		return false
	}
	if b.card == nil || b.card.view != *s.View {
		b.card = &cardCorrelation{view: *s.View, jobs: map[string]cardJobExpectation{}}
	}
	b.card.snapshotJobs = map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.card.snapshotJobs[id] = true
		}
	}
	for id := range b.card.jobs {
		if !b.card.snapshotJobs[id] {
			delete(b.card.jobs, id)
		}
	}
	return true
}

func (b *StableBridge) correlateCardEnvelope(req pilot.CardRequest, r pilot.CardResponse, a cardAuthority, view pilot.ManifestViewPin) bool {
	if r.View == nil || *r.View != view || r.StoreID != a.storeID {
		return false
	}
	_, current, ok := b.cardContext()
	if !ok || current != view {
		return false
	}
	switch req.Command {
	case "card_status":
		return r.Status != nil && r.Status.Event.Saturday == req.Saturday && validCardStatus(a, *r.Status)
	case "card_record":
		return r.Record != nil && r.Record.Status.Event.Saturday == req.Saturday && validCardStatus(a, r.Record.Status) && b.validCardRecord(a, r.Record.Record)
	case "card_history":
		if r.History == nil || r.History.AfterCardID != req.AfterCardID || len(r.History.Cards) > int(req.Limit) {
			return false
		}
		for _, v := range r.History.Cards {
			if !validCardStatus(a, v) {
				return false
			}
		}
		return true

	case "card_build", "card_job", "card_cancel":
		return r.Job != nil && r.Job.View == view && r.Job.StoreID == a.storeID && (cardBegin(req.Command) || r.Job.JobID == req.JobID)
	}
	return false
}
func (b *StableBridge) validCardSource(a seedAuthority, r gymseed.CardSource) bool {
	if r.SourceReceipt.Input.Event.CutoffMS <= a.createdMS {
		return false
	}
	// The Card parser checked bounded transport shape and canonical digests.
	// Bind the trusted helper projection to the established immutable source.
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
func validCardStatus(a cardAuthority, s gymcard.Status) bool {
	ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin, s.Event.Saturday)
	if e != nil || ev != s.Event || s.StoreID != a.storeID || s.Exists && s.CreatedAtMS < a.createdMS {
		return false
	}
	id, e := gymcard.CardID(a.pin, ev)
	if e != nil || id != s.CardID {
		return false
	}
	key, e := gymseed.EventKey(a.pin.SourcePin, ev)
	return e == nil && s.Source.Status.StoreID == a.pin.SourceStoreID && s.Source.Status.EventKey == key
}
func (b *StableBridge) validCardRecord(a cardAuthority, r gymcard.Record) bool {
	if r.StoreID != a.storeID || r.CreatedAtMS < a.createdMS {
		return false
	}
	id, e := gymcard.CardID(a.pin, r.Source.SourceReceipt.Input.Event)
	if e != nil || id != r.CardID {
		return false
	}
	return b.validCardSource(seedAuthority{pin: a.pin.SourcePin, storeID: a.pin.SourceStoreID, createdMS: a.sourceCreatedMS}, r.Source)
}
func cardJobDigest(j pilot.CardJob) string {
	raw, e := json.Marshal(j)
	if e != nil || len(raw) > pilot.MaxCardJobSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (b *StableBridge) correlateCardJob(req pilot.CardRequest, j pilot.CardJob, e cardJobExpectation, a cardAuthority, view pilot.ManifestViewPin) bool {
	event, err := gymevent.EventForDate(a.pin.SourcePin.SourcePin, j.Saturday)
	if err != nil {
		return false
	}
	key, err := gymcard.CardID(a.pin, event)
	if err != nil || j.CardID != key || j.CardID != e.cardID || j.Saturday != e.saturday || j.Kind != e.kind || j.StoreID != a.storeID || j.View != view {
		return false
	}
	if cardBegin(req.Command) && j.State != "queued" || e.state == "running" && j.State == "queued" {
		return false
	}
	if eventTerminal(e.state) {
		return j.State == e.state && cardJobDigest(j) == e.terminalDigest
	}
	if j.Result != nil {
		return j.Result.Record.CardID == key && b.validCardRecord(a, j.Result.Record) && validCardStatus(a, j.Result.Status)
	}
	return j.State != "completed"
}
func (b *StableBridge) rememberCardJob(j pilot.CardJob, e cardJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.card == nil || b.cardPin == nil || j.View != b.card.view || j.StoreID != b.cardPin.storeID {
		return false
	}
	if began || eventTerminal(j.State) {
		for id, old := range b.card.jobs {
			if id != j.JobID && eventTerminal(old.state) {
				delete(b.card.jobs, id)
			}
		}
	}
	if _, known := b.card.jobs[j.JobID]; !known && len(b.card.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if eventTerminal(j.State) {
		e.terminalDigest = cardJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.card.jobs[j.JobID] = e
	return true
}
