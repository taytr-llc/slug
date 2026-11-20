// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"testing"
	"time"
)

func TestPoolPipelineParentPurposeLeasesRemainOriginal(t *testing.T) {
	c, children := poolLeagueParent(t)
	poolParentLogin(t, c, "zero256")
	original := (*children)[0]
	if original.account.HistoryPublicationLease == nil || original.account.ResultNodeLease == nil {
		t.Fatal("missing purpose lease")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	check := func(_ gymaccount.Account, guard func() error) error { return guard() }
	if e := original.account.HistoryPublicationLease(ctx, check); e != nil {
		t.Fatal(e)
	}
	if e := original.account.ResultNodeLease(ctx, check); e != nil {
		t.Fatal(e)
	}
	if e := c.Logout(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	poolParentLogin(t, c, "incrementing256")
	for _, lease := range []func(context.Context, func(gymaccount.Account, func() error) error) error{original.account.HistoryPublicationLease, original.account.ResultNodeLease} {
		called := false
		e := lease(ctx, func(gymaccount.Account, func() error) error { called = true; return nil })
		if !errors.Is(e, gymaccount.ErrUnauthorized) || called {
			t.Fatal("old purpose lease substituted current account", e)
		}
	}
}
func TestPoolPipelineParentHoldBlocksPublicationButAllowsHistory(t *testing.T) {
	c, children := poolLeagueParent(t)
	poolParentLogin(t, c, "zero256")
	if e := c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	held := (*children)[len(*children)-1]
	if !c.Snapshot().Account.Held {
		t.Fatal("missing hold")
	}
	ctx := context.Background()
	called := false
	for _, capability := range []gymaccount.Capability{gymaccount.SeedPublish, gymaccount.ResultPublish} {
		e := held.account.Lease(ctx, capability, func(gymaccount.Account, func() error) error { called = true; return nil })
		if e == nil || called {
			t.Fatal("held publication allowed")
		}
	}
	if e := held.account.HistoryPublicationLease(ctx, func(_ gymaccount.Account, guard func() error) error { return guard() }); e != nil {
		t.Fatal("held history refused", e)
	}
}
func TestPoolPipelineParentHistoryPublicationFencesTransition(t *testing.T) {
	c, children := poolLeagueParent(t)
	poolParentLogin(t, c, "zero256")
	f := (*children)[0]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.mu.Lock()
	c.operation = true
	c.mu.Unlock()
	called := false
	e := f.account.HistoryPublicationLease(ctx, func(gymaccount.Account, func() error) error { called = true; return nil })
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
	if !errors.Is(e, gymaccount.ErrUnauthorized) || called {
		t.Fatal("publication escaped parent transition", e)
	}
	// Native purpose admission is equally narrow; it cannot create a new node owner.
	c.mu.Lock()
	c.operation = true
	c.mu.Unlock()
	e = f.account.ResultNodeLease(ctx, func(gymaccount.Account, func() error) error { called = true; return nil })
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
	if !errors.Is(e, gymaccount.ErrUnauthorized) || called {
		t.Fatal("node lease escaped transition", e)
	}
}

var _ pilot.PoolSurfaceAPI = (*PoolController)(nil)
