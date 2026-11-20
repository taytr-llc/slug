// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

func ownedCallsValid(ctx context.Context, c *gymcard.OwnedStore, s *gymseed.OwnedStore, e *gymevent.OwnedStore, i *gymentry.OwnedStore) bool {
	return ctx != nil && c != nil && s != nil && e != nil && i != nil
}
func (s *OwnedStore) observe(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, id string, decodeRow bool) (OwnedRecord, OwnedCardStatus, error) {
	var rec OwnedRecord
	var out OwnedCardStatus
	mutated := false
	e := cards.WithBoutView(ctx, seeds, events, entries, session, date, func(v gymcard.OwnedBoutView, check func() error) error {
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
		if e := check(); e != nil {
			return e
		}
		if s.life.Err() != nil {
			return ErrClosed
		}
		if ctx.Err() != nil {
			return ErrCancelled
		}
		// The full bounded index also fences saved rows omitted by a different
		// same-OwnedPin source incarnation, including an absent or empty card.
		for _, saved := range s.index {
			if saved.CardID != v.State.CardID {
				continue
			}
			found := false
			for _, ref := range v.Bouts {
				if ref.Bout.BoutID == saved.BoutID && ref.SourceDigest == saved.Saved.SourceDigest && equal(ref.Bout, saved.Saved.Bout) {
					found = true
					break
				}
			}
			if !found {
				return ErrSourceChanged
			}
		}
		rows := make([]OwnedSavedBout, 0, len(v.Bouts))
		for _, ref := range v.Bouts {
			if id != "" && ref.Bout.BoutID != id {
				continue
			}
			if ctx.Err() != nil {
				return ErrCancelled
			}
			r, index, e := s.retained(ref.Bout.BoutID, decodeRow)
			if e != nil {
				return e
			}
			b := OwnedSavedBout{Bout: ref.Bout, SourceDigest: ref.SourceDigest}
			if index != nil {
				if index.CardID != v.State.CardID || index.Saturday != date || index.Saved.SourceDigest != ref.SourceDigest || !equal(index.Saved.Bout, ref.Bout) {
					return ErrSourceChanged
				}
				b = index.Saved
				b.Usable = v.State.Usable
				if decodeRow {
					if !ownedSameState(r.Source, v.State) {
						return ErrSourceChanged
					}
					rec = r
				}
			}
			rows = append(rows, b)
		}
		// A missing current ref must not hide a previously saved row.
		if id != "" && len(rows) == 0 && s.indexed(id) != nil {
			return ErrSourceChanged
		}
		before := s.meta.ClockMS
		n, e := s.observeLocked(v.ObservedAtMS)
		mutated = mutated || s.meta.ClockMS != before || e == ErrOutcomeUnknown
		if e != nil {
			return e
		}
		state := v.State
		state.ObservedAtMS = n
		state.EscalationDue = n >= state.Event.IncidentDeadlineMS && (state.FinalizedAtMS == 0 || state.ConflictCount > 0 || state.ConflictOverflow)
		out = OwnedCardStatus{Version: 1, StoreID: s.meta.StoreID, CardID: state.CardID, ObservedAtMS: n, Source: state, Bouts: rows}
		s.mark("read-observed")
		if e := check(); e != nil {
			return e
		}
		if ctx.Err() != nil || s.life.Err() != nil {
			return ErrCancelled
		}
		return nil
	})
	if e != nil || ctx.Err() != nil || s.life.Err() != nil {
		if mutated {
			return OwnedRecord{}, OwnedCardStatus{}, ErrOutcomeUnknown
		}
		if e != nil {
			return OwnedRecord{}, OwnedCardStatus{}, ownedMapped(e)
		}
		if s.life.Err() != nil {
			return OwnedRecord{}, OwnedCardStatus{}, ErrClosed
		}
		return OwnedRecord{}, OwnedCardStatus{}, ErrCancelled
	}
	return rec, out, nil
}
func ownedSelectedStatus(v OwnedCardStatus) (OwnedStatus, error) {
	if len(v.Bouts) != 1 {
		return OwnedStatus{}, ErrNotFound
	}
	return OwnedStatus{Version: 1, StoreID: v.StoreID, CardID: v.CardID, ObservedAtMS: v.ObservedAtMS, Source: v.Source, Saved: v.Bouts[0]}, nil
}
func (s *OwnedStore) readCall(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, id string, decodeRow bool) (OwnedRecord, OwnedCardStatus, error) {
	if !ownedCallsValid(ctx, cards, seeds, events, entries) || id != "" && !hashID(id) {
		return OwnedRecord{}, OwnedCardStatus{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return OwnedRecord{}, OwnedCardStatus{}, e
	}
	defer done()
	return s.observe(work, cards, seeds, events, entries, session, date, id, decodeRow)
}
func (s *OwnedStore) CardStatus(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date string) (OwnedCardStatus, error) {
	_, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, "", false)
	return v, e
}
func (s *OwnedStore) Status(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, id string) (OwnedStatus, error) {
	if !hashID(id) {
		return OwnedStatus{}, ErrInvalid
	}
	_, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, id, false)
	if e != nil {
		return OwnedStatus{}, e
	}
	return ownedSelectedStatus(v)
}
func (s *OwnedStore) Read(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, id string) (OwnedReadResult, error) {
	if !hashID(id) {
		return OwnedReadResult{}, ErrInvalid
	}
	r, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, id, true)
	if e != nil {
		return OwnedReadResult{}, e
	}
	st, e := ownedSelectedStatus(v)
	if e != nil || !st.Saved.Exists {
		return OwnedReadResult{}, ErrNotFound
	}
	return OwnedReadResult{Version: 1, Record: r, Status: st}, nil
}
