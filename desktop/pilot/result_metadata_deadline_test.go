//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymresult"
)

func TestResultMetadataPreservesShorterCallerDeadline(t *testing.T) {
	s, event := resultLaneFixture(t)
	original := s.stableConfig.Lease
	for _, command := range []string{"status", "record"} {
		t.Run(command, func(t *testing.T) {
			defer func() { s.stableConfig.Lease = original }()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			deadline, _ := ctx.Deadline()
			var calls atomic.Int32
			observed := make(chan time.Time, 1)
			s.stableConfig.Lease = func(work context.Context, capability gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				if calls.Add(1) == 1 {
					d, _ := work.Deadline()
					observed <- d
					return original(work, capability, fn)
				}
				// If preparation finishes within the caller's short deadline,
				// block the source authorization until cancellation. A deadline
				// reached earlier in preparation must also refuse disclosure.
				<-work.Done()
				return work.Err()
			}
			start := time.Now()
			var err error
			payload := false
			switch command {
			case "status":
				value, e := s.ResultStatus(ctx, s.view.ViewID, event.Saturday, strings.Repeat("a", 64))
				err, payload = e, value.Version != 0
			case "record":
				value, e := s.ResultRecord(ctx, s.view.ViewID, event.Saturday, strings.Repeat("a", 64))
				err, payload = e, value.Version != 0
			}
			if !errors.Is(err, gymresult.ErrBusy) || payload || ctx.Err() != context.DeadlineExceeded {
				t.Fatal("short caller deadline disclosed a result", err, payload)
			}
			select {
			case d := <-observed:
				if d.IsZero() || d.After(deadline) {
					t.Fatal("admitted deadline exceeded caller deadline", d, deadline)
				}
			default:
				t.Fatal("initial authorization was not reached")
			}
			if time.Since(start) >= resultMetadataTimeout/2 {
				t.Fatal("controlled cancellation waited for the family budget")
			}
		})
	}
}
