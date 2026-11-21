// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/poolround"
)

// dailyRoundCall is private to the joined serial round-maintenance loop. Its
// caller supplies the existing single 30-second step context: retries never
// reset that budget. A background private caller is bounded to 30 seconds too.
// Legacy mode invokes the original operation exactly once with its original ctx.
// Each selected call retries only its current Sync, List, Refresh or BeginLive;
// completed earlier phases and their durable observations are not repeated.
func dailyRoundCall[T any](ctx context.Context, daily bool, call func(context.Context) (T, error)) (T, error) {
	if !daily {
		return call(ctx)
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var zero T
	for {
		if err := work.Err(); err != nil {
			return zero, err
		}
		value, err := call(work)
		// All participating concrete owners return this exact sentinel for
		// known gate contention. Do not retry wrappers, mixed failures, node
		// errors, source-unavailable, corruption, poison or uncertain writes.
		if err != poolround.ErrBusy {
			return value, err
		}
		// The original operation released every concrete gate before returning.
		// A short cancellable pause avoids coupling to the public/intake loop's
		// identical maintenance cadence; no owner or lifecycle mutex is held.
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-work.Done():
			timer.Stop()
			return zero, work.Err()
		case <-timer.C:
		}
	}
}
