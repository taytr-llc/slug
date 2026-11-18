//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"time"
)

func NewBoutsForTestClock(ctx context.Context, c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymcard.ErrInvalid
	}
	d := eventFactoriesForTestClock(clock)
	d.child = func(c pilot.Config, a pilot.StableAccountConfig) (*pilot.StableController, error) {
		return pilot.NewBoutManifestAccountForTestClock(c, a, clock)
	}
	seeds := seedAccountFactories{d, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.OpenForTestClock(ctx, path, p, e, clock)
	}, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.ResumePreparedForTestClock(ctx, path, p, e, clock)
	}}
	cards := cardAccountFactories{seeds, func(ctx context.Context, path string, p gymcard.Pin, s *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.OpenForTestClock(ctx, path, p, s, clock)
	}, func(ctx context.Context, path string, p gymcard.Pin, s *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.ResumePreparedForTestClock(ctx, path, p, s, clock)
	}}
	return newBouts(ctx, c, boutAccountFactories{cards, func(ctx context.Context, path string, p gymbout.Pin, c *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.OpenForTestClock(ctx, path, p, c, clock)
	}, func(ctx context.Context, path string, p gymbout.Pin, c *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.ResumePreparedForTestClock(ctx, path, p, c, clock)
	}})
}
