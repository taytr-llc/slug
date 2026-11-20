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
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
)

type resultAuthority struct {
	pin             gymresult.Pin
	storeID         string
	createdMS       int64
	sourceCreatedMS int64
}
type resultJobExpectation struct {
	cardID, saturday, boutID, kind, state, terminalDigest string
	source                                                resultSourceExpectation
	known                                                 resultCommitments
}
type resultCorrelation struct {
	view         pilot.ManifestViewPin
	jobs         map[string]resultJobExpectation
	snapshotJobs map[string]bool
	selected     *resultSelection
}

func LaunchResults(helper, dataDir, node string) (*StableBridge, error) {
	b, e := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--candidate-results", "--data-dir", dataDir, "--node-binary", node))
	if e == nil {
		b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled, b.boutsEnabled, b.resultsEnabled = true, true, true, true, true, true
	}
	return b, e
}
func resultFailure(id uint64, code string) pilot.ResultResponse {
	if id == 0 {
		id = 1
	}
	return pilot.ResultResponse{ID: id, Error: code}
}
func (b *StableBridge) clearResultView() {
	b.mu.Lock()
	b.result = nil
	if b.history != nil {
		b.history, b.historyFatal = nil, true
	}
	b.mu.Unlock()
}
func (b *StableBridge) resultContext() (resultAuthority, pilot.ManifestViewPin, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.resultPin == nil || b.result == nil || b.view == nil || b.result.view != *b.view {
		return resultAuthority{}, pilot.ManifestViewPin{}, false
	}
	return *b.resultPin, b.result.view, true
}
func (b *StableBridge) resultExpectation(id string) (resultJobExpectation, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.result == nil {
		return resultJobExpectation{}, false, false
	}
	e, ok := b.result.jobs[id]
	return e, ok, b.result.snapshotJobs[id]
}
func (b *StableBridge) canTrackResultJob() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.result == nil {
		return false
	}
	for _, e := range b.result.jobs {
		if !eventTerminal(e.state) {
			return false
		}
	}
	return true
}
func resultBegin(command string) bool {
	return oneOf(command, "result_publish", "result_reconcile", "result_attest", "result_export")
}

