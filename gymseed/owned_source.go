// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *OwnedStore) withSource(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, cap gymaccount.Capability, fn func(int64, func() error) error) error {
	if events == nil || entries == nil || fn == nil {
		return ErrInvalid
	}
	return ownedMapped(events.WithSeedView(ctx, entries, session, cap, func(v gymevent.OwnedSeedSourceInfo, check func() error) error {
		if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) || !ownedTimestamp(v.ObservedAtMS) || check == nil {
			return ErrForbidden
		}
		if cap == gymaccount.SeedPublish && (v.ParticipantAccountID != s.meta.Pin.PublisherAccountID || v.ParticipantGymID != s.meta.Pin.PublisherGymID) {
			return ErrForbidden
		}
		return fn(v.ObservedAtMS, check)
	}))
}
