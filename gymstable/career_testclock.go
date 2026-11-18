//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"time"

	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

func OpenCareerForTestClock(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openCareerWithClock(path, expected, source, clock, &activation, false)
}
func ResumeCareerBindingForTestClock(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openCareerWithClock(path, expected, source, clock, &activation, true)
}
