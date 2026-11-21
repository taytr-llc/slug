// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

// ErrDailyRefreshExpired means the original sampled ledger day no longer
// matches the caller's admitted refresh day. No financial effect was made.
var ErrDailyRefreshExpired = errors.New("pool settlement completed-day refresh expired")

// RefreshCompletedDaily refreshes accounting for an already completed daily
// payment slot. It audits the original source and signed history, imports new
// confirmations atomically and inspects the entire current funding frontier.
// It never quotes, signs, broadcasts or occupies a new payment day. Pending
// delivery must be recovered separately before calling this method.
func (s *Store) RefreshCompletedDaily(ctx context.Context, expectedDay string) (Report, error) {
	if wire.TestPoW {
		return Report{}, ErrProfile
	}
	day, err := time.Parse("2006-01-02", expectedDay)
	if ctx == nil || err != nil || day.Format("2006-01-02") != expectedDay {
		return Report{}, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return Report{}, err
	}
	if s == nil || s.db == nil {
		return Report{}, ErrClosed
	}
	if s.daily == nil || s.state.Config.Version != Version2 || s.auditOnly || s.backend == nil {
		return Report{}, ErrInvalid
	}
	return s.runDailySettlementSelected(ctx, false, expectedDay)
}
