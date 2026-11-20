// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

// WithResultPreparedView authenticates before outside-lease consumer preparation.
// Its final callback may not reenter any source or account store.
func (s *OwnedStore) WithResultPreparedView(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, capability gymaccount.Capability, date string, prepare func(OwnedInfo, []OwnedBoutSource) (func(OwnedBoutView, func() error) error, error)) error {
	if !ownedCallValid(ctx, seeds, events, entries) || prepare == nil || (capability != gymaccount.View && capability != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	ctx = work
	ev, e := gymevent.OwnedEventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return ErrCalendar
	}
	id := ownedCardID(s.meta.Pin, ev)
	mutated := false
	e = seeds.WithCardPreparedResultView(ctx, events, entries, session, capability, func(info gymseed.OwnedInfo) ([]string, func(gymseed.OwnedCardView, func() error) error, error) {
		if e := s.checkPreparedInfo(info); e != nil {
			return nil, nil, e
		}
		pinRaw, _ := json.Marshal(s.meta.Pin)
		var ownedPin OwnedPin
		if json.Unmarshal(pinRaw, &ownedPin) != nil {
			return nil, nil, ErrCorrupt
		}
		r, ok, e := s.record(id)
		if e != nil {
			return nil, nil, e
		}
		var rp *OwnedRecord
		projected := []OwnedBoutSource{}
		refs := []BoutRef{}
		if ok {
			rp = &r
			projected, e = ownedProjectBooked(s.meta.Pin, r)
			if e != nil {
				return nil, nil, e
			}
			for _, src := range projected {
				refs = append(refs, BoutRef{src.Bout, ownedBoutSourceDigest(src)})
			}
		}
		// Consumer preparation receives distinct owned projections. Neither its
		// mutation nor cancellation can alter the final source commitments.
		infoCard := OwnedInfo{Version: 1, StoreID: s.meta.StoreID, Pin: ownedPin, SourceCreatedAtMS: s.meta.SourceCreatedAtMS, CreatedAtMS: s.meta.CreatedAtMS, EventCount: s.meta.EventCount}
		rawPrepared, _ := json.Marshal(projected)
		var ownedSources []OwnedBoutSource
		if json.Unmarshal(rawPrepared, &ownedSources) != nil {
			return nil, nil, ErrCorrupt
		}
		commit, e := prepare(infoCard, ownedSources)
		if e != nil {
			if s.life.Err() != nil && resultCancellation(e) {
				return nil, nil, ErrClosed
			}
			return nil, nil, e
		}
		if commit == nil {
			return nil, nil, ErrInvalid
		}
		if s.life.Err() != nil {
			return nil, nil, ErrClosed
		}
		if ctx.Err() != nil {
			return nil, nil, ErrCancelled
		}
		// The OwnedInfo OwnedPin passed to prepare is not reused for the final view.
		var finalPin OwnedPin
		if json.Unmarshal(pinRaw, &finalPin) != nil {
			return nil, nil, ErrCorrupt
		}
		return []string{date}, func(v gymseed.OwnedCardView, check func() error) error {
			if !s.mu.TryLock() {
				return ErrBusy
			}
			defer s.mu.Unlock()
			if e := s.stateLocked(); e != nil {
				return e
			}
			if e := s.checkView(v); e != nil {
				return e
			}
			if len(v.States) != 1 {
				return ErrCorrupt
			}
			if ok && v.States[0].SourceDigest != r.SourceDigest {
				return ErrSourceChanged
			}
			if e := check(); e != nil {
				return e
			}
			if s.life.Err() != nil {
				return ErrClosed
			}
			if ctx.Err() != nil {
				return ErrCancelled
			}
			before := s.meta.ClockMS
			n, e := s.observeLocked(v.ObservedAtMS)
			mutated = mutated || s.meta.ClockMS != before || e == ErrOutcomeUnknown
			if e != nil {
				return e
			}
			state := ownedProjectBoutState(ownedStatusFor(s.meta.Pin, ev, rp, v.States[0], n))
			view := OwnedBoutView{Version: 1, StoreID: s.meta.StoreID, Pin: finalPin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, State: state, Bouts: refs}
			guard := func() error {
				if e := check(); e != nil {
					return e
				}
				if s.life.Err() != nil {
					return ErrClosed
				}
				if ctx.Err() != nil {
					return ErrCancelled
				}
				return s.stateLocked()
			}
			s.mark("bout-view-before-commit")
			e = commit(view, guard)
			if (e != nil || ctx.Err() != nil || s.life.Err() != nil) && mutated {
				return ErrOutcomeUnknown
			}
			if e != nil {
				return e
			}
			return guard()
		}, nil
	})
	if e != nil {
		e = observationError(ctx, e, mutated)
		if s.life.Err() != nil && resultCancellation(e) {
			return ErrClosed
		}
		return e
	}
	return nil
}
