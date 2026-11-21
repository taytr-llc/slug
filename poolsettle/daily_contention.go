// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/wire/v2"
)

const dailyContentionLimit = 30 * time.Second
const dailyContentionPause = 10 * time.Millisecond

// One serial invocation owns one budget. Its first contention starts a fixed
// monotonic deadline, shared by ledger admission and every later source read.
// Uncontended audits retain their original caller context and cost. This is a
// bound on added waiting/retry admission, not a replacement audit deadline.
type dailyContention struct {
	deadline time.Time
}

func (w *dailyContention) context(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.deadline.IsZero() {
		w.deadline = time.Now().Add(dailyContentionLimit)
	}
	return context.WithDeadline(ctx, w.deadline)
}

// A mixed structural, cancellation or unavailable error must never become a
// contention retry merely because another leaf is Busy.
func dailyContentionBusy(err error) bool {
	if err == ErrBusy || err == poolround.ErrBusy {
		return true
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		leaves := multi.Unwrap()
		if len(leaves) == 0 {
			return false
		}
		for _, leaf := range leaves {
			if !dailyContentionBusy(leaf) {
				return false
			}
		}
		return true
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		return dailyContentionBusy(single.Unwrap())
	}
	return false
}

func (w *dailyContention) pause(ctx context.Context) error {
	work, cancel := w.context(ctx)
	defer cancel()
	if err := work.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(dailyContentionPause)
	defer timer.Stop()
	select {
	case <-work.Done():
		return work.Err()
	case <-timer.C:
		return work.Err()
	}
}

// Only the selected writable daily binding/mutation paths queue. Existing
// public reads and V1 begin retain their nonblocking gates. A queued sender on
// the original gate can progress when recurring nonblocking readers release it.
func (s *Store) beginDailyContention(ctx context.Context, w *dailyContention) (func(), error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || w == nil {
		return nil, ErrInvalid
	}
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}
	if s.daily == nil || s.auditOnly {
		return nil, ErrConfig
	}
	end, err := s.begin(ctx)
	if err != ErrBusy {
		return end, err
	}
	work, cancel := w.context(ctx)
	defer cancel()
	if err = work.Err(); err != nil {
		return nil, err
	}
	// Never hold lifecycle while waiting: Close must be able to mark the
	// original owner closing and wait for an already admitted operation.
	select {
	case <-work.Done():
		return nil, work.Err()
	case s.gate <- struct{}{}:
	}
	end = func() { <-s.gate }
	s.lifecycle.Lock()
	closing := s.closing
	s.lifecycle.Unlock()
	if closing {
		end()
		return nil, ErrClosed
	}
	if s.poisoned {
		end()
		return nil, ErrPoisoned
	}
	if err = work.Err(); err != nil {
		end()
		return nil, err
	}
	return end, nil
}

// Both operations below are full original-owner reads. A failed attempt owns
// no staged credit or financial effect. Nothing outside these reads is retried.
func (s *Store) auditDailyContention(ctx context.Context, w *dailyContention) (*dailyView, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		view, err := s.auditDaily(ctx)
		if !dailyContentionBusy(err) {
			return view, err
		}
		if err = w.pause(ctx); err != nil {
			return nil, err
		}
	}
}

func (s *Store) dailySourceContention(ctx context.Context, w *dailyContention, snapshot *leveldb.Snapshot, head dailyHeader, importNew bool) ([]Credit, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		credits, err := s.dailySource(ctx, snapshot, head, importNew)
		if !dailyContentionBusy(err) {
			return credits, err
		}
		if err = w.pause(ctx); err != nil {
			return nil, err
		}
	}
}
