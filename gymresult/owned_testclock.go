//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"time"
)

// UseOwnedClockForTest is absent from ordinary binaries. It changes only this
// exact result owner's clock source; FenceCareer must still observe and persist
// the cutoff through the original entry/session. It creates no award or fence.
func (s *OwnedStore) UseOwnedClockForTest(ctx context.Context, expected OwnedPin, clock func() time.Time) error {
	if s == nil || clock == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly || !equal(s.meta.Pin, expected) {
		return ErrConflict
	}
	n := clock().UnixMilli()
	if !ownedTimestamp(n) || n < s.clockSeen || n < s.meta.ClockMS || n < s.meta.CreatedAtMS {
		return ErrClock
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	s.now = clock
	return nil
}
