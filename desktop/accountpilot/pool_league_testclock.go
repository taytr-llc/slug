//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
)

// NewPoolForTestClock changes only shared stable/entry/event calendar clocks.
// Actual node, wallet, account/provider/session and settlement clocks stay real.
// This tagged constructor has no corresponding normal Config or CLI option.
func NewPoolForTestClock(ctx context.Context, config pilot.Config, clock func() time.Time) (*PoolController, error) {
	return NewPoolForTestClockWithOptions(ctx, config, clock, pilot.PoolOptions{})
}
func NewPoolForTestClockWithOptions(ctx context.Context, config pilot.Config, clock func() time.Time, options pilot.PoolOptions) (*PoolController, error) {
	if clock == nil {
		return nil, gymevent.ErrInvalid
	}
	return newPoolWithOptions(ctx, config, options, func(ctx context.Context, config pilot.Config, accounts *gymaccount.Store) (*pilot.PoolRuntime, error) {
		return pilot.NewPoolRuntimeForTestClockWithOptions(ctx, config, accounts, clock, options)
	})
}
