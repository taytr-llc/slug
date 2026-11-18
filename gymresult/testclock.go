//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
	"time"
)

func CreateForTestClock(ctx context.Context, path string, pin Pin, seeds *gymbout.Store, clock func() time.Time) (*Store, error) {
	return create(ctx, path, pin, seeds, clock, nil)
}
func OpenForTestClock(ctx context.Context, path string, pin Pin, seeds *gymbout.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, false)
}
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, seeds *gymbout.Store, clock func() time.Time) (*Store, error) {
	return open(ctx, path, pin, seeds, clock, true)
}

func CheckCareerActivationForTestClock(ctx context.Context, path string, pin Pin, bouts *gymbout.Store, clock func() time.Time) error {
	return checkCareerActivation(ctx, path, pin, bouts, clock)
}
func ActivateCareerForTestClock(ctx context.Context, path string, pin Pin, bouts *gymbout.Store, clock func() time.Time) (*Store, error) {
	return openCareer(ctx, path, pin, bouts, clock, "activate", nil)
}
func ResumeCareerActivationForTestClock(ctx context.Context, path string, pin Pin, bouts *gymbout.Store, clock func() time.Time) (*Store, error) {
	return openCareer(ctx, path, pin, bouts, clock, "resume", nil)
}
func OpenCareerForTestClock(ctx context.Context, path string, pin Pin, bouts *gymbout.Store, clock func() time.Time) (*Store, error) {
	return openCareer(ctx, path, pin, bouts, clock, "open", nil)
}
