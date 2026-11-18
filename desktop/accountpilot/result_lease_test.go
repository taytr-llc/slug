// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"testing"
)

// The public fixture child is a test double; enrollment, session validity,
// generation, hold and each WithSession recheck are the actual account store.
func TestResultNodeLeaseOriginalEpochHeldAndTransitionalGuards(t *testing.T) {
	c, children := manifestTestController(t)
	manifestLogin(t, c, "zero256")
	c.mu.Lock()
	session, view, child := c.session, c.viewID, c.child
	c.mu.Unlock()
	check := func(want bool) {
		t.Helper()
		called := false
		e := c.withResultNodeLease(t.Context(), session, view, child, func(a gymaccount.Account, guard func() error) error {
			called = true
			if a.ID != session.Account.ID || a.Generation != session.Account.Generation {
				t.Fatal("identity substituted")
			}
			return guard()
		})
		if (e == nil) != want || called != want {
			t.Fatal("node admission", e, called, want)
		}
	}
	check(true)
	for _, state := range []string{"signing_in", "resetting", "signing_out", "expired", "failed"} {
		c.mu.Lock()
		c.state = state
		c.mu.Unlock()
		check(false)
	}
	c.mu.Lock()
	c.state = "signed_in"
	c.operation = true
	c.mu.Unlock()
	check(false)
	starts := (*children)[0].starts.Load()
	if e := c.Start(); e == nil {
		t.Fatal("busy Start accepted")
	}
	if (*children)[0].starts.Load() != starts || (*children)[0].closes.Load() != 0 {
		t.Fatal("refused Start affected existing child owner")
	}
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
	// A parent transition during the short callback cannot transfer readiness.
	e := c.withResultNodeLease(t.Context(), session, view, child, func(gymaccount.Account, func() error) error {
		c.mu.Lock()
		c.operation = true
		c.mu.Unlock()
		return nil
	})
	if !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("missing final parent fence", e)
	}
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	manifestWaitAccount(t, c)
	check(false)
	c.mu.Lock()
	session, view, child = c.session, c.viewID, c.child
	c.mu.Unlock()
	if !c.Snapshot().Account.Held {
		t.Fatal("reset did not retain hold")
	}
	check(true)
	called := false
	e = c.withCapability(context.Background(), session, view, child, gymaccount.ResultPublish, func(gymaccount.Account, func() error) error { called = true; return nil })
	if !errors.Is(e, gymaccount.ErrHeld) || called {
		t.Fatal("held publishing admitted", e)
	}
	if e = c.Logout(); e != nil {
		t.Fatal(e)
	}
	manifestWaitAccount(t, c)
	check(false)
}
