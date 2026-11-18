//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func OpenCareerForTestClock(ctx context.Context, path string, expected Pin, source *gymseed.Store, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openWithCareer(ctx, path, expected, source, clock, false, &activation, false)
}
func ResumeCareerBindingForTestClock(ctx context.Context, path string, expected Pin, source *gymseed.Store, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openWithCareer(ctx, path, expected, source, clock, false, &activation, true)
}
