// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

// SeedSourceInfo is an owned in-process source observation, never an imported
// verification verdict. A foreign callback must not reenter these stores.
type SeedSourceInfo struct {
	Version              uint32
	StoreID              string
	Pin                  Pin
	ObservedAtMS         int64
	ParticipantAccountID string
	ParticipantGymID     string
}

func (s *Store) seedView(ctx context.Context, source *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, fn func(SeedSourceInfo, func() error) error) error {
	if source == nil || fn == nil || (cap != gymaccount.View && cap != gymaccount.SeedPublish) {
		return ErrInvalid
	}
	return s.publicationView(ctx, source, session, cap, false, fn)
}
func (s *Store) publicationView(ctx context.Context, source *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, result bool, fn func(SeedSourceInfo, func() error) error) error {
	observe := source.WithSeedView
	if result {
		observe = source.WithResultView
	}
	return mapped(observe(ctx, session, cap, func(v gymentry.SnapshotSourceInfo, check func() error) error {
		p, e := participant(s.meta.Pin, v.Pin.StablePin.Identity.GymID)
		if e != nil || !equal(p, v.Pin) {
			return ErrForbidden
		}
		id, _ := gymentry.ID(p)
		if v.Version != 1 || v.StoreID != id || !timestamp(v.ObservedAtMS) || check == nil {
			return ErrCorrupt
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if e = check(); e != nil {
			return e
		}
		n, e := s.observeLocked(v.ObservedAtMS)
		if e != nil {
			return e
		}
		pin := s.meta.Pin
		pin.Participants = append([]gymentry.Pin(nil), pin.Participants...)
		return fn(SeedSourceInfo{Version: 1, StoreID: s.meta.StoreID, Pin: pin, ObservedAtMS: n, ParticipantAccountID: p.StablePin.AccountID, ParticipantGymID: p.StablePin.Identity.GymID}, check)
	}))
}
func (s *Store) WithSeedView(ctx context.Context, source *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, fn func(SeedSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.SeedPublish) {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.seedView(work, source, session, cap, fn)
}
func (s *Store) WithSeedSnapshot(ctx context.Context, source *gymentry.Store, session gymstable.Session, saturday string, prepare func(SnapshotReceipt) (func(SeedSourceInfo, func() error) error, error)) error {
	if prepare == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return e
	}
	if e = s.seedView(work, source, session, gymaccount.View, func(_ SeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	receipt, ok, e := s.receipt(ev)
	if e != nil {
		return e
	}
	if !ok {
		return ErrIncomplete
	}
	owned, e := ValidateSnapshotReceipt(s.meta.Pin, receipt)
	if e != nil {
		return ErrCorrupt
	}
	// Canonical validation and consumer preparation are outside the account lease.
	canonical, e := MarshalSnapshotReceipt(s.meta.Pin, owned)
	if e != nil {
		return ErrCorrupt
	}
	commit, e := prepare(owned)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	return s.seedView(work, source, session, gymaccount.View, func(info SeedSourceInfo, check func() error) error {
		raw, exists, e := s.raw(key('s', ev.EventID), MaxReceiptSize)
		if e != nil {
			return e
		}
		if !exists || string(raw) != string(canonical) {
			return ErrConflict
		}
		if e = check(); e != nil {
			return e
		}
		return commit(info, check)
	})
}
