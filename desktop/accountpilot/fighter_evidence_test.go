// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type evidenceParentChild struct {
	*poolParentChild
	pilot.FighterEvidenceSurfaceAPI
	account pilot.StableAccountConfig
	calls   atomic.Int32
	block   func(context.Context) error
}

func (f *evidenceParentChild) BeginFighterEvidence(ctx context.Context, v, hjob, hid, id string, _ recoveryfile.Pin) (pilot.FighterEvidenceJob, error) {
	f.calls.Add(1)
	if f.block != nil {
		if e := f.block(ctx); e != nil {
			return pilot.FighterEvidenceJob{}, e
		}
	}
	if v != f.a.ViewID {
		return pilot.FighterEvidenceJob{}, gymstable.ErrStaleView
	}
	e := f.account.HistoryPublicationLease(ctx, func(_ gymaccount.Account, check func() error) error { return check() })
	return pilot.FighterEvidenceJob{}, e
}
func evidenceParent(t *testing.T) (*PoolController, *[]*evidenceParentChild) {
	c, _ := poolParent(t)
	children := []*evidenceParentChild{}
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		f := &evidenceParentChild{poolParentChild: &poolParentChild{a: a.Manifest, source: c.sourceID}, account: a}
		children = append(children, f)
		return f, nil
	}
	return c, &children
}
func TestFighterEvidenceParentOriginalViewNoLeaseAcrossIO(t *testing.T) {
	c, children := evidenceParent(t)
	s := poolParentLogin(t, c, "zero256")
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
	go func() {
		_, e := c.BeginFighterEvidence(ctx, s.View.ViewID, "history", "capture", "export", recoveryfile.Pin{})
		done <- e
	}()
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
		t.Fatal("account lease retained during IO", e)
	}
	once.Do(func() { close(release) })
	select {
	case e = <-done:
		if !errors.Is(e, gymaccount.ErrUnauthorized) {
			t.Fatal("revoked result disclosed", e)
		}
	case <-ctx.Done():
		t.Fatal("child did not finish")
	}
	if f.calls.Load() != 1 {
		t.Fatal("substitute dispatch")
	}
}
func TestFighterEvidenceParentHeldViewAndSharedPermit(t *testing.T) {
	c, children := evidenceParent(t)
	first := poolParentLogin(t, c, "zero256")
	old := (*children)[0]
	if e := c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	held := c.Snapshot()
	next := (*children)[1]
	if !held.Account.Held || held.Account.CanExport || held.View == nil {
		t.Fatal("held fixture unavailable")
	}
	if old.account.EvidencePermit == nil || old.account.EvidencePermit != next.account.EvidencePermit {
		t.Fatal("parent permit replaced across generation")
	}
	if _, e := c.BeginFighterEvidence(t.Context(), first.View.ViewID, "h", "c", "e", recoveryfile.Pin{}); !errors.Is(e, gymstable.ErrStaleView) {
		t.Fatal("old view reached new child", e)
	}
	if _, e := c.BeginFighterEvidence(t.Context(), held.View.ViewID, "h", "c", "e", recoveryfile.Pin{}); e != nil {
		t.Fatal("held public view refused", e)
	}
	c.mu.Lock()
	token := c.session.Token
	c.mu.Unlock()
	e := c.store.WithSession(t.Context(), token, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() })
	if !errors.Is(e, gymaccount.ErrHeld) {
		t.Fatal("private recovery hold weakened", e)
	}
}
