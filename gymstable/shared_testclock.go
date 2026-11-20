//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"time"
)

func CreateSharedForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return createShared(path, pin, source, clock, nil)
}
func OpenSharedForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openSharedCareerWithClock(path, pin, source, clock, nil, false)
}
func OpenSharedCareerForTestClock(path string, pin Pin, source *registry.ManifestStore, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openSharedCareerWithClock(path, pin, source, clock, &activation, false)
}
func ResumeSharedCareerBindingForTestClock(path string, pin Pin, source *registry.ManifestStore, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openSharedCareerWithClock(path, pin, source, clock, &activation, true)
}
