//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"path/filepath"
	"testing"
	"time"
)

func seedFixture(t *testing.T) (*eventFixture, *Store, string) {
	t.Helper()
	x := newEventFixture(t, true)
	p, e := CandidatePin(x.p)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(filepath.Dir(x.path), "seeds")
	s, e := CreateForTestClock(context.Background(), path, p, x.s, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	x.now = x.cutoff(t) + 1000
	for i := range x.entry {
		if _, e = x.s.Capture(context.Background(), x.entry[i], x.session[i], "2026-11-21"); e != nil {
			t.Fatal("genuine source cutoff", e)
		}
	}
	return x, s, path
}
func TestSeedConcretePrepareReplayReopenAndDeadline(t *testing.T) {
	x, s, path := seedFixture(t)
	ctx := context.Background()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	if r.SourceReceipt.Input.Event.CutoffMS != x.cutoff(t) || len(r.SourceReceipt.Input.Participants[0].Members) != 1 {
		t.Fatal("source receipt")
	}
	x.now++
	replay, e := s.Prepare(ctx, x.s, x.entry[1], x.session[1], "2026-11-21")
	if e != nil || !equal(replay, r) {
		t.Fatal("replay", e)
	}
	if _, e = s.Publish(ctx, x.s, x.entry[0], x.session[0], nil, "2026-11-21"); e == nil {
		t.Fatal("nil publisher")
	}
	s.Close()
	s, e = OpenForTestClock(ctx, path, s.meta.Pin, x.s, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r2, e := s.Record(ctx, x.s, x.entry[1], x.session[1], "2026-11-21")
	if e != nil || !equal(r, r2) {
		t.Fatal("reopen", e)
	}
	x.now = r.SourceReceipt.Input.Event.IncidentDeadlineMS
	status, e := s.Status(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !status.EscalationDue || status.IncidentAtMS != x.now || status.ChainObservedAtMS != 0 {
		t.Fatal("deadline", status, e)
	}
	future := "2026-11-28"
	ev, _ := gymevent.EventForDate(x.p, future)
	x.now = ev.IncidentDeadlineMS
	absent, e := s.Status(ctx, x.s, x.entry[0], x.session[0], future)
	if e != nil || absent.Exists || !absent.EscalationDue || absent.IncidentAtMS != 0 {
		t.Fatal("absent deadline", absent, e)
	}
	history, e := s.History(ctx, x.s, x.entry[0], x.session[0], "", 8)
	if e != nil || len(history.Events) != 1 || history.NextEventKey != "" {
		t.Fatal(history, e)
	}
	x.now--
	if _, e = s.Status(ctx, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrClock) {
		t.Fatal("rollback", e)
	}
}
func TestSeedPrepareLostAcknowledgementAndOriginalView(t *testing.T) {
	x, s, path := seedFixture(t)
	ctx := context.Background()
	real := s.writeBatch
	s.checkpoint = func(stage string) {
		if stage == "before-prepare" {
			s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := real(b, o); e != nil {
					return e
				}
				return errors.New("lost durable acknowledgement")
			}
		}
	}
	if _, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	s.Close()
	s, e := OpenForTestClock(ctx, path, s.meta.Pin, x.s, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || r.PreparedAtMS != x.now {
		t.Fatal(e)
	}
	x.revoke[0] = true
	if _, e = s.Record(ctx, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("revoked view", e)
	}
	x.revoke[0] = false
	// The exact zero restriction lives in the concrete entry bridge, even if a
	// trusted test lease can otherwise grant all capability names.
	for i, p := range x.p.Participants {
		err := x.entry[i].WithSeedView(ctx, x.session[i], gymaccount.SeedPublish, func(_ gymentry.SnapshotSourceInfo, check func() error) error { return check() })
		zero, _ := gymaccount.FixtureIdentity("zero256")
		if p.StablePin.Identity == zero && err != nil {
			t.Fatal(err)
		}
		if p.StablePin.Identity != zero && !errors.Is(err, gymentry.ErrForbidden) {
			t.Fatal("other gym publisher", err)
		}
	}
}

func TestSeedOfflinePreservesHistoricalFinalityAndLateEscalation(t *testing.T) {
	x, s, _ := seedFixture(t)
	ctx := context.Background()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	// State-machine injection only: source receipts are genuine; these headers
	// are synthetic and are never passed as a trusted publisher response.
	x.now++
	r = syntheticFinality(t, s.meta.Pin, r, x.now)
	r, e = s.saveAction(ctx, x.s, x.entry[0], x.session[0], gymaccount.View, r, s.meta, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	finalized := r.Finality.ObservedAtMS
	observed := r.Observation.ChainObservedAtMS
	x.now = r.SourceReceipt.Input.Event.IncidentDeadlineMS
	before := s.meta.ClockMS
	if _, e = s.Reconcile(ctx, x.s, x.entry[0], x.session[0], nil, "2026-11-21"); !errors.Is(e, ErrNodeOffline) {
		t.Fatal(e)
	}
	if s.meta.ClockMS != before {
		t.Fatal("nil capability advanced clock")
	}
	// Model the failure reporting branch after a concrete attachment was
	// established. The actual adapter/offline pipeline is tested separately.
	work, done, e := s.begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	e = s.failedChainAttempt(work, x.s, x.entry[0], x.session[0], gymaccount.View, r.EventKey, ErrNodeOffline)
	done()
	if !errors.Is(e, ErrNodeOffline) {
		t.Fatal(e)
	}
	status, e := s.Status(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || status.EscalationDue || status.IncidentAtMS != 0 || status.FinalizedAtMS != finalized || status.ChainObservedAtMS != observed || status.NodeState != "offline" || !status.CurrentFinality {
		t.Fatal("offline rewrote historical observation", status, e)
	}
	retained, e := s.Record(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || retained.Observation.ObservedAtMS != x.now || !equal(retained.Finality, r.Finality) {
		t.Fatal(e)
	}
}
func TestSeedLateFinalityCannotEraseFirstDueObservation(t *testing.T) {
	x, s, _ := seedFixture(t)
	ctx := context.Background()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	x.now = r.SourceReceipt.Input.Event.IncidentDeadlineMS
	r = syntheticFinality(t, s.meta.Pin, r, x.now)
	r, e = s.saveAction(ctx, x.s, x.entry[0], x.session[0], gymaccount.View, r, s.meta, nil, nil)
	if e != nil || r.IncidentAtMS != x.now {
		t.Fatal("late finality erased deadline", r.IncidentAtMS, e)
	}
	x.now++
	status, e := s.Status(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || status.EscalationDue || status.IncidentAtMS != x.now-1 {
		t.Fatal("resolved incident history", status, e)
	}
}
