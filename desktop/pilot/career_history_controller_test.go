//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func historyLaneFixture(t *testing.T) (*StableController, string) {
	t.Helper()
	s := careerLaneFixture(t)
	p, e := s.stableStore.Inventory(t.Context(), s.session(), 0, 1)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e)
	}
	return s, p.Items[0].Assignment.ProspectID
}
func historyDone(t *testing.T, s *StableController, pid string) CareerHistoryJob {
	t.Helper()
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	j, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || j.State != "done" || j.Capture == nil {
		t.Fatalf("done %s %s %v", j.State, j.Error, e)
	}
	return j
}
func TestCareerHistoryCaptureReplayAndStop(t *testing.T) {
	s, pid := historyLaneFixture(t)
	stages := map[string]int{}
	s.historyCheckpoint = func(x string) { stages[x]++ }
	before := *s.stableStatus
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	task := s.queued
	if task == nil || task.ctx.Err() != nil {
		t.Fatal("metadata context cancelled admitted task")
	}
	deadline, _ := task.ctx.Deadline()
	if time.Until(deadline) < 20*time.Second {
		t.Fatal("task inherited metadata budget")
	}
	if _, e = s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("second capture queued", e)
	}
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	j, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || j.State != "done" || j.Capture == nil || j.Capture.EventCount != 0 {
		t.Fatal(j.State, j.Error, e)
	}
	if *s.stableStatus != before || s.historyBytes != careerHistoryAllowance || s.historyHandle == nil || s.historyHandle.RetainedBytes() > careerHistoryCaptureAllowance {
		t.Fatal("capture/cache ownership")
	}
	b, e := MarshalCareerHistoryResponse(CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &j})
	if e != nil {
		t.Fatal(e)
	}
	j.Capture.Status.Slots[0] = strings.Repeat("f", 64)
	j.Capture.Totals.XP = 15
	for i := 0; i < 3; i++ {
		again, e := s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID)
		if e != nil {
			t.Fatal(e)
		}
		bb, _ := MarshalCareerHistoryResponse(CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &again})
		if !bytes.Equal(bb, b) {
			t.Fatal("caller changed retained terminal")
		}
		p, e := s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8)
		if e != nil || p.Page.Items == nil || len(p.Page.Items) != 0 || p.Page.HasMore || p.Page.Next != 0 {
			t.Fatal("empty page", e)
		}
		p.Page.Capture.Status.Slots[0] = "mutated"
		snap, e := s.CareerHistorySnapshot(t.Context())
		if e != nil || snap.State != "ready" || snap.LastJobID != j.JobID {
			t.Fatal(snap, e)
		}
	}
	if stages["before-capture"] != 1 || stages["captured"] != 1 || stages["before-publication"] != 1 {
		t.Fatal("metadata repeated staged capture", stages)
	}
	if _, e = s.StableJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("foreign family", e)
	}
	s.Stop()
	if s.historyHandle != nil || s.historyLast != nil || s.historyBytes != 0 {
		t.Fatal("Stop retained owner token")
	}
	if _, e = s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("Stop page", e)
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("Stop old job", e)
	}
}
func TestCareerHistoryCancellationAndSharedLane(t *testing.T) {
	s, pid := historyLaneFixture(t)
	old := historyDone(t, s, pid)
	s.mu.Lock()
	charge := s.historyBytes
	s.resultBytes = MaxManifestCacheBytes
	s.mu.Unlock()
	if _, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, gymstable.ErrCapacity) {
		t.Fatal(e)
	}
	if s.historyLast.JobID != old.JobID || s.historyBytes != charge {
		t.Fatal("refusal changed retained selection")
	}
	s.mu.Lock()
	s.resultBytes = MaxResultMetadataSize
	s.mu.Unlock()
	j, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginStableSync(t.Context(), s.view.ViewID); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal(e)
	}
	cancelled, e := s.CancelCareerHistory(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || cancelled.State != "cancelled" || s.historyHandle != nil {
		t.Fatal(cancelled, e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	s.historyCheckpoint = func(stage string) {
		if stage == "captured" {
			close(entered)
			<-release
		}
	}
	j, e = s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.cycle(t.Context()) }()
	<-entered
	if _, e = s.BeginCareerHistory(t.Context(), s.view.ViewID, pid); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("active history admitted another", e)
	}
	running, e := s.CancelCareerHistory(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || running.State != "running" {
		t.Fatal("cancel falsely claimed join", running, e)
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	j, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || j.State != "cancelled" || j.Capture != nil || s.historyHandle != nil {
		t.Fatal(j, e)
	}
	// Other eligible family work can own the active lane while one History task queues.
	c, cancel := context.WithCancel(t.Context())
	s.stableMu.Lock()
	s.active = &stableTask{ctx: c, cancel: cancel, result: StableJob{Kind: "inventory"}}
	s.stableMu.Unlock()
	j, e = s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil || j.State != "queued" {
		t.Fatal(e)
	}
	s.stableMu.Lock()
	s.active = nil
	s.stableMu.Unlock()
	cancel()
	_, _ = s.CancelCareerHistory(t.Context(), s.view.ViewID, j.JobID)
}
func TestCareerHistoryUnpublishedRollbackCannotClobberSuccessor(t *testing.T) {
	s, pid := historyLaneFixture(t)
	old := historyDone(t, s, pid)
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		e := original(ctx, cap, fn)
		if once.CompareAndSwap(false, true) {
			close(entered)
			<-release
			return context.Canceled
		}
		return e
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, e := s.BeginCareerHistory(ctx, s.view.ViewID, pid); result <- e }()
	<-entered
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.historyLast.JobID != old.JobID || s.queued == nil || s.queued.published {
		t.Fatal("unpublished reservation visible/runnable")
	}
	cancel()
	snap, e := s.CareerHistorySnapshot(t.Context())
	if e != nil || snap.LastJobID != old.JobID {
		t.Fatal("expired reservation lost prior selection", e)
	}
	next, e := s.BeginCareerHistory(t.Context(), s.view.ViewID, pid)
	if e != nil {
		t.Fatal(e)
	}
	reserved := s.queued
	close(release)
	if e = <-result; e == nil {
		t.Fatal("failed admission acknowledged")
	}
	if s.queued != reserved || s.queued.history.JobID != next.JobID || s.historyBytes != careerHistoryAllowance {
		t.Fatal("late rollback clobbered successor")
	}
	s.stableConfig.Lease = original
	_, _ = s.CancelCareerHistory(t.Context(), s.view.ViewID, next.JobID)
}
func TestCareerHistoryOriginalViewDeadlineFatalAndUnknown(t *testing.T) {
	s, pid := historyLaneFixture(t)
	j := historyDone(t, s, pid)
	original := s.stableConfig.Lease
	for _, op := range []string{"snapshot", "job", "page", "begin"} {
		t.Run(op, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
			defer cancel()
			want, _ := ctx.Deadline()
			s.stableConfig.Lease = func(ctx context.Context, _ gymaccount.Capability, _ func(gymaccount.Account, func() error) error) error {
				got, _ := ctx.Deadline()
				if got.After(want) {
					t.Error("earlier deadline widened")
				}
				<-ctx.Done()
				return ctx.Err()
			}
			defer func() { s.stableConfig.Lease = original }()
			var e error
			switch op {
			case "snapshot":
				_, e = s.CareerHistorySnapshot(ctx)
			case "job":
				_, e = s.CareerHistoryJob(ctx, s.view.ViewID, j.JobID)
			case "page":
				_, e = s.CareerHistoryPage(ctx, s.view.ViewID, j.JobID, 0, 8)
			case "begin":
				_, e = s.BeginCareerHistory(ctx, s.view.ViewID, pid)
			}
			if !errors.Is(e, gymstable.ErrBusy) {
				t.Fatal("metadata deadline", e)
			}
		})
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if x, e := s.CareerHistoryPage(t.Context(), s.view.ViewID, j.JobID, 0, 8); e == nil || x.JobID != "" {
		t.Fatal("revoked disclosure", e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return errors.Join(context.DeadlineExceeded, gymstable.ErrOutcomeUnknown)
	}
	if _, e := s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, gymstable.ErrOutcomeUnknown) || careerHistoryProtocolError(e) != "history_observation_unknown" {
		t.Fatal("unknown downgraded", e)
	}
	s.stableConfig.Lease = original
	if e := s.resultStore.Close(); e != nil {
		t.Fatal(e)
	}
	snap, e := s.CareerHistorySnapshot(t.Context())
	if e != nil || snap.State != "unavailable" || snap.Error != "history_unavailable" {
		t.Fatal(snap, e)
	}
	if s.historyHandle != nil || s.historyLast != nil || s.historyBytes != 0 {
		t.Fatal("learned owner loss retained history")
	}
	if _, e = s.CareerHistoryJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(e, careerstate.ErrClosed) {
		t.Fatal("learned owner failure repaired", e)
	}
}
func TestCareerHistoryAllLazyCacheAdmissionsIncludeHistory(t *testing.T) {
	s, pid := historyLaneFixture(t)
	_ = historyDone(t, s, pid)
	s.mu.Lock()
	s.previewBytes = 176 * 1024
	s.mu.Unlock()
	// Every call below is a different family reservation, not a forged cache verdict.
	for _, f := range []func() error{
		func() error {
			_, e := s.BeginFightPreview(t.Context(), s.view.ViewID, strings.Repeat("1", 64), strings.Repeat("2", 64))
			return e
		},
		func() error { _, e := s.BeginStableInventory(t.Context(), s.view.ViewID, 0, 8); return e },
	} {
		if e := f(); !errors.Is(e, gymstable.ErrCapacity) {
			t.Fatal("lazy admission omitted retained history", e)
		}
	}
	b, _ := json.Marshal(s.historyLast)
	if len(b) > MaxCareerHistoryJobSize {
		t.Fatal("terminal bound")
	}
}
