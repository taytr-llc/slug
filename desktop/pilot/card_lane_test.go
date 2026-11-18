//go:build !slugtestpow

package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cardLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	t.Helper()
	s, ev := seedLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymcard.CandidatePin(s.seedPin)
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymcard.Create(t.Context(), filepath.Join(root, "cards"), p, s.seedStore)
	if e != nil {
		t.Fatal(e)
	}
	i, e := x.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	s.cardEnabled = true
	s.cardAttached = true
	s.cardStore = x
	s.cardPin = p
	s.cardID = i.StoreID
	s.cardCreated = i.CreatedAtMS
	s.cardSourceCreated = i.SourceCreatedAtMS
	s.cardBytes = MaxCardMetadataSize
	t.Cleanup(func() { _ = x.Close() })
	return s, ev
}
func cardJobResponse(t *testing.T, s *StableController, j CardJob) []byte {
	t.Helper()
	v := s.viewPin()
	b, e := MarshalCardResponse(CardResponse{ID: 1, OK: true, View: &v, StoreID: s.cardID, Job: &j})
	if e != nil {
		t.Fatalf("job %s/%s: %v", j.State, j.Error, e)
	}
	return b
}
func TestCardLaneIncompleteAndTerminalImmutability(t *testing.T) {
	s, ev := cardLaneFixture(t)
	ctx := t.Context()
	v := s.view.ViewID
	snap := s.CardSnapshot()
	if snap.State != "ready" || snap.Pin == nil {
		t.Fatal(snap)
	}
	if _, e := MarshalCardResponse(CardResponse{ID: 1, OK: true, View: snap.View, StoreID: snap.StoreID, Snapshot: &snap}); e != nil {
		t.Fatal(e)
	}
	status, e := s.CardStatus(ctx, v, ev.Saturday)
	if e != nil || status.Exists || status.CardID == "" {
		t.Fatal(status, e)
	}
	view := s.viewPin()
	if _, e = MarshalCardResponse(CardResponse{ID: 2, OK: true, View: &view, StoreID: s.cardID, Status: &status}); e != nil {
		t.Fatal(e)
	}
	ack, e := s.BeginCardBuild(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	cardJobResponse(t, s, ack)
	if s.EventSnapshot().QueuedJobID != "" || s.EntrySnapshot().QueuedJobID != "" || s.StableSnapshot().QueuedJobID != "" || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("card leaked family")
	}
	if _, e = s.BeginCardBuild(ctx, v, ev.Saturday); !errors.Is(e, gymcard.ErrBusy) {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := s.CardJob(ctx, v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "card_source_incomplete" || j.Result != nil {
		t.Fatal(j, e)
	}
	first := cardJobResponse(t, s, j)
	again, e := s.CardJob(ctx, v, j.JobID)
	if e != nil || string(first) != string(cardJobResponse(t, s, again)) {
		t.Fatal("mutable terminal", e)
	}
	ack, e = s.BeginCardBuild(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e = s.CardJob(ctx, v, ack.JobID)
	if e != nil || j.Error != "card_source_incomplete" {
		t.Fatal(j, e)
	}
	s.Stop()
	if _, e = s.CardStatus(ctx, v, ev.Saturday); e != nil {
		t.Fatal(e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.CardJob(ctx, v, j.JobID); e == nil || s.CardSnapshot().View != nil {
		t.Fatal("revoked release")
	}
}
func TestCardLaneACKFenceAndReciprocalReservation(t *testing.T) {
	s, ev := cardLaneFixture(t)
	v := s.view.ViewID
	e, _ := gymentry.EventForDate(s.entryPin, ev.Saturday)
	_ = e
	event, _ := s.eventStore.Observe(t.Context(), s.entryStore, s.session(), ev.Saturday)
	key, _ := gymcard.CardID(s.cardPin, event.Event)
	old := CardJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "build", State: "cancelled", View: s.viewPin(), StoreID: s.cardID, CardID: key, Saturday: ev.Saturday, Error: "card_cancelled"}
	s.retainCardLocked(old)
	prior := s.cardBytes
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
	go func() { _, e := s.BeginCardBuild(context.Background(), v, ev.Saturday); done <- e }()
	<-entered
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.cardLast.JobID != old.JobID {
		t.Fatal("unpublished task ran")
	}
	close(release)
	if <-done == nil {
		t.Fatal("accepted revoked ACK")
	}
	s.stableConfig.Lease = orig
	if s.queued != nil || s.cardLast.JobID != old.JobID || s.cardBytes != prior {
		t.Fatal("failed ack lost old result")
	}
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxCardCacheSize}
	if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); !errors.Is(e, gymcard.ErrCapacity) {
		t.Fatal("reservation", e)
	}
	delete(s.cache, 100)
	s.laneGate <- struct{}{}
	start := time.Now()
	if _, e := s.CardHistory(t.Context(), v, "", 8); !errors.Is(e, gymcard.ErrBusy) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("blocking metadata", e)
	}
	<-s.laneGate
	ack, e2 := s.BeginCardBuild(t.Context(), v, ev.Saturday)
	if e2 != nil {
		t.Fatal(e2)
	}
	j, e2 := s.CancelCardJob(t.Context(), v, ack.JobID)
	if e2 != nil || j.State != "cancelled" || s.cardBytes >= MaxCardCacheSize {
		t.Fatal(j, e2)
	}
	b, _ := json.Marshal(j)
	if len(b) > MaxCardJobSize {
		t.Fatal("job cap")
	}
}
func TestCardLaneSharedFamiliesAndStopJoin(t *testing.T) {
	for _, first := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "card-first"}[first], func(t *testing.T) {
			s, ev := cardLaneFixture(t)
			v := s.view.ViewID
			entered, release := make(chan struct{}), make(chan struct{})
			if first {
				s.cardCheckpoint = func(stage string) {
					if stage == "before-card" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); e != nil {
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
			if first {
				if _, e := s.BeginEventCapture(t.Context(), v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			}
			snap := s.CardSnapshot()
			if first && snap.ActiveJobID == "" || !first && snap.QueuedJobID == "" {
				t.Fatal("family discovery")
			}
			if _, e := s.BeginCardBuild(t.Context(), v, ev.Saturday); !errors.Is(e, gymcard.ErrBusy) {
				t.Fatal(e)
			}
			stopped := make(chan struct{})
			go func() { s.Stop(); close(stopped) }()
			select {
			case <-stopped:
				t.Fatal("Stop did not join")
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			<-stopped
			if s.CardSnapshot().ActiveJobID != "" || s.CardSnapshot().QueuedJobID != "" {
				t.Fatal("active after join")
			}
		})
	}
}

func beginCardFamily(t *testing.T, s *StableController, ev gymentry.Event, family string) string {
	t.Helper()
	ctx := t.Context()
	v := s.view.ViewID
	var id string
	var e error
	switch family {
	case "card":
		var j CardJob
		j, e = s.BeginCardBuild(ctx, v, ev.Saturday)
		id = j.JobID
	case "stable":
		var j StableJob
		j, e = s.BeginStableInventory(ctx, v, 0, 1)
		id = j.JobID
	case "preview":
		var j PreviewJob
		j, e = s.BeginFightPreview(ctx, v, strings.Repeat("1", 64), strings.Repeat("2", 64))
		id = j.JobID
	case "entry":
		var j EntryJob
		j, e = s.BeginEntryCheck(ctx, v, ev.Saturday)
		id = j.JobID
	case "event":
		var j EventJob
		j, e = s.BeginEventCapture(ctx, v, ev.Saturday)
		id = j.JobID
	case "seed":
		var j SeedJob
		j, e = s.BeginSeedPrepare(ctx, v, ev.Saturday)
		id = j.JobID
	default:
		t.Fatal(family)
	}
	if e != nil {
		t.Fatal(family, e)
	}
	return id
}
func TestCardLaneAllFamilyOrderingAndDrain(t *testing.T) {
	for _, family := range []string{"stable", "preview", "entry", "event", "seed"} {
		for _, cardFirst := range []bool{true, false} {
			label := family + "-first"
			if cardFirst {
				label = "card-before-" + family
			}
			t.Run(label, func(t *testing.T) {
				s, ev := cardLaneFixture(t)
				first, second := family, "card"
				if cardFirst {
					first, second = "card", family
				}
				firstID := beginCardFamily(t, s, ev, first)
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
				secondID := beginCardFamily(t, s, ev, second)
				snap := s.CardSnapshot()
				wantActive, wantQueued := "", secondID
				if cardFirst {
					wantActive, wantQueued = firstID, ""
				}
				if snap.ActiveJobID != wantActive || snap.QueuedJobID != wantQueued {
					t.Fatal("card family identity", snap)
				}
				if _, e := s.BeginCardBuild(t.Context(), s.view.ViewID, ev.Saturday); !errors.Is(e, gymcard.ErrBusy) {
					t.Fatal("second Card accepted", e)
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
func TestCardReservationPreservesOtherFamilyRefusal(t *testing.T) {
	for _, family := range []string{"preview", "entry", "event", "seed"} {
		t.Run(family, func(t *testing.T) {
			s, ev := cardLaneFixture(t)
			// The already reserved Card result consumes its fixed maximum. Every new
			// family's reservation must count it, without losing the original charge.
			s.cardBytes = MaxCardCacheSize
			s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxCardCacheSize - 64*1024}
			before := s.cardBytes
			v := s.view.ViewID
			var e error
			switch family {
			case "preview":
				_, e = s.BeginFightPreview(t.Context(), v, strings.Repeat("1", 64), strings.Repeat("2", 64))
			case "entry":
				_, e = s.BeginEntryCheck(t.Context(), v, ev.Saturday)
			case "event":
				_, e = s.BeginEventCapture(t.Context(), v, ev.Saturday)
			case "seed":
				_, e = s.BeginSeedPrepare(t.Context(), v, ev.Saturday)
			}
			if e == nil || s.queued != nil || s.cardBytes != before {
				t.Fatal("reciprocal reservation admitted or changed Card", e)
			}
			// Stable publication and lazy appearance/export use this same admission
			// predicate, with the reciprocal family sum supplied by their callers.
			if stableCacheFits(s.cache, s.receiptSlot, s.cardBytes+s.seedBytes+s.eventBytes+s.entryBytes+s.previewBytes+128*1024) {
				t.Fatal("aggregate admission ignored Card")
			}
		})
	}
}
