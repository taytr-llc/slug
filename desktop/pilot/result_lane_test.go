//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func resultLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	t.Helper()
	s, ev := boutLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymresult.CandidatePin(s.boutPin)
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymresult.Create(t.Context(), filepath.Join(root, "results"), p, s.boutStore)
	if e != nil {
		t.Fatal(e)
	}
	i, e := x.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	s.resultEnabled, s.resultAttached = true, true
	s.resultStore, s.resultPin, s.resultID = x, p, i.StoreID
	s.resultCreated, s.resultSourceCreated = i.CreatedAtMS, i.SourceCreatedAtMS
	s.resultBytes = MaxResultMetadataSize
	t.Cleanup(func() { _ = x.Close() })
	return s, ev
}
func resultJobResponse(t *testing.T, s *StableController, j ResultJob) []byte {
	t.Helper()
	v := s.viewPin()
	b, e := MarshalResultResponse(ResultResponse{ID: 1, OK: true, View: &v, StoreID: s.resultID, Job: &j})
	if e != nil {
		t.Fatalf("job %s/%s: %v", j.State, j.Error, e)
	}
	return b
}
func TestResultLaneTerminalAndFamilyIsolation(t *testing.T) {
	s, ev := resultLaneFixture(t)
	ctx := t.Context()
	v := s.view.ViewID
	bid := strings.Repeat("a", 64)
	snap := s.ResultSnapshot()
	if snap.State != "ready" || snap.Pin == nil {
		t.Fatal(snap)
	}
	status, e := s.ResultStatus(ctx, v, ev.Saturday, bid)
	if !errors.Is(e, gymresult.ErrSourceIncomplete) || status.Version != 0 {
		t.Fatal(status, e)
	}
	ack, e := s.BeginResultPublish(ctx, v, ev.Saturday, bid)
	if e != nil {
		t.Fatal(e)
	}
	resultJobResponse(t, s, ack)
	if s.CardSnapshot().QueuedJobID != "" || s.SeedSnapshot().QueuedJobID != "" || s.EventSnapshot().QueuedJobID != "" || s.EntrySnapshot().QueuedJobID != "" || s.StableSnapshot().QueuedJobID != "" || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("Result ID leaked into old family")
	}
	if _, e = s.BeginResultPublish(ctx, v, ev.Saturday, bid); !errors.Is(e, gymresult.ErrBusy) {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := s.ResultJob(ctx, v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "result_source_incomplete" || j.Result != nil {
		t.Fatal(j, e)
	}
	before := resultJobResponse(t, s, j)
	again, e := s.ResultJob(ctx, v, j.JobID)
	if e != nil || string(before) != string(resultJobResponse(t, s, again)) {
		t.Fatal("terminal changed", e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.ResultJob(ctx, v, j.JobID); e == nil || s.ResultSnapshot().View != nil {
		t.Fatal("revoked disclosure")
	}
}
func TestResultLaneACKRollbackAndReciprocalReservation(t *testing.T) {
	s, ev := resultLaneFixture(t)
	v := s.view.ViewID
	bid := strings.Repeat("a", 64)
	event, e := gymevent.EventForDate(s.eventPin, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	key, e := gymcard.CardID(s.cardPin, event)
	if e != nil {
		t.Fatal(e)
	}
	old := ResultJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "publish", State: "cancelled", View: s.viewPin(), StoreID: s.resultID, CardID: key, Saturday: ev.Saturday, BoutID: bid, Error: "result_cancelled"}
	old.ResultKey, e = gymresult.ResultKey(s.resultPin, key, bid)
	if e != nil {
		t.Fatal(e)
	}
	s.retainResultLocked(old)
	prior := s.resultBytes
	orig := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if e := orig(ctx, cap, fn); e != nil {
			return e
		}
		close(entered)
		<-release
		return gymaccount.ErrUnauthorized
	}
	done := make(chan error, 1)
	go func() { _, e := s.BeginResultPublish(context.Background(), v, ev.Saturday, bid); done <- e }()
	<-entered
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.resultLast.JobID != old.JobID {
		t.Fatal("unpublished work ran")
	}
	close(release)
	if <-done == nil {
		t.Fatal("revoked ACK accepted")
	}
	s.stableConfig.Lease = orig
	if s.queued != nil || s.resultLast.JobID != old.JobID || s.resultBytes != prior {
		t.Fatal("rejected ACK damaged previous result")
	}
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxResultCacheSize}
	if _, e = s.BeginResultPublish(t.Context(), v, ev.Saturday, bid); !errors.Is(e, gymresult.ErrCapacity) {
		t.Fatal("reciprocal reservation", e)
	}
	delete(s.cache, 100)
	s.laneGate <- struct{}{}
	if _, e = s.ResultStatus(t.Context(), v, ev.Saturday, bid); !errors.Is(e, gymresult.ErrBusy) {
		t.Fatal(e)
	}
	<-s.laneGate
	ack, e := s.BeginResultPublish(t.Context(), v, ev.Saturday, bid)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.CancelResultJob(t.Context(), v, ack.JobID)
	if e != nil || j.State != "cancelled" || s.resultBytes >= MaxResultCacheSize {
		t.Fatal(j, e)
	}
	raw, _ := json.Marshal(j)
	if len(raw) > MaxResultJobSize {
		t.Fatal("job cap")
	}
}
func TestResultLaneForeignWorkAndStopJoin(t *testing.T) {
	for _, resultFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "result-first"}[resultFirst], func(t *testing.T) {
			s, ev := resultLaneFixture(t)
			v := s.view.ViewID
			bid := strings.Repeat("a", 64)
			entered, release := make(chan struct{}), make(chan struct{})
			if resultFirst {
				s.resultCheckpoint = func(stage string) {
					if stage == "before-result" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginResultPublish(t.Context(), v, ev.Saturday, bid); e != nil {
					t.Fatal(e)
				}
			} else {
				s.laneCheckpoint = func(string) { close(entered); <-release }
				if _, e := s.BeginStableInventory(t.Context(), v, 0, 1); e != nil {
					t.Fatal(e)
				}
			}
			done := make(chan error, 1)
			go func() { done <- s.cycle(context.Background()) }()
			<-entered
			if resultFirst {
				if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := s.BeginResultPublish(t.Context(), v, ev.Saturday, bid); e != nil {
					t.Fatal(e)
				}
			}
			snap := s.ResultSnapshot()
			if snap.State != "busy" || resultFirst && snap.ActiveJobID == "" || !resultFirst && snap.QueuedJobID == "" {
				t.Fatal("family discovery", snap)
			}
			stopped := make(chan struct{})
			go func() { s.Stop(); close(stopped) }()
			close(release)
			<-done
			<-stopped
			if s.active != nil || s.queued != nil {
				t.Fatal("Stop did not drain global lane")
			}
		})
	}
}

