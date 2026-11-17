//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"github.com/taytr-llc/slug/registry"
	"time"
)

func CreateForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return create(path, pin, source, clock, nil)
}
func OpenForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store, error) {
	if clock == nil {
		return nil, ErrInvalid
	}
	return openWithClock(path, pin, source, clock)
}
