// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

// WithSeedView is a bounded concrete-source lease. It does not establish chain
// publication authority. The callback cannot reenter source or account stores.
func (s *Store) WithSeedView(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(SnapshotSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.SeedPublish) {
		return ErrInvalid
	}
	if s == nil {
		return ErrUnavailable
	}
	if cap == gymaccount.SeedPublish {
		zero, e := gymaccount.FixtureIdentity("zero256")
		s.mu.Lock()
		identity := s.meta.Pin.StablePin.Identity
		s.mu.Unlock()
		if e != nil || identity != zero {
			return ErrForbidden
		}
	}
	work, done, e := s.begin(ctx, session, cap)
	if e != nil {
		return e
	}
	defer done()
	return s.withLease(work, session, cap, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now, e := s.observeLocked(nil, 0, nil)
		if e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		return fn(SnapshotSourceInfo{Version: 1, StoreID: s.meta.StoreID, Pin: s.meta.Pin, ObservedAtMS: now}, check)
	})
}
