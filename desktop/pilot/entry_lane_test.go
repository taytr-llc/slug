//go:build !slugtestpow

package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func entryLaneFixture(t *testing.T) (*StableController, gymentry.Event) {
	t.Helper()
	s, _ := stableLaneFixture(t)
	ctx := context.Background()
	if e := s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.EntryPin()
	if e != nil {
		t.Fatal(e)
	}
	store, e := gymentry.Create(ctx, filepath.Join(s.config.DataDir, "entries"), p, s.stableStore)
	if e != nil {
		t.Fatal(e)
	}
	s.entryEnabled = true
	s.entryStore = store
	s.entryPin = p
	s.entryStoreID, _ = gymentry.ID(p)
	u, e := s.EntryUpcoming(ctx, s.view.ViewID)
	if e != nil {
		t.Fatal(e)
	}
	return s, u.Event
}
func entryDone(t *testing.T, s *StableController, j EntryJob) EntryJob {
	t.Helper()
	if e := s.cycle(context.Background()); e != nil {
		t.Fatal(e)
	}
	out, e := s.EntryJob(context.Background(), s.view.ViewID, j.JobID)
	if e != nil || out.State != "complete" {
		t.Fatalf("job %+v error %v", out, e)
	}
	view := s.viewPin()
	if _, e = MarshalEntryResponse(EntryResponse{ID: 1, OK: true, View: &view, StoreID: s.entryStoreID, Job: &out}); e != nil {
		t.Fatal("produced codec", e)
	}
	return out
}
func TestEntryLaneDurablePrepareApplyReplayAndStoppedMetadata(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	oldStable := s.StableSnapshot().LastJobID
	req := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	ack, e := s.BeginEntryPrepare(ctx, v, req)
	if e != nil {
		t.Fatal(e)
	}
	if s.StableSnapshot().QueuedJobID != "" || s.StableSnapshot().LastJobID != oldStable || s.PreviewSnapshot().QueuedJobID != "" {
		t.Fatal("entry leaked another family")
	}
	if _, e = s.BeginStableSync(ctx, v); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("unbounded shared queue", e)
	}
	if _, e = s.StableJob(ctx, v, ack.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("cross family query", e)
	}
	prep := entryDone(t, s, ack)
	if s.EntrySnapshot().PendingOperationID != prep.OperationID {
		t.Fatal("pending not discovered")
	}
	ack, e = s.BeginEntryApply(ctx, v, prep.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	applied := entryDone(t, s, ack)
	if applied.Mutation.Receipt.Revision != 1 || applied.Mutation.Replayed {
		t.Fatal("initial mutation")
	}
	ack, e = s.BeginEntryApply(ctx, v, prep.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	replay := entryDone(t, s, ack)
	if !replay.Mutation.Replayed || replay.Mutation.Receipt.OperationID != applied.Mutation.Receipt.OperationID || replay.Mutation.Receipt.Revision != 1 {
		t.Fatal("lost ACK replay")
	}
	s.Stop()
	pending, e := s.EntryPending(ctx, v)
	if e != nil || pending.Operation != nil {
		t.Fatal("stopped reconciliation", e)
	}
	op, e := s.EntryOperation(ctx, v, prep.OperationID)
	if e != nil || op.Operation == nil || op.Operation.State != "committed" {
		t.Fatal("stopped history", e)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if _, e = s.EntryJob(ctx, v, replay.JobID); e == nil || s.EntrySnapshot().View != nil {
		t.Fatal("revoked view disclosed cached result")
	}
}
func TestEntryLaneFailedPublicationPreservesTerminalAndCannotRun(t *testing.T) {
	s, ev := entryLaneFixture(t)
	v := s.view.ViewID
	old := EntryJob{Version: 1, JobID: strings.Repeat("9", 64), Kind: "check", State: "cancelled", View: s.viewPin(), StoreID: s.entryStoreID, EventID: ev.EventID, Saturday: ev.Saturday, Error: "entry_cancelled"}
	s.retainEntryLocked(old)
	before := s.entryBytes
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if e := original(ctx, c, fn); e != nil {
			return e
		}
		close(entered)
		<-release
		return gymaccount.ErrUnauthorized
	}
	done := make(chan error, 1)
	go func() { _, e := s.BeginEntryCheck(context.Background(), v, ev.Saturday); done <- e }()
	<-entered
	if e := s.cycle(context.Background()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.entryLast.JobID != old.JobID {
		t.Fatal("unpublished task ran")
	}
	close(release)
	if e := <-done; e == nil {
		t.Fatal("failed lease accepted")
	}
	s.stableConfig.Lease = original
	if s.queued != nil || s.entryLast.JobID != old.JobID || s.entryBytes != before {
		t.Fatal("failed publication changed terminal/reservation")
	}
}
func TestEntryLaneBudgetCancellationAndMetadataContention(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	s.laneGate <- struct{}{}
	start := time.Now()
	_, e := s.EntryPending(ctx, v)
	if !errors.Is(e, gymentry.ErrBusy) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("metadata waited behind proof", e)
	}
	<-s.laneGate
	s.cache[100] = &manifestCached{manifestBytes: MaxManifestCacheBytes - MaxEntryCacheSize}
	if _, e = s.BeginEntryCheck(ctx, v, ev.Saturday); !errors.Is(e, gymentry.ErrCapacity) || s.queued != nil {
		t.Fatal("reservation exceeded aggregate", e)
	}
	delete(s.cache, 100)
	ack, e := s.BeginEntryCheck(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	cancelled, e := s.CancelEntryJob(ctx, v, ack.JobID)
	if e != nil || cancelled.State != "cancelled" || s.entryBytes >= MaxEntryCacheSize {
		t.Fatal("cancel reservation", e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.queued != nil {
		t.Fatal("cancelled task ran")
	}
}
func TestEntryLanePostCommitLeaseLossReportsUnknown(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	req := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	ack, e := s.BeginEntryPrepare(ctx, v, req)
	if e != nil {
		t.Fatal(e)
	}
	original := s.stableConfig.Lease
	s.entryCheckpoint = func(stage string) {
		if stage == "before-entry-publication" {
			s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
				return gymaccount.ErrUnauthorized
			}
		}
	}
	if e = s.cycle(ctx); e == nil {
		t.Fatal("unknown dependency did not stop lane")
	}
	s.stableConfig.Lease = original
	s.entryCheckpoint = nil
	job, e := s.EntryJob(ctx, v, ack.JobID)
	if e != nil || job.Error != "outcome_unknown" || job.Intent != nil {
		t.Fatalf("uncertain publication %+v %v", job, e)
	}
	pending, e := s.EntryPending(ctx, v)
	if e != nil || pending.Operation == nil {
		t.Fatal("durable intent lost", e)
	}
	if pending.Operation.Intent.OperationID != job.OperationID {
		t.Fatal("reconciliation identity")
	}
}
func TestEntryLaneSavedMemberCheckAndHistoricalPending(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	id := s.last.Sync.Items[0].Assignment.ProspectID
	a, e := s.BeginStablePrepare(ctx, v, gymstable.SlotRequest{Slot: 1, ProspectID: id})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := s.StableJob(ctx, v, a.JobID)
	if e != nil || j.Intent == nil {
		t.Fatal(e)
	}
	a, e = s.BeginStableApply(ctx, v, j.Intent.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e = s.StableJob(ctx, v, a.JobID)
	if e != nil || j.Mutation == nil {
		t.Fatal(e)
	}
	r := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "replace", ExpectedRosterRevision: j.Mutation.Receipt.Revision, ProspectIDs: []string{id}}
	ack, e := s.BeginEntryPrepare(ctx, v, r)
	if e != nil {
		t.Fatal(e)
	}
	p := entryDone(t, s, ack)
	ack, e = s.BeginEntryApply(ctx, v, p.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	saved := entryDone(t, s, ack)
	if len(saved.Mutation.Receipt.Members) != 1 {
		t.Fatal("saved member missing")
	}
	ack, e = s.BeginEntryCheck(ctx, v, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	checked := entryDone(t, s, ack)
	if len(checked.Check.Members) != 1 || checked.Check.Members[0].Eligibility != "eligible" || checked.Check.Members[0].Item.Receipt().Version != 0 {
		t.Fatal("check evidence")
	}
	r.Action = "withdraw"
	r.ExpectedRevision = 1
	r.ExpectedRosterRevision = 0
	r.ProspectIDs = []string{}
	ack, e = s.BeginEntryPrepare(ctx, v, r)
	if e != nil {
		t.Fatal(e)
	}
	next := entryDone(t, s, ack)
	if _, e = s.AbandonEntryOperation(ctx, v, p.OperationID); e != nil {
		t.Fatal(e)
	}
	if s.EntrySnapshot().PendingOperationID != next.OperationID {
		t.Fatal("historical abandon erased newer pending")
	}
}
func TestEntryLaneFamiliesShareOneActiveAndOneQueued(t *testing.T) {
	for _, entryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-first", true: "entry-first"}[entryFirst], func(t *testing.T) {
			s, ev := entryLaneFixture(t)
			ctx := context.Background()
			v := s.view.ViewID
			entered, release := make(chan struct{}), make(chan struct{})
			if entryFirst {
				s.entryCheckpoint = func(stage string) {
					if stage == "before-entry" {
						close(entered)
						<-release
					}
				}
				if _, e := s.BeginEntryCheck(ctx, v, ev.Saturday); e != nil {
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
			if entryFirst {
				if _, e := s.BeginStableSync(ctx, v); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := s.BeginEntryCheck(ctx, v, ev.Saturday); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := s.BeginEntryCheck(ctx, v, ev.Saturday); !errors.Is(e, gymentry.ErrBusy) {
				t.Fatal("second queue", e)
			}
			es, ss := s.EntrySnapshot(), s.StableSnapshot()
			if entryFirst {
				if es.ActiveJobID == "" || ss.ActiveJobID != "" || ss.QueuedJobID == "" {
					t.Fatal("entry active family IDs")
				}
			} else {
				if es.ActiveJobID != "" || ss.ActiveJobID == "" || es.QueuedJobID == "" {
					t.Fatal("stable active family IDs")
				}
			}
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			s.entryCheckpoint = nil
			s.laneCheckpoint = nil
			if e := s.cycle(ctx); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestEntryLaneStopJoinsActiveBeforeClosingStores(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	s.entryCheckpoint = func(stage string) {
		if stage == "before-entry" {
			close(entered)
			<-release
		}
	}
	ack, e := s.BeginEntryCheck(ctx, s.view.ViewID, ev.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	run := make(chan error, 1)
	go func() { run <- s.cycle(ctx) }()
	<-entered
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop did not join work")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e = <-run; e != nil {
		t.Fatal(e)
	}
	<-stopped
	s.entryCheckpoint = nil
	j, e := s.EntryJob(ctx, s.view.ViewID, ack.JobID)
	if e != nil || j.State != "cancelled" {
		t.Fatal("joined cancellation", j, e)
	}
	if _, e = s.EntryPending(ctx, s.view.ViewID); e != nil {
		t.Fatal("stopped store unavailable", e)
	}
}

func TestEntryLaneCommittedWrapperValidationFailureIsUnknown(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	r := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	ack, e := s.BeginEntryPrepare(ctx, v, r)
	if e != nil {
		t.Fatal(e)
	}
	// Inject corruption only into the queued public wrapper, leaving the real
	// session and core request unchanged. This is not an authorization bypass.
	s.stableMu.Lock()
	s.queued.entry.View.Generation++
	s.stableMu.Unlock()
	if e = s.cycle(ctx); e == nil {
		t.Fatal("corrupt publication did not stop lane")
	}
	j, e := s.EntryJob(ctx, v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "outcome_unknown" || j.Intent != nil || j.Mutation != nil {
		t.Fatalf("known commit reported definite failure: state=%s code=%s err=%v", j.State, j.Error, e)
	}
	pending, e := s.EntryPending(ctx, v)
	if e != nil || pending.Operation == nil || pending.Operation.Intent.OperationID != j.OperationID || pending.Operation.Intent.Generation != s.view.Generation {
		t.Fatal("durable original intent lost", e)
	}
}
func TestEntryAbandonPostCommitCancellationReconciles(t *testing.T) {
	s, ev := entryLaneFixture(t)
	ctx := context.Background()
	v := s.view.ViewID
	r := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	ack, e := s.BeginEntryPrepare(ctx, v, r)
	if e != nil {
		t.Fatal(e)
	}
	prep := entryDone(t, s, ack)
	callctx, cancel := context.WithCancel(ctx)
	defer cancel()
	original := s.stableConfig.Lease
	manageCalls := 0
	s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		e := original(ctx, c, fn)
		if c == gymaccount.EntryManage {
			manageCalls++
			if manageCalls == 2 && e == nil {
				cancel()
			}
		}
		return e
	}
	result, e := s.AbandonEntryOperation(callctx, v, prep.OperationID)
	s.stableConfig.Lease = original
	if !errors.Is(e, gymentry.ErrOutcomeUnknown) || result.Version != 0 || manageCalls != 2 {
		t.Fatal("postcommit cancellation misclassified", e, manageCalls)
	}
	recovered, e := s.EntryOperation(ctx, v, prep.OperationID)
	if e != nil || recovered.Operation == nil || recovered.Operation.State != "abandoned" {
		t.Fatal("exact operation reconciliation", e)
	}
	pending, e := s.EntryPending(ctx, v)
	if e != nil || pending.Operation != nil {
		t.Fatal("durable pending not cleared", e)
	}
}
