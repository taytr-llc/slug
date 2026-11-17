//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"time"
)

func NewSeedsForTestClock(c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymseed.ErrInvalid
	}
	d := eventFactoriesForTestClock(clock)
	d.child = func(c pilot.Config, a pilot.StableAccountConfig) (*pilot.StableController, error) {
		return pilot.NewSeedManifestAccountForTestClock(c, a, clock)
	}
	return newSeeds(c, seedAccountFactories{d, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.OpenForTestClock(ctx, path, p, e, clock)
	}, func(ctx context.Context, path string, p gymseed.Pin, e *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.ResumePreparedForTestClock(ctx, path, p, e, clock)
	}})
}
