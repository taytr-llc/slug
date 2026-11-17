//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, source *gymstable.Store, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return create(ctx, path, pin, source, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, source *gymstable.Store, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openWithClock(ctx, path, pin, source, clock)
}