func (b *StableBridge) ResultRequest(ctx context.Context, req pilot.ResultRequest) pilot.ResultResponse {
	if !b.resultsEnabled {
		return resultFailure(0, "result_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, e := pilot.MarshalResultRequest(pre); e != nil {
		return resultFailure(0, "invalid_request")
	}
	if req.Command == "result_snapshot" {
		// CardSnapshot establishes every prior authority before Result discovery.
		e := b.BoutRequest(ctx, pilot.BoutRequest{Command: "bout_snapshot"})
		if !e.OK {
			if e.Error == "busy" {
				return resultFailure(0, "busy")
			}
			return resultFailure(0, "result_unavailable")
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return resultFailure(0, "busy")
		}
		return resultFailure(0, "result_unavailable")
	}
	defer func() { <-b.gate }()
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.resultRequestHeld(c, id, req)
}

func (b *StableBridge) resultRequestHeld(ctx context.Context, id uint64, req pilot.ResultRequest) pilot.ResultResponse {
	req.ID = id
	raw, e := pilot.MarshalResultRequest(req)
	if e != nil {
		return resultFailure(id, "invalid_request")
	}
	a, view, ready := b.resultContext()
	if req.Command != "result_snapshot" && (!ready || view.ViewID != req.ViewID) {
		return resultFailure(id, "stale_view")
	}
	var captured resultSourceExpectation
	knownCommitments := b.resultKnown(req.Saturday, req.BoutID)
	if oneOf(req.Command, "result_status", "result_record", "result_publish", "result_reconcile", "result_attest", "result_export") {
		prior := b.boutRequestHeld(ctx, id, pilot.BoutRequest{Command: "bout_status", ViewID: req.ViewID, Saturday: req.Saturday, BoutID: req.BoutID})
		if !prior.OK {
			code := prior.Error
			if !oneOf(code, "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown") {
				code = "result_source_incomplete"
			}
			return resultFailure(id, code)
		}
		if prior.Status == nil {
			return resultFailure(id, "outcome_unknown")
		}
		captured = resultSourceFrom(*prior.Status)
		id = b.nextEntryIDHeld()
		if id == 0 {
			return resultFailure(0, "outcome_unknown")
		}
		req.ID = id
		raw, e = pilot.MarshalResultRequest(req)
		if e != nil {
			return resultFailure(id, "invalid_request")
		}
	}
	var expected resultJobExpectation
	discoveredJob := false
	if resultBegin(req.Command) {
		if !b.canTrackResultJob() {
			return resultFailure(id, "busy")
		}
		event, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin.SourcePin, req.Saturday)
		if e != nil {
			return resultFailure(id, "result_calendar")
		}
		key, e := gymcard.CardID(a.pin.SourcePin.SourcePin, event)
		if e != nil {
			return resultFailure(id, "invalid_request")
		}
		expected = resultJobExpectation{cardID: key, saturday: req.Saturday, boutID: req.BoutID, kind: strings.TrimPrefix(req.Command, "result_"), source: captured, known: knownCommitments}
	}
	if oneOf(req.Command, "result_job", "result_cancel") {
		var known, discovered bool
		expected, known, discovered = b.resultExpectation(req.JobID)
		discoveredJob = !known && discovered
		if !known && !discovered {
			return resultFailure(id, "result_not_found")
		}
		if !known && req.Command == "result_cancel" {
			found := b.resultRequestHeld(ctx, id, pilot.ResultRequest{Command: "result_job", ViewID: req.ViewID, JobID: req.JobID})
			if !found.OK {
				return found
			}
			expected, known, _ = b.resultExpectation(req.JobID)
			if !known {
				return resultFailure(id, "outcome_unknown")
			}
			id = b.nextEntryIDHeld()
			if id == 0 {
				return resultFailure(0, "outcome_unknown")
			}
			req.ID = id
			raw, e = pilot.MarshalResultRequest(req)
			if e != nil {
				return resultFailure(id, "invalid_request")
			}
		}
	}
	unknown := resultBegin(req.Command) || req.Command == "result_cancel" || req.Command == "result_job" && discoveredJob || oneOf(req.Command, "result_status", "result_record", "result_status")
	if ctx.Err() != nil {
		return resultFailure(id, "result_cancelled")
	}
	frame, e := b.exchangeResultFamily(ctx, id, false, false, false, false, false, false, false, true, raw)
	if e != nil || frame.result == nil {
		b.clearResultView()
		if unknown {
			return resultFailure(id, "outcome_unknown")
		}
		return resultFailure(id, "result_unavailable")
	}
	r := *frame.result
	if !r.OK {
		// Optional context on a refusal is still authority-bearing. A stale
		// or substituted error envelope must not become a definite refusal.
		if (r.View != nil && *r.View != view) || (r.StoreID != "" && (!ready || r.StoreID != a.storeID)) {
			b.fail(ErrUnavailable)
			if unknown {
				return resultFailure(id, "outcome_unknown")
			}
			return resultFailure(id, "result_unavailable")
		}
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		// A well-formed definite refusal is evidence, not transport uncertainty.
		// Only a lost/malformed dispatched exchange uses the rule above.
		return r
	}
	valid := false
	if req.Command == "result_snapshot" {
		valid = b.rememberResultSnapshot(r)
	} else {
		valid = b.correlateResultEnvelope(req, r, a, view)
		if valid && r.Status != nil {
			valid = captured.matches(r.Status.Source) && knownCommitments.matchesStatus(*r.Status)
		}
		if valid && r.Record != nil {
			valid = captured.matches(r.Record.Status.Source) && knownCommitments.matches(r.Record.Record) && knownCommitments.matchesStatus(r.Record.Status)
		}
		if valid && r.Job != nil {
			j := *r.Job
			if expected.cardID == "" {
				expected = resultJobExpectation{cardID: j.CardID, saturday: j.Saturday, boutID: j.BoutID, kind: j.Kind}
			}
			valid = b.correlateResultJob(req, j, expected, a, view) && b.rememberResultJob(j, expected, resultBegin(req.Command))
		}
	}
	if !valid {
		b.fail(ErrUnavailable)
		if unknown {
			return resultFailure(id, "outcome_unknown")
		}
		return resultFailure(id, "result_unavailable")
	}
	if r.Record != nil {
		b.rememberResultSelection(view, r.Record.Record, false)
	}
	if r.Job != nil && r.Job.Result != nil {
		b.rememberResultSelection(view, r.Job.Result.Record, true)
	}
	return r
}

