// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
)

func checkedTime(now, previous int64) error {
	if now < 1 || now > MaxTimeMS || now < previous {
		return ErrClock
	}
	return nil
}
func nextWake(c Config, now int64) (int64, error) {
	if checkedTime(now, 0) != nil || now > MaxTimeMS-int64(c.RetryMS) {
		return 0, ErrClock
	}
	wake := now + int64(c.RetryMS)
	t := time.UnixMilli(now).UTC()
	due := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(c.DailyUTCMinute) * time.Minute).UnixMilli()
	if due <= now {
		due = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC).Add(time.Duration(c.DailyUTCMinute) * time.Minute).UnixMilli()
	}
	if due > now && due < wake {
		wake = due
	}
	return wake, nil
}
func dailyDue(s State, now int64) bool {
	if checkedTime(now, s.LastClockMS) != nil {
		return false
	}
	t := time.UnixMilli(now).UTC()
	minute := uint32(t.Hour()*60 + t.Minute())
	return minute >= s.Config.DailyUTCMinute && s.DailyCursor < date(now)
}
func admit(s State, kind string, now int64) (State, error) {
	if checkedTime(now, s.LastClockMS) != nil {
		return State{}, ErrClock
	}
	if kind != "recovery" && kind != "daily" {
		return State{}, ErrInvalid
	}
	// Reserve both admission and completion revisions before any store dispatch.
	if s.Revision >= math.MaxUint32-1 {
		return State{}, ErrCapacity
	}
	seq := uint32(1)
	if s.Attempt != nil {
		if s.Attempt.Sequence == math.MaxUint32 {
			return State{}, ErrCapacity
		}
		seq = s.Attempt.Sequence + 1
	}
	wake, e := nextWake(s.Config, now)
	if e != nil {
		return State{}, e
	}
	s = cloneState(s)
	s.Revision++
	s.LastClockMS = now
	s.NextWakeMS = wake
	s.Attempt = &Attempt{Sequence: seq, Kind: kind, AdmittedDay: date(now), StartedAtMS: now, Result: "running"}
	return s, nil
}
func errorCode(e error) string {
	switch {
	case errors.Is(e, ErrPoisoned), errors.Is(e, poolsettle.ErrPoisoned), errors.Is(e, ErrStorage), errors.Is(e, poolsettle.ErrCorrupt):
		return "storage"
	case errors.Is(e, ErrClock), errors.Is(e, poolsettle.ErrClock):
		return "clock"
	case errors.Is(e, ErrClosed), errors.Is(e, poolsettle.ErrClosed):
		return "closed"
	case errors.Is(e, ErrProfile), errors.Is(e, poolsettle.ErrProfile):
		return "profile"
	case errors.Is(e, ErrInvalid), errors.Is(e, ErrCapacity), errors.Is(e, poolsettle.ErrInvalid), errors.Is(e, poolsettle.ErrConfig), errors.Is(e, poolsettle.ErrCapacity):
		return "invalid"
	case errors.Is(e, context.Canceled):
		return "cancelled"
	case errors.Is(e, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(e, ErrBusy), errors.Is(e, poolsettle.ErrBusy):
		return "busy"
	case errors.Is(e, ErrUnavailable), errors.Is(e, poolsettle.ErrSource), errors.Is(e, poolsettle.ErrBackend):
		return "unavailable"
	default:
		return "other"
	}
}
func retryable(e error) bool {
	c := errorCode(e)
	return c == "busy" || c == "unavailable" || c == "deadline"
}
func finish(s State, r poolsettle.Report, callErr error, now int64) (State, error) {
	if s.Attempt == nil || s.Attempt.Result != "running" {
		return State{}, ErrInvalid
	}
	if checkedTime(now, s.LastClockMS) != nil {
		return State{}, ErrClock
	}
	if s.Revision == math.MaxUint32 {
		return State{}, ErrCapacity
	}
	wake, e := nextWake(s.Config, now)
	if e != nil {
		return State{}, e
	}
	s = cloneState(s)
	s.Revision++
	s.LastClockMS = now
	s.NextWakeMS = wake
	s.Attempt.FinishedAtMS = now
	if callErr != nil {
		s.Attempt.Result = "failed"
		s.Attempt.ErrorCode = errorCode(callErr)
		return s, nil
	}
	if !validDay(r.Day) || !validReport(r.State, r.Reason) {
		return State{}, ErrInvalid
	}
	// A preparation crossing midnight retains its actual store-assigned day. A
	// future report cannot advance this scheduler's cursor or claim completion.
	if s.Attempt.Kind == "daily" && r.Day > date(now) {
		return State{}, ErrClock
	}
	s.Attempt.Result = "returned"
	s.Attempt.ReportState = r.State
	s.Attempt.ReportReason = r.Reason
	if s.Attempt.Kind == "daily" && r.State == "complete" && r.Reason == "daily-batch-complete" && r.Day > s.DailyCursor {
		s.DailyCursor = r.Day
	}
	return s, nil
}
