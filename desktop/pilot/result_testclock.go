//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "time"

func NewResultManifestAccountForTestClock(c Config, a StableAccountConfig, clock func() time.Time) (*StableController, error) {
	s, e := NewBoutManifestAccountForTestClock(c, a, clock)
	if e == nil {
		s.resultEnabled = true
	}
	return s, e
}
