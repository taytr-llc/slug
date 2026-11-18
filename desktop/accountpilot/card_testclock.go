//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"time"
)

func NewCardsForTestClock(c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymcard.ErrInvalid
	}
	d := eventFactoriesForTestClock(clock)
	d.child = func(c pilot.Config, a pilot.StableAccountConfig) (*pilot.StableController, error) {
		return pilot.NewCardManifestAccountForTestClock(c, a, clock)
	}
	seeds := seedAccountFactories{d, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.OpenForTestClock(ctx, path, p, e, clock)
	}, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.ResumePreparedForTestClock(ctx, path, p, e, clock)
	}}
	return newCards(c, cardAccountFactories{seeds, func(ctx context.Context, path string, p gymcard.Pin, s *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.OpenForTestClock(ctx, path, p, s, clock)
	}, func(ctx context.Context, path string, p gymcard.Pin, s *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.ResumePreparedForTestClock(ctx, path, p, s, clock)
	}})
}
