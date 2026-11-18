//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
)

func careerLaneFixture(t *testing.T) *StableController {
	t.Helper()
	s, _ := resultLaneFixture(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "career-results")
	x, e := gymresult.Create(t.Context(), path, s.resultPin, s.boutStore)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.Close(); e != nil {
		t.Fatal(e)
	}
	x, e = gymresult.ActivateCareer(t.Context(), path, s.resultPin, s.boutStore)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = x.Close() })
	r, e := x.CareerReader()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.BindCareer(t.Context(), r, gymstable.OpenCareer); e != nil {
		t.Fatal(e)
	}
	s.resultStore = x
	return s
}
func TestCareerReadOriginalViewRevocationAndShortDeadline(t *testing.T) {
	s := careerLaneFixture(t)
	original := s.stableConfig.Lease
	jobID := careerPageJob(t, s, false, 0, 1, 8)
	p, e := s.CareerInventory(t.Context(), s.view.ViewID, jobID, 0, 8)
	if e != nil || p.Version != 1 || len(p.Items) != 3 {
		t.Fatal("first career page", len(p.Items), e)
	}
	for _, item := range p.Items {
		if item.Totals.XP != 0 || item.Condition.ProXP != 0 {
			t.Fatal("unawarded fixture received XP")
		}
	}
	if _, e = s.CareerInventory(t.Context(), "", jobID, 0, 8); e == nil {
		t.Fatal("missing original view")
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if p, e = s.CareerInventory(t.Context(), s.view.ViewID, jobID, 0, 8); e == nil || p.Version != 0 {
		t.Fatal("revoked disclosure", p, e)
	}
	s.stableConfig.Lease = original
	for _, roster := range []bool{false, true} {
		jobID = careerPageJob(t, s, roster, 0, 1, 8)
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		want, _ := ctx.Deadline()
		var calls atomic.Int32
		s.stableConfig.Lease = func(work context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			got, ok := work.Deadline()
			if !ok || got.After(want) {
				t.Error("caller deadline widened", got, want)
			}
			if calls.Add(1) == 1 {
				return original(work, cap, fn)
			}
			<-work.Done()
			return work.Err()
		}
		var payload bool
		if roster {
			v, err := s.CareerRoster(ctx, s.view.ViewID, jobID, 1, 8)
			e, payload = err, v.Version != 0
		} else {
			v, err := s.CareerInventory(ctx, s.view.ViewID, jobID, 0, 8)
			e, payload = err, v.Version != 0
		}
		if !errors.Is(e, gymstable.ErrBusy) || payload || ctx.Err() != context.DeadlineExceeded {
			t.Fatal("short deadline", e, payload)
		}
		cancel()
		s.stableConfig.Lease = original
	}
}

func careerPageJob(t *testing.T, s *StableController, roster bool, after uint64, first, limit uint32) string {
	t.Helper()
	var j StableJob
	var e error
	if roster {
		j, e = s.BeginStableRoster(t.Context(), s.view.ViewID, first, limit)
	} else {
		j, e = s.BeginStableInventory(t.Context(), s.view.ViewID, after, limit)
	}
	if e != nil {
		t.Fatal("career page job admission", e)
	}
	id := j.JobID
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if e = s.cycle(t.Context()); e != nil {
			t.Fatal("career page lane", e)
		}
		j, e = s.StableJob(t.Context(), s.view.ViewID, id)
		if e != nil {
			t.Fatal("career page job", e)
		}
		if j.State == "done" {
			return id
		}
		if j.State != "queued" && j.State != "running" {
			t.Fatal("career page failed", j.State, j.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("career page deadline")
	return ""
}

func TestCareerReadExactJobAndTransactionalReservation(t *testing.T) {
	s := careerLaneFixture(t)
	id := careerPageJob(t, s, false, 0, 1, 8)
	first, e := s.CareerInventory(t.Context(), s.view.ViewID, id, 0, 8)
	if e != nil {
		t.Fatal(e)
	}
	handle := s.careerHandle
	if handle == nil || handle.RetainedBytes() > s.careerBytes || s.careerBytes != MaxCareerCacheSize {
		t.Fatal("unreserved handle")
	}
	for _, q := range []struct {
		id    string
		after uint64
	}{{strings.Repeat("e", 64), 0}, {id, 1}} {
		p, e := s.CareerInventory(t.Context(), s.view.ViewID, q.id, q.after, 8)
		if !errors.Is(e, gymstable.ErrNotFound) || p.Version != 0 {
			t.Fatal("unbound page disclosed", e)
		}
	}
	first.Items[0].Condition.RatingHundredths++
	again, e := s.CareerInventory(t.Context(), s.view.ViewID, id, 0, 8)
	if e != nil || again.Items[0].Condition.RatingHundredths == first.Items[0].Condition.RatingHundredths {
		t.Fatal("returned overlay alias", e)
	}
	old := s.previewBytes
	s.previewBytes = MaxManifestCacheBytes
	if _, e = s.BeginStableInventory(t.Context(), s.view.ViewID, 0, 8); !errors.Is(e, gymstable.ErrCapacity) {
		t.Fatal("unreserved admission", e)
	}
	if s.careerHandle != handle || s.careerJobID != id || s.careerBytes != MaxCareerCacheSize {
		t.Fatal("failed admission replaced prior page")
	}
	s.previewBytes = old
	next, e := s.BeginStableInventory(t.Context(), s.view.ViewID, 0, 8)
	if e != nil {
		t.Fatal(e)
	}
	if p, e := s.CareerInventory(t.Context(), s.view.ViewID, id, 0, 8); !errors.Is(e, gymstable.ErrNotFound) || p.Version != 0 {
		t.Fatal("superseded page", e)
	}
	if _, e = s.CancelStableJob(t.Context(), s.view.ViewID, next.JobID); e != nil {
		t.Fatal(e)
	}
	if s.careerHandle != nil || s.careerBytes != 0 {
		t.Fatal("cancelled page retained reservation")
	}
}

func TestCareerReservationConstrainsOlderResultAndLazyAdmissions(t *testing.T) {
	s := careerLaneFixture(t)
	_ = careerPageJob(t, s, false, 0, 1, 8)
	old := s.previewBytes
	// Establish a concrete positive control without Career's additional retained
	// bytes, then exercise the actual older-family admission with that charge.
	for s.previewBytes = old; stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+MaxResultCacheSize+1024); s.previewBytes += 1024 {
	}
	if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+MaxResultCacheSize) {
		t.Fatal("boundary control")
	}
	if _, e := s.BeginResultReconcile(t.Context(), s.view.ViewID, "2026-11-21", strings.Repeat("a", 64)); !errors.Is(e, gymresult.ErrCapacity) {
		t.Fatal("older result ignored Career reservation", e)
	}
	if s.queued != nil {
		t.Fatal("refused result queued")
	}
	s.previewBytes = old
	for s.previewBytes = old; stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes+1024); s.previewBytes += 1024 {
	}
	if s.cacheFits(s.cache, s.receiptSlot) {
		t.Fatal("lazy receipt/portrait ignored Career reservation")
	}
	s.previewBytes = old
}
