//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/taytr-llc/slug/gymevent"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, events *gymevent.Store, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return create(ctx, path, pin, events, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, events *gymevent.Store, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return open(ctx, path, pin, events, clock, false)
}
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, events *gymevent.Store, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return open(ctx, path, pin, events, clock, true)
}
