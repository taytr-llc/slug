//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

// NewPoolRuntimeForTestClock scopes a calendar clock to stable, entry and event
// stores and every later reopen. Mining, account/provider/session, wallet and
// settlement clocks remain real. This constructor is absent from the ordinary
// executable; Config, CLI, environment and protocol expose no clock bypass.
func NewPoolRuntimeForTestClock(ctx context.Context, config Config, accounts *gymaccount.Store, clock func() time.Time) (*PoolRuntime, error) {
	if clock == nil {
		return nil, gymevent.ErrInvalid
	}
	r, e := NewPoolRuntime(ctx, config, accounts)
	if e != nil {
		return nil, e
	}
	r.leagueFactories = &poolLeagueFactories{
		createStable: func(p string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.CreateSharedForTestClock(p, pin, source, clock)
		},
		openStable: func(p string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.OpenSharedForTestClock(p, pin, source, clock)
		},
		createEntry: func(ctx context.Context, p string, pin gymentry.Pin, source *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.CreateForTestClock(ctx, p, pin, source, clock)
		},
		openEntry: func(ctx context.Context, p string, pin gymentry.Pin, source *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.OpenForTestClock(ctx, p, pin, source, clock)
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
	}
	return r, nil
}
