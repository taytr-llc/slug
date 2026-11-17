//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"testing"
	"time"
)

func TestEventGenuineSourcesLateCaptureAndImmutableReplay(t *testing.T) {
	x := newEventFixture(t, true)
	ctx := context.Background()
	c := x.cutoff(t)
	if _, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrNotDue) {
		t.Fatal("early", e)
	}
	info, _ := x.s.Info(ctx)
	if info.EventCount != 0 {
		t.Fatal("early reservation")
	}
	x.now = c + 1000
	first, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal("first", e)
	}
	if first.Replayed || first.Status.Phase != "capturing" || len(first.Capture.Snapshot.Members) != 1 || first.Capture.Snapshot.Members[0].ConditionAtCutoff.AsOfMS != c {
		t.Fatal(first)
	}
	if _, e = x.s.Read(ctx, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrIncomplete) {
		t.Fatal(e)
	}
	x.reopen(t)
	x.now++
	second, e := x.s.Capture(ctx, x.entry[1], x.session[1], "2026-11-21")
	if e != nil {
		t.Fatal("second", e)
	}
	if second.Status.Phase != "snapshotted" || second.Status.SnapshotDigest == "" {
		t.Fatal(second)
	}
	original, e := x.s.Read(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = MarshalSnapshotReceipt(x.p, original); e != nil {
		t.Fatal("complete codec", e)
	}
	// Post-cutoff roster change is real, but cannot change retained noon inputs.
	x.now = c + 2000
	op, e := x.stable[0].PrepareSlot(ctx, x.session[0], gymstable.SlotRequest{ExpectedRevision: 1, Slot: 1})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.stable[0].ApplySlot(ctx, x.session[0], op.OperationID); e != nil {
		t.Fatal(e)
	}
	replay, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !replay.Replayed || !equal(replay.Capture, first.Capture) {
		t.Fatal("replay changed", e)
	}
	again, e := x.s.Read(ctx, x.entry[1], x.session[1], "2026-11-21")
	if e != nil || !equal(original, again) {
		t.Fatal("immutable receipt", e)
	}
	history, e := x.s.History(ctx, x.entry[1], x.session[1], "", 8)
	if e != nil || len(history.Events) != 1 || history.Events[0].Phase != "snapshotted" {
		t.Fatal(history, e)
	}
	x.reopen(t)
	x.now--
	if _, e = x.s.Observe(ctx, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrClock) {
		t.Fatal("rollback", e)
	}
}
func TestEventCaptureLostACKReopenAndNoRehashReplay(t *testing.T) {
	x := newEventFixture(t, false)
	ctx := context.Background()
	x.now = x.cutoff(t)
	real := x.s.writeBatch
	x.s.checkpoint = func(stage string) {
		if stage == "before-capture-commit" {
			x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := real(b, o); e != nil {
					return e
				}
				return errors.New("lost sync acknowledgement")
			}
		}
	}
	if _, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	if _, e := x.s.Info(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal(e)
	}
	x.reopen(t)
	v, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !v.Replayed || len(v.Capture.Snapshot.Members) != 0 || v.Capture.Evidence.StableFence.ObservedAtMS != x.now {
		t.Fatal(v, e)
	}
}
func TestEventFinalViewRevocationAfterCommitIsUnknown(t *testing.T) {
	x := newEventFixture(t, false)
	x.now = x.cutoff(t)
	x.s.checkpoint = func(stage string) {
		if stage == "capture-committed" {
			x.revoke[0] = true
		}
	}
	if _, e := x.s.Capture(context.Background(), x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	x.s.checkpoint = nil
	x.revoke[0] = false
	x.reopen(t)
	v, e := x.s.Capture(context.Background(), x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !v.Replayed {
		t.Fatal(v, e)
	}
}

func TestEventReplayRechecksOriginalViewAfterSemanticValidation(t *testing.T) {
	x := newEventFixture(t, false)
	x.now = x.cutoff(t)
	ctx := context.Background()
	first, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	x.s.checkpoint = func(stage string) {
		if stage == "replay-validated" {
			x.revoke[0] = true
		}
	}
	out, e := x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if !errors.Is(e, ErrUnauthorized) || out.Version != 0 {
		t.Fatal("revoked replay disclosed", out, e)
	}
	x.s.checkpoint = nil
	x.revoke[0] = false
	out, e = x.s.Capture(ctx, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !out.Replayed || !equal(first.Capture, out.Capture) {
		t.Fatal(e)
	}
}

func TestEventReservedCapacityStillCompletesExistingEvent(t *testing.T) {
	x := newEventFixture(t, false)
	ctx := context.Background()
	// Structural reservation setup only: the source proof/authority path is still
	// exercised by Capture. No invented source receipt or expected digest is used.
	batch := new(leveldb.Batch)
	start, _ := time.Parse("2006-01-02", "2026-11-21")
	var last Event
	for i := 0; i < MaxEvents; i++ {
		ev, e := EventForDate(x.p, start.AddDate(0, 0, 7*i).Format("2006-01-02"))
		if e != nil {
			t.Fatal(e)
		}
		last = ev
		if e = put(batch, key('e', ev.EventID), eventRow{Version: 1, Event: ev, Phase: "awaiting_capture", CreatedAtMS: ev.CutoffMS}, MaxEventSize); e != nil {
			t.Fatal(e)
		}
	}
	m := x.s.meta
	m.EventCount = MaxEvents
	m.ClockMS = last.CutoffMS
	if e := x.s.commitLocked(batch, m); e != nil {
		t.Fatal(e)
	}
	x.now = last.CutoffMS + 1
	x.reopen(t)
	next := start.AddDate(0, 0, 7*MaxEvents).Format("2006-01-02")
	ev, _ := EventForDate(x.p, next)
	x.now = ev.CutoffMS
	if _, e := x.s.Capture(ctx, x.entry[0], x.session[0], next); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e := x.s.Capture(ctx, x.entry[i], x.session[i], last.Saturday); e != nil {
			t.Fatal("admitted completion blocked", e)
		}
	}
	out, e := x.s.Read(ctx, x.entry[0], x.session[0], last.Saturday)
	if e != nil || out.SnapshotDigest == "" {
		t.Fatal(e)
	}
	page, e := x.s.History(ctx, x.entry[0], x.session[0], "", 8)
	if e != nil || !page.HasMore || len(page.Events) != 8 || page.NextEventID != page.Events[7].Event.EventID {
		t.Fatal(page, e)
	}
	terminal, e := x.s.History(ctx, x.entry[0], x.session[0], last.EventID, 8)
	if e != nil || terminal.HasMore || terminal.NextEventID != "" || len(terminal.Events) != 0 {
		t.Fatal(terminal, e)
	}
}

func TestEventCompleteAuditRejectsMissingCrosslinks(t *testing.T) {
	for _, kind := range []string{"missing-capture", "missing-receipt", "wrong-phase", "changed-digest", "clock-before-capture"} {
		t.Run(kind, func(t *testing.T) {
			x := newEventFixture(t, false)
			x.now = x.cutoff(t)
			ctx := context.Background()
			for i := 0; i < 2; i++ {
				if _, e := x.s.Capture(ctx, x.entry[i], x.session[i], "2026-11-21"); e != nil {
					t.Fatal(e)
				}
			}
			ev, _ := EventForDate(x.p, "2026-11-21")
			batch := new(leveldb.Batch)
			m := x.s.meta
			switch kind {
			case "missing-capture":
				batch.Delete(key('c', ev.EventID, x.p.Participants[0].StablePin.Identity.GymID))
			case "missing-receipt":
				batch.Delete(key('s', ev.EventID))
			case "wrong-phase":
				r, _, _ := x.s.event(ev)
				r.Phase = "capturing"
				r.CompletedAtMS = 0
				r.SnapshotDigest = ""
				put(batch, key('e', ev.EventID), r, MaxEventSize)
			case "changed-digest":
				r, _, _ := x.s.receipt(ev)
				r.SnapshotDigest = strings.Repeat("f", 64)
				put(batch, key('s', ev.EventID), r, MaxReceiptSize)
			case "clock-before-capture":
				m.ClockMS = ev.CutoffMS - 1
			}
			if e := x.s.commitLocked(batch, m); e != nil {
				t.Fatal(e)
			}
			x.s.Close()
			x.s = nil
			s, e := OpenForTestClock(ctx, x.path, x.p, func() time.Time { return time.UnixMilli(x.now) })
			if !errors.Is(e, ErrCorrupt) {
				if s != nil {
					s.Close()
				}
				t.Fatal(e)
			}
		})
	}
}

func TestEventCoordinatorBeforeCutoffIsNotDueDespiteAheadSource(t *testing.T) {
	x := newEventFixture(t, false)
	c := x.cutoff(t)
	x.now = c
	x.s.now = func() time.Time { return time.UnixMilli(c - 1) }
	if _, e := x.s.Capture(context.Background(), x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrNotDue) {
		t.Fatal("own cutoff precedence", e)
	}
	i, e := x.s.Info(context.Background())
	if e != nil || i.EventCount != 0 || x.s.meta.ClockMS != c-1 {
		t.Fatal(i, e)
	}
}
