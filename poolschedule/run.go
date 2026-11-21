// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

// Run starts with recovery even when a persisted timer or daily cursor is
// ahead. It never owns the borrowed settlement lifetime and never replaces it.
// A cancelled call must return before another Run can start on this owner.
func (s *Scheduler) Run(ctx context.Context) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	if s.closing || s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.running {
		s.mu.Unlock()
		return ErrBusy
	}
	if e := s.auditLocked(); e != nil {
		s.mu.Unlock()
		return e
	}
	work, cancel := context.WithCancel(ctx)
	s.runCancel = cancel
	s.runDone = make(chan struct{})
	done := s.runDone
	s.running = true
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.running = false
		s.runCancel = nil
		close(done)
		s.mu.Unlock()
	}()
	for {
		if e := work.Err(); e != nil {
			return e
		}
		e := s.cycle(work)
		if ce := work.Err(); ce != nil {
			return errors.Join(e, ce)
		}
		if e != nil && !retryable(e) {
			return e
		}
		s.mu.Lock()
		if s.closing || s.closed {
			s.mu.Unlock()
			return ErrClosed
		}
		if s.poisoned {
			s.mu.Unlock()
			return ErrPoisoned
		}
		now := time.Now().UTC().UnixMilli()
		wake, we := nextWake(s.state.Config, now)
		if checkedTime(now, s.state.LastClockMS) != nil {
			we = ErrClock
		}
		// Successful dispatch already persisted its next wake. A pre-dispatch Busy
		// has no attempt to complete; its retry still uses the same immutable bound.
		if e == nil && s.state.NextWakeMS > now {
			wake = s.state.NextWakeMS
		}
		s.mu.Unlock()
		if we != nil {
			return we
		}
		if e = waitUntil(work, wake, now); e != nil {
			return e
		}
	}
}
func waitUntil(ctx context.Context, wake, previous int64) error {
	for {
		now := time.Now().UTC().UnixMilli()
		if e := checkedTime(now, previous); e != nil {
			return e
		}
		previous = now
		if now >= wake {
			return nil
		}
		d := time.Hour
		// Valid timestamps can exceed time.Duration; wake is always within the
		// immutable one-hour retry bound in live use, and this protects pure callers.
		if wake-now <= 3600000 {
			d = time.Duration(wake-now) * time.Millisecond
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (s *Scheduler) cycle(ctx context.Context) error {
	r, e := s.dispatch(ctx, "recovery")
	if e != nil {
		return e
	}
	if r.State != "complete" || r.Reason != "" {
		return nil
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.mu.Lock()
	now := time.Now().UTC().UnixMilli()
	e = checkedTime(now, s.state.LastClockMS)
	due := dailyDue(s.state, now)
	s.mu.Unlock()
	if e != nil {
		return e
	}
	if !due {
		return nil
	}
	_, e = s.dispatch(ctx, "daily")
	return e
}
func (s *Scheduler) dispatch(ctx context.Context, kind string) (poolsettle.Report, error) {
	if e := ctx.Err(); e != nil {
		return poolsettle.Report{}, e
	}
	binding, e := s.settlementBinding(ctx)
	if e != nil {
		return poolsettle.Report{}, e
	}
	s.mu.Lock()
	if s.closing || s.closed || !s.running {
		s.mu.Unlock()
		return poolsettle.Report{}, ErrClosed
	}
	if binding != s.state.Config.SettlementBindingID {
		s.mu.Unlock()
		return poolsettle.Report{}, ErrInvalid
	}
	now := time.Now().UTC().UnixMilli()
	if kind == "daily" && !dailyDue(s.state, now) {
		s.mu.Unlock()
		return poolsettle.Report{}, ErrClock
	}
	next, e := admit(s.state, kind, now)
	if e == nil {
		e = s.writeLocked(next)
	}
	s.mu.Unlock()
	if e != nil {
		return poolsettle.Report{}, e
	}
	if e = ctx.Err(); e != nil {
		return poolsettle.Report{}, e
	} // durable unknown attempt; next start reconciles
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	var report poolsettle.Report
	if kind == "recovery" {
		report, e = s.store.RecoverPending(call)
	} else {
		report, e = s.store.RunDaily(call)
	}
	cancel()
	s.mu.Lock()
	now = time.Now().UTC().UnixMilli()
	next, finishErr := finish(s.state, report, e, now)
	if finishErr == nil {
		finishErr = s.writeLocked(next)
	}
	s.mu.Unlock()
	if finishErr != nil {
		return poolsettle.Report{}, errors.Join(e, finishErr)
	}
	if ce := ctx.Err(); ce != nil {
		return poolsettle.Report{}, errors.Join(e, ce)
	}
	return report, e
}
