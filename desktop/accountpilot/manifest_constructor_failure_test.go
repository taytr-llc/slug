// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

func TestManifestAccountConstructorFailure(t *testing.T) {
	constructorErr := errors.New("fixture child construction failed")
	cases := []struct {
		name string
		fail func() (manifestChild, error)
	}{
		{"manifest", func() (manifestChild, error) {
			var child *pilot.ManifestController
			return child, constructorErr
		}},
		{"stable", func() (manifestChild, error) {
			var child *pilot.StableController
			return child, constructorErr
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, retry := range []bool{false, true} {
				name := "close"
				if retry {
					name = "retry"
				}
				t.Run(name, func(t *testing.T) {
					c, children := manifestTestController(t)
					successfulFactory := c.newChild
					var original gymaccount.Session
					var authorized bool
					var authorizationErr error
					c.newChild = func(_ pilot.Config, account pilot.ManifestAccountConfig) (manifestChild, error) {
						c.mu.Lock()
						original = c.session
						c.mu.Unlock()
						authorizationErr = c.store.WithSession(context.Background(), original.Token, gymaccount.Enrollment, func(a gymaccount.Account, recheck func() error) error {
							authorized = a.ID == account.Account.AccountID && a.Generation == account.Generation
							return recheck()
						})
						return tc.fail()
					}
					if err := c.LoginFixture("zero256"); err != nil {
						t.Fatal(err)
					}
					manifestWaitAccount(t, c)
					if original.Token == "" || !authorized || authorizationErr != nil {
						t.Fatalf("constructor did not receive a live original login: authorized=%v, error=%v", authorized, authorizationErr)
					}
					snapshot := c.Snapshot()
					if snapshot.Account.State != "failed" || snapshot.Account.LastError != "account_unavailable" || snapshot.View != nil {
						t.Fatalf("constructor failure snapshot: %+v", snapshot.Account)
					}
					c.mu.Lock()
					cleared := c.child == nil && c.session.Token == "" && c.mining.Token == "" && c.viewID == "" && !c.operation
					c.mu.Unlock()
					if !cleared || len(*children) != 0 {
						t.Fatal("failed constructor retained child, credentials, view, or operation")
					}
					called := false
					err := c.store.WithSession(context.Background(), original.Token, gymaccount.Enrollment, func(gymaccount.Account, func() error) error {
						called = true
						return nil
					})
					if !errors.Is(err, gymaccount.ErrUnauthorized) || called {
						t.Fatalf("original login was not revoked: called=%v, error=%v", called, err)
					}
					if retry {
						c.newChild = successfulFactory
						recovered := manifestLogin(t, c, "zero256")
						if recovered.Account.ID != original.Account.ID || recovered.Account.Generation != original.Account.Generation || len(*children) != 1 || (*children)[0].loads.Load() != 1 || (*children)[0].starts.Load() != 0 {
							t.Fatal("retry did not recover the same account with only a cold load")
						}
						c.mu.Lock()
						freshSession := c.session.Token != "" && c.session.Token != original.Token
						c.mu.Unlock()
						if !freshSession {
							t.Fatal("retry reused the revoked login")
						}
					}
					if err := c.Close(); err != nil {
						t.Fatal("close after constructor failure:", err)
					}
					if retry && (*children)[0].closes.Load() != 1 {
						t.Fatal("close did not release the recovered child exactly once")
					}
				})
			}
		})
	}
}
