// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
)

func mapped(e error) error {
	if e == nil {
		return nil
	}
	switch {
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded), errors.Is(e, gymevent.ErrCancelled):
		return ErrCancelled
	case errors.Is(e, gymevent.ErrUnauthorized), errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired):
		return ErrUnauthorized
	case errors.Is(e, gymevent.ErrForbidden), errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymaccount.ErrHeld):
		return ErrForbidden
	case errors.Is(e, gymevent.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymevent.ErrClock), errors.Is(e, gymaccount.ErrClock):
		return ErrClock
	case errors.Is(e, gymevent.ErrCalendar):
		return ErrCalendar
	case errors.Is(e, gymevent.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymevent.ErrIncomplete), errors.Is(e, gymevent.ErrNotFound):
		return ErrSourceIncomplete
	case errors.Is(e, gymevent.ErrOutcomeUnknown), errors.Is(e, seednode.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymevent.ErrCapacity), errors.Is(e, seednode.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, seednode.ErrOffline):
		return ErrNodeOffline
	case errors.Is(e, seednode.ErrNotMature):
		return ErrNotMature
	case errors.Is(e, seednode.ErrInvalid), errors.Is(e, gymevent.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, seednode.ErrUnavailable), errors.Is(e, gymevent.ErrUnavailable), errors.Is(e, gymevent.ErrClosed), errors.Is(e, gymevent.ErrPoisoned):
		return ErrUnavailable
	}
	return e
}
func (s *Store) withSource(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, fn func(int64, func() error) error) error {
	if events == nil || entries == nil || fn == nil {
		return ErrInvalid
	}
	return mapped(events.WithSeedView(ctx, entries, session, cap, func(v gymevent.SeedSourceInfo, check func() error) error {
		if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) || !timestamp(v.ObservedAtMS) || check == nil {
			return ErrForbidden
		}
		if cap == gymaccount.SeedPublish && (v.ParticipantAccountID != s.meta.Pin.PublisherAccountID || v.ParticipantGymID != s.meta.Pin.PublisherGymID) {
			return ErrForbidden
		}
		return fn(v.ObservedAtMS, check)
	}))
}
