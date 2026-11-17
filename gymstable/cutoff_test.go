// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"sync/atomic"
	"testing"
	"time"
)

func cutoffAssign(t *testing.T, x *coreFixture, rev uint64, slot uint32, id string) MutationResult {
	t.Helper()
	in, e := x.s.PrepareSlot(context.Background(), x.session, SlotRequest{rev, slot, id})
	if e != nil {
		t.Fatal(e)
	}
	r, e := x.s.ApplySlot(context.Background(), x.session, in.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func cutoffRead(t *testing.T, x *coreFixture, c int64, ids []string) (CutoffSelection, CutoffFence) {
	t.Helper()
	var selected CutoffSelection
	var fence CutoffFence
	e := x.s.WithCutoffSelection(context.Background(), x.session, c, ids, func(v CutoffSelection) (func(CutoffFence, func() error) error, error) {
		selected = v
		return func(f CutoffFence, check func() error) error { fence = f; return check() }, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return selected, fence
}
func TestStableCutoffNoopAndPostCutoffIntentDoNotRewriteHistory(t *testing.T) {
	x := newCoreFixture(t, "zero256", 1)
	items := mustSync(t, x)
	id := items.Items[0].Assignment.ProspectID
	x.now += 100
	first := cutoffAssign(t, x, 0, 1, id)
	x.now++
	noop := cutoffAssign(t, x, 1, 1, id)
	if noop.Receipt.Changed {
		t.Fatal("no-op changed")
	}
	c := x.now + 1
	// Prepared before C, committed exactly C: final historical snapshot excludes it.
	in, e := x.s.PrepareSlot(context.Background(), x.session, SlotRequest{ExpectedRevision: 1, Slot: 1})
	if e != nil {
		t.Fatal(e)
	}
	x.now = c
	before, f := cutoffRead(t, x, c, []string{id})
	if before.Roster.CommittedCount != 2 || before.Roster.Revision != 1 || before.Roster.Slots[0] != id || before.Roster.AssignedAtMS[0] != first.Receipt.CommittedAtMS || f.ObservedAtMS != c || f.OperationCount != 3 {
		t.Fatal(before, f)
	}
	if _, e = x.s.ApplySlot(context.Background(), x.session, in.OperationID); e != nil {
		t.Fatal(e)
	}
	after, _ := cutoffRead(t, x, c, []string{id})
	if !equalEncoded(before, after) {
		t.Fatal("post-C committed intent changed historical input")
	}
	x.reopen(t)
	again, _ := cutoffRead(t, x, c, []string{id})
	if !equalEncoded(before, again) {
		t.Fatal("reopen changed history")
	}
	x.now = c - 1
	if e = x.s.WithCutoffSelection(context.Background(), x.session, c, nil, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
		t.Fatal("rollback reached preparation")
		return nil, nil
	}); !errors.Is(e, ErrClock) {
		t.Fatal(e)
	}
}
func TestStableCutoffDrainsPreCutoffSampledWriter(t *testing.T) {
	x := newCoreFixture(t, "zero256", 1)
	id := mustSync(t, x).Items[0].Assignment.ProspectID
	var now atomic.Int64
	now.Store(x.now + 100)
	x.s.now = func() time.Time { return time.UnixMilli(now.Load()) }
	c := now.Load() + 1
	in, e := x.s.PrepareSlot(context.Background(), x.session, SlotRequest{Slot: 1, ProspectID: id})
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	x.s.checkpoint = func(stage string) {
		if stage == "before_apply" {
			close(entered)
			<-release
		}
	}
	apply := make(chan error, 1)
	go func() { _, e := x.s.ApplySlot(context.Background(), x.session, in.OperationID); apply <- e }()
	<-entered
	now.Store(c)
	selection := make(chan CutoffSelection, 1)
	result := make(chan error, 1)
	go func() {
		result <- x.s.WithCutoffSelection(context.Background(), x.session, c, []string{id}, func(v CutoffSelection) (func(CutoffFence, func() error) error, error) {
			selection <- v
			return func(_ CutoffFence, check func() error) error { return check() }, nil
		})
	}()
	select {
	case <-selection:
		t.Fatal("capture bypassed writer gate")
	case e := <-result:
		t.Fatal("capture returned while writer held", e)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e = <-apply; e != nil {
		t.Fatal(e)
	}
	if e = <-result; e != nil {
		t.Fatal(e)
	}
	v := <-selection
	if v.Roster.Slots[0] != id || v.Roster.AssignedAtMS[0] != c-1 || v.Roster.CommittedCount != 1 {
		t.Fatal(v)
	}
}
func TestStableCutoffRejectsCorruptHistoryAndFuturePlacement(t *testing.T) {
	for _, kind := range []string{"missing-operation", "duplicate-sequence", "assignment-after-placement"} {
		t.Run(kind, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 1)
			id := mustSync(t, x).Items[0].Assignment.ProspectID
			x.now += 100
			first := cutoffAssign(t, x, 0, 1, id)
			x.now++
			second := cutoffAssign(t, x, 1, 1, id)
			x.now++
			b := new(leveldb.Batch)
			switch kind {
			case "missing-operation":
				b.Delete(idKey('o', first.Receipt.OperationID))
			case "duplicate-sequence":
				o, e := x.s.operation(second.Receipt.OperationID)
				if e != nil {
					t.Fatal(e)
				}
				o.Intent.Sequence = 1
				if e = encodedPut(b, idKey('o', second.Receipt.OperationID), o, MaxOperationSize); e != nil {
					t.Fatal(e)
				}
			case "assignment-after-placement":
				a, e := x.s.assignment(id)
				if e != nil {
					t.Fatal(e)
				}
				a.AcceptedAtMS = first.Receipt.CommittedAtMS + 1
				encodedPut(b, seqKey(a.Sequence), a, MaxAssignmentSize)
			}
			if e := x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			e := x.s.WithCutoffSelection(context.Background(), x.session, x.now, []string{id}, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
				t.Fatal("corrupt history disclosed")
				return nil, nil
			})
			if !errors.Is(e, ErrCorrupt) {
				t.Fatal(e)
			}
		})
	}
}
func TestStableCutoffFenceUncertainWritePreventsForeignCommit(t *testing.T) {
	x := newCoreFixture(t, "zero256", 0)
	c := x.now
	x.now++
	real := x.s.writeBatch
	calls := 0
	e := x.s.WithCutoffSelection(context.Background(), x.session, c, nil, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
		x.now++
		x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
			if e := real(b, o); e != nil {
				return e
			}
			return errors.New("lost fence ACK")
		}
		return func(CutoffFence, func() error) error { calls++; return nil }, nil
	})
	if !errors.Is(e, ErrOutcomeUnknown) || calls != 0 || !x.s.poisoned {
		t.Fatal(e, calls)
	}
	x.reopen(t)
	if x.s.meta.ClockMS != x.now {
		t.Fatal("lost committed fence")
	}
}
