//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"sync/atomic"
	"testing"
)

func TestCareerHistoryPublicationInsideLeaseAndImmutableAfterCancel(t *testing.T) {
	s, pid := historyLaneFixture(t)
	original := s.stableConfig.Lease
	var inside atomic.Bool
	var publications atomic.Int32
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.stableConfig.HistoryPublicationLease = func(work context.Context, fn func(gymaccount.Account, func() error) error) error {
		return original(work, gymaccount.View, func(a gymaccount.Account, guard func() error) error {
			inside.Store(true)
			defer inside.Store(false)
			e := fn(a, guard)
			if publications.Load() == 2 {
				cancel()
			}
			return e
		})
	}
	s.historyCheckpoint = func(stage string) {
		if stage == "before-admission-publication" || stage == "terminal-publication" {
			if !inside.Load() {
				t.Error("publication escaped original lease", stage)
			}
			publications.Add(1)
		}
	}
	j, e := s.BeginCareerHistory(ctx, s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if ctx.Err() != context.Canceled || publications.Load() != 2 {
		t.Fatal("missing post-terminal cancellation")
	}
	j, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || j.State != "done" || j.Capture == nil {
		t.Fatal("successful terminal retroactively cancelled", j.State, e)
	}
}
func TestCareerHistoryFinalAdmissionGuardPreservesPrior(t *testing.T) {
	s, pid := historyLaneFixture(t)
	old := historyDone(t, s, pid)
	original := s.stableConfig.Lease
	var revoked atomic.Bool
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return original(ctx, gymaccount.View, func(a gymaccount.Account, check func() error) error {
			return fn(a, func() error {
				if revoked.Load() {
					return gymaccount.ErrUnauthorized
				}
				return check()
			})
		})
	}
	s.historyCheckpoint = func(stage string) {
		if stage == "before-admission-publication" {
			revoked.Store(true)
		}
	}
	if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("final guard bypass", e)
	}
	if s.historyLast == nil || s.historyLast.JobID != old.JobID || s.historyHandle == nil || s.queued != nil {
		t.Fatal("refused final admission lost old capture")
	}
}
func TestCareerHistoryAdmittedLostAckDoesNotRetract(t *testing.T) {
	s, pid := historyLaneFixture(t)
	original := s.stableConfig.Lease
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		e := original(ctx, gymaccount.View, fn)
		if e != nil {
			return e
		}
		if s.queued != nil && s.queued.published {
			return context.Canceled
		}
		return nil
	}
	if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, errHistoryReplyUnknown) {
		t.Fatal("lost publication ACK certainty", e)
	}
	if s.queued == nil || !s.queued.published {
		t.Fatal("admitted work retracted")
	}
	id := s.queued.history.JobID
	s.stableConfig.HistoryPublicationLease = nil
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	j, e := s.CareerHistoryJob(t.Context(), s.view.ViewID, id)
	if e != nil || j.State != "done" {
		t.Fatal(j.State, e)
	}
}
