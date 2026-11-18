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

func callsValid(ctx context.Context, c *gymcard.Store, s *gymseed.Store, e *gymevent.Store, i *gymentry.Store) bool {
	return ctx != nil && c != nil && s != nil && e != nil && i != nil
}
func (s *Store) observe(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, id string, decodeRow bool) (Record, CardStatus, error) {
	var rec Record
	var out CardStatus
	mutated := false
	e := cards.WithBoutView(ctx, seeds, events, entries, session, date, func(v gymcard.BoutView, check func() error) error {
		s.mu.Lock()
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
		// same-Pin source incarnation, including an absent or empty card.
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
		rows := make([]SavedBout, 0, len(v.Bouts))
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
			b := SavedBout{Bout: ref.Bout, SourceDigest: ref.SourceDigest}
			if index != nil {
				if index.CardID != v.State.CardID || index.Saturday != date || index.Saved.SourceDigest != ref.SourceDigest || !equal(index.Saved.Bout, ref.Bout) {
					return ErrSourceChanged
				}
				b = index.Saved
				b.Usable = v.State.Usable
				if decodeRow {
					if !sameState(r.Source, v.State) {
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
		out = CardStatus{Version: 1, StoreID: s.meta.StoreID, CardID: state.CardID, ObservedAtMS: n, Source: state, Bouts: rows}
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
			return Record{}, CardStatus{}, ErrOutcomeUnknown
		}
		if e != nil {
			return Record{}, CardStatus{}, mapped(e)
		}
		if s.life.Err() != nil {
			return Record{}, CardStatus{}, ErrClosed
		}
		return Record{}, CardStatus{}, ErrCancelled
	}
	return rec, out, nil
}
func selectedStatus(v CardStatus) (Status, error) {
	if len(v.Bouts) != 1 {
		return Status{}, ErrNotFound
	}
	return Status{Version: 1, StoreID: v.StoreID, CardID: v.CardID, ObservedAtMS: v.ObservedAtMS, Source: v.Source, Saved: v.Bouts[0]}, nil
}
func (s *Store) readCall(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, id string, decodeRow bool) (Record, CardStatus, error) {
	if !callsValid(ctx, cards, seeds, events, entries) || id != "" && !hashID(id) {
		return Record{}, CardStatus{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, CardStatus{}, e
	}
	defer done()
	return s.observe(work, cards, seeds, events, entries, session, date, id, decodeRow)
}
func (s *Store) CardStatus(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (CardStatus, error) {
	_, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, "", false)
	return v, e
}
func (s *Store) Status(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, id string) (Status, error) {
	if !hashID(id) {
		return Status{}, ErrInvalid
	}
	_, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, id, false)
	if e != nil {
		return Status{}, e
	}
	return selectedStatus(v)
}
func (s *Store) Read(ctx context.Context, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, id string) (ReadResult, error) {
	if !hashID(id) {
		return ReadResult{}, ErrInvalid
	}
	r, v, e := s.readCall(ctx, cards, seeds, events, entries, session, date, id, true)
	if e != nil {
		return ReadResult{}, e
	}
	st, e := selectedStatus(v)
	if e != nil || !st.Saved.Exists {
		return ReadResult{}, ErrNotFound
	}
	return ReadResult{Version: 1, Record: r, Status: st}, nil
}
