// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ownedChildFenceFixture(t *testing.T) (*OwnedGymChild, gymaccount.Account) {
	t.Helper()
	f, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	c := &OwnedGymChild{life: life, cancel: cancel, view: OwnedGymView{Version: 1, ViewID: strings.Repeat("1", 64), AccountAuthorityID: strings.Repeat("2", 64), AccountID: strings.Repeat("3", 64), Generation: 1, Fixture: "zero256", RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}, snapshot: OwnedGymChildSnapshot{State: "stopped"}}
	a := gymaccount.Account{ID: c.view.AccountID, Generation: 1, Enrollment: &f}
	c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return fn(a, func() error { return ctx.Err() })
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, a
}
func TestOwnedGymChildOriginalFence(t *testing.T) {
	for _, kind := range []string{"missing", "double", "swallowed", "outer_failure", "foreign_account", "foreign_gym", "stale_generation"} {
		t.Run(kind, func(t *testing.T) {
			c, a := ownedChildFenceFixture(t)
			rejected := errors.New("original callback refused")
			var leases atomic.Uint32
			c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				// begin takes the first original View fence; the second is
				// the run-publication fence under test. Neither grants Mining.
				if cap != gymaccount.View {
					t.Error("Start requested a login capability other than View")
					return gymaccount.ErrForbidden
				}
				if leases.Add(1) == 1 {
					return fn(a, func() error { return nil })
				}
				switch kind {
				case "missing":
					return nil
				case "double":
					_ = fn(a, func() error { return nil })
					return fn(a, func() error { return nil })
				case "swallowed":
					_ = fn(a, func() error { return rejected })
					return nil
				case "outer_failure":
					_ = fn(a, func() error { return nil })
					return rejected
				case "foreign_account":
					a.ID = strings.Repeat("4", 64)
				case "foreign_gym":
					v := *a.Enrollment
					v.GymID = strings.Repeat("4", 64)
					a.Enrollment = &v
				case "stale_generation":
					a.Generation++
				}
				return fn(a, func() error { return nil })
			}
			if e := c.Start(context.Background(), c.view.ViewID); e == nil {
				t.Fatal("broken original authority dispatched mining")
			}
			c.mu.Lock()
			run := c.run
			c.mu.Unlock()
			if run != nil {
				t.Fatal("failed admission retained a dispatched run")
			}
		})
	}
}
func TestOwnedGymChildCloseCancelsAndJoins(t *testing.T) {
	c, _ := ownedChildFenceFixture(t)
	var released atomic.Bool
	c.release = func() { released.Store(true) }
	work, done, e := c.begin(context.Background(), c.view.ViewID)
	if e != nil {
		t.Fatal(e)
	}
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("close did not cancel work")
	}
	if released.Load() {
		t.Fatal("released borrowed wallet before join")
	}
	select {
	case <-closed:
		t.Fatal("close skipped admitted work")
	default:
	}
	done()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not join")
	}
	if !released.Load() {
		t.Fatal("joined owner was not released")
	}
	if _, _, e = c.begin(context.Background(), c.view.ViewID); !errors.Is(e, gymstable.ErrClosed) {
		t.Fatalf("closed admission: %v", e)
	}
}
func TestOwnedGymChildCleanupFailureRetainsBorrow(t *testing.T) {
	c, _ := ownedChildFenceFixture(t)
	failure := errors.New("scratch close uncertainty")
	c.backupCloseErr = failure
	var released atomic.Bool
	c.release = func() { released.Store(true) }
	if e := c.Close(); !errors.Is(e, failure) {
		t.Fatalf("lost cleanup result: %v", e)
	}
	if released.Load() {
		t.Fatal("cleanup failure released runtime namespace")
	}
	if e := c.Close(); !errors.Is(e, failure) {
		t.Fatal("cleanup failure was not sticky")
	}
}

func TestOwnedGymChildStopJoinsBackup(t *testing.T) {
	c, _ := ownedChildFenceFixture(t)
	ctx, cancel := context.WithCancel(c.life)
	op := &poolBackupOperation{ctx: ctx, cancel: cancel, done: make(chan struct{}), verify: true}
	c.backup = op
	cleanup := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(cleanup) }) }
	t.Cleanup(release)
	go func() {
		<-ctx.Done()
		<-cleanup
		c.mu.Lock()
		c.backup = nil
		close(op.done)
		c.mu.Unlock()
	}()
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(context.Background(), c.view.ViewID) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel backup")
	}
	select {
	case e := <-stopped:
		t.Fatalf("Stop skipped backup cleanup: %v", e)
	default:
	}
	release()
	select {
	case e := <-stopped:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not join cleanup")
	}
}
