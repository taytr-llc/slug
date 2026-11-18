//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Both fixtures use the same fixed candidate StoreIDs. The real sealed reader
// comes from an activated concrete Result owner, never a caller-supplied Pair or
// XP value. The extra normal-proof admissions are modeled test setup; no new
// engine output becomes a golden and this is not a live admission/award proof.
func careerPreviewLaneFixture(t *testing.T) (*StableController, [2]string) {
	t.Helper()
	s, ids := previewLanePairFixture(t)
	owner := careerLaneFixture(t)
	if s.stableStatus.StoreID != owner.stableStatus.StoreID {
		t.Fatal("fixture scope differs")
	}
	if e := s.BindCareer(t.Context(), owner.careerReader, gymstable.OpenCareer); e != nil {
		t.Fatal(e)
	}
	s.resultStore = owner.resultStore
	return s, ids
}
func TestCareerPreviewTrustedSuccessReplayOwnershipAndOriginalView(t *testing.T) {
	s, ids := careerPreviewLaneFixture(t)
	before := *s.stableStatus
	stages := map[string]int{}
	s.careerPreviewCheckpoint = func(stage string) { stages[stage]++ }
	ack, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, ids[1], ids[0])
	if e != nil {
		t.Fatal(e)
	}
	if s.previewLast != nil || s.previewBytes != previewAllowance {
		t.Fatal("shared reservation")
	}
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	done, e := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, ack.JobID)
	if e != nil || done.State != "done" || done.Result == nil {
		t.Fatalf("done %s %s %v", done.State, done.Error, e)
	}
	if !careerPreviewJobValid(done) {
		t.Fatal("invalid completed result")
	}
	for _, f := range done.Result.Pair.Fighters {
		if f.Totals.XP != 0 || f.Condition.ProXP != 0 {
			t.Fatal("unawarded trusted fixture")
		}
	}
	if *s.stableStatus != before {
		t.Fatal("captured Status overwrote shared status")
	}
	for _, k := range []string{"pair-read", "before-engine", "before-validation", "before-token-read", "before-publication"} {
		if stages[k] != 1 {
			t.Fatal(k, stages)
		}
	}
	raw, e := MarshalCareerPreviewResponse(CareerPreviewResponse{ID: 1, Version: 1, OK: true, Job: &done})
	if e != nil {
		t.Fatal(e)
	}
	if len(raw) >= MaxResponseSize || s.previewBytes >= previewAllowance || s.previewLast != nil {
		t.Fatal("terminal accounting")
	}
	done.Result.Pair.Fighters[0].Totals.XP = 15
	done.Result.EngineResult.Rounds[0].Ticks[0].Strike = "mutated"
	for n := 0; n < 3; n++ {
		again, e := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, ack.JobID)
		if e != nil {
			t.Fatal(e)
		}
		b, e := MarshalCareerPreviewResponse(CareerPreviewResponse{ID: 1, Version: 1, OK: true, Job: &again})
		if e != nil || string(b) != string(raw) {
			t.Fatal("replay changed", e)
		}
		_ = s.CareerPreviewSnapshot()
	}
	// These are stage-hook assertions only, not actual Simulate-entry counts.
	if stages["pair-read"] != 1 || stages["before-engine"] != 1 {
		t.Fatal("poll caused staged work")
	}
	if _, e = s.FightPreviewJob(t.Context(), s.view.ViewID, ack.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("cross-family lookup", e)
	}
	original := s.stableConfig.Lease
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if x, e := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, ack.JobID); e == nil || x.Version != 0 || s.CareerPreviewSnapshot().View != nil {
		t.Fatal("revoked disclosure", e)
	}
	s.stableConfig.Lease = original
}
func TestCareerPreviewSharedActiveExclusionAndCancellation(t *testing.T) {
	for _, career := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "career"}[career], func(t *testing.T) {
			s, ids := careerPreviewLaneFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var engineStage atomic.Int32
			hook := func(stage string) {
				if stage == "pair-read" {
					close(entered)
					<-release
				}
				if stage == "before-engine" {
					engineStage.Add(1)
				}
			}
			var id string
			if career {
				s.careerPreviewCheckpoint = hook
				j, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, ids[0], ids[1])
				if e != nil {
					t.Fatal(e)
				}
				id = j.JobID
			} else {
				s.previewCheckpoint = hook
				j, e := s.BeginFightPreview(t.Context(), s.view.ViewID, ids[0], ids[1])
				if e != nil {
					t.Fatal(e)
				}
				id = j.JobID
			}
			finished := make(chan error, 1)
			go func() { finished <- s.cycle(t.Context()) }()
			<-entered
			if _, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, ids[0], ids[1]); !errors.Is(e, gymstable.ErrBusy) {
				t.Fatal("career bypassed active preview", e)
			}
			if _, e := s.BeginFightPreview(t.Context(), s.view.ViewID, ids[0], ids[1]); !errors.Is(e, gymstable.ErrBusy) {
				t.Fatal("legacy bypassed active preview", e)
			}
			if !career {
				charge := s.previewBytes
				// The live legacy task is held at its real pair-read boundary.
				// A new-family learned owner failure must not uncharge it.
				if e := s.resultStore.Close(); e != nil {
					t.Fatal(e)
				}
				if x := s.CareerPreviewSnapshot(); x.State != "unavailable" || x.Error != "career_closed" {
					t.Fatal(x)
				}
				if s.previewBytes != charge || charge != previewAllowance {
					t.Fatal("legacy active reservation cleared")
				}
			}
			queued, e := s.BeginStableInventory(t.Context(), s.view.ViewID, 0, 1)
			if e != nil {
				t.Fatal(e)
			}
			if career {
				_, e = s.CancelCareerFightPreview(t.Context(), s.view.ViewID, id)
			} else {
				_, e = s.CancelFightPreview(t.Context(), s.view.ViewID, id)
			}
			if e != nil {
				t.Fatal(e)
			}
			close(release)
			if e = <-finished; e != nil {
				t.Fatal(e)
			}
			if engineStage.Load() != 0 {
				t.Fatal("cancel entered engine stage")
			}
			if career {
				j, e := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, id)
				if e != nil || j.State != "cancelled" || j.Result != nil {
					t.Fatal(j.State, e)
				}
			}
			if _, e = s.CancelStableJob(t.Context(), s.view.ViewID, queued.JobID); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestCareerPreviewAdmissionRollbackPreservesOtherFamily(t *testing.T) {
	s := careerLaneFixture(t)
	s.needSync = false
	a, b := strings.Repeat("1", 64), strings.Repeat("2", 64)
	old := PreviewJob{Version: 1, JobID: strings.Repeat("9", 64), State: "cancelled", View: s.viewPin(), StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: previewEngineID, FighterAID: a, FighterBID: b, Error: "preview_cancelled"}
	s.retainPreviewLocked(old)
	before := s.previewBytes
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if e := original(ctx, cap, fn); e != nil {
			return e
		}
		close(entered)
		<-release
		return gymaccount.ErrUnauthorized
	}
	finished := make(chan error, 1)
	go func() { _, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b); finished <- e }()
	<-entered
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.previewLast == nil || s.previewLast.JobID != old.JobID {
		t.Fatal("unpublished task ran")
	}
	close(release)
	if e := <-finished; e == nil {
		t.Fatal("final unauthorized admitted")
	}
	s.stableConfig.Lease = original
	if s.queued != nil || s.careerPreviewLast != nil || s.previewLast.JobID != old.JobID || s.previewBytes != before {
		t.Fatal("refusal evicted replay")
	}
	// Only successful admission replaces the shared terminal slot.
	ack, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b)
	if e != nil {
		t.Fatal(e)
	}
	if s.previewLast != nil {
		t.Fatal("duplicate old/new terminal")
	}
	j, e := s.CancelCareerFightPreview(t.Context(), s.view.ViewID, ack.JobID)
	if e != nil || j.State != "cancelled" {
		t.Fatal(e)
	}
	if s.previewBytes >= previewAllowance {
		t.Fatal("cancel reservation not released")
	}
	if _, e = MarshalCareerPreviewResponse(CareerPreviewResponse{ID: 1, Version: 1, OK: true, Job: &j}); e != nil {
		t.Fatal(e)
	}
}
func TestCareerPreviewCapacityReciprocityAndReaderLifetime(t *testing.T) {
	s := careerLaneFixture(t)
	s.needSync = false
	a, b := strings.Repeat("1", 64), strings.Repeat("2", 64)
	s.mu.Lock()
	old := s.resultBytes
	s.resultBytes = MaxManifestCacheBytes
	s.mu.Unlock()
	if _, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b); !errors.Is(e, gymstable.ErrCapacity) || s.queued != nil {
		t.Fatal("cross-family budget", e)
	}
	s.mu.Lock()
	s.resultBytes = old
	s.mu.Unlock()
	j, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b)
	if e != nil {
		t.Fatal(e)
	}
	if s.previewBytes != previewAllowance {
		t.Fatal("unreserved pair token")
	}
	// The actual shared lazy-cache path includes the one preview reservation.
	s.mu.Lock()
	s.cache[1] = &manifestCached{manifestBytes: MaxManifestCacheBytes - previewAllowance}
	fits := s.cacheFits(s.cache, s.receiptSlot)
	delete(s.cache, 1)
	s.mu.Unlock()
	if fits {
		t.Fatal("lazy admission omitted preview charge")
	}
	if _, e = s.CancelCareerFightPreview(t.Context(), s.view.ViewID, j.JobID); e != nil {
		t.Fatal(e)
	}
	if e = s.resultStore.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b); !errors.Is(e, careerstate.ErrClosed) {
		t.Fatal("closed concrete reader", e)
	}
	snap := s.CareerPreviewSnapshot()
	if snap.State != "unavailable" || snap.Error != "career_closed" || snap.View != nil {
		t.Fatal(snap)
	}
	if s.careerPreviewLast != nil || s.previewBytes != 0 {
		t.Fatal("learned owner failure retained payload")
	}
	if cached, err := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, j.JobID); !errors.Is(err, careerstate.ErrClosed) || cached.Version != 0 {
		t.Fatal("cached job after learned failure", err)
	}
	if cached, err := s.CancelCareerFightPreview(t.Context(), s.view.ViewID, j.JobID); !errors.Is(err, careerstate.ErrClosed) || cached.Version != 0 {
		t.Fatal("cancel disclosure after learned failure", err)
	}
	if _, e = json.Marshal(snap); e != nil {
		t.Fatal(e)
	}
}

