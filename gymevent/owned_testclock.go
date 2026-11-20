//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"time"
)

// UseOwnedClockForTest is absent from ordinary binaries. Historical capture
// still requires the concrete source's signed/replayed exact cutoff.
func (s *OwnedStore) UseOwnedClockForTest(ctx context.Context, expected OwnedPin, clock func() time.Time) error {
	if s == nil || clock == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	if s.readOnly || !ownedcodec.Equal(s.meta.Pin, expected) {
		return ErrConflict
	}
	n := clock().UnixMilli()
	if !timestamp(n) || n < s.meta.ClockMS || n < s.meta.CreatedAtMS {
		return ErrClock
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.now = clock
	return nil
}
