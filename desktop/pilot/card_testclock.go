//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "time"

func NewCardManifestAccountForTestClock(c Config, a StableAccountConfig, clock func() time.Time) (*StableController, error) {
	s, e := NewSeedManifestAccountForTestClock(c, a, clock)
	if e == nil {
		s.cardEnabled = true
	}
	return s, e
}
