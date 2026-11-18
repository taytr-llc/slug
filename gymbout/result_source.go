// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

// withResultViewHeld is deliberately private: its caller already owns the Bout
// gate. Lower source gates and the original account lease are reacquired here.
func (s *Store) withResultViewHeld(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, capability gymaccount.Capability, date, id string, commit func(ResultView, func() error) error) error {
	mutated := false
	e := cards.WithResultPreparedView(ctx, seeds, events, entries, session, capability, date, func(info gymcard.Info, sources []gymcard.BoutSource) (func(gymcard.BoutView, func() error) error, error) {
		if info.Version != 1 || info.StoreID != s.meta.Pin.SourceStoreID || !equal(info.Pin, s.meta.Pin.SourcePin) {
			return nil, ErrForbidden
		}
		if info.CreatedAtMS != s.meta.SourceCreatedAtMS {
			return nil, ErrSourceChanged
		}
		// Fence every saved row for this date against the immutable booked source.
		for _, row := range s.index {
			if row.Saturday != date {
				continue
			}
			found := false
			for _, src := range sources {
				if src.Bout.BoutID == row.BoutID && src.CardID == row.CardID && sourceDigest(src) == row.Saved.SourceDigest && equal(src.Bout, row.Saved.Bout) {
					found = true
					break
				}
			}
			if !found {
				return nil, ErrSourceChanged
			}
		}
		var selected *gymcard.BoutSource
		for i := range sources {
			if sources[i].Bout.BoutID == id {
				selected = &sources[i]
				break
			}
		}
		if selected == nil {
			if s.indexed(id) != nil {
				return nil, ErrSourceChanged
			}
			return nil, ErrNotFound
		}
		rec, index, e := s.retained(id, true)
		if e != nil {
			return nil, e
		}
		saved := SavedBout{Bout: selected.Bout, SourceDigest: sourceDigest(*selected)}
		var projected *ResultSource
		if index != nil {
			if index.CardID != selected.CardID || index.Saturday != date || index.Saved.SourceDigest != saved.SourceDigest || !equal(rec.Source, *selected) {
				return nil, ErrSourceChanged
			}
			saved = index.Saved
			projected = &ResultSource{Version: 1, StoreID: s.meta.StoreID, StoreCreatedAtMS: s.meta.CreatedAtMS, RecordDigest: index.RecordDigest, SourceDigest: rec.SourceDigest, InputID: rec.InputID, ResultDigest: rec.ResultDigest, CreatedAtMS: rec.CreatedAtMS, Source: rec.Source, Summary: ResultSummary{saved.Winner, saved.Method, saved.EndRound, saved.EndTick}}
		}
		pinRaw, _ := json.Marshal(s.meta.Pin)
		var pin Pin
		if json.Unmarshal(pinRaw, &pin) != nil {
			return nil, ErrCorrupt
		}
		return func(v gymcard.BoutView, check func() error) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if e := s.stateLocked(); e != nil {
				return e
			}
			if e := s.checkView(v); e != nil {
				return e
			}
			matched := false
			for _, ref := range v.Bouts {
				if ref.Bout.BoutID == id && ref.SourceDigest == saved.SourceDigest && equal(ref.Bout, saved.Bout) {
					matched = true
					break
				}
			}
			if !matched || v.State.CardID != selected.CardID || (projected != nil && !sameState(rec.Source, v.State)) {
				return ErrSourceChanged
			}
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
			if e := guard(); e != nil {
				return e
			}
			before := s.meta.ClockMS
			n, e := s.observeLocked(v.ObservedAtMS)
			mutated = mutated || before != s.meta.ClockMS || e == ErrOutcomeUnknown
			if e != nil {
				return e
			}
			state := v.State
			state.ObservedAtMS = n
			state.EscalationDue = n >= state.Event.IncidentDeadlineMS && (state.FinalizedAtMS == 0 || state.ConflictCount > 0 || state.ConflictOverflow)
			saved.Usable = saved.Exists && state.Usable
			out := ResultView{Version: 1, StoreID: s.meta.StoreID, Pin: pin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, Status: Status{Version: 1, StoreID: s.meta.StoreID, CardID: state.CardID, ObservedAtMS: n, Source: state, Saved: saved}, Source: projected}
			s.mark("result-view-before-commit")
			e = commit(out, guard)
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
		if mutated {
			return ErrOutcomeUnknown
		}
		e = mapped(e)
		if s.life.Err() != nil && resultCancellation(e) {
			return ErrClosed
		}
		return e
	}
	// The completed lower traversal includes the final original-session guard.
	// Later Close/cancellation cannot retroactively replace this success.
	s.mark("result-view-complete")
	return nil
}
func (s *Store) WithResultView(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, capability gymaccount.Capability, date, id string, commit func(ResultView, func() error) error) error {
	if !callsValid(ctx, cards, seeds, events, entries) || !hashID(id) || commit == nil || (capability != gymaccount.View && capability != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.withResultViewHeld(work, cards, seeds, events, entries, session, capability, date, id, commit)
}
func (s *Store) WithResultSource(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, id string, prepare func(ResultSource) (func(ResultView, func() error) error, error)) error {
	if !callsValid(ctx, cards, seeds, events, entries) || !hashID(id) || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	var selected ResultSource
	var saved, healthy bool
	e = s.withResultViewHeld(work, cards, seeds, events, entries, session, gymaccount.ResultPublish, date, id, func(v ResultView, check func() error) error {
		saved = v.Source != nil
		healthy = v.Status.Saved.Usable
		if saved {
			selected = *v.Source
		}
		return check()
	})
	if e != nil {
		return e
	}
	if !saved {
		return ErrSourceIncomplete
	}
	if !healthy {
		return ErrSourceConflict
	}
	digest := resultSourceDigest(selected)
	raw, _ := json.Marshal(selected)
	var owned ResultSource
	if json.Unmarshal(raw, &owned) != nil {
		return ErrCorrupt
	}
	// All lower gates/leases are released; only this Bout gate remains held.
	s.mark("result-source-prepared")
	commit, e := prepare(owned)
	if e != nil {
		if s.life.Err() != nil && resultCancellation(e) {
			return ErrClosed
		}
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	if s.life.Err() != nil {
		return ErrClosed
	}
	if work.Err() != nil {
		return ErrCancelled
	}
	return s.withResultViewHeld(work, cards, seeds, events, entries, session, gymaccount.ResultPublish, date, id, func(v ResultView, check func() error) error {
		if v.Source == nil || resultSourceDigest(*v.Source) != digest {
			return ErrSourceChanged
		}
		if !v.Status.Saved.Usable {
			return ErrSourceConflict
		}
		return commit(v, check)
	})
}

// Consumer errors remain opaque. Only actual cancellation identities may be
// classified as dependency closure before a consumer commit has begun.
func resultCancellation(e error) bool {
	return errors.Is(e, ErrCancelled) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded)
}
