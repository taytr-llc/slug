//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func TestCareerHistoryFatalActiveReservationLivesUntilJoin(t *testing.T) {
	s, pid := historyLaneFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	s.historyCheckpoint = func(stage string) {
		if stage == "captured" {
			close(entered)
			<-release
		}
	}
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.cycle(t.Context()) }()
	<-entered
	if e = s.resultStore.Close(); e != nil {
		t.Fatal(e)
	}
	snap, e := s.CareerHistorySnapshot(t.Context())
	if e != nil || snap.State != "unavailable" {
		t.Fatal(snap, e)
	}
	s.stableMu.Lock()
	active := s.active
	s.stableMu.Unlock()
	s.mu.Lock()
	charge := s.historyBytes
	oldResult := s.resultBytes
	s.resultBytes = 400 * 1024
	s.mu.Unlock()
	if active == nil || charge != careerHistoryAllowance {
		t.Fatal("cancel released active capture reservation", charge)
	}
	if _, e = s.BeginFightPreview(t.Context(), s.view.ViewID, strings.Repeat("1", 64), strings.Repeat("2", 64)); !errors.Is(e, gymstable.ErrCapacity) {
		t.Fatal("other family ignored retained active history", e)
	}
	s.mu.Lock()
	s.resultBytes = oldResult
	s.mu.Unlock()
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.historyBytes != 0 || s.historyHandle != nil || s.historyLast != nil {
		t.Fatal("joined fatal task leaked ownership")
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); e == nil {
		t.Fatal("fatal owner repaired")
	}
}
func TestCareerHistoryFatalStableFailuresLatchWhileReaderHealthy(t *testing.T) {
	for _, stage := range []string{"before-capture", "captured"} {
		t.Run(stage, func(t *testing.T) {
			s, pid := historyLaneFixture(t)
			s.historyCheckpoint = func(got string) {
				if got == stage {
					if e := s.stableStore.Close(); e != nil {
						t.Error(e)
					}
				}
			}
			j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.cycle(t.Context()); e != nil {
				t.Fatal(e)
			}
			if _, e = s.careerReader.Activation(); e != nil {
				t.Fatal("test lost separate Result reader", e)
			}
			if !errors.Is(s.historyFailure, gymstable.ErrClosed) || s.historyHandle != nil || s.historyLast != nil || s.historyBytes != 0 {
				t.Fatal("Stable fatal did not latch", s.historyFailure, s.historyBytes)
			}
			if _, e = s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, gymstable.ErrClosed) {
				t.Fatal("new Begin repaired closed Stable", e)
			}
			if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrClosed) {
				t.Fatal("fatal terminal disclosed", e)
			}
		})
	}
}
func TestCareerHistoryKnownUnknownSurvivesReaderCloseAndBusy(t *testing.T) {
	s, pid := historyLaneFixture(t)
	original := s.stableConfig.Lease
	failed := false
	injected := false
	// This private account-boundary test injects a known observation-unknown
	// result; the core suite independently exercises the real Sync failure.
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if injected {
			if e := s.resultStore.Close(); e != nil {
				t.Error(e)
			}
		}
		return original(ctx, gymaccount.View, fn)
	}
	s.historyCheckpoint = func(stage string) {
		if stage == "before-capture" {
			failed = true
		}
	}
	s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if failed {
			injected = true
			return original(ctx, c, func(a gymaccount.Account, _ func() error) error {
				return fn(a, func() error { return gymstable.ErrOutcomeUnknown })
			})
		}
		return original(ctx, c, fn)
	}
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(s.historyFailure, gymstable.ErrOutcomeUnknown) || s.historyHandle != nil || s.historyBytes != 0 {
		t.Fatal("reader close replaced known uncertainty", s.historyFailure)
	}
	s.setStableFailure(gymstable.ErrClosed)
	s.laneGate <- struct{}{}
	_, e = s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8)
	<-s.laneGate
	if !errors.Is(e, gymstable.ErrOutcomeUnknown) || careerHistoryProtocolError(e) != "history_observation_unknown" {
		t.Fatal("busy/Close replaced uncertainty", e)
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrOutcomeUnknown) {
		t.Fatal(e)
	}
}
func TestCareerHistoryRefusedTerminalReleasesJoinedReservation(t *testing.T) {
	s, pid := historyLaneFixture(t)
	original := s.stableConfig.Lease
	refuse := false
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if refuse {
			return gymaccount.ErrUnauthorized
		}
		return original(ctx, gymaccount.View, fn)
	}
	s.historyCheckpoint = func(stage string) {
		if stage == "before-publication" {
			refuse = true
		}
	}
	if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); e != nil {
		t.Fatal(e)
	}
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.historyBytes != 0 || s.historyHandle != nil || s.historyLast != nil {
		t.Fatal("unpublished joined task leaked reservation")
	}
}

