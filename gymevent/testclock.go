//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return create(ctx, path, pin, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return open(ctx, path, pin, clock, false)
}
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return open(ctx, path, pin, clock, true)
}
