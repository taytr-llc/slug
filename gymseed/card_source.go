// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"math"
)

// Card corners reference full prospect IDs without a gym qualifier. The older
// seed/event schemas did not require this cross-participant uniqueness.
func cardProspectsUnique(receipt gymevent.SnapshotReceipt) bool {
	seen := make(map[string]bool, 10)
	for _, p := range receipt.Input.Participants {
		for _, m := range p.Members {
			id := m.Assignment.ProspectID
			if seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	return true
}

func validateCardSource(p Pin, v CardSource) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != eventKey(p, v.SourceReceipt.Input.Event) {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateSnapshotReceipt(p.SourcePin, v.SourceReceipt); e != nil || !cardProspectsUnique(v.SourceReceipt) {
		return ErrInvalid
	}
	want, e := expectedDeclaration(p, v.SourceReceipt)
	if e != nil || !equal(want, v.Declaration.Declaration) || validateSignedDeclaration(p, v.Declaration) != nil || validateAnchor(p, v.Anchor) != nil || validateTarget(p, v.Target) != nil || validateFinality(p, v.Finality) != nil {
		return ErrInvalid
	}
	payload, e := carrier(want)
	if e != nil || v.Anchor.Payload != hex.EncodeToString(payload[:]) || v.Anchor.Height == math.MaxInt32 || v.Target.Height != v.Anchor.Height+1 || v.SourceReceipt.CompletedAtMS > v.Target.ObservedAtMS || v.Target.ObservedAtMS > v.Finality.ObservedAtMS || v.Finality.Checkpoint.Height != int32(v.Target.Height) || v.Finality.Checkpoint.Hash.String() != v.Target.Hash {
		return ErrInvalid
	}
	h, e := header(v.Target.Header)
	if e != nil || h.PrevBlock.String() != v.Anchor.Hash {
		return ErrInvalid
	}
	return nil
}
func validateCardState(p Pin, v CardState) error {
	if v.Version != 1 || validateStatus(p, v.Status) != nil || !optionalID(v.SourceDigest) || (v.SourceDigest != "") != (v.Status.FinalizedAtMS > 0) {
		return ErrInvalid
	}
	return nil
}
func validateCardView(p Pin, v CardView) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.CreatedAtMS) || !timestamp(v.ObservedAtMS) || v.CreatedAtMS > v.ObservedAtMS || v.States == nil || len(v.States) > 8 {
		return ErrInvalid
	}
	found := false
	for _, x := range p.SourcePin.Participants {
		if x.StablePin.AccountID == v.ParticipantAccountID && x.StablePin.Identity.GymID == v.ParticipantGymID {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	last := ""
	for _, x := range v.States {
		if validateCardState(p, x) != nil || x.Status.Event.Saturday <= last || x.Status.ObservedAtMS != v.ObservedAtMS {
			return ErrInvalid
		}
		last = x.Status.Event.Saturday
	}
	return nil
}
func CardSourceDigest(p Pin, v CardSource) (string, error) {
	b, e := MarshalCardSource(p, v)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate card source\x00v1\x00", lp(b)), nil
}
func cardProjection(r Record) (CardSource, bool) {
	if r.Anchor == nil || r.Target == nil || r.Finality == nil {
		return CardSource{}, false
	}
	return CardSource{1, r.StoreID, r.EventKey, r.SourceReceipt, r.Declaration, *r.Anchor, *r.Target, *r.Finality}, true
}

// The concrete seed gate is held throughout preparation and final source leases.
// No source mutation follows the foreign callback in the same lease invocation.
func (s *Store) WithCardView(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, dates []string, commit func(CardView, func() error) error) error {
	if ctx == nil || events == nil || entries == nil || dates == nil || len(dates) > 8 || commit == nil {
		return ErrInvalid
	}
	work, done, e := s.beginCard(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.cardView(work, events, entries, session, dates, commit)
}
func (s *Store) cardAuthorize(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, fn func(gymevent.SeedSourceInfo, func() error) error) error {
	return mapped(events.WithSeedView(ctx, entries, session, gymaccount.View, func(v gymevent.SeedSourceInfo, check func() error) error {
		if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) || check == nil || !timestamp(v.ObservedAtMS) {
			return ErrForbidden
		}
		if s.life.Err() != nil {
			return ErrClosed
		}
		return fn(v, check)
	}))
}
func (s *Store) cardView(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, dates []string, commit func(CardView, func() error) error) error {
	evs := make([]gymevent.Event, 0, len(dates))
	last := ""
	for _, d := range dates {
		if d <= last {
			return ErrInvalid
		}
		ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin, d)
		if e != nil {
			return ErrCalendar
		}
		evs = append(evs, ev)
		last = d
	}
	if e := s.cardAuthorize(ctx, events, entries, session, func(_ gymevent.SeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	rows := []Record{}
	rowIndex := make([]int, len(evs))
	digests := make([]string, len(evs))
	for i, ev := range evs {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		rowIndex[i] = -1
		r, ok, e := s.record(eventKey(s.meta.Pin, ev))
		if e != nil {
			return e
		}
		if !ok {
			continue
		}
		rowIndex[i] = len(rows)
		rows = append(rows, r)
		if source, complete := cardProjection(r); complete {
			if !cardProspectsUnique(source.SourceReceipt) {
				return ErrCorrupt
			}
			// s.record already validates every source/declaration/anchor/target/
			// finality relation (and the stronger retained-record chronology).
			// Hash its owned projection without repeating that full audit.
			b, e := json.Marshal(source)
			if e != nil || len(b) > MaxCardSourceSize {
				return ErrCorrupt
			}
			digests[i] = hash("SLUG candidate card source\x00v1\x00", lp(b))
		}
	}
	funding, e := s.fundingState(ctx)
	if e != nil {
		return e
	}
	pin, e := ValidatePin(s.meta.Pin)
	if e != nil {
		return ErrCorrupt
	}
	mutated := false
	e = s.cardAuthorize(ctx, events, entries, session, func(v gymevent.SeedSourceInfo, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ErrCancelled
		}
		n, m, e := s.observeRecordsLocked(v.ObservedAtMS, rows)
		mutated = mutated || m
		if e != nil {
			return e
		}
		out := CardView{Version: 1, StoreID: s.meta.StoreID, Pin: pin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, States: []CardState{}}
		for i, ev := range evs {
			state := absentStatus(pin, ev, n)
			if rowIndex[i] >= 0 {
				state = statusFor(pin, rows[rowIndex[i]], n, funding, s.meta.FundingCursor.Generation)
			}
			out.States = append(out.States, CardState{1, state, digests[i]})
		}
		if e := check(); e != nil {
			return e
		}
		return commit(out, check)
	})
	if e != nil || ctx.Err() != nil {
		if mutated {
			return ErrOutcomeUnknown
		}
		if e != nil {
			return e
		}
		return ErrCancelled
	}
	return nil
}
func (s *Store) WithCardSource(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string, prepare func(CardSource) (func(CardView, func() error) error, error)) error {
	if ctx == nil || events == nil || entries == nil || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.beginCard(ctx)
	if e != nil {
		return e
	}
	defer done()
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin, date)
	if e != nil {
		return ErrCalendar
	}
	if e = s.cardAuthorize(work, events, entries, session, func(_ gymevent.SeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	r, ok, e := s.record(eventKey(s.meta.Pin, ev))
	if e != nil {
		return e
	}
	if !ok {
		return ErrSourceIncomplete
	}
	if len(r.Conflicts) > 0 || r.ConflictOverflow {
		return ErrConflict
	}
	source, ok := cardProjection(r)
	if !ok {
		return ErrSourceIncomplete
	}
	source, e = ValidateCardSource(s.meta.Pin, source)
	if e != nil {
		return ErrCorrupt
	}
	raw, _ := json.Marshal(source)
	digest := hash("SLUG candidate card source\x00v1\x00", lp(raw))
	commit, e := prepare(source)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	// Source mutation cannot race while this seed gate is held. Recheck its exact
	// immutable projection under the final original-session lease via cardView.
	return s.cardView(work, events, entries, session, []string{date}, func(v CardView, check func() error) error {
		if len(v.States) != 1 || v.States[0].SourceDigest != digest {
			return ErrConflict
		}
		st := v.States[0].Status
		if st.ConflictCount > 0 || st.ConflictOverflow {
			return ErrConflict
		}
		return commit(v, check)
	})
}

// New protected source calls preserve closed-dependency precedence without an
// uncancellable mutex before the resource gate.
func (s *Store) beginCard(ctx context.Context) (context.Context, func(), error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if s.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		if s.life.Err() != nil {
			return nil, nil, ErrClosed
		}
		return nil, nil, e
	}
	if s.life.Err() != nil {
		done()
		return nil, nil, ErrClosed
	}
	return work, done, nil
}
