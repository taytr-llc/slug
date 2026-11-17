// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"strings"
	"testing"
	"time"
)

func TestPreviewPairBoundsClassCancellation(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	mustSync(t, x)
	ctx := context.Background()
	calls := 0
	x.s.proofRead = func() { calls++ }
	ids := [2]string{x.records[0].Verified.ProspectID, x.records[1].Verified.ProspectID}
	for _, bad := range [][2]string{{ids[0], ids[0]}, {strings.ToUpper(ids[0]), ids[1]}, {ids[0], strings.Repeat("0", 64)}, {ids[0], strings.Repeat("f", 64)}} {
		p, e := x.s.ReadPreviewPair(ctx, x.session, bad)
		if e == nil || p.Version != 0 || calls != 0 {
			t.Fatal("cheap refusal", e, calls)
		}
	}
	p, e := x.s.ReadPreviewPair(ctx, x.session, ids)
	if !errors.Is(e, ErrClassMismatch) || p.Version != 0 || calls != 2 {
		t.Fatal("verified class mismatch", e, calls)
	}
	calls = 0
	c, cancel := context.WithCancel(ctx)
	x.s.proofRead = func() { calls++; cancel() }
	p, e = x.s.ReadPreviewPair(c, x.session, ids)
	if !errors.Is(e, ErrCancelled) || p.Version != 0 || calls != 1 {
		t.Fatal("cancel", e, calls)
	}
	calls = 0
	x.s.proofRead = func() { calls++ }
	expired := x.session
	expired.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return ErrUnauthorized
	}
	p, e = x.s.ReadPreviewPair(ctx, expired, ids)
	if e == nil || p.Version != 0 || calls != 0 {
		t.Fatal("expired", e, calls)
	}
}
func TestPreviewPairCommonTimeAndPreservedAssignments(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 6)
	sync := mustSync(t, x)
	var ids [2]string
	found := false
	for i, a := range sync.Items {
		for _, b := range sync.Items[i+1:] {
			if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
				ids = [2]string{a.Assignment.ProspectID, b.Assignment.ProspectID}
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("six records must include a shared class")
	}
	// Both are deliberately off-roster and observed after every career endpoint.
	x.now += int64(100) * 604800000
	x.s.now = func() time.Time { return time.UnixMilli(x.now) }
	before := x.s.meta
	roster := x.s.roster
	calls := 0
	x.s.proofRead = func() { calls++ }
	p, e := x.s.ReadPreviewPair(context.Background(), x.session, ids)
	if e != nil {
		t.Fatal(e)
	}
	if calls != 2 || p.Fighters[0].Assignment.ProspectID >= p.Fighters[1].Assignment.ProspectID || p.AsOfMS != x.now || p.Status.LastAsOfMS != x.now {
		t.Fatal("pair pins", calls, p.AsOfMS)
	}
	for _, f := range p.Fighters {
		if f.Condition.AsOfMS != x.now || f.Condition.ProXP != 0 || f.Condition.Phase != "retired" {
			t.Fatal("shared retired projection", f.Condition)
		}
	}
	after := x.s.meta
	after.ClockMS = before.ClockMS
	after.LastAsOfMS = before.LastAsOfMS
	if after != before || x.s.roster != roster {
		t.Fatal("observation changed assignment/roster metadata")
	}
	calls = 0
	reversed, e := x.s.ReadPreviewPair(context.Background(), x.session, [2]string{ids[1], ids[0]})
	if e != nil || calls != 2 || reversed.Fighters[0].Assignment != p.Fighters[0].Assignment || reversed.Fighters[1].Assignment != p.Fighters[1].Assignment || reversed.Fighters[0].Condition != p.Fighters[0].Condition {
		t.Fatal("reverse", e, calls)
	}
}

func TestPreviewPairFinalLeaseAndUnknownObservation(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 6)
	synced := mustSync(t, x)
	var ids [2]string
	for i, a := range synced.Items {
		for _, b := range synced.Items[i+1:] {
			if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
				ids = [2]string{a.Assignment.ProspectID, b.Assignment.ProspectID}
			}
		}
	}
	original := x.session.Lease
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	for _, refuse := range []int{3, 4} {
		calls := 0
		proofs = 0
		session := x.session
		session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			calls++
			if calls == refuse {
				return ErrUnauthorized
			}
			return original(ctx, cap, fn)
		}
		p, e := x.s.ReadPreviewPair(context.Background(), session, ids)
		want := 1
		if refuse == 4 {
			want = 2
		}
		if !errors.Is(e, ErrUnauthorized) || p.Version != 0 || proofs != want {
			t.Fatal("original lease boundary", refuse, calls, proofs, e)
		}
	}
	originalWrite := x.s.writeBatch
	x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if e := originalWrite(b, o); e != nil {
			return e
		}
		return errors.New("lost sync acknowledgement")
	}
	p, e := x.s.ReadPreviewPair(context.Background(), x.session, ids)
	if !errors.Is(e, ErrOutcomeUnknown) || p.Version != 0 || !x.s.poisoned {
		t.Fatal("observation uncertainty", e)
	}
}

func TestPreviewPairAssignmentBindingAndBacklog(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 6)
	synced := mustSync(t, x)
	var ids [2]string
	for i, a := range synced.Items {
		for _, b := range synced.Items[i+1:] {
			if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
				ids = [2]string{a.Assignment.ProspectID, b.Assignment.ProspectID}
			}
		}
	}
	expandCoreFixture(t, x, 7)
	calls := 0
	x.s.proofRead = func() { calls++ }
	p, e := x.s.ReadPreviewPair(context.Background(), x.session, ids)
	if e != nil || calls != 2 || p.Status.Checkpoint != 6 || p.Status.ObservedSourceCount != 7 || p.Status.CaughtUp || p.Status.OperationCount != 0 || p.Status.RosterRevision != 0 {
		t.Fatal("backlog unexpectedly synchronized", p.Status, calls, e)
	}
	a, e := x.s.assignment(p.Fighters[0].Assignment.ProspectID)
	if e != nil {
		t.Fatal(e)
	}
	a.SourceRecordDigest = strings.Repeat("f", 64)
	b := new(leveldb.Batch)
	if e = encodedPut(b, seqKey(a.Sequence), a, MaxAssignmentSize); e != nil {
		t.Fatal(e)
	}
	if e = x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	calls = 0
	p, e = x.s.ReadPreviewPair(context.Background(), x.session, ids)
	if !errors.Is(e, ErrCorrupt) || p.Version != 0 || calls != 1 {
		t.Fatal("consumed binding changed", calls, e)
	}
}