func TestCareerPreviewFinalPublicationAndCancellationBoundaries(t *testing.T) {
	for _, stage := range []string{"before-validation", "before-token-read", "before-publication"} {
		t.Run(stage, func(t *testing.T) {
			s, ids := careerPreviewLaneFixture(t)
			original := s.stableConfig.Lease
			s.careerPreviewCheckpoint = func(at string) {
				if at != stage {
					return
				}
				if stage == "before-publication" {
					s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
						return gymaccount.ErrUnauthorized
					}
				} else {
					s.stableMu.Lock()
					s.active.cancel()
					s.stableMu.Unlock()
				}
			}
			ack, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, ids[0], ids[1])
			if e != nil {
				t.Fatal(e)
			}
			if e = s.cycle(t.Context()); e != nil {
				t.Fatal(e)
			}
			s.stableConfig.Lease = original
			got, e := s.CareerFightPreviewJob(t.Context(), s.view.ViewID, ack.JobID)
			if e != nil || got.Result != nil || got.State == "done" {
				t.Fatal("late disclosure", got.State, e)
			}
			if stage == "before-publication" {
				if got.Error != "unauthorized" {
					t.Fatal("original revocation relabelled", got.Error)
				}
			} else if got.State != "cancelled" || got.Error != "preview_cancelled" {
				t.Fatal("cancel", got.State, got.Error)
			}
			if s.previewBytes >= previewAllowance {
				t.Fatal("terminal token reservation retained")
			}
		})
	}
}