func beginResultFamily(t *testing.T, s *StableController, ev gymentry.Event, family string) string {
	t.Helper()
	if family != "result" {
		return beginBoutFamily(t, s, ev, family)
	}
	j, e := s.BeginResultPublish(t.Context(), s.view.ViewID, ev.Saturday, strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	return j.JobID
}
func TestResultLaneAllFamilyOrderingAndDrain(t *testing.T) {
	for _, family := range []string{"stable", "preview", "entry", "event", "seed", "card", "bout"} {
		for _, resultFirst := range []bool{true, false} {
			label := family + "-first"
			if resultFirst {
				label = "result-before-" + family
			}
			t.Run(label, func(t *testing.T) {
				s, ev := resultLaneFixture(t)
				first, second := family, "result"
				if resultFirst {
					first, second = "result", family
				}
				firstID := beginResultFamily(t, s, ev, first)
				entered, release := make(chan struct{}), make(chan struct{})
				// Block before the first real account acquisition, without holding a lease.
				// This works even for Preview before its source lookup/proof work.
				orig := s.stableConfig.Lease
				var seen atomic.Bool
				s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
					if seen.CompareAndSwap(false, true) {
						close(entered)
						<-release
					}
					return orig(ctx, cap, fn)
				}
				done := make(chan error, 1)
				go func() { done <- s.cycle(context.Background()) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("active family did not acquire its original authority")
				}
				secondID := beginResultFamily(t, s, ev, second)
				snap := s.ResultSnapshot()
				wantActive, wantQueued := "", secondID
				if resultFirst {
					wantActive, wantQueued = firstID, ""
				}
				if snap.ActiveJobID != wantActive || snap.QueuedJobID != wantQueued {
					t.Fatal("card family identity", snap)
				}
				if _, e := s.BeginResultPublish(t.Context(), s.view.ViewID, ev.Saturday, strings.Repeat("a", 64)); !errors.Is(e, gymresult.ErrBusy) {
					t.Fatal("second Result accepted", e)
				}
				stopped := make(chan struct{})
				go func() { s.Stop(); close(stopped) }()
				select {
				case <-stopped:
					t.Fatal("Stop failed to join active family")
				case <-time.After(10 * time.Millisecond):
				}
				close(release)
				if e := <-done; e != nil {
					t.Fatal(e)
				}
				<-stopped
				if s.active != nil || s.queued != nil {
					t.Fatal("lane survived Stop")
				}
			})
		}
	}
}
func TestResultReservationPreservesOtherFamilyRefusal(t *testing.T) {
	for _, family := range []string{"preview", "entry", "event", "seed", "card", "bout"} {
		t.Run(family, func(t *testing.T) {
			s, ev := resultLaneFixture(t)
			// The already reserved Result result consumes its fixed maximum. Every new
			// family's reservation must count it, without losing the original charge.
			s.resultBytes = MaxResultCacheSize
			s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxResultCacheSize - 64*1024}
			before := s.resultBytes
			v := s.view.ViewID
			var e error
			switch family {
			case "preview":
				_, e = s.BeginFightPreview(t.Context(), v, strings.Repeat("1", 64), strings.Repeat("2", 64))
			case "entry":
				_, e = s.BeginEntryCheck(t.Context(), v, ev.Saturday)
			case "event":
				_, e = s.BeginEventCapture(t.Context(), v, ev.Saturday)
			case "bout":
				_, e = s.BeginBoutRun(t.Context(), v, ev.Saturday, strings.Repeat("a", 64))
			case "card":
				_, e = s.BeginCardBuild(t.Context(), v, ev.Saturday)
			case "seed":
				_, e = s.BeginSeedPrepare(t.Context(), v, ev.Saturday)
			}
			if e == nil || s.queued != nil || s.resultBytes != before {
				t.Fatal("reciprocal reservation admitted or changed Result", e)
			}
			// Stable publication and lazy appearance/export use this same admission
			// predicate, with the reciprocal family sum supplied by their callers.
			if stableCacheFits(s.cache, s.receiptSlot, s.resultBytes+s.boutBytes+s.cardBytes+s.seedBytes+s.eventBytes+s.entryBytes+s.previewBytes+128*1024) {
				t.Fatal("aggregate admission ignored Result")
			}
		})
	}
}
