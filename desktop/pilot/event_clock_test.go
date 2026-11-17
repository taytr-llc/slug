//go:build slugtestclock && !slugtestpow

package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func eventClockFixture(t *testing.T) (*StableController, gymentry.Event, *atomic.Int64) {
	t.Helper()
	s, ev := entryLaneFixture(t)
	n := new(atomic.Int64)
	n.Store(ev.CutoffMS - 60000)
	clock := func() time.Time { return time.UnixMilli(n.Load()) }
	if e := s.closePair(); e != nil {
		t.Fatal(e)
	}
	s.eventFactories = &eventStoreFactories{
		createStable: func(p string, pin gymstable.Pin, r *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.CreateForTestClock(p, pin, r, clock)
		},
		openStable: func(p string, pin gymstable.Pin, r *registry.ManifestStore) (*gymstable.Store, error) {
			return gymstable.OpenForTestClock(p, pin, r, clock)
		},
		createEntry: func(ctx context.Context, p string, pin gymentry.Pin, r *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.CreateForTestClock(ctx, p, pin, r, clock)
		},
		openEntry: func(ctx context.Context, p string, pin gymentry.Pin, r *gymstable.Store) (*gymentry.Store, error) {
			return gymentry.OpenForTestClock(ctx, p, pin, r, clock)
		},
	}
	if e := s.openPair(nil); e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	x, e := gymevent.CreateForTestClock(context.Background(), filepath.Join(root, "events"), eventFixturePin(t), clock)
	if e != nil {
		t.Fatal(e)
	}
	attachEventFixture(t, s, x)
	return s, ev, n
}
func eventSavedFixture(t *testing.T, s *StableController, ev gymentry.Event) string {
	t.Helper()
	ctx := context.Background()
	v := s.view.ViewID
	id := s.last.Sync.Items[0].Assignment.ProspectID
	ack, e := s.BeginStablePrepare(ctx, v, gymstable.SlotRequest{Slot: 1, ProspectID: id})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	prep, e := s.StableJob(ctx, v, ack.JobID)
	if e != nil {
		t.Fatal(e)
	}
	ack, e = s.BeginStableApply(ctx, v, prep.Intent.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	applied, e := s.StableJob(ctx, v, ack.JobID)
	if e != nil || applied.Mutation == nil {
		t.Fatal(e)
	}
	req := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "replace", ExpectedRosterRevision: applied.Mutation.Receipt.Revision, ProspectIDs: []string{id}}
	ep, e := s.BeginEntryPrepare(ctx, v, req)
	if e != nil {
		t.Fatal(e)
	}
	ej := entryDone(t, s, ep)
	ep, e = s.BeginEntryApply(ctx, v, ej.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	entryDone(t, s, ep)
	return id
}
func eventClockDone(t *testing.T, s *StableController, ev gymentry.Event) EventJob {
	t.Helper()
	ack, e := s.BeginEventCapture(context.Background(), s.view.ViewID, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(context.Background()); e != nil {
		t.Fatal("cycle", e)
	}
	j, e := s.EventJob(context.Background(), s.view.ViewID, ack.JobID)
	if e != nil || j.State != "completed" || j.Result == nil {
		t.Fatalf("capture %s/%s %v", j.State, j.Error, e)
	}
	eventJobResponse(t, s, j)
	return j
}
func TestEventClockHistoricalCaptureImmutableReplayAndNoProofPolling(t *testing.T) {
	s, ev, n := eventClockFixture(t)
	id := eventSavedFixture(t, s, ev)
	ctx := context.Background()
	v := s.view.ViewID
	before, e := s.entryStore.Pending(ctx, s.session())
	if e != nil {
		t.Fatal(e)
	}
	n.Store(ev.CutoffMS)
	j := eventClockDone(t, s, ev)
	r := j.Result.Capture
	if r.Snapshot.Entry.LatestReceipt == nil || len(r.Snapshot.Members) != 1 || r.Snapshot.Members[0].Assignment.ProspectID != id || r.Snapshot.Members[0].ConditionAtCutoff.AsOfMS != ev.CutoffMS || r.Snapshot.Members[0].Eligibility != "eligible" {
		t.Fatal("cutoff selection")
	}
	raw := eventJobResponse(t, s, j)
	// These cached operations cannot dispatch source work: keep the sole proof
	// lane occupied, revoke the source handles, and require identical job bytes.
	s.laneGate <- struct{}{}
	if e = s.closePair(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		q, e := s.EventJob(ctx, v, j.JobID)
		if e != nil || string(eventJobResponse(t, s, q)) != string(raw) {
			t.Fatal("cached poll", e)
		}
		if s.EventSnapshot().LastJobID != j.JobID {
			t.Fatal("snapshot cache")
		}
	}
	if e = s.openPair(nil); e != nil {
		t.Fatal(e)
	}
	<-s.laneGate
	n.Store(ev.CutoffMS + 1000)
	// A real later roster mutation must not rewrite the captured historical input.
	a, e := s.BeginStablePrepare(ctx, v, gymstable.SlotRequest{ExpectedRevision: 1, Slot: 1, ProspectID: ""})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	p, _ := s.StableJob(ctx, v, a.JobID)
	a, e = s.BeginStableApply(ctx, v, p.Intent.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	replay := eventClockDone(t, s, ev)
	if !replay.Result.Replayed || !reflect.DeepEqual(replay.Result.Capture, r) || replay.Result.Status.ObservedAtMS <= j.Result.Status.ObservedAtMS {
		t.Fatal("replay replaced historical capture")
	}
	after, e := s.entryStore.Pending(ctx, s.session())
	if e != nil || !reflect.DeepEqual(before.Operation, after.Operation) {
		t.Fatal("event mutated entry", e)
	}
	h, e := s.EventHistory(ctx, v, "", 8)
	if e != nil || len(h.Events) != 1 {
		t.Fatal("history", e)
	}
	view := s.viewPin()
	if _, e = MarshalEventResponse(EventResponse{ID: 3, OK: true, View: &view, StoreID: s.eventID, History: &h}); e != nil {
		t.Fatal("history transport", e)
	}
	s.Stop()
	if _, e = s.EventStatus(ctx, v, ev.Saturday); e != nil {
		t.Fatal("reopened bound clock", e)
	}
}
func TestEventClockPostCommitPublicationIsUnknown(t *testing.T) {
	for _, kind := range []string{"wrapper", "lease"} {
		t.Run(kind, func(t *testing.T) {
			s, ev, n := eventClockFixture(t)
			n.Store(ev.CutoffMS)
			ctx := context.Background()
			v := s.view.ViewID
			ack, e := s.BeginEventCapture(ctx, v, ev.Saturday)
			if e != nil {
				t.Fatal(e)
			}
			lease := s.stableConfig.Lease
			if kind == "wrapper" {
				s.stableMu.Lock()
				s.queued.event.ParticipantID = s.eventPin.Participants[0].StablePin.Identity.GymID
				s.queued.event.View.AccountID = "bad"
				s.stableMu.Unlock()
			} else {
				s.eventCheckpoint = func(stage string) {
					if stage == "before-event-publication" {
						s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
							return gymaccount.ErrUnauthorized
						}
					}
				}
			}
			_ = s.cycle(ctx)
			s.stableConfig.Lease = lease
			s.eventCheckpoint = nil
			j, e := s.EventJob(ctx, v, ack.JobID)
			if e != nil || j.State != "failed" || j.Error != "outcome_unknown" || j.Result != nil {
				t.Fatal("lost durable ACK", j, e)
			}
			status, e := s.EventStatus(ctx, v, ev.Saturday)
			if e != nil || len(status.CapturedParticipants) != 1 {
				t.Fatal("durable capture absent", e)
			}
		})
	}
}
func TestEventClockCancelledAfterDurableCaptureKeepsUnknown(t *testing.T) {
	s, ev, n := eventClockFixture(t)
	n.Store(ev.CutoffMS)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.eventCheckpoint = func(stage string) {
		if stage == "before-event-publication" {
			cancel()
		}
	}
	ack, e := s.BeginEventCapture(ctx, s.view.ViewID, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	_ = s.cycle(context.Background())
	j, e := s.EventJob(context.Background(), s.view.ViewID, ack.JobID)
	if e != nil || j.Error != "outcome_unknown" || j.State != "failed" {
		t.Fatal("commit labeled cancellation", j, e)
	}
	if _, e = s.EventRecord(context.Background(), s.view.ViewID, ev.Saturday); !errors.Is(e, gymevent.ErrIncomplete) {
		t.Fatal("partial snapshot read", e)
	}
	b, _ := json.Marshal(j)
	if string(b) == "null" {
		t.Fatal("missing retained failure")
	}
}
