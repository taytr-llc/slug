//go:build !slugtestpow

package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	t.Helper()
	s, ev := eventLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymseed.CandidatePin(s.eventPin)
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymseed.Create(t.Context(), filepath.Join(root, "seeds"), p, s.eventStore)
	if e != nil {
		t.Fatal(e)
	}
	i, e := x.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	s.seedEnabled = true
	s.seedAttached = true
	s.seedStore = x
	s.seedPin = p
	s.seedID = i.StoreID
	s.seedCreated = i.CreatedAtMS
	s.seedBytes = MaxSeedMetadataSize
	t.Cleanup(func() { _ = x.Close() })
	return s, ev
}
func seedJobResponse(t *testing.T, s *StableController, j SeedJob) []byte {
	t.Helper()
	v := s.viewPin()
	b, e := MarshalSeedResponse(SeedResponse{ID: 1, OK: true, View: &v, StoreID: s.seedID, Job: &j})
	if e != nil {
		t.Fatalf("job %s/%s: %v", j.State, j.Error, e)
	}
	return b
}
func TestSeedLaneIncompleteOfflineAndTerminalImmutability(t *testing.T) {
	s, ev := seedLaneFixture(t)
	ctx := t.Context()
	v := s.view.ViewID
	snap := s.SeedSnapshot()
	if snap.State != "ready" || snap.Pin == nil || snap.NodeState != "offline" {
		t.Fatal(snap)
	}
	if _, e := MarshalSeedResponse(SeedResponse{ID: 1, OK: true, View: snap.View, StoreID: snap.StoreID, Snapshot: &snap}); e != nil {
		t.Fatal(e)
	}
	status, e := s.SeedStatus(ctx, v, ev.Saturday)
	if e != nil || status.Exists || status.Phase != "absent" {
		t.Fatal(status, e)
	}
	view := s.viewPin()
	if _, e = MarshalSeedResponse(SeedResponse{ID: 2, OK: true, View: &view, StoreID: s.seedID, Status: &status}); e != nil {
		t.Fatal(e)
	}
	ack, e := s.BeginSeedPrepare(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	seedJobResponse(t, s, ack)
	if s.EventSnapshot().QueuedJobID != "" || s.EntrySnapshot().QueuedJobID != "" || s.StableSnapshot().QueuedJobID != "" || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("seed leaked family")
	}
	if _, e = s.BeginSeedReconcile(ctx, v, ev.Saturday); !errors.Is(e, gymseed.ErrBusy) {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := s.SeedJob(ctx, v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "seed_source_incomplete" || j.Result != nil {
		t.Fatal(j, e)
	}
	first := seedJobResponse(t, s, j)
	again, e := s.SeedJob(ctx, v, j.JobID)
	if e != nil || string(first) != string(seedJobResponse(t, s, again)) {
		t.Fatal("mutable terminal", e)
	}
	ack, e = s.BeginSeedReconcile(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e = s.SeedJob(ctx, v, ack.JobID)
	if e != nil || j.Error != "seed_node_offline" {
		t.Fatal(j, e)
	}
	s.Stop()
	if _, e = s.SeedStatus(ctx, v, ev.Saturday); e != nil {
		t.Fatal(e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.SeedJob(ctx, v, j.JobID); e == nil || s.SeedSnapshot().View != nil {
		t.Fatal("revoked release")
	}
}
func TestSeedLaneACKFenceAndReciprocalReservation(t *testing.T) {
	s, ev := seedLaneFixture(t)
	v := s.view.ViewID
	e, _ := gymentry.EventForDate(s.entryPin, ev.Saturday)
	_ = e
	event, _ := s.eventStore.Observe(t.Context(), s.entryStore, s.session(), ev.Saturday)
	key, _ := gymseed.EventKey(s.seedPin, event.Event)
	old := SeedJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "prepare", State: "cancelled", View: s.viewPin(), StoreID: s.seedID, EventKey: key, Saturday: ev.Saturday, Error: "seed_cancelled"}
	s.retainSeedLocked(old)
	prior := s.seedBytes
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
	go func() { _, e := s.BeginSeedPrepare(context.Background(), v, ev.Saturday); done <- e }()
	<-entered
	if e := s.cycle(t.Context()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.seedLast.JobID != old.JobID {
		t.Fatal("unpublished task ran")
	}
	close(release)
	if <-done == nil {
		t.Fatal("accepted revoked ACK")
	}
	s.stableConfig.Lease = orig
	if s.queued != nil || s.seedLast.JobID != old.JobID || s.seedBytes != prior {
		t.Fatal("failed ack lost old result")
	}
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxSeedCacheSize}
	if _, e := s.BeginSeedPrepare(t.Context(), v, ev.Saturday); !errors.Is(e, gymseed.ErrCapacity) {
		t.Fatal("reservation", e)
	}
	delete(s.cache, 100)
	s.laneGate <- struct{}{}
	start := time.Now()
	if _, e := s.SeedHistory(t.Context(), v, "", 8); !errors.Is(e, gymseed.ErrBusy) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("blocking metadata", e)
	}
	<-s.laneGate
	ack, e2 := s.BeginSeedPrepare(t.Context(), v, ev.Saturday)
	if e2 != nil {
		t.Fatal(e2)
	}
	j, e2 := s.CancelSeedJob(t.Context(), v, ack.JobID)
	if e2 != nil || j.State != "cancelled" || s.seedBytes >= MaxSeedCacheSize {
		t.Fatal(j, e2)
	}
	b, _ := json.Marshal(j)
	if len(b) > MaxSeedJobSize {
		t.Fatal("job cap")
	}
}
func TestSeedLaneSharedFamiliesAndStopJoin(t *testing.T) {
	for _, first := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "seed-first"}[first], func(t *testing.T) {
			s, ev := seedLaneFixture(t)
			v := s.view.ViewID
			entered, release := make(chan struct{}), make(chan struct{})
			if first {
				s.seedCheckpoint = func(stage string) {
					if stage == "before-seed" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginSeedPrepare(t.Context(), v, ev.Saturday); e != nil {
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
				if _, e := s.BeginSeedPrepare(t.Context(), v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			}
			snap := s.SeedSnapshot()
			if first && snap.ActiveJobID == "" || !first && snap.QueuedJobID == "" {
				t.Fatal("family discovery")
			}
			if _, e := s.BeginSeedReconcile(t.Context(), v, ev.Saturday); !errors.Is(e, gymseed.ErrBusy) {
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
			if s.SeedSnapshot().ActiveJobID != "" || s.SeedSnapshot().QueuedJobID != "" {
				t.Fatal("active after join")
			}
		})
	}
}

func TestSeedOwnedRunRegistrationAndDrain(t *testing.T) {
	s, _ := seedLaneFixture(t)
	primary, peer := make(chan struct{}), make(chan struct{})
	n := &pilotNodes{primary: &childNode{done: primary}, peer: &childNode{done: peer}}
	// This only tests the owner's registration/lifetime bookkeeping. A Publisher
	// still requires AttachOwned's real process/config/topology verification.
	s.registerSeedNodes(n)
	if s.SeedSnapshot().NodeState != "ready" {
		t.Fatal("zero owner not registered")
	}
	close(peer)
	if s.SeedSnapshot().NodeState != "offline" {
		t.Fatal("exited peer remained ready")
	}
	s.seedNodes = nil
	original := s.account.AccountID
	s.account.AccountID = strings.Repeat("f", 64)
	s.registerSeedNodes(n)
	if s.seedNodes != nil {
		t.Fatal("foreign fixture became publication owner")
	}
	s.account.AccountID = original
	peer = make(chan struct{})
	n.peer.done = peer
	s.registerSeedNodes(n)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.active = &stableTask{seed: &SeedJob{JobID: strings.Repeat("a", 64)}, ctx: ctx, cancel: cancel}
	s.laneGate <- struct{}{}
	done := make(chan struct{})
	go func() { s.unregisterSeedNodes(n); close(done) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("active borrow not cancelled")
	}
	select {
	case <-done:
		t.Fatal("owner failed to join lane before closing nodes")
	case <-time.After(20 * time.Millisecond):
	}
	<-s.laneGate
	<-done
	s.active = nil
	if s.SeedSnapshot().NodeState != "offline" {
		t.Fatal("registration retained after drain")
	}
}
