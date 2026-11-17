//go:build !slugtestpow

package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func eventFixturePin(t *testing.T) gymevent.Pin {
	t.Helper()
	var corpus struct{ Pins []struct{ Pin gymstable.Pin } }
	raw, e := os.ReadFile("../../gymstable/testdata/reference/stable_v1.json")
	if e != nil || json.Unmarshal(raw, &corpus) != nil {
		t.Fatal(e)
	}
	p := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{}}
	for _, x := range corpus.Pins {
		id, e := gymstable.ID(x.Pin)
		if e != nil {
			t.Fatal(e)
		}
		ns, e := gymentry.NamespaceID(x.Pin.Identity.Network, x.Pin.Identity.Genesis, x.Pin.AccountAuthorityID)
		if e != nil {
			t.Fatal(e)
		}
		p.Participants = append(p.Participants, gymentry.Pin{Version: 1, NamespaceID: ns, StableStoreID: id, StablePin: x.Pin, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID()})
	}
	sort.Slice(p.Participants, func(i, j int) bool {
		return p.Participants[i].StablePin.Identity.GymID < p.Participants[j].StablePin.Identity.GymID
	})
	p, e = gymevent.ValidatePin(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func attachEventFixture(t *testing.T, s *StableController, x *gymevent.Store) {
	t.Helper()
	i, e := x.Info(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	s.eventEnabled = true
	s.eventAttached = true
	s.eventStore = x
	s.eventPin = i.Pin
	s.eventID = i.StoreID
	s.eventCreated = i.CreatedAtMS
	s.eventParticipant = s.entryPin.StablePin.Identity.GymID
	s.eventBytes = eventMetadataBytes
	t.Cleanup(func() {
		if e := x.Close(); e != nil {
			t.Error(e)
		}
	})
}
func eventLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	s, ev := entryLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymevent.Create(context.Background(), filepath.Join(root, "events"), eventFixturePin(t))
	if e != nil {
		t.Fatal(e)
	}
	attachEventFixture(t, s, x)
	return s, ev
}
func eventJobResponse(t *testing.T, s *StableController, j EventJob) []byte {
	t.Helper()
	v := s.viewPin()
	b, e := MarshalEventResponse(EventResponse{ID: 1, OK: true, View: &v, StoreID: s.eventID, Job: &j})
	if e != nil {
		t.Fatalf("invalid produced event job %s/%s: %v", j.State, j.Error, e)
	}
	return b
}
func TestEventLaneNotDueCacheAndStoppedMetadata(t *testing.T) {
	s, ev := eventLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	snap := s.EventSnapshot()
	if snap.State != "ready" || snap.Pin == nil {
		t.Fatal("snapshot", snap)
	}
	if _, e := MarshalEventResponse(EventResponse{ID: 1, OK: true, View: snap.View, StoreID: snap.StoreID, Snapshot: &snap}); e != nil {
		t.Fatal(e)
	}
	status, e := s.EventStatus(ctx, v, ev.Saturday)
	if e != nil || status.Phase != "before_cutoff" {
		t.Fatal("before cutoff", status.Phase, e)
	}
	view := s.viewPin()
	if _, e = MarshalEventResponse(EventResponse{ID: 2, OK: true, View: &view, StoreID: s.eventID, Status: &status}); e != nil {
		t.Fatal("status codec", e)
	}
	ack, e := s.BeginEventCapture(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	eventJobResponse(t, s, ack)
	if s.EntrySnapshot().QueuedJobID != "" || s.StableSnapshot().QueuedJobID != "" || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("event leaked old family")
	}
	if _, e = s.BeginEventCapture(ctx, v, ev.Saturday); !errors.Is(e, gymevent.ErrBusy) {
		t.Fatal("second event", e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	done, e := s.EventJob(ctx, v, ack.JobID)
	if e != nil || done.State != "failed" || done.Error != "event_not_due" || done.Result != nil {
		t.Fatal(done, e)
	}
	before := eventJobResponse(t, s, done)
	again, e := s.EventJob(ctx, v, ack.JobID)
	if e != nil || string(before) != string(eventJobResponse(t, s, again)) {
		t.Fatal("terminal poll changed", e)
	}
	s.Stop()
	if _, e = s.EventStatus(ctx, v, ev.Saturday); e != nil {
		t.Fatal("stopped metadata", e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.EventJob(ctx, v, ack.JobID); e == nil || s.EventSnapshot().View != nil {
		t.Fatal("revoked cached disclosure")
	}
}
func TestEventLanePublicationFenceAndReservation(t *testing.T) {
	s, ev := eventLaneFixture(t)
	v := s.view.ViewID
	old := EventJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "capture", State: "cancelled", View: s.viewPin(), StoreID: s.eventID, Saturday: ev.Saturday, ParticipantID: s.eventParticipant, Error: "event_cancelled"}
	event, _ := gymevent.EventForDate(s.eventPin, ev.Saturday)
	old.EventID = event.EventID
	s.retainEventLocked(old)
	prior := s.eventBytes
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
	done := make(chan error, 1)
	go func() { _, e := s.BeginEventCapture(context.Background(), v, ev.Saturday); done <- e }()
	<-entered
	if e := s.cycle(context.Background()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.eventLast.JobID != old.JobID {
		t.Fatal("unpublished job ran")
	}
	close(release)
	if e := <-done; e == nil {
		t.Fatal("lease loss accepted")
	}
	s.stableConfig.Lease = original
	if s.queued != nil || s.eventLast.JobID != old.JobID || s.eventBytes != prior {
		t.Fatal("failed ack lost prior cache")
	}
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxEventCacheSize}
	if _, e := s.BeginEventCapture(context.Background(), v, ev.Saturday); !errors.Is(e, gymevent.ErrCapacity) {
		t.Fatal("aggregate reservation", e)
	}
	delete(s.cache, 100)
	s.laneGate <- struct{}{}
	start := time.Now()
	if _, e := s.EventHistory(context.Background(), v, "", 8); !errors.Is(e, gymevent.ErrBusy) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("metadata blocked", e)
	}
	<-s.laneGate
	ack, e := s.BeginEventCapture(context.Background(), v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.CancelEventJob(context.Background(), v, ack.JobID)
	if e != nil || j.State != "cancelled" || s.eventBytes >= MaxEventCacheSize {
		t.Fatal("cancel", e)
	}
}
func TestEventLaneSharedFamiliesAndStopJoin(t *testing.T) {
	for _, eventFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "event-first"}[eventFirst], func(t *testing.T) {
			s, ev := eventLaneFixture(t)
			ctx := context.Background()
			v := s.view.ViewID
			entered, release := make(chan struct{}), make(chan struct{})
			if eventFirst {
				s.eventCheckpoint = func(stage string) {
					if stage == "before-event" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginEventCapture(ctx, v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			} else {
				s.laneCheckpoint = func(string) { close(entered); <-release }
				if _, e := s.BeginStableInventory(ctx, v, 0, 1); e != nil {
					t.Fatal(e)
				}
			}
			done := make(chan error, 1)
			go func() { done <- s.cycle(ctx) }()
			<-entered
			if eventFirst {
				if _, e := s.BeginEntryCheck(ctx, v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := s.BeginEventCapture(ctx, v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			}
			es, ss := s.EventSnapshot(), s.StableSnapshot()
			if eventFirst {
				if es.ActiveJobID == "" || ss.ActiveJobID != "" || s.EntrySnapshot().QueuedJobID == "" {
					t.Fatal("event active identity")
				}
			} else {
				if es.QueuedJobID == "" || ss.QueuedJobID != "" || ss.ActiveJobID == "" {
					t.Fatal("event queued identity")
				}
			}
			if _, e := s.BeginEventCapture(ctx, v, ev.Saturday); !errors.Is(e, gymevent.ErrBusy) {
				t.Fatal("third job", e)
			}
			stopped := make(chan struct{})
			go func() { s.Stop(); close(stopped) }()
			select {
			case <-stopped:
				t.Fatal("Stop failed to join")
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			<-stopped
			if s.EventSnapshot().ActiveJobID != "" || s.EventSnapshot().QueuedJobID != "" {
				t.Fatal("joined task retained as active")
			}
		})
	}
}

func TestEventBorrowedStoreLifetimeAndFatalFailure(t *testing.T) {
	s, ev := eventLaneFixture(t)
	x := s.eventStore
	s.Stop()
	if _, e := x.Info(context.Background()); e != nil {
		t.Fatal("Stop closed parent store", e)
	}
	s.stableMu.Lock()
	s.mining = true
	s.stableMu.Unlock()
	if e := x.Close(); e != nil {
		t.Fatal(e)
	}
	ack, e := s.BeginEventCapture(context.Background(), s.view.ViewID, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(context.Background()); e == nil {
		t.Fatal("closed dependency did not fail lane")
	}
	j, e := s.EventJob(context.Background(), s.view.ViewID, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "event_unavailable" || j.Result != nil {
		t.Fatal("fatal publication", j, e)
	}
	if s.EventSnapshot().State != "failed" {
		t.Fatal("fatal snapshot")
	}
}
