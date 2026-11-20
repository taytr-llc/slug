//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"time"
)

// UseOwnedClockForTest is absent from ordinary binaries. The fixture parent
// must have joined all children before changing the original concrete owner.
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
	if !ownedTime(n) || n < s.meta.ClockMS || n < s.meta.LastAsOfMS {
		return ErrClock
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.now = clock
	return nil
}