func (b *StableBridge) rememberResultSnapshot(r pilot.ResultResponse) bool {
	s := r.Snapshot
	if s == nil {
		return false
	}
	if s.State == "unavailable" {
		if s.View != nil || r.View != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" {
			return false
		}
		b.clearResultView()
		return true
	}
	source, _, ok := b.boutContext()
	if s.State == "setup_incomplete" {
		// A source can still be incomplete. The established Manifest view is
		// required, but this response cannot establish any Result authority.
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view || b.resultPin != nil || s.Pin != nil || s.StoreID != "" || r.StoreID != "" || s.CreatedAtMS != 0 {
			return false
		}
		b.result = nil

		return true
	}
	if !ok || s.Pin == nil || s.CreatedAtMS <= 0 {
		return false
	}
	expected, e := gymresult.CandidatePin(source.pin)
	if e != nil {
		return false
	}
	pin, e := gymresult.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	want, e := gymresult.MarshalPin(expected)
	if e != nil {
		return false
	}
	got, e := gymresult.MarshalPin(pin)
	if e != nil || string(got) != string(want) {
		return false
	}
	id, e := gymresult.ID(expected)
	if e != nil || id != s.StoreID || r.StoreID != id || s.SourceCreatedAtMS != source.createdMS || s.CreatedAtMS < source.createdMS {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil || s.View == nil || r.View == nil || *s.View != *b.view || *r.View != *b.view {
		return false
	}
	if b.resultPin == nil {
		b.resultPin = &resultAuthority{pin: pin, storeID: id, createdMS: s.CreatedAtMS, sourceCreatedMS: s.SourceCreatedAtMS}
	} else if b.resultPin.storeID != id || b.resultPin.createdMS != s.CreatedAtMS || b.resultPin.sourceCreatedMS != s.SourceCreatedAtMS {
		return false
	}
	if b.result == nil || b.result.view != *s.View {
		b.result = &resultCorrelation{view: *s.View, jobs: map[string]resultJobExpectation{}}
	}
	b.result.snapshotJobs = map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.result.snapshotJobs[id] = true
		}
	}
	for id := range b.result.jobs {
		if !b.result.snapshotJobs[id] {
			delete(b.result.jobs, id)
		}
	}
	return true
}

