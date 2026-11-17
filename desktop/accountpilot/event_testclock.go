//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"time"
)

// NewEventsForTestClock scopes one owned fixture clock to every construction
// and reopen. It is absent from the normal build and from the CLI grammar.
func NewEventsForTestClock(c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymevent.ErrInvalid
	}
	d := eventAccountFactories{
		createAccount: func(p string, c gymaccount.Config) (*gymaccount.Store, error) {
			return gymaccount.CreateFixtureForTestClock(p, c, clock)
		},
		openAccount: func(p string, c gymaccount.Config) (*gymaccount.Store, error) {
			return gymaccount.OpenFixtureForTestClock(p, c, clock)
		},
		openStable: func(p string, pin gymstable.Pin, src *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.OpenForTestClock(p, pin, src, clock)
		},
		openEntry: func(ctx context.Context, p string, pin gymentry.Pin, src *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.OpenForTestClock(ctx, p, pin, src, clock)
		},
		createEvent: func(ctx context.Context, p string, pin gymevent.Pin) (*gymevent.Store, error) {
			return gymevent.CreateForTestClock(ctx, p, pin, clock)
		},
		openEvent: func(ctx context.Context, p string, pin gymevent.Pin) (*gymevent.Store, error) {
			return gymevent.OpenForTestClock(ctx, p, pin, clock)
		},
		resumeEvent: func(ctx context.Context, p string, pin gymevent.Pin) (*gymevent.Store, error) {
			return gymevent.ResumePreparedForTestClock(ctx, p, pin, clock)
		},
		child: func(c pilot.Config, a pilot.StableAccountConfig) (*pilot.StableController, error) {
			return pilot.NewEventManifestAccountForTestClock(c, a, clock)
		}, now: clock,
		verify: func(ctx context.Context, fixture string, reset bool) (gymaccount.Principal, error) {
			p, e := verifyFixture(ctx, fixture, reset)
			if e != nil {
				return gymaccount.Principal{}, e
			}
			if e = ctx.Err(); e != nil {
				return gymaccount.Principal{}, e
			}
			p.AuthenticatedAtMS = clock().UnixMilli()
			return p, nil
		},
	}
	return newEvents(c, d)
}
