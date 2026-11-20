// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"sync"
	"testing"
	"time"
)

func TestWithdrawalParentRevokesBeforeBlockedDrain(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "logout", true: "reset"}[reset], func(t *testing.T) {
			c, children := poolParent(t)
			s := poolParentLogin(t, c, "zero256")
			child := (*children)[0]
			c.mu.Lock()
			original := c.session
			c.mu.Unlock()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			child.stopGate = func() {
				select {
				case <-entered:
				default:
					close(entered)
				}
				<-release
			}
			var e error
			if reset {
				e = c.ResetFixtureAccount()
			} else {
				e = c.Logout()
			}
			if e != nil {
				t.Fatal(e)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("drain did not begin")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			e = c.store.WithSession(ctx, original.Token, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
			if !errors.Is(e, gymaccount.ErrUnauthorized) {
				t.Fatal("old session remains authorized while drain blocked", e)
			}
			if e = c.withCapability(ctx, original, s.View.ViewID, child, gymaccount.Withdrawal, func(_ gymaccount.Account, check func() error) error { return check() }); !errors.Is(e, gymaccount.ErrUnauthorized) {
				t.Fatal("old withdrawal fence revived", e)
			}
			c.mu.Lock()
			view, operating := c.viewID, c.operation
			c.mu.Unlock()
			if view != "" || !operating {
				t.Fatal("old view published while draining")
			}
			once.Do(func() { close(release) })
			poolParentWait(t, c)
			if reset {
				fresh := c.Snapshot()
				if fresh.View == nil || fresh.View.ViewID == s.View.ViewID || !fresh.Account.Held {
					t.Fatal("reset did not reopen held original wallet")
				}
			}
		})
	}
}
func TestWithdrawalParentOriginalAuthenticationAndMutationGate(t *testing.T) {
	c, _ := poolParent(t)
	var config pilot.StableAccountConfig
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		config = a
		return &poolParentChild{a: a.Manifest, source: c.sourceID}, nil
	}
	poolParentLogin(t, c, "zero256")
	if config.AuthenticatedAtMS <= 0 || config.SessionExpiresAtMS <= config.AuthenticatedAtMS {
		t.Fatal("original login times not retained")
	}
	use := func() error {
		return config.Lease(t.Context(), gymaccount.Withdrawal, func(_ gymaccount.Account, check func() error) error { return check() })
	}
	if e := use(); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	c.operation = true
	c.mu.Unlock()
	if e := use(); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("mutation allowed during parent transition", e)
	}
	c.mu.Lock()
	c.operation = false
	c.mu.Unlock()
}