func (b *StableBridge) correlateResultEnvelope(req pilot.ResultRequest, r pilot.ResultResponse, a resultAuthority, v pilot.ManifestViewPin) bool {
	if r.View == nil || *r.View != v || r.StoreID != a.storeID {
		return false
	}
	_, current, ok := b.resultContext()
	if !ok || current != v {
		return false
	}
	switch req.Command {
	case "result_status":
		return r.Status != nil && r.Status.Event.Saturday == req.Saturday && r.Status.BoutID == req.BoutID && b.validResultStatus(a, *r.Status)
	case "result_record":
		return r.Record != nil && r.Record.Status.Event.Saturday == req.Saturday && r.Record.Status.BoutID == req.BoutID && b.validResultStatus(a, r.Record.Status) && b.validResultRecord(a, r.Record.Record)
	case "result_publish", "result_reconcile", "result_attest", "result_export", "result_job", "result_cancel":
		return r.Job != nil && r.Job.View == v && r.Job.StoreID == a.storeID && (resultBegin(req.Command) || r.Job.JobID == req.JobID)
	}
	return false
}
func (b *StableBridge) validResultStatus(a resultAuthority, s gymresult.Status) bool {
	ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin.SourcePin, s.Event.Saturday)
	if e != nil || ev != s.Event {
		return false
	}
	id, e := gymcard.CardID(a.pin.SourcePin.SourcePin, ev)
	if e != nil || id != s.CardID {
		return false
	}
	key, e := gymresult.ResultKey(a.pin, id, s.BoutID)
	source, _, ok := b.boutContext()
	return e == nil && key == s.ResultKey && s.StoreID == a.storeID && s.ObservedAtMS >= a.createdMS && (!s.Exists || s.PreparedAtMS >= a.createdMS) && ok && validBoutStatus(source, s.Source)
}
func (b *StableBridge) validResultRecord(a resultAuthority, r gymresult.Record) bool {
	s := r.Statement.Statement
	p := a.pin
	src := s.Source
	if r.StoreID != a.storeID || s.StoreID != a.storeID || r.PreparedAtMS < a.createdMS || s.SourceStoreID != p.SourceStoreID || src.StoreID != p.SourceStoreID || src.StoreCreatedAtMS != a.sourceCreatedMS || src.CreatedAtMS < a.sourceCreatedMS || s.PublisherChainID != p.PublisherChainID || s.Network != p.Network || s.Genesis != p.Genesis || s.RulesID != p.RulesID || s.PublisherPublicKey != p.PublisherPublicKey || s.CheckpointPublicKey != p.CheckpointPublicKey {
		return false
	}
	ev, e := gymevent.EventForDate(p.SourcePin.SourcePin.SourcePin.SourcePin, src.Source.Event.Saturday)
	if e != nil || ev != src.Source.Event {
		return false
	}
	card, e := gymcard.CardID(p.SourcePin.SourcePin, ev)
	if e != nil || card != src.Source.CardID {
		return false
	}
	key, e := gymresult.ResultKey(p, card, src.Source.Bout.BoutID)
	if e != nil || key != r.ResultKey {
		return false
	}
	seed, e := gymseed.EventKey(p.SourcePin.SourcePin.SourcePin, ev)
	if e != nil || seed != src.Source.SeedEventKey {
		return false
	}
	for _, corner := range src.Source.Corners {
		found := false
		for _, part := range p.SourcePin.SourcePin.SourcePin.SourcePin.Participants {
			owner := gymevent.AssignmentOwner{OwnershipBasis: "candidate_assignment", AccountAuthorityID: part.StablePin.AccountAuthorityID, AccountID: part.StablePin.AccountID, GymID: part.StablePin.Identity.GymID}
			if corner.Owner == owner && corner.Member.Assignment.DescriptorID == part.StablePin.Source.DescriptorID && corner.Member.Assignment.GymID == owner.GymID {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func resultJobDigest(j pilot.ResultJob) string {
	raw, e := json.Marshal(j)
	if e != nil || len(raw) > pilot.MaxResultJobSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (b *StableBridge) correlateResultJob(req pilot.ResultRequest, j pilot.ResultJob, e resultJobExpectation, a resultAuthority, view pilot.ManifestViewPin) bool {
	event, err := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin.SourcePin, j.Saturday)
	if err != nil {
		return false
	}
	key, err := gymcard.CardID(a.pin.SourcePin.SourcePin, event)
	if err != nil || j.CardID != key || j.CardID != e.cardID || j.BoutID != e.boutID || j.Saturday != e.saturday || j.Kind != e.kind || j.StoreID != a.storeID || j.View != view || j.ResultKey != resultExpectedKey(a.pin, key, j.BoutID) {
		return false
	}
	if resultBegin(req.Command) && j.State != "queued" || e.state == "running" && j.State == "queued" {
		return false
	}
	if eventTerminal(e.state) {
		return j.State == e.state && resultJobDigest(j) == e.terminalDigest
	}
	if j.Result != nil {
		return j.Result.Status.CardID == key && b.validResultRecord(a, j.Result.Record) && b.validResultStatus(a, j.Result.Status) && e.source.matches(j.Result.Status.Source) && e.known.matches(j.Result.Record) && e.known.matchesStatus(j.Result.Status)
	}
	if j.Export != nil {
		k, err := gymresult.ResultKey(a.pin, key, j.BoutID)
		return err == nil && j.Export.ResultKey == k && j.Export.CardID == key && j.Export.BoutID == j.BoutID && (j.Export.FinalizedAtMS == 0 || j.Export.FinalizedAtMS >= a.createdMS) && (e.known.finality == "" || j.Export.FinalizedAtMS == e.known.finalizedMS) && (e.known.txid == "" || j.Export.TxID == e.known.txid) && (!e.source.set || j.Export.SourceRecordDigest == e.source.record && j.Export.InputID == e.source.input && j.Export.ResultDigest == e.source.result)
	}
	return j.State != "completed"
}
func (b *StableBridge) rememberResultJob(j pilot.ResultJob, e resultJobExpectation, began bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.result == nil || b.resultPin == nil || j.View != b.result.view || j.StoreID != b.resultPin.storeID {
		return false
	}
	if began || eventTerminal(j.State) {
		for id, old := range b.result.jobs {
			if id != j.JobID && eventTerminal(old.state) {
				delete(b.result.jobs, id)
			}
		}
	}
	if _, known := b.result.jobs[j.JobID]; !known && len(b.result.jobs) >= 3 {
		return false
	}
	e.state = j.State
	if eventTerminal(j.State) {
		e.terminalDigest = resultJobDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	b.result.jobs[j.JobID] = e
	return true
}

func resultExpectedKey(p gymresult.Pin, card, bid string) string {
	k, _ := gymresult.ResultKey(p, card, bid)
	return k
}
