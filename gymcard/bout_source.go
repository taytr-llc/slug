// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

// Projection is private: the row was validated by Open/Build and its exact bytes
// fenced by record(). Current seed evidence is independently validated each call.
func projectBooked(p Pin, r Record) ([]BoutSource, error) {
	out := make([]BoutSource, 0, len(r.Plan.Bouts))
	for _, b := range r.Plan.Bouts {
		c := make([]BoutCorner, 0, 2)
		for _, id := range []string{b.CornerA, b.CornerB} {
			for _, part := range r.Source.SourceReceipt.Input.Participants {
				for _, m := range part.Members {
					if m.Assignment.ProspectID == id {
						c = append(c, BoutCorner{part.Owner, m})
					}
				}
			}
		}
		if len(c) != 2 {
			return nil, ErrCorrupt
		}
		out = append(out, BoutSource{Version: 1, StoreID: r.StoreID, CardID: r.CardID, CardRecordDigest: recordDigest(r), CardSourceDigest: r.SourceDigest, CardCreatedAtMS: r.CreatedAtMS, Event: r.Source.SourceReceipt.Input.Event, SnapshotDigest: r.Source.SourceReceipt.SnapshotDigest, SeedEventKey: r.Source.EventKey, TargetHeight: r.Source.Target.Height, TargetHash: r.Source.Target.Hash, FinalizedAtMS: r.Source.Finality.ObservedAtMS, Bout: b, Corners: c})
	}
	return out, nil
}
func projectBoutState(v Status) BoutState {
	x := v.Source.Status
	return BoutState{Version: 1, CardID: v.CardID, Event: v.Event, CardExists: v.Exists, CardRecordDigest: v.RecordDigest, CardSourceDigest: func() string {
		if v.Exists {
			return v.Source.SourceDigest
		}
		return ""
	}(), CardCreatedAtMS: v.CreatedAtMS, FinalizedAtMS: x.FinalizedAtMS, ConflictCount: x.ConflictCount, ConflictOverflow: x.ConflictOverflow, IncidentAtMS: x.IncidentAtMS, EscalationDue: x.EscalationDue, ChainObservedAtMS: x.ChainObservedAtMS, TargetDescendants: x.TargetDescendants, CurrentFinality: x.CurrentFinality, NodeState: x.NodeState, ObservedAtMS: v.ObservedAtMS, Usable: v.Usable}
}
func (s *Store) withBoutViewHeld(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string, commit func(BoutView, []BoutSource, func() error) error) error {
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return ErrCalendar
	}
	id := cardID(s.meta.Pin, ev)
	mutated := false
	e = seeds.WithCardPreparedView(ctx, events, entries, session, func(info gymseed.Info) ([]string, func(gymseed.CardView, func() error) error, error) {
		if e := s.checkPreparedInfo(info); e != nil {
			return nil, nil, e
		}
		pinRaw, _ := json.Marshal(s.meta.Pin)
		var ownedPin Pin
		if json.Unmarshal(pinRaw, &ownedPin) != nil {
			return nil, nil, ErrCorrupt
		}
		r, ok, e := s.record(id)
		if e != nil {
			return nil, nil, e
		}
		var rp *Record
		projected := []BoutSource{}
		refs := []BoutRef{}
		if ok {
			rp = &r
			projected, e = projectBooked(s.meta.Pin, r)
			if e != nil {
				return nil, nil, e
			}
			for _, src := range projected {
				refs = append(refs, BoutRef{src.Bout, boutSourceDigest(src)})
			}
		}
		return []string{date}, func(v gymseed.CardView, check func() error) error {
			s.mu.Lock()
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
			state := projectBoutState(statusFor(s.meta.Pin, ev, rp, v.States[0], n))
			view := BoutView{Version: 1, StoreID: s.meta.StoreID, Pin: ownedPin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, State: state, Bouts: refs}
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
			e = commit(view, projected, guard)
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
		return observationError(ctx, e, mutated)
	}
	return nil
}
func (s *Store) WithBoutView(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string, commit func(BoutView, func() error) error) error {
	if !callValid(ctx, seeds, events, entries) || commit == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v BoutView, _ []BoutSource, check func() error) error { return commit(v, check) })
}
func (s *Store) WithBoutSource(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date, boutID string, prepare func(BoutSource) (func(BoutView, func() error) error, error)) error {
	if !callValid(ctx, seeds, events, entries) || !hashID(boutID) || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	var selected BoutSource
	var found, exists, healthy bool
	e = s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v BoutView, src []BoutSource, check func() error) error {
		exists = v.State.CardExists
		healthy = v.State.Usable
		for _, x := range src {
			if x.Bout.BoutID == boutID {
				selected = x
				found = true
			}
		}
		return check()
	})
	if e != nil {
		return e
	}
	if !exists {
		return ErrSourceIncomplete
	}
	if !found {
		return ErrNotFound
	}
	if !healthy {
		return ErrSourceConflict
	}
	// Lower source gates and every account lease are released before preparation.
	// Only this card gate remains held while a consumer derives/simulates.
	raw, _ := json.Marshal(selected)
	var owned BoutSource
	if json.Unmarshal(raw, &owned) != nil {
		return ErrCorrupt
	}
	digest := boutSourceDigest(selected)
	s.mark("bout-source-prepared")
	commit, e := prepare(owned)
	if e != nil {
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
	return s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v BoutView, _ []BoutSource, check func() error) error {
		if !v.State.CardExists || v.State.CardRecordDigest != selected.CardRecordDigest || v.State.CardSourceDigest != selected.CardSourceDigest || v.State.CardCreatedAtMS != selected.CardCreatedAtMS {
			return ErrSourceChanged
		}
		matched := false
		for _, r := range v.Bouts {
			if r.Bout.BoutID == boutID && r.SourceDigest == digest {
				matched = true
			}
		}
		if !matched {
			return ErrSourceChanged
		}
		if !v.State.Usable {
			return ErrSourceConflict
		}
		return commit(v, check)
	})
}
