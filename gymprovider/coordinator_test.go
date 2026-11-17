// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

func coreLogin(t *testing.T, f *providerFixture, subject, origin string, epoch uint64) (LoginFlow, gymaccount.Session) {
	t.Helper()
	ctx := context.Background()
	flow, e := f.Coordinator.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	token := signLogin(t, flow, subject, origin, flow.ID, "fresh", epoch, time.Now().UnixMilli())
	session, e := f.Coordinator.FinishLogin(ctx, flow.ID, token)
	if e != nil {
		t.Fatal(e)
	}
	if session.Token == "" || session.Kind != "login" {
		t.Fatal("private login credential missing")
	}
	if got, e := f.Coordinator.FinishLogin(ctx, flow.ID, token); !errors.Is(e, ErrConsumed) || got.Token != "" {
		t.Fatal("completed login replay returned a credential", e)
	}
	return flow, session
}
func reopenCore(t *testing.T, f *providerFixture) {
	t.Helper()
	if e := f.Coordinator.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.Authority.Close(); e != nil {
		t.Fatal(e)
	}
	var e error
	f.Authority, e = gymaccount.OpenIdentity(filepath.Join(f.Root, "authority"), f.IdentityConfig)
	if e != nil {
		t.Fatal(e)
	}
	f.Coordinator, e = Open(context.Background(), filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
	if e != nil {
		t.Fatal(e)
	}
}
func TestCoordinatorLoginResetReopen(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	origin := strings.Repeat("00", 32)
	first, alice := coreLogin(t, f, "alice", origin, 1)
	_, bob := coreLogin(t, f, "bob", strings.Repeat("22", 32), 1)
	if alice.Account.ID == bob.Account.ID {
		t.Fatal("principals shared an account")
	}
	// One accepted original authentication can be reused only under its exact
	// original time/epoch; the new nonce does not freshen that authentication.
	var retained loginRecord
	if e := f.Coordinator.load("login:"+first.ID, &retained); e != nil {
		t.Fatal(e)
	}
	second, e := f.Coordinator.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	reused, e := f.Coordinator.FinishLogin(ctx, second.ID, signLogin(t, second, "alice", origin, first.ID, "reuse", 1, retained.Semantic.AuthenticatedAtMS))
	if e != nil || reused.Account.ID != alice.Account.ID {
		t.Fatal("exact authentication reuse failed", e)
	}
	req, e := f.Coordinator.IssueReset(ctx)
	if e != nil {
		t.Fatal(e)
	}
	verified := time.Now().UnixMilli()
	op := strings.Repeat("33", 32)
	begin := signReset(t, req, "alice", op, "begin", 1, 2, verified)
	pending, e := f.Coordinator.BeginReset(ctx, req.EventID, begin)
	if e != nil || !pending.CanCommit || pending.Phase != "pending" || pending.Barrier.EventID == req.EventID {
		t.Fatal("pending reset binding", e)
	}
	if e = f.Authority.WithSession(ctx, alice.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); e == nil {
		t.Fatal("old session survived reset begin")
	}
	if e = f.Authority.WithSession(ctx, bob.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); e != nil {
		t.Fatal("other principal was revoked", e)
	}
	reopenCore(t, f)
	again, e := f.Coordinator.BeginReset(ctx, req.EventID, begin)
	if e != nil || again != pending {
		t.Fatal("pending replay changed original receipt", e)
	}
	completed, e := f.Coordinator.CompleteReset(ctx, req.EventID, signReset(t, req, "alice", op, "complete", 1, 2, verified))
	if e != nil || completed.CanCommit || completed.Phase != "completed" || completed.Barrier.AccountID != alice.Account.ID || completed.Barrier.HoldUntilMS-completed.Barrier.CompletedAtMS != gymaccount.RecoveryHoldMS {
		t.Fatal("completed reset binding", e)
	}
	old, e := f.Coordinator.BeginReset(ctx, req.EventID, begin)
	if e != nil || old.CanCommit || old.Barrier != completed.Barrier {
		t.Fatal("historical begin became a credential-mutation permit", e)
	}
	_, fresh := coreLogin(t, f, "alice", strings.Repeat("44", 32), 2)
	if fresh.Account.ID != alice.Account.ID || fresh.Account.Generation != 2 || fresh.Account.HoldUntilMS != completed.Barrier.HoldUntilMS {
		t.Fatal("reset changed account identity or hold")
	}
	reopenCore(t, f)
	old, e = f.Coordinator.CompleteReset(ctx, req.EventID, signReset(t, req, "alice", op, "complete", 1, 2, verified))
	if e != nil || old != completed {
		t.Fatal("completed receipt changed on reopen", e)
	}
	// The journal has semantic public bindings, never the transport assertion or
	// private session credential. Status similarly has no subject/nonce fields.
	it := f.Coordinator.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		for _, secret := range []string{alice.Token, bob.Token, reused.Token, fresh.Token, begin} {
			if bytes.Contains(it.Value(), []byte(secret)) {
				t.Fatal("journal retained a transport/session credential")
			}
		}
	}
	if e := it.Error(); e != nil {
		t.Fatal(e)
	}
}
func TestCoordinatorConfigAndCapacity(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	mutations := []func(*Config){func(c *Config) { c.Provider = "real-provider" }, func(c *Config) { c.Issuer = "https://real.invalid" }, func(c *Config) { c.VerifierConfigHash = strings.Repeat("ab", 32) }, func(c *Config) { c.TenantID = "tenant/alias" }, func(c *Config) { c.MaxLoginFlows = 4097 }}
	for i, change := range mutations {
		cfg := f.Config
		change(&cfg)
		path := filepath.Join(t.TempDir(), "must-not-create")
		if got, e := Create(ctx, path, cfg, f.Authority, f.Verifier); e == nil || got != nil {
			t.Fatalf("invalid config%d accepted", i)
		}
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("invalid config created state")
		}
	}
	cfg := f.Config
	cfg.MaxLoginFlows = 2
	cfg.MaxResetFlows = 1
	path := filepath.Join(t.TempDir(), "bounded")
	c, e := Create(ctx, path, cfg, f.Authority, f.Verifier)
	if e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if _, e := c.IssueLogin(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := c.IssueLogin(ctx); !errors.Is(e, ErrCapacity) {
		t.Fatal("login capacity", e)
	}
	if _, e := c.IssueReset(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := c.IssueReset(ctx); !errors.Is(e, ErrCapacity) {
		t.Fatal("reset capacity", e)
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = Open(ctx, path, cfg, f.Authority, f.Verifier)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e := c.IssueLogin(ctx); !errors.Is(e, ErrCapacity) {
		t.Fatal("reopen forgot abandoned flow capacity", e)
	}
	changed := cfg
	changed.MaxResetFlows = 2
	if other, e := Open(ctx, path, changed, f.Authority, f.Verifier); e == nil || other != nil {
		t.Fatal("configuration silently expanded")
	}
}
