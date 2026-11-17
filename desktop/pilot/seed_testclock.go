//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "time"

func NewSeedManifestAccountForTestClock(c Config, a StableAccountConfig, clock func() time.Time) (*StableController, error) {
	s, e := NewEventManifestAccountForTestClock(c, a, clock)
	if e == nil {
		s.seedEnabled = true
	}
	return s, e
}