func TestCareerHistoryStopJoinsWithoutLatchingHealthyOwner(t *testing.T) {
	s, pid := historyLaneFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	s.historyCheckpoint = func(stage string) {
		if stage == "captured" {
			close(entered)
			<-release
		}
	}
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	task := s.queued
	cycleDone := make(chan error, 1)
	go func() { cycleDone <- s.cycle(t.Context()) }()
	<-entered
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	<-task.ctx.Done()
	s.mu.Lock()
	charge := s.historyBytes
	s.mu.Unlock()
	if charge != careerHistoryAllowance {
		t.Fatal("Stop released unjoined capture", charge)
	}
	close(release)
	if e = <-cycleDone; e != nil {
		t.Fatal(e)
	}
	<-stopped
	if s.historyFailure != nil || s.historyBytes != 0 {
		t.Fatal("normal Stop poisoned healthy family", s.historyFailure, s.historyBytes)
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("Stop resurrected job", e)
	}
	s.historyCheckpoint = nil
	historyDone(t, s, pid)
}

func TestCareerHistoryKnownUnknownSurvivesStopWithoutSelection(t *testing.T) {
	s, pid := historyLaneFixture(t)
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	inject := false
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return original(ctx, gymaccount.View, fn)
	}
	s.historyCheckpoint = func(stage string) {
		if stage == "before-capture" {
			inject = true
		}
	}
	s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if inject {
			return original(ctx, c, func(a gymaccount.Account, _ func() error) error {
				return fn(a, func() error {
					close(entered)
					<-release
					return gymstable.ErrOutcomeUnknown
				})
			})
		}
		return original(ctx, c, fn)
	}
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	task := s.queued
	done := make(chan error, 1)
	go func() { done <- s.cycle(t.Context()) }()
	<-entered
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	<-task.ctx.Done()
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	<-stopped
	if !errors.Is(s.historyFailure, gymstable.ErrOutcomeUnknown) || s.historyLast != nil || s.historyHandle != nil || s.historyBytes != 0 {
		t.Fatal("Stop discarded known uncertainty or retained selection", s.historyFailure, s.historyBytes)
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrOutcomeUnknown) {
		t.Fatal(e)
	}
}
func TestCareerHistoryPageUnknownClearsHandleAndRemainsLatched(t *testing.T) {
	s, pid := historyLaneFixture(t)
	j := historyDone(t, s, pid)
	original := s.stableConfig.Lease
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return original(ctx, gymaccount.View, fn)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymstable.ErrOutcomeUnknown
	}
	if p, e := s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8); !errors.Is(e, gymstable.ErrOutcomeUnknown) || p.JobID != "" {
		t.Fatal("page uncertainty", e)
	}
	if !errors.Is(s.historyFailure, gymstable.ErrOutcomeUnknown) || s.historyHandle != nil || s.historyLast != nil || s.historyBytes != 0 {
		t.Fatal("uncertain page retained/repairable")
	}
	s.setStableFailure(gymstable.ErrPoisoned)
	if _, e := s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrOutcomeUnknown) {
		t.Fatal("later poison replaced uncertainty", e)
	}
}

func TestCareerHistoryKnownClockAndUnfencedLatch(t *testing.T) {
	for _, fault := range []error{gymstable.ErrClock, careerstate.ErrUnfenced} {
		t.Run(fault.Error(), func(t *testing.T) {
			s, pid := historyLaneFixture(t)
			original := s.stableConfig.Lease
			inject := false
			s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
				return original(ctx, gymaccount.View, fn)
			}
			s.historyCheckpoint = func(stage string) {
				if stage == "before-capture" {
					inject = true
				}
			}
			s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				if inject {
					return fault
				}
				return original(ctx, c, fn)
			}
			if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); e != nil {
				t.Fatal(e)
			}
			if e := s.cycle(t.Context()); e != nil {
				t.Fatal(e)
			}
			s.stableConfig.Lease = original
			if _, e := s.careerReader.Activation(); e != nil {
				t.Fatal(e)
			}
			snap, e := s.CareerHistorySnapshot(t.Context())
			if e != nil || snap.State != "unavailable" || snap.Error != careerHistoryProtocolError(fault) {
				t.Fatal(snap, e)
			}
			if !errors.Is(s.historyFailure, fault) || s.historyBytes != 0 || s.historyLast != nil || s.historyHandle != nil {
				t.Fatal("known fatal category repaired", s.historyFailure)
			}
			if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, fault) {
				t.Fatal("healthy later lease repaired fatal family", e)
			}
		})
	}
}
func TestCareerHistoryInternalPageMismatchLatchesCorruption(t *testing.T) {
	s, pid := historyLaneFixture(t)
	j := historyDone(t, s, pid)
	s.stableMu.Lock()
	s.historyLast.Capture.Totals.Wins++
	s.stableMu.Unlock()
	if p, e := s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8); !errors.Is(e, gymstable.ErrCorrupt) || p.JobID != "" {
		t.Fatal("mismatched capture disclosed", e)
	}
	if !errors.Is(s.historyFailure, gymstable.ErrCorrupt) || s.historyHandle != nil || s.historyLast != nil || s.historyBytes != 0 {
		t.Fatal("corruption not latched", s.historyFailure)
	}
}
