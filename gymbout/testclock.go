//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"github.com/taytr-llc/slug/gymcard"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, seeds *gymcard.Store, clock func() time.Time) (*Store, error) {
	return create(ctx, path, pin, seeds, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, seeds *gymcard.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, false)
}
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, seeds *gymcard.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, true)
}
