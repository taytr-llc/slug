//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"time"
)

// NewEventManifestAccountForTestClock is available only in the explicit
// normal-PoW test-clock executable. No CLI/DTO/environment enables it.
func NewEventManifestAccountForTestClock(c Config, a StableAccountConfig, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, gymevent.ErrInvalid
	}
	f := &eventStoreFactories{
		createStable: func(p string, pin gymstable.Pin, src *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.CreateForTestClock(p, pin, src, clock)
		},
		openStable: func(p string, pin gymstable.Pin, src *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.OpenForTestClock(p, pin, src, clock)
		},
		createEntry: func(ctx context.Context, p string, pin gymentry.Pin, src *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.CreateForTestClock(ctx, p, pin, src, clock)
		},
		openEntry: func(ctx context.Context, p string, pin gymentry.Pin, src *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.OpenForTestClock(ctx, p, pin, src, clock)
		},
	}
	return newEventManifestAccount(c, a, f)
}
