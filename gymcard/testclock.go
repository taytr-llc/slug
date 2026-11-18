//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"github.com/taytr-llc/slug/gymseed"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, seeds *gymseed.Store, clock func() time.Time) (*Store, error) {
	return create(ctx, path, pin, seeds, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, seeds *gymseed.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, false)
}
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, seeds *gymseed.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, true)
}