func TestCareerPreviewLearnedFailurePreservesQueuedLegacyReservation(t *testing.T) {
	s := careerLaneFixture(t)
	s.needSync = false
	j, e := s.BeginFightPreview(t.Context(), s.view.ViewID, strings.Repeat("1", 64), strings.Repeat("2", 64))
	if e != nil {
		t.Fatal(e)
	}
	before := s.previewBytes
	if e = s.resultStore.Close(); e != nil {
		t.Fatal(e)
	}
	if snap := s.CareerPreviewSnapshot(); snap.Error != "career_closed" {
		t.Fatal(snap)
	}
	if s.previewBytes != before || before != previewAllowance || s.queued == nil || s.queued.preview == nil {
		t.Fatal("legacy queued charge lost")
	}
	if _, e = s.CancelFightPreview(t.Context(), s.view.ViewID, j.JobID); e != nil {
		t.Fatal(e)
	}
	if s.previewLast == nil || s.previewBytes == 0 {
		t.Fatal("legacy terminal was invalidated")
	}
}

func TestCareerPreviewExpiredUnpublishedReservationCannotClobberSuccessor(t *testing.T) {
	for _, firstCareer := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-first", true: "career-first"}[firstCareer], func(t *testing.T) {
			s := careerLaneFixture(t)
			s.needSync = false
			a, b := strings.Repeat("1", 64), strings.Repeat("2", 64)
			old := PreviewJob{Version: 1, JobID: strings.Repeat("9", 64), State: "cancelled", View: s.viewPin(), StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: previewEngineID, FighterAID: a, FighterBID: b, Error: "preview_cancelled"}
			s.retainPreviewLocked(old)
			original := s.stableConfig.Lease
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				if e := original(ctx, cap, fn); e != nil {
					return e
				}
				if calls.Add(1) == 1 {
					close(entered)
					<-release
					return gymaccount.ErrUnauthorized
				}
				return nil
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			first := make(chan error, 1)
			go func() {
				var e error
				if firstCareer {
					_, e = s.BeginCareerFightPreview(ctx, s.view.ViewID, a, b)
				} else {
					_, e = s.BeginFightPreview(ctx, s.view.ViewID, a, b)
				}
				first <- e
			}()
			<-entered
			cancel()
			var id string
			if firstCareer {
				j, e := s.BeginFightPreview(t.Context(), s.view.ViewID, a, b)
				if e != nil {
					t.Fatal(e)
				}
				id = j.JobID
			} else {
				j, e := s.BeginCareerFightPreview(t.Context(), s.view.ViewID, a, b)
				if e != nil {
					t.Fatal(e)
				}
				id = j.JobID
			}
			if s.previewBytes != previewAllowance {
				t.Fatal("successor was unreserved")
			}
			close(release)
			if e := <-first; e == nil {
				t.Fatal("expired first admission succeeded")
			}
			s.stableConfig.Lease = original
			if s.queued == nil || s.previewBytes != previewAllowance {
				t.Fatal("old rollback clobbered successor")
			}
			if firstCareer {
				_, e := s.CancelFightPreview(t.Context(), s.view.ViewID, id)
				if e != nil {
					t.Fatal(e)
				}
			} else {
				_, e := s.CancelCareerFightPreview(t.Context(), s.view.ViewID, id)
				if e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
