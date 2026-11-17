// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"testing"
)

func TestEntryCutoffReplaysOnlyCommittedTargetEvent(t *testing.T) {
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	r := x.request(t, "withdraw", 0)
	ev, _ := EventForDate(x.pin, r.Saturday)
	first, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	a, e := x.s.Apply(ctx, x.session, first.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	r.ExpectedRevision = 1
	pending, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	before, e := x.s.cutoffEntry(ctx, ev)
	if e != nil || before.CommittedCount != 1 || before.LastSequence != 1 || before.LatestReceipt == nil || !equal(before.LatestReceipt, &a.Receipt) {
		t.Fatal(before, e)
	}
	// Resolving a pre-cutoff intent after C is excluded from the committed prefix.
	x.now = ev.CutoffMS
	if _, e = x.s.Abandon(ctx, x.session, pending.OperationID); e != nil {
		t.Fatal(e)
	}
	after, e := x.s.cutoffEntry(ctx, ev)
	if e != nil || !equal(before, after) {
		t.Fatal("abandon changed past", e)
	}
	// A later event commit participates in global audit but never in this root.
	future := x.request(t, "withdraw", 0)
	if future.EventID == r.EventID {
		t.Fatal("date did not roll")
	}
	next, e := x.s.Prepare(ctx, x.session, future)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.Apply(ctx, x.session, next.OperationID); e != nil {
		t.Fatal(e)
	}
	final, e := x.s.cutoffEntry(ctx, ev)
	if e != nil || !equal(before, final) {
		t.Fatal("future event changed past", e)
	}
}
func TestEntryCutoffRejectsMissingCrosslinksAndRegression(t *testing.T) {
	for _, kind := range []string{"missing-index", "wrong-index", "missing-operation", "event-revision"} {
		t.Run(kind, func(t *testing.T) {
			x := newEntryFixture(t, 0)
			ctx := context.Background()
			r := x.request(t, "withdraw", 0)
			ev, _ := EventForDate(x.pin, r.Saturday)
			in, e := x.s.Prepare(ctx, x.session, r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = x.s.Apply(ctx, x.session, in.OperationID); e != nil {
				t.Fatal(e)
			}
			b := new(leveldb.Batch)
			switch kind {
			case "missing-index":
				b.Delete(seqKey(1))
			case "wrong-index":
				b.Put(seqKey(1), make([]byte, 32))
			case "missing-operation":
				b.Delete(key("o", in.OperationID))
			case "event-revision":
				row, _, e := x.s.event(ev)
				if e != nil {
					t.Fatal(e)
				}
				row.Revision = 0
				row.LatestOperationID = ""
				put(b, key("e", ev.EventID), row, MaxEventSize)
			}
			if e = x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			if _, e = x.s.cutoffEntry(ctx, ev); !errors.Is(e, ErrCorrupt) {
				t.Fatal(e)
			}
		})
	}
}
func TestEntryCutoffClockDisagreementCannotCallPreparation(t *testing.T) {
	x := newEntryFixture(t, 0)
	r := x.request(t, "withdraw", 0)
	ev, _ := EventForDate(x.pin, r.Saturday)
	x.now = ev.CutoffMS
	e := x.s.WithCutoffSnapshot(context.Background(), x.session, r.Saturday, func(CutoffSnapshot) (func(CutoffFences, func() error) error, error) {
		t.Fatal("source below C reached prepare")
		return nil, nil
	})
	if !errors.Is(e, ErrClock) {
		t.Fatal(e)
	}
	if x.s.meta.ClockMS < ev.CutoffMS {
		t.Fatal("entry observation was not retained")
	}
}
