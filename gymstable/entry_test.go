// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"sort"
	"strings"
	"testing"
)

func TestStableEntrySelectionSourceFenceAndNoWriteAfterCallback(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	ctx := context.Background()
	synced := mustSync(t, x)
	ids := []string{}
	for _, i := range synced.Items {
		ids = append(ids, i.Assignment.ProspectID)
	}
	sort.Strings(ids)
	info, e := x.s.EntrySourceInfo(ctx)
	if e != nil || info.Checkpoint != 3 {
		t.Fatal(info, e)
	}
	before := x.s.meta
	writes := 0
	real := x.s.writeBatch
	x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error { writes++; return real(b, o) }
	v, e := x.s.ReadEntrySelection(ctx, x.session, ids)
	if e != nil || len(v.Members) != 3 {
		t.Fatal(e)
	}
	for _, m := range v.Members {
		if m.Slot != 0 || m.Item.Receipt().Version != 0 {
			t.Fatal("private source evidence exposed")
		}
	}
	if v.Status.Checkpoint != before.Checkpoint || v.Status.RosterRevision != before.RosterRevision {
		t.Fatal("selection mutated authority")
	}
	called := 0
	afterCount := 0
	e = x.s.WithEntrySelection(ctx, x.session, 0, ids, func(sel EntrySelection, check func() error) error {
		called++
		afterCount = writes
		if sel.AsOfMS != x.now || len(sel.Members) != 3 {
			t.Fatal(sel)
		}
		return check()
	})
	if e != nil || called != 1 || writes != afterCount {
		t.Fatal("post-callback write", called, writes, afterCount, e)
	}
	sentinel := errors.New("entry callback rejected")
	original := x.session.Lease
	bad := x.session
	bad.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		_ = original(ctx, c, fn)
		return nil
	}
	if e = x.s.WithEntrySelection(ctx, bad, 0, ids, func(EntrySelection, func() error) error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal("callback swallowed", e)
	}
	if _, e = x.s.ReadEntrySelection(ctx, x.session, []string{strings.Repeat("f", 64)}); !errors.Is(e, ErrSyncRequired) {
		t.Fatal("unconsumed", e)
	}
	if e = x.s.WithEntrySelection(ctx, x.session, 1, ids, func(EntrySelection, func() error) error { t.Fatal("stale revision entered"); return nil }); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}
func TestStableEntrySelectionCancellationAndAuthorizationBetweenProofs(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	synced := mustSync(t, x)
	ids := []string{}
	for _, i := range synced.Items {
		ids = append(ids, i.Assignment.ProspectID)
	}
	sort.Strings(ids)
	ctx, cancel := context.WithCancel(context.Background())
	proofs := 0
	x.s.proofRead = func() {
		proofs++
		if proofs == 1 {
			cancel()
		}
	}
	if _, e := x.s.ReadEntrySelection(ctx, x.session, ids); !errors.Is(e, ErrCancelled) || proofs != 1 {
		t.Fatal("cancel did not bound source work", proofs, e)
	}
	x.s.proofRead = nil
	calls := 0
	session := x.session
	original := session.Lease
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		calls++
		if calls == 3 {
			return gymaccount.ErrUnauthorized
		}
		return original(ctx, cap, fn)
	}
	proofs = 0
	x.s.proofRead = func() { proofs++ }
	if _, e := x.s.ReadEntrySelection(context.Background(), session, ids); !errors.Is(e, ErrUnauthorized) || proofs != 1 {
		t.Fatal("authorization not rechecked between proofs", calls, proofs, e)
	}
}
