// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
)

// This is a separate daily-only accounting decision. dailyDue and its V1
// payment cursor semantics remain unchanged.
func completedDailyRefreshDue(s State, now int64) bool {
	if s.Version != Version2 || s.Config.Version != Version2 || checkedTime(now, s.LastClockMS) != nil {
		return false
	}
	t := time.UnixMilli(now).UTC()
	minute := uint32(t.Hour()*60 + t.Minute())
	return s.DailyCursor == date(now) && minute >= s.Config.DailyUTCMinute
}

func (s *Scheduler) refreshCompletedDaily(ctx context.Context, admittedDay string) (poolsettle.Report, error) {
	r, err := s.store.RefreshCompletedDaily(ctx, admittedDay)
	// Only the exact no-effect rollover sentinel is retryable. A mixed failure
	// must retain its other cause and must not become benign unavailability.
	if err == poolsettle.ErrDailyRefreshExpired {
		err = ErrUnavailable
	}
	return r, err
}
