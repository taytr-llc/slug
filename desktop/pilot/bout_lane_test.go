//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func boutLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	t.Helper()
	s, ev := cardLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymbout.CandidatePin(s.cardPin)
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymbout.Create(t.Context(), filepath.Join(root, "bouts"), p, s.cardStore)
	if e != nil {
		t.Fatal(e)
	}
	i, e := x.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	s.boutEnabled, s.boutAttached = true, true
	s.boutStore, s.boutPin, s.boutID = x, p, i.StoreID
	s.boutCreated, s.boutSourceCreated = i.CreatedAtMS, i.SourceCreatedAtMS
	s.boutBytes = MaxBoutMetadataSize
	t.Cleanup(func() { _ = x.Close() })
	return s, ev
}
func boutJobResponse(t *testing.T, s *StableController, j BoutJob) []byte {
	t.Helper()
	v := s.viewPin()
	b, e := MarshalBoutResponse(BoutResponse{ID: 1, OK: true, View: &v, StoreID: s.boutID, Job: &j})
	if e != nil {
		t.Fatalf("job %s/%s: %v", j.State, j.Error, e)
	}
	return b
}
func TestBoutLaneTerminalAndFamilyIsolation(t *testing.T) {
	s, ev := boutLaneFixture(t)
	ctx := t.Context()
	v := s.view.ViewID
	bid := strings.Repeat("a", 64)
	snap := s.BoutSnapshot()
	if snap.State != "ready" || snap.Pin == nil {
		t.Fatal(snap)
	}
	status, e := s.BoutCardStatus(ctx, v, ev.Saturday)
	if e != nil || len(status.Bouts) != 0 {
		t.Fatal(status, e)
	}
	ack, e := s.BeginBoutRun(ctx, v, ev.Saturday, bid)
	if e != nil {
		t.Fatal(e)
	}
	boutJobResponse(t, s, ack)
	if s.CardSnapshot().QueuedJobID != "" || s.SeedSnapshot().QueuedJobID != "" || s.EventSnapshot().QueuedJobID != "" || s.EntrySnapshot().QueuedJobID != "" || s.StableSnapshot().QueuedJobID != "" || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("Bout ID leaked into old family")
	}
	if _, e = s.BeginBoutRun(ctx, v, ev.Saturday, bid); !errors.Is(e, gymbout.ErrBusy) {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := s.BoutJob(ctx, v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "bout_source_incomplete" || j.Result != nil {
		t.Fatal(j, e)
	}
	before := boutJobResponse(t, s, j)
	again, e := s.BoutJob(ctx, v, j.JobID)
	if e != nil || string(before) != string(boutJobResponse(t, s, again)) {
		t.Fatal("terminal changed", e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.BoutJob(ctx, v, j.JobID); e == nil || s.BoutSnapshot().View != nil {
		t.Fatal("revoked disclosure")
	}
}
func TestBoutLaneACKRollbackAndReciprocalReservation(t *testing.T) {
	s, ev := boutLaneFixture(t)
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
	old := BoutJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "run", State: "cancelled", View: s.viewPin(), StoreID: s.boutID, CardID: key, Saturday: ev.Saturday, BoutID: bid, Error: "bout_cancelled"}
	s.retainBoutLocked(old)
	prior := s.boutBytes
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
	go func() { _, e := s.BeginBoutRun(context.Background(), v, ev.Saturday, bid); done <- e }()
	<-entered
	if e = s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.boutLast.JobID != old.JobID {
		t.Fatal("unpublished work ran")
	}
	close(release)
	if <-done == nil {
		t.Fatal("revoked ACK accepted")
	}
	s.stableConfig.Lease = orig
	if s.queued != nil || s.boutLast.JobID != old.JobID || s.boutBytes != prior {
		t.Fatal("rejected ACK damaged previous result")
	}
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxBoutCacheSize}
	if _, e = s.BeginBoutRun(t.Context(), v, ev.Saturday, bid); !errors.Is(e, gymbout.ErrCapacity) {
		t.Fatal("reciprocal reservation", e)
	}
	delete(s.cache, 100)
	s.laneGate <- struct{}{}
	if _, e = s.BoutCardStatus(t.Context(), v, ev.Saturday); !errors.Is(e, gymbout.ErrBusy) {
		t.Fatal(e)
	}
	<-s.laneGate
	ack, e := s.BeginBoutRun(t.Context(), v, ev.Saturday, bid)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.CancelBoutJob(t.Context(), v, ack.JobID)
	if e != nil || j.State != "cancelled" || s.boutBytes >= MaxBoutCacheSize {
		t.Fatal(j, e)
	}
	raw, _ := json.Marshal(j)
	if len(raw) > MaxBoutJobSize {
		t.Fatal("job cap")
	}
}
func TestBoutLaneForeignWorkAndStopJoin(t *testing.T) {
	for _, boutFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "bout-first"}[boutFirst], func(t *testing.T) {
			s, ev := boutLaneFixture(t)
			v := s.view.ViewID
			bid := strings.Repeat("a", 64)
			entered, release := make(chan struct{}), make(chan struct{})
			if boutFirst {
				s.boutCheckpoint = func(stage string) {
					if stage == "before-bout" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginBoutRun(t.Context(), v, ev.Saturday, bid); e != nil {
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
			if boutFirst {
				if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := s.BeginBoutRun(t.Context(), v, ev.Saturday, bid); e != nil {
					t.Fatal(e)
				}
			}
			snap := s.BoutSnapshot()
			if snap.State != "busy" || boutFirst && snap.ActiveJobID == "" || !boutFirst && snap.QueuedJobID == "" {
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

func beginBoutFamily(t *testing.T, s *StableController, ev gymentry.Event, family string) string {
	t.Helper()
	if family != "bout" {
		return beginCardFamily(t, s, ev, family)
	}
	j, e := s.BeginBoutRun(t.Context(), s.view.ViewID, ev.Saturday, strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	return j.JobID
}
func TestBoutLaneAllFamilyOrderingAndDrain(t *testing.T) {
	for _, family := range []string{"stable", "preview", "entry", "event", "seed", "card"} {
		for _, boutFirst := range []bool{true, false} {
			label := family + "-first"
			if boutFirst {
				label = "bout-before-" + family
			}
			t.Run(label, func(t *testing.T) {
				s, ev := boutLaneFixture(t)
				first, second := family, "bout"
				if boutFirst {
					first, second = "bout", family
				}
				firstID := beginBoutFamily(t, s, ev, first)
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
				secondID := beginBoutFamily(t, s, ev, second)
				snap := s.BoutSnapshot()
				wantActive, wantQueued := "", secondID
				if boutFirst {
					wantActive, wantQueued = firstID, ""
				}
				if snap.ActiveJobID != wantActive || snap.QueuedJobID != wantQueued {
					t.Fatal("card family identity", snap)
				}
				if _, e := s.BeginBoutRun(t.Context(), s.view.ViewID, ev.Saturday, strings.Repeat("a", 64)); !errors.Is(e, gymbout.ErrBusy) {
					t.Fatal("second Bout accepted", e)
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
func TestBoutReservationPreservesOtherFamilyRefusal(t *testing.T) {
	for _, family := range []string{"preview", "entry", "event", "seed", "card"} {
		t.Run(family, func(t *testing.T) {
			s, ev := boutLaneFixture(t)
			// The already reserved Bout result consumes its fixed maximum. Every new
			// family's reservation must count it, without losing the original charge.
			s.boutBytes = MaxBoutCacheSize
			s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxBoutCacheSize - 64*1024}
			before := s.boutBytes
			v := s.view.ViewID
			var e error
			switch family {
			case "preview":
				_, e = s.BeginFightPreview(t.Context(), v, strings.Repeat("1", 64), strings.Repeat("2", 64))
			case "entry":
				_, e = s.BeginEntryCheck(t.Context(), v, ev.Saturday)
			case "event":
				_, e = s.BeginEventCapture(t.Context(), v, ev.Saturday)
			case "card":
				_, e = s.BeginCardBuild(t.Context(), v, ev.Saturday)
			case "seed":
				_, e = s.BeginSeedPrepare(t.Context(), v, ev.Saturday)
			}
			if e == nil || s.queued != nil || s.boutBytes != before {
				t.Fatal("reciprocal reservation admitted or changed Bout", e)
			}
			// Stable publication and lazy appearance/export use this same admission
			// predicate, with the reciprocal family sum supplied by their callers.
			if stableCacheFits(s.cache, s.receiptSlot, s.boutBytes+s.cardBytes+s.seedBytes+s.eventBytes+s.entryBytes+s.previewBytes+128*1024) {
				t.Fatal("aggregate admission ignored Bout")
			}
		})
	}
}
