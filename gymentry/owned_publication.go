// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *OwnedStore) WithSeedView(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(OwnedSnapshotSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.SeedPublish) {
		return ErrInvalid
	}
	return s.publicationView(ctx, session, cap, fn)
}

func (s *OwnedStore) WithResultView(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(OwnedSnapshotSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	return s.publicationView(ctx, session, cap, fn)
}

func (s *OwnedStore) publicationView(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(OwnedSnapshotSourceInfo, func() error) error) error {
	ctx, done, e := s.begin(ctx, session, cap)
	if e != nil {
		return e
	}
	defer done()
	if cap != gymaccount.View {
		fixture, e := gymaccount.FixtureIdentity("zero256")
		if e != nil || fixture != s.meta.Pin.StablePin.Stable.Identity {
			return ErrForbidden
		}
	}
	now, e := s.observe(ctx, session, cap, nil)
	if e != nil {
		return e
	}
	return s.fence(ctx, session, cap, func(check func() error) error {
		return fn(OwnedSnapshotSourceInfo{1, s.meta.StoreID, s.meta.Pin, now}, check)
	})
}
