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
func ownedProjectBooked(p OwnedPin, r OwnedRecord) ([]OwnedBoutSource, error) {
	out := make([]OwnedBoutSource, 0, len(r.Plan.Bouts))
	for _, b := range r.Plan.Bouts {
		c := make([]OwnedBoutCorner, 0, 2)
		for _, id := range []string{b.CornerA, b.CornerB} {
			for _, part := range r.Source.SourceReceipt.Input.Participants {
				for _, m := range part.Members {
					if m.Birth.ProspectID == id {
						c = append(c, OwnedBoutCorner{part.Owner, m})
					}
				}
			}
		}
		if len(c) != 2 {
			return nil, ErrCorrupt
		}
		out = append(out, OwnedBoutSource{Version: 1, StoreID: r.StoreID, CardID: r.CardID, CardRecordDigest: ownedRecordDigest(r), CardSourceDigest: r.SourceDigest, CardCreatedAtMS: r.CreatedAtMS, Event: r.Source.SourceReceipt.Input.Event, SnapshotDigest: r.Source.SourceReceipt.SnapshotDigest, SeedEventKey: r.Source.EventKey, TargetHeight: r.Source.Target.Height, TargetHash: r.Source.Target.Hash, FinalizedAtMS: r.Source.Finality.ObservedAtMS, Bout: b, Corners: c})
	}
	return out, nil
}
func ownedProjectBoutState(v OwnedStatus) OwnedBoutState {
	x := v.Source.Status
	return OwnedBoutState{Version: 1, CardID: v.CardID, Event: v.Event, CardExists: v.Exists, CardRecordDigest: v.RecordDigest, CardSourceDigest: func() string {
		if v.Exists {
			return v.Source.SourceDigest
		}
		return ""
	}(), CardCreatedAtMS: v.CreatedAtMS, FinalizedAtMS: x.FinalizedAtMS, ConflictCount: x.ConflictCount, ConflictOverflow: x.ConflictOverflow, IncidentAtMS: x.IncidentAtMS, EscalationDue: x.EscalationDue, ChainObservedAtMS: x.ChainObservedAtMS, TargetDescendants: x.TargetDescendants, CurrentFinality: x.CurrentFinality, NodeState: x.NodeState, ObservedAtMS: v.ObservedAtMS, Usable: v.Usable}
}
func (s *OwnedStore) withBoutViewHeld(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date string, commit func(OwnedBoutView, []OwnedBoutSource, func() error) error) error {
	ev, e := gymevent.OwnedEventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return ErrCalendar
	}
	id := ownedCardID(s.meta.Pin, ev)
	mutated := false
	e = seeds.WithCardPreparedView(ctx, events, entries, session, func(info gymseed.OwnedInfo) ([]string, func(gymseed.OwnedCardView, func() error) error, error) {
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
			view := OwnedBoutView{Version: 1, StoreID: s.meta.StoreID, Pin: ownedPin, CreatedAtMS: s.meta.CreatedAtMS, ParticipantAccountID: v.ParticipantAccountID, ParticipantGymID: v.ParticipantGymID, ObservedAtMS: n, State: state, Bouts: refs}
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
func (s *OwnedStore) WithBoutView(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date string, commit func(OwnedBoutView, func() error) error) error {
	if !ownedCallValid(ctx, seeds, events, entries) || commit == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v OwnedBoutView, _ []OwnedBoutSource, check func() error) error { return commit(v, check) })
}
func (s *OwnedStore) WithBoutSource(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, boutID string, prepare func(OwnedBoutSource) (func(OwnedBoutView, func() error) error, error)) error {
	if !ownedCallValid(ctx, seeds, events, entries) || !hashID(boutID) || prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	var selected OwnedBoutSource
	var found, exists, healthy bool
	e = s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v OwnedBoutView, src []OwnedBoutSource, check func() error) error {
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
	var ownedValue OwnedBoutSource
	if json.Unmarshal(raw, &ownedValue) != nil {
		return ErrCorrupt
	}
	digest := ownedBoutSourceDigest(selected)
	s.mark("bout-source-prepared")
	commit, e := prepare(ownedValue)
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
	return s.withBoutViewHeld(work, seeds, events, entries, session, date, func(v OwnedBoutView, _ []OwnedBoutSource, check func() error) error {
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
