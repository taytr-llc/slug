//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymaccount

import (
	"github.com/taytr-llc/slug/wire/v2"
	"time"
)

// CreateFixtureForTestClock is compiled only for explicit fixture composition.
// It never changes a process global or enables identity/account providers.
func CreateFixtureForTestClock(path string, config Config, clock func() time.Time) (*Store, error) {
	if wire.TestPoW || clock == nil {
		return nil, ErrForbidden
	}
	return createStoreWithClock(path, config, nil, clock)
}
func OpenFixtureForTestClock(path string, config Config, clock func() time.Time) (*Store, error) {
	if wire.TestPoW || clock == nil {
		return nil, ErrForbidden
	}
	return openStoreWithClock(path, config, nil, clock)
}
