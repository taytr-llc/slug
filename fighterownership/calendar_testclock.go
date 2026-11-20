//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"time"
)

// UseCalendarClockForTest changes only the retained public-store clock, never
// its source/node/replay evidence. CaptureBoundary still waits and makes both
// real RPC observations. This method is absent from ordinary binaries.
func (s *Store) UseCalendarClockForTest(ctx context.Context, expected Pin, id StoreIdentity, clock func() time.Time) error {
	if s == nil || clock == nil || s.pin != expected || s.identity != id {
		return ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return e
	}
	defer done()
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if e = s.state(); e != nil {
		return e
	}
	if s.work != nil {
		return ErrBusy
	}
	n := clock().UnixMilli()
	if n < s.meta.ClockHighWaterMS || n < 0 || n > MaxTimestampMS {
		return ErrClock
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.now = clock
	return nil
}
