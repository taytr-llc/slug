// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

type poolLeagueParentChild struct {
	*poolParentChild
	account    pilot.StableAccountConfig
	entryCalls atomic.Int32
	block      func(context.Context) error
}

func (f *poolLeagueParentChild) EntryUpcoming(ctx context.Context, view string) (gymentry.EventStatus, error) {
	f.entryCalls.Add(1)
	if f.block != nil {
		if e := f.block(ctx); e != nil {
			return gymentry.EventStatus{}, e
		}
	}
	if view != f.a.ViewID {
		return gymentry.EventStatus{}, gymstable.ErrStaleView
	}
	e := f.account.Lease(ctx, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
	return gymentry.EventStatus{}, e
}
func (f *poolLeagueParentChild) BeginEntryPrepare(ctx context.Context, view string, _ gymentry.Request) (pilot.EntryJob, error) {
	f.entryCalls.Add(1)
	if view != f.a.ViewID {
		return pilot.EntryJob{}, gymstable.ErrStaleView
	}
	e := f.account.Lease(ctx, gymaccount.EntryManage, func(_ gymaccount.Account, check func() error) error { return check() })
	return pilot.EntryJob{}, e
}
func poolLeagueParent(t *testing.T) (*PoolController, *[]*poolLeagueParentChild) {
	t.Helper()
	c, _ := poolParent(t)
	children := []*poolLeagueParentChild{}
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		f := &poolLeagueParentChild{poolParentChild: &poolParentChild{a: a.Manifest, source: c.sourceID}, account: a}
		children = append(children, f)
		return f, nil
	}
	return c, &children
}

func TestPoolLeagueParentForwardsWithoutLeaseAndRechecksOriginalSession(t *testing.T) {
	c, children := poolLeagueParent(t)
	s := poolParentLogin(t, c, "zero256")
	f := (*children)[0]
	started, release := make(chan struct{}), make(chan struct{})
	f.block = func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := c.EntryUpcoming(ctx, s.View.ViewID); done <- e }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("entry call did not start")
	}
	// A slow journal/source call must not retain either the account store gate
	// or parent mutex. A verified reset can revoke it before final disclosure.
	principal, e := verifyFixture(ctx, "zero256", true)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = c.store.ApplyReset(ctx, principal); e != nil {
		t.Fatal("reset blocked behind entry IO", e)
	}
	close(release)
	select {
	case e := <-done:
		if !errors.Is(e, gymaccount.ErrUnauthorized) {
			t.Fatal("revoked original session disclosed", e)
		}
	case <-ctx.Done():
		t.Fatal("entry did not return")
	}
	if f.entryCalls.Load() != 1 {
		t.Fatal("entry retried with substitute session")
	}
}

func TestPoolLeagueParentRejectsPreviousViewBeforeDispatch(t *testing.T) {
	c, children := poolLeagueParent(t)
	first := poolParentLogin(t, c, "zero256")
	old := (*children)[0]
	if e := c.Logout(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	second := poolParentLogin(t, c, "incrementing256")
	next := (*children)[1]
	if first.View.ViewID == second.View.ViewID {
		t.Fatal("view not replaced")
	}
	if _, e := c.EntryUpcoming(context.Background(), first.View.ViewID); !errors.Is(e, gymstable.ErrStaleView) {
		t.Fatal("old view accepted", e)
	}
	if old.entryCalls.Load() != 0 || next.entryCalls.Load() != 0 {
		t.Fatal("stale request reached a child")
	}
	if _, e := c.BeginEntryPrepare(context.Background(), second.View.ViewID, gymentry.Request{}); e != nil {
		t.Fatal("current original lease rejected", e)
	}
	if next.entryCalls.Load() != 1 {
		t.Fatal("current operation not forwarded")
	}
}

func TestPoolLeagueParentHeldSessionRetainsEntryCapability(t *testing.T) {
	c, children := poolLeagueParent(t)
	poolParentLogin(t, c, "zero256")
	if e := c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	s := c.Snapshot()
	if !s.Account.Held || s.View == nil {
		t.Fatal("reset hold missing")
	}
	if _, e := c.EntryUpcoming(context.Background(), s.View.ViewID); e != nil {
		t.Fatal("held view refused", e)
	}
	if _, e := c.BeginEntryPrepare(context.Background(), s.View.ViewID, gymentry.Request{}); e != nil {
		t.Fatal("held allowed entry capability refused", e)
	}
	if (*children)[len(*children)-1].entryCalls.Load() != 2 {
		t.Fatal("wrong child dispatched")
	}
}
