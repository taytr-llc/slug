//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "time"

func NewBoutManifestAccountForTestClock(c Config, a StableAccountConfig, clock func() time.Time) (*StableController, error) {
	s, e := NewCardManifestAccountForTestClock(c, a, clock)
	if e == nil {
		s.boutEnabled = true
	}
	return s, e
}
