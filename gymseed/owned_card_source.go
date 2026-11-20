// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"math"
)

// Card corners reference full prospect IDs without a gym qualifier. The older
// seed/event schemas did not require this cross-participant uniqueness.
func ownedValidateCardSource(p OwnedPin, v OwnedCardSource) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != ownedEventKey(p, v.SourceReceipt.Input.Event) {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedSnapshotReceipt(p.SourcePin, v.SourceReceipt); e != nil || !ownedCardProspectsUnique(v.SourceReceipt) {
		return ErrInvalid
	}
	want, e := expectedOwnedDeclaration(p, v.SourceReceipt)
	if e != nil || !equal(want, v.Declaration.Declaration) || validateOwnedSignedDeclaration(p, v.Declaration) != nil || validateAnchor(physicalOwnedPin(p), v.Anchor) != nil || validateTarget(physicalOwnedPin(p), v.Target) != nil || validateFinality(physicalOwnedPin(p), v.Finality) != nil {
		return ErrInvalid
	}
	payload, e := ownedCarrier(want)
	if e != nil || v.Anchor.Payload != hex.EncodeToString(payload[:]) || v.Anchor.Height == math.MaxInt32 || v.Target.Height != v.Anchor.Height+1 || v.SourceReceipt.CompletedAtMS > v.Target.ObservedAtMS || v.Target.ObservedAtMS > v.Finality.ObservedAtMS || v.Finality.Checkpoint.Height != int32(v.Target.Height) || v.Finality.Checkpoint.Hash.String() != v.Target.Hash {
		return ErrInvalid
	}
	h, e := header(v.Target.Header)
	if e != nil || h.PrevBlock.String() != v.Anchor.Hash {
		return ErrInvalid
	}
	return nil
}
func ownedValidateCardState(p OwnedPin, v OwnedCardState) error {
	if v.Version != 1 || validateOwnedStatus(p, v.Status) != nil || !optionalID(v.SourceDigest) || (v.SourceDigest != "") != (v.Status.FinalizedAtMS > 0) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateCardView(p OwnedPin, v OwnedCardView) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !ownedTimestamp(v.CreatedAtMS) || !ownedTimestamp(v.ObservedAtMS) || v.CreatedAtMS > v.ObservedAtMS || v.States == nil || len(v.States) > 8 {
		return ErrInvalid
	}
	found := false
	for _, x := range p.SourcePin.Participants {
		if x.StablePin.Stable.AccountID == v.ParticipantAccountID && x.StablePin.Stable.Identity.GymID == v.ParticipantGymID {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	last := ""
	for _, x := range v.States {
		if ownedValidateCardState(p, x) != nil || x.Status.Event.Saturday <= last || x.Status.ObservedAtMS != v.ObservedAtMS {
			return ErrInvalid
		}
		last = x.Status.Event.Saturday
	}
	return nil
}
func OwnedCardSourceDigest(p OwnedPin, v OwnedCardSource) (string, error) {
	b, e := MarshalOwnedCardSource(p, v)
	if e != nil {
		return "", e
	}
	_ = b
	return ownedcodec.Hash("seed card source", v)
}
func ownedCardProjection(r OwnedRecord) (OwnedCardSource, bool) {
	if r.Anchor == nil || r.Target == nil || r.Finality == nil {
		return OwnedCardSource{}, false
	}
	return OwnedCardSource{1, r.StoreID, r.EventKey, r.SourceReceipt, r.Declaration, *r.Anchor, *r.Target, *r.Finality}, true
}

// The concrete seed gate is held throughout preparation and final source leases.
// No source mutation follows the foreign callback in the same lease invocation.
func (s *OwnedStore) WithCardView(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, dates []string, commit func(OwnedCardView, func() error) error) error {
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
func (s *OwnedStore) cardAuthorize(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, fn func(gymevent.OwnedSeedSourceInfo, func() error) error) error {
	return s.cardAuthorizePurpose(ctx, events, entries, session, gymaccount.View, false, fn)
}
func (s *OwnedStore) cardAuthorizePurpose(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, capability gymaccount.Capability, result bool, fn func(gymevent.OwnedSeedSourceInfo, func() error) error) error {
	observe := events.WithSeedView
	if result {
		observe = events.WithResultView
	}
	return ownedMapped(observe(ctx, entries, session, capability, func(v gymevent.OwnedSeedSourceInfo, check func() error) error {
		if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) || check == nil || !ownedTimestamp(v.ObservedAtMS) {
			return ErrForbidden
		}
		if s.life.Err() != nil {
			return ErrClosed
		}
		return fn(v, check)
	}))
}
func (s *OwnedStore) cardView(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, dates []string, commit func(OwnedCardView, func() error) error) error {
	evs, e := s.cardDates(dates)
	if e != nil {
		return e
	}
	if e := s.cardAuthorize(ctx, events, entries, session, func(_ gymevent.OwnedSeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	return s.cardViewAuthorized(ctx, events, entries, session, evs, commit)
}

// WithCardPreparedView authorizes the original View before the consumer reads
// and validates its retained rows outside the account lease. The same seed gate
// stays held until the existing final source observation and original-View
// publication guard finish. Neither callback may reenter these source stores.
func (s *OwnedStore) WithCardPreparedView(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, prepare func(OwnedInfo) ([]string, func(OwnedCardView, func() error) error, error)) error {
	return s.withCardPreparedPurpose(ctx, events, entries, session, gymaccount.View, false, prepare)
}
func (s *OwnedStore) withCardPreparedPurpose(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, capability gymaccount.Capability, result bool, prepare func(OwnedInfo) ([]string, func(OwnedCardView, func() error) error, error)) error {
	if ctx == nil || events == nil || entries == nil || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.beginCard(ctx)
	if e != nil {
		return e
	}
	defer done()
	if e = s.cardAuthorizePurpose(work, events, entries, session, capability, result, func(_ gymevent.OwnedSeedSourceInfo, check func() error) error { return check() }); e != nil {
		return s.cardPreparedError(e)
	}
	if s.life.Err() != nil {
		return ErrClosed
	}
	if work.Err() != nil {
		return ErrCancelled
	}
	// Immutable identity and bounded metadata are owned copies. Do not call
	// Info here: that public method would reenter the resource gate we hold.
	m := s.meta
	info, e := ValidateOwnedInfo(m.Pin, OwnedInfo{Version: 1, StoreID: m.StoreID, Pin: m.Pin, CreatedAtMS: m.CreatedAtMS, EventCount: m.EventCount, FundingHeight: m.FundingCursor.Height, FundingHash: m.FundingCursor.Hash, FundingComplete: m.FundingComplete, FundingGeneration: m.FundingCursor.Generation, FundingRestartCount: uint32(len(m.FundingRestarts))})
	if e != nil {
		return ErrCorrupt
	}
	dates, commit, e := prepare(info)
	if e != nil {
		return s.cardPreparedError(e)
	}
	if s.life.Err() != nil {
		return ErrClosed
	}
	if work.Err() != nil {
		return ErrCancelled
	}
	if commit == nil {
		return ErrInvalid
	}
	evs, e := s.cardDates(dates)
	if e != nil {
		return e
	}
	e = s.cardViewAuthorizedPurpose(work, events, entries, session, capability, result, evs, commit)
	// The body's final guard is the success linearization. Never replace a nil
	// completed result because Close started afterward, or reinterpret an opaque
	// consumer error (including its own distinct outcome-unknown error identity).
	return s.cardPreparedError(e)
}

func (s *OwnedStore) cardPreparedError(e error) error {
	if errors.Is(e, ErrCancelled) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		if s.life.Err() != nil {
			return ErrClosed
		}
		return ErrCancelled
	}
	return e
}

func (s *OwnedStore) cardDates(dates []string) ([]gymevent.Event, error) {
	if dates == nil || len(dates) > 8 {
		return nil, ErrInvalid
	}
	evs := make([]gymevent.Event, 0, len(dates))
	last := ""
	for _, d := range dates {
		if d <= last {
			return nil, ErrInvalid
		}
		ev, e := gymevent.OwnedEventForDate(s.meta.Pin.SourcePin, d)
		if e != nil {
			return nil, ErrCalendar
		}
		evs = append(evs, ev)
		last = d
	}
	return evs, nil
}

func (s *OwnedStore) cardViewAuthorized(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, evs []gymevent.Event, commit func(OwnedCardView, func() error) error) error {
	return s.cardViewAuthorizedPurpose(ctx, events, entries, session, gymaccount.View, false, evs, commit)
}
func (s *OwnedStore) cardViewAuthorizedPurpose(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, capability gymaccount.Capability, result bool, evs []gymevent.Event, commit func(OwnedCardView, func() error) error) error {
	rows := []OwnedRecord{}
	rowIndex := make([]int, len(evs))
	digests := make([]string, len(evs))
	for i, ev := range evs {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		rowIndex[i] = -1
		r, ok, e := s.record(ownedEventKey(s.meta.Pin, ev))
		if e != nil {
			return e
		}
		if !ok {
			continue
		}
		rowIndex[i] = len(rows)
		rows = append(rows, r)
		if source, complete := ownedCardProjection(r); complete {
			if !ownedCardProspectsUnique(source.SourceReceipt) {
				return ErrCorrupt
			}
			// s.record already validates every source/declaration/anchor/target/
			// finality relation (and the stronger retained-record chronology).
			// Hash its owned projection without repeating that full audit.
			b, e := json.Marshal(source)
			if e != nil || len(b) > OwnedMaxCardSourceSize {
				return ErrCorrupt
			}
			_ = b
			digests[i], _ = OwnedCardSourceDigest(s.meta.Pin, source)
		}
	}
	funding, e := s.fundingState(ctx)
	if e != nil {
		return e
	}
	pin, e := ValidateOwnedPin(s.meta.Pin)
	if e != nil {
		return ErrCorrupt
	}
	mutated := false
	e = s.cardAuthorizePurpose(ctx, events, entries, session, capability, result, func(v gymevent.OwnedSeedSourceInfo, check func() error) error {
		if !s.mu.TryLock() {
			return ErrBusy
		}
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
		out := OwnedCardView{Version: 1, StoreID: s.meta.StoreID, Pin: pin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, States: []OwnedCardState{}}
		for i, ev := range evs {
			state := absentOwnedStatus(pin, ev, n)
			if rowIndex[i] >= 0 {
				state = ownedStatusFor(pin, rows[rowIndex[i]], n, funding, s.meta.FundingCursor.Generation)
			}
			out.States = append(out.States, OwnedCardState{1, state, digests[i]})
		}
		if e := check(); e != nil {
			return e
		}
		return commit(out, check)
	})
	if e != nil || ctx.Err() != nil || s.life.Err() != nil {
		if mutated {
			return ErrOutcomeUnknown
		}
		if e != nil {
			return e
		}
		if s.life.Err() != nil {
			return ErrClosed
		}
		return ErrCancelled
	}
	return nil
}
func (s *OwnedStore) WithCardSource(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date string, prepare func(OwnedCardSource) (func(OwnedCardView, func() error) error, error)) error {
	if ctx == nil || events == nil || entries == nil || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.beginCard(ctx)
	if e != nil {
		return e
	}
	defer done()
	ev, e := gymevent.OwnedEventForDate(s.meta.Pin.SourcePin, date)
	if e != nil {
		return ErrCalendar
	}
	if e = s.cardAuthorize(work, events, entries, session, func(_ gymevent.OwnedSeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	r, ok, e := s.record(ownedEventKey(s.meta.Pin, ev))
	if e != nil {
		return e
	}
	if !ok {
		return ErrSourceIncomplete
	}
	if len(r.Conflicts) > 0 || r.ConflictOverflow {
		return ErrConflict
	}
	source, ok := ownedCardProjection(r)
	if !ok {
		return ErrSourceIncomplete
	}
	source, e = ValidateOwnedCardSource(s.meta.Pin, source)
	if e != nil {
		return ErrCorrupt
	}
	digest, _ := OwnedCardSourceDigest(s.meta.Pin, source)
	commit, e := prepare(source)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	// Source mutation cannot race while this seed gate is held. Recheck its exact
	// immutable projection under the final original-session lease via cardView.
	return s.cardView(work, events, entries, session, []string{date}, func(v OwnedCardView, check func() error) error {
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
func (s *OwnedStore) beginCard(ctx context.Context) (context.Context, func(), error) {
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
