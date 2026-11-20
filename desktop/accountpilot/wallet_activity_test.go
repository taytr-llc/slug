// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"sync"
	"testing"
	"time"
)

type activityParentChild struct {
	*poolParentChild
	pilot.WalletActivitySurfaceAPI
	account pilot.StableAccountConfig
	block   func(context.Context) error
}

func (f *activityParentChild) BeginWalletActivity(ctx context.Context, v, id string) (pilot.WalletActivityJob, error) {
	if f.block != nil {
		if e := f.block(ctx); e != nil {
			return pilot.WalletActivityJob{}, e
		}
	}
	if v != f.a.ViewID {
		return pilot.WalletActivityJob{}, gymstable.ErrStaleView
	}
	e := f.account.HistoryPublicationLease(ctx, func(_ gymaccount.Account, check func() error) error { return check() })
	return pilot.WalletActivityJob{}, e
}
func activityParent(t *testing.T) (*PoolController, *[]*activityParentChild) {
	c, _ := poolParent(t)
	children := []*activityParentChild{}
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		f := &activityParentChild{poolParentChild: &poolParentChild{a: a.Manifest, source: c.sourceID}, account: a}
		children = append(children, f)
		return f, nil
	}
	return c, &children
}
func TestWalletActivityParentResetDuringReadNoLeaseAndNoReplacement(t *testing.T) {
	c, children := activityParent(t)
	snapshot := poolParentLogin(t, c, "zero256")
	f := (*children)[0]
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.block = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := c.BeginWalletActivity(ctx, snapshot.View.ViewID, "capture"); done <- e }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("child did not enter")
	}
	principal, e := verifyFixture(ctx, "zero256", true)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = c.store.ApplyReset(ctx, principal); e != nil {
		t.Fatal("read held account lease", e)
	}
	once.Do(func() { close(release) })
	select {
	case e = <-done:
		if !errors.Is(e, gymaccount.ErrUnauthorized) {
			t.Fatal("old view returned after reset", e)
		}
	case <-ctx.Done():
		t.Fatal("read did not join")
	}
}
func TestWalletActivityParentHeldViewIsReadOnly(t *testing.T) {
	c, _ := activityParent(t)
	old := poolParentLogin(t, c, "zero256")
	if e := c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	held := c.Snapshot()
	if held.View == nil || !held.Account.Held || held.Account.CanExport {
		t.Fatal("held fixture unavailable")
	}
	if _, e := c.BeginWalletActivity(t.Context(), old.View.ViewID, "old"); !errors.Is(e, gymstable.ErrStaleView) {
		t.Fatal("old view substituted", e)
	}
	if _, e := c.BeginWalletActivity(t.Context(), held.View.ViewID, "new"); e != nil {
		t.Fatal("held View refused", e)
	}
	c.mu.Lock()
	token := c.session.Token
	c.mu.Unlock()
	e := c.store.WithSession(t.Context(), token, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() })
	if !errors.Is(e, gymaccount.ErrHeld) {
		t.Fatal("recovery hold weakened", e)
	}
}
