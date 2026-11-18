//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"time"
)

func NewResultsForTestClock(ctx context.Context, c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymcard.ErrInvalid
	}
	return newResults(ctx, c, resultFactoriesForTestClock(clock))
}
func resultFactoriesForTestClock(clock func() time.Time) resultAccountFactories {
	d := eventFactoriesForTestClock(clock)
	d.child = func(c pilot.Config, a pilot.StableAccountConfig) (*pilot.StableController, error) {
		return pilot.NewResultManifestAccountForTestClock(c, a, clock)
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
	bouts := boutAccountFactories{cards, func(ctx context.Context, path string, p gymbout.Pin, c *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.OpenForTestClock(ctx, path, p, c, clock)
	}, func(ctx context.Context, path string, p gymbout.Pin, c *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.ResumePreparedForTestClock(ctx, path, p, c, clock)
	}}
	return resultAccountFactories{bouts, func(ctx context.Context, path string, p gymresult.Pin, b *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.OpenForTestClock(ctx, path, p, b, clock)
	}, func(ctx context.Context, path string, p gymresult.Pin, b *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.ResumePreparedForTestClock(ctx, path, p, b, clock)
	}}
}
