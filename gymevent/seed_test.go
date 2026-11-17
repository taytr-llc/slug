//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"testing"
)

func TestSeedSourceOriginalViewOwnedPinAndForeignCallback(t *testing.T) {
	x := newEventFixture(t, false)
	ctx := context.Background()
	x.now = x.cutoff(t)
	for i := range x.entry {
		if _, e := x.s.Capture(ctx, x.entry[i], x.session[i], "2026-11-21"); e != nil {
			t.Fatal(e)
		}
	}
	before, e := x.s.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.s.WithSeedView(ctx, x.entry[0], x.session[0], gymaccount.View, func(v SeedSourceInfo, check func() error) error {
		v.Pin.Participants[0].StablePin.AccountID = "changed"
		return check()
	}); e != nil {
		t.Fatal(e)
	}
	after, _ := x.s.Info(ctx)
	if !equal(before.Pin, after.Pin) {
		t.Fatal("foreign callback aliased source pin")
	}
	committed := false
	e = x.s.WithSeedSnapshot(ctx, x.entry[0], x.session[0], "2026-11-21", func(v SnapshotReceipt) (func(SeedSourceInfo, func() error) error, error) {
		if v.SnapshotDigest == "" {
			t.Fatal("missing concrete receipt")
		}
		x.revoke[0] = true
		return func(_ SeedSourceInfo, _ func() error) error { committed = true; return nil }, nil
	})
	if !errors.Is(e, ErrUnauthorized) || committed {
		t.Fatal("old-view final callback", e, committed)
	}
	x.revoke[0] = false
	sentinel := errors.New("foreign commit refused")
	e = x.s.WithSeedSnapshot(ctx, x.entry[0], x.session[0], "2026-11-21", func(_ SnapshotReceipt) (func(SeedSourceInfo, func() error) error, error) {
		return func(_ SeedSourceInfo, check func() error) error {
			if e := check(); e != nil {
				return e
			}
			return sentinel
		}, nil
	})
	if !errors.Is(e, sentinel) {
		t.Fatal("lost foreign error", e)
	}
	if e = x.s.WithSeedView(ctx, x.entry[0], x.session[0], gymaccount.Withdrawal, func(_ SeedSourceInfo, _ func() error) error { t.Fatal("unrelated capability invoked"); return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
