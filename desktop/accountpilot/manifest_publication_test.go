// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestManifestLoginPublicationSnapshot(t *testing.T) {
	c, _ := manifestTestController(t)
	reached, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	// Exercise the real login and original account lease. Keep begin's existing
	// operation flag set after login returns, without adding a runtime hook.
	if err := c.begin("signing_in", func(ctx context.Context) error {
		if err := c.login(ctx, "incrementing256"); err != nil {
			return err
		}
		close(reached)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("login publication window not reached")
	}
	c.mu.Lock()
	busy, state := c.operation, c.state
	c.mu.Unlock()
	if !busy || state != "signed_in" {
		t.Fatalf("internal login state %q, busy=%v", state, busy)
	}
	want := pilot.BlankManifestSnapshot()
	want.Account = pilot.AccountSnapshot{State: "signing_in", Fixture: "incrementing256"}
	if got := c.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatal("incomplete login disclosed identity or a contradictory blank state")
	}
	c.mu.Lock()
	if !c.operation || c.state != "signed_in" {
		c.mu.Unlock()
		t.Fatal("snapshot changed internal publication or operation state")
	}
	c.mu.Unlock()
	close(release)
	released = true
	manifestWaitAccount(t, c)
	s := c.Snapshot()
	if s.Account.State != "signed_in" || s.Account.ID == "" || s.Account.Generation == 0 || s.View == nil {
		t.Fatal("completed login did not disclose its authorized identity")
	}
}

func TestManifestUnpublishableSnapshotStates(t *testing.T) {
	for _, tc := range []struct {
		name, state, want string
		closed, busy      bool
		child             bool
		view              string
	}{
		{name: "closed", state: "signed_in", closed: true, want: "signed_out"},
		{name: "closing-busy", state: "signed_in", closed: true, busy: true, want: "signed_out"},
		{name: "publication", state: "signed_in", busy: true, want: "signing_in"},
		{name: "missing-child", state: "signed_in", view: "present", want: "expired"},
		{name: "missing-view", state: "signed_in", child: true, want: "expired"},
		{name: "signing-in", state: "signing_in", busy: true, want: "signing_in"},
		{name: "resetting", state: "resetting", busy: true, want: "resetting"},
		{name: "signing-out", state: "signing_out", busy: true, want: "signing_out"},
		{name: "failed", state: "failed", want: "failed"},
		{name: "expired", state: "expired", want: "expired"},
		{name: "signed-out", state: "signed_out", want: "signed_out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ManifestController{state: tc.state, fixture: "zero256", lastError: "account_unavailable", closed: tc.closed, operation: tc.busy, viewID: tc.view}
			if tc.child {
				c.child = &manifestFakeChild{}
			}
			want := pilot.BlankManifestSnapshot()
			want.Account = pilot.AccountSnapshot{State: tc.want, Fixture: "zero256", LastError: "account_unavailable"}
			if got := c.Snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatal("nonpublishable snapshot leaked identity or lost its state/error")
			}
			if c.state != tc.state || c.operation != tc.busy || c.closed != tc.closed {
				t.Fatal("snapshot mutated controller lifecycle")
			}
		})
	}
}
