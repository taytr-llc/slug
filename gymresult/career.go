// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

type CareerActivation = careerstate.Activation

var (
	ErrCareerLegacyFinality = errors.New("career_legacy_finality")
	ErrCareerInactive       = careerstate.ErrInactive
	ErrCareerNotDue         = errors.New("career_not_due")
)

func CareerPolicyID() string { return careerstate.PolicyID() }
func (s *Store) CareerInfo(ctx context.Context) (CareerActivation, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return CareerActivation{}, e
	}
	defer done()
	if work.Err() != nil {
		return CareerActivation{}, ErrCancelled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meta.Career == nil {
		return CareerActivation{}, ErrCareerInactive
	}
	return *s.meta.Career, nil
}
func (s *Store) careerState(ctx context.Context) (careerstate.State, error) {
	if s == nil {
		return careerstate.State{}, careerstate.ErrInactive
	}
	if ctx == nil {
		return careerstate.State{}, careerstate.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.life.Err() != nil {
		return careerstate.State{}, careerstate.ErrClosed
	}
	if s.poisoned {
		return careerstate.State{}, careerstate.ErrPoisoned
	}
	if ctx.Err() != nil {
		return careerstate.State{}, careerstate.ErrCancelled
	}
	if s.meta.Career == nil {
		return careerstate.State{}, careerstate.ErrInactive
	}
	p := s.meta.Pin.SourcePin.SourcePin
	cid, e := gymcard.ID(p)
	if e != nil {
		return careerstate.State{}, careerstate.ErrCorrupt
	}
	participants := p.SourcePin.SourcePin.Participants
	if len(participants) != 2 {
		return careerstate.State{}, careerstate.ErrCorrupt
	}
	scope := careerstate.Scope{CardStoreID: cid}
	for i, x := range participants {
		id, e := gymstable.ID(x.StablePin)
		if e != nil {
			return careerstate.State{}, careerstate.ErrCorrupt
		}
		scope.StableStoreIDs[i] = id
	}
	return careerstate.State{Activation: *s.meta.Career, Scope: scope, ClockMS: s.meta.ClockMS, Events: append([]careerstate.Event{}, s.careerIndex...)}, nil
}
func (s *Store) CareerReader() (*careerstate.Reader, error) {
	state, e := s.careerState(context.Background())
	if e != nil {
		return nil, e
	}
	return careerstate.NewReader(state.Activation, state.Scope, s.careerState)
}
func careerEvent(a CareerActivation, r Record) (careerstate.Event, error) {
	if r.Finality == nil {
		return careerstate.Event{}, ErrInvalid
	}
	v := r.Statement.Statement.Source
	src := v.Source
	if len(src.Corners) != 2 {
		return careerstate.Event{}, ErrCorrupt
	}
	corners := [2]careerstate.Corner{}
	for i, x := range src.Corners {
		corners[i] = careerstate.Corner{ProspectID: x.Member.Assignment.ProspectID, AccountID: x.Owner.AccountID, GymID: x.Owner.GymID}
	}
	b, e := json.Marshal(r.Finality)
	if e != nil {
		return careerstate.Event{}, ErrCorrupt
	}
	return careerstate.Event{Version: 1, AwardID: careerstate.AwardID(a, r.ResultKey), ResultKey: r.ResultKey, CardID: src.CardID, BoutID: src.Bout.BoutID, SourceRecordDigest: v.RecordDigest, InputID: v.InputID, ResultDigest: v.ResultDigest, FinalityDigest: careerstate.FinalityDigest(b), EffectiveAtMS: r.Finality.ObservedAtMS, Saturday: src.Event.Saturday, CornerA: corners[0], CornerB: corners[1], Winner: v.Summary.Winner, Method: v.Summary.Method}, nil
}
func (s *Store) proposedCareerIndex(r Record) ([]careerstate.Event, error) {
	if s.meta.Career == nil {
		return nil, nil
	}
	events := append([]careerstate.Event{}, s.careerIndex...)
	found := -1
	for i, x := range events {
		if x.ResultKey == r.ResultKey {
			found = i
			break
		}
	}
	if r.Finality == nil {
		if found >= 0 {
			return nil, ErrCorrupt
		}
		return events, nil
	}
	event, e := careerEvent(*s.meta.Career, r)
	if e != nil {
		return nil, e
	}
	if found >= 0 {
		if events[found] != event {
			return nil, ErrCorrupt
		}
	} else {
		events = append(events, event)
	}
	out, e := careerstate.ValidateTransition(*s.meta.Career, s.careerIndex, events)
	if e != nil {
		return nil, ErrCorrupt
	}
	return out, nil
}

// FenceCareer runs before consumer gates. It never enters a source chain while
// holding the result mutex and never fabricates a future wall-clock observation.
func (s *Store) FenceCareer(ctx context.Context, entries *gymentry.Store, session gymstable.Session, saturday string) error {
	if entries == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	if s.meta.Career == nil {
		return ErrCareerInactive
	}
	pin := s.meta.Pin.SourcePin.SourcePin.SourcePin.SourcePin
	event, e := gymevent.EventForDate(pin, saturday)
	if e != nil {
		return ErrCalendar
	}
	// Refuse a future cutoff before the entry observation can write its own clock.
	s.mu.Lock()
	now, e := s.sampleLocked(0)
	s.mu.Unlock()
	if e != nil {
		return e
	}
	if now < event.CutoffMS {
		return ErrCareerNotDue
	}
	mutated := false
	e = entries.WithSnapshotView(work, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
		matched := false
		for _, p := range pin.Participants {
			id, _ := gymentry.ID(p)
			if info.StoreID == id && equal(info.Pin, p) {
				matched = true
				break
			}
		}
		if !matched {
			return ErrForbidden
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		c := sourceCall{ctx: work}
		if e := s.guard(c, check); e != nil {
			return e
		}
		n, e := s.sampleLocked(max(event.CutoffMS, info.ObservedAtMS))
		if e != nil {
			return e
		}
		if n != s.meta.ClockMS {
			m := s.meta
			m.ClockMS = n
			if e = s.commitLocked(new(leveldb.Batch), m); e != nil {
				return e
			}
			mutated = true
		}
		return s.guard(c, check)
	})
	return s.finish(sourceCall{ctx: work}, careerEntryError(e), mutated)
}

func careerEntryError(e error) error {
	pairs := [][2]error{{gymentry.ErrUnauthorized, ErrUnauthorized}, {gymentry.ErrForbidden, ErrForbidden}, {gymentry.ErrStaleView, ErrStaleView}, {gymentry.ErrCancelled, ErrCancelled}, {gymentry.ErrOutcomeUnknown, ErrOutcomeUnknown}, {gymentry.ErrClock, ErrClock}, {gymentry.ErrCorrupt, ErrCorrupt}, {gymentry.ErrInvalid, ErrInvalid}, {gymentry.ErrBusy, ErrBusy}, {gymentry.ErrClosed, ErrUnavailable}, {gymentry.ErrPoisoned, ErrUnavailable}, {gymentry.ErrUnavailable, ErrUnavailable}, {gymentry.ErrProfile, ErrProfile}}
	for _, p := range pairs {
		if errors.Is(e, p[0]) {
			return p[1]
		}
	}
	return e
}
