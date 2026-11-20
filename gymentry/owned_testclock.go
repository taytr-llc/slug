//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"time"
)

// UseOwnedClockForTest is absent from ordinary binaries. No receipt, cutoff,
// account capability or current owner is supplied by this clock seam.
func (s *OwnedStore) UseOwnedClockForTest(ctx context.Context, expected OwnedPin, clock func() time.Time) error {
	if s == nil || clock == nil {
		return ErrInvalid
	}
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if s.readOnly || s.meta.Pin != expected {
		return ErrConflict
	}
	n := clock().UnixMilli()
	if !timestamp(n) || n < s.meta.ClockMS {
		return ErrClock
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.now = clock
	return nil
}
