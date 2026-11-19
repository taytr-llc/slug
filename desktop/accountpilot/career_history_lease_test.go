// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"testing"
	"time"
)

func TestHistoryPublicationLeaseOriginalEpochAndParentMutex(t *testing.T) {
	c, _ := manifestTestController(t)
	manifestLogin(t, c, "zero256")
	c.mu.Lock()
	session, view, child := c.session, c.viewID, c.child
	c.mu.Unlock()
	check := func(want bool) {
		t.Helper()
		called := false
		e := c.withHistoryPublicationLease(t.Context(), session, view, child, func(a gymaccount.Account, guard func() error) error {
			called = true
			if a.ID != session.Account.ID || a.Generation != session.Account.Generation {
				t.Fatal("replaced account")
			}
			if c.mu.TryLock() {
				c.mu.Unlock()
				t.Fatal("parent transition was not fenced")
			}
			return guard()
		})
		if (e == nil) != want || called != want {
			t.Fatal(e, called, want)
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
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	e := c.withHistoryPublicationLease(ctx, session, view, child, func(gymaccount.Account, func() error) error { cancel(); return nil })
	if e != nil {
		t.Fatal("completed publication rewritten after cancellation", e)
	}
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	manifestWaitAccount(t, c)
	check(false)
	c.mu.Lock()
	session, view, child = c.session, c.viewID, c.child
	c.mu.Unlock()
	if !c.Snapshot().Account.Held {
		t.Fatal("expected held recovery")
	}
	check(true)
	if e = c.Logout(); e != nil {
		t.Fatal(e)
	}
	manifestWaitAccount(t, c)
	check(false)
	called := false
	e = c.withHistoryPublicationLease(t.Context(), session, view, child, func(gymaccount.Account, func() error) error { called = true; return nil })
	if !errors.Is(e, gymaccount.ErrUnauthorized) || called {
		t.Fatal("logout repaired original", e)
	}
}

// Hold the parent lookup mutex until the caller's earlier deadline has expired.
// The deadline must already cover this first lookup, and cannot be restarted by
// the child after the lock is released.
func TestHistoryParentBudgetIncludesInitialLookup(t *testing.T) {
	c, _ := manifestTestController(t)
	manifestLogin(t, c, "zero256")
	s := &StableController{c}
	c.mu.Lock()
	view := c.viewID
	c.mu.Unlock()
	id := strings.Repeat("a", 64)
	for _, name := range []string{"snapshot", "begin", "job", "cancel", "page"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
			defer cancel()
			c.mu.Lock()
			done := make(chan error, 1)
			go func() {
				var e error
				switch name {
				case "snapshot":
					_, e = s.CareerHistorySnapshot(ctx)
				case "begin":
					_, e = s.BeginCareerHistory(ctx, view, id)
				case "job":
					_, e = s.CareerHistoryJob(ctx, view, id)
				case "cancel":
					_, e = s.CancelCareerHistory(ctx, view, id)
				case "page":
					_, e = s.CareerHistoryPage(ctx, view, id, 0, 8)
				}
				done <- e
			}()
			<-ctx.Done()
			c.mu.Unlock()
			if e := <-done; !errors.Is(e, gymstable.ErrBusy) {
				t.Fatal("expired parent lookup was not busy", e)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := s.CareerHistoryJob(ctx, "wrong original view", id); !errors.Is(e, gymstable.ErrStaleView) {
		t.Fatal("original view refusal lost precedence", e)
	}
}
