package accountpilot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
)

func TestDailyRemoteOriginalAccountCancellationProvenance(t *testing.T) {
	for _, daily := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "daily"}[daily], func(t *testing.T) {
			c := remoteControllerSignedAccount(t)
			c.daily = daily
			login, view := c.session, c.viewID
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e := c.epoch(login, view)(ctx, func(check func() error) error { cancel(); return check() })
			want := error(gymaccount.ErrUnauthorized)
			if daily {
				want = context.Canceled
			}
			if e != want {
				t.Fatal("original epoch cause", daily, e)
			}
			if c.life.Err() != nil || remoteControllerCheckView(c, view) != nil {
				t.Fatal("admission cancellation changed original owner")
			}
			// Cancellation before Snapshot has no disclosure, no replacement, and no
			// implied attachment readiness: this is the same real signed account owner.
			snap, e := c.Snapshot(ctx)
			if e != context.Canceled || snap.View != nil || snap.Remote != nil || snap.ConfigID != "" {
				t.Fatal("canceled Snapshot disclosed", e)
			}
			if e = c.Close(); e != nil {
				t.Fatal("original signed account join", e)
			}
		})
	}
}
func TestDailyRemoteObservedEpochLossSurvivesCancellation(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	c.daily = true
	login, view := c.session, c.viewID
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := c.epoch(login, view)(ctx, func(check func() error) error {
		// The exact existing epoch mutex is held here; no concurrent unsynchronized
		// test mutation and no replacement session can satisfy this original check.
		c.viewID = strings.Repeat("4", 64)
		cancel()
		return check()
	})
	if !errors.Is(e, gymaccount.ErrUnauthorized) || e == context.Canceled {
		t.Fatal("observed original-view loss erased", e)
	}
	c.mu.Lock()
	c.viewID = view
	c.mu.Unlock()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	e = c.withView(ctx2, view, nil, func(_ gymaccount.Account, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		cancel2()
		return gymaccount.ErrUnauthorized
	})
	if e != gymaccount.ErrUnauthorized {
		t.Fatal("observed operation refusal erased", e)
	}
}
