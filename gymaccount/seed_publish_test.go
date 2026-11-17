// Copyright (c) 2026 The SLUG developers. ISC license.
package gymaccount

import (
	"context"
	"errors"
	"testing"
)

func TestSeedPublishOriginalFreshUnheldEnrollment(t *testing.T) {
	s, clock, _ := accountTestStore(t)
	ctx := context.Background()
	login := accountTestLogin(t, s, "alice", clock)
	use := func(token string, cap Capability) error {
		return s.WithSession(ctx, token, cap, func(_ Account, check func() error) error { return check() })
	}
	if e := use(login.Token, SeedPublish); !errors.Is(e, ErrForbidden) {
		t.Fatal("unenrolled", e)
	}
	accountTestEnroll(t, s, login, "alice", "enroll")
	if e := use(login.Token, SeedPublish); e != nil {
		t.Fatal("fresh", e)
	}
	mining, e := s.MintMining(ctx, login.Token)
	if e != nil {
		t.Fatal(e)
	}
	if e = use(mining.Token, SeedPublish); !errors.Is(e, ErrForbidden) {
		t.Fatal("mining escalation", e)
	}
	clock.Add(FreshAuthenticationMS + 1)
	if e = use(login.Token, SeedPublish); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("stale authentication", e)
	}
	if e = use(login.Token, View); e != nil {
		t.Fatal("view unexpectedly lost", e)
	}
	login = accountTestLogin(t, s, "alice", clock)
	if e = use(login.Token, SeedPublish); e != nil {
		t.Fatal(e)
	}
	reset := accountTestPrincipal("alice", clock)
	reset.RecoveryVerified = true
	reset.RecoveryEventID = "seed-hold"
	if _, _, e = s.ApplyReset(ctx, reset); e != nil {
		t.Fatal(e)
	}
	if e = use(login.Token, SeedPublish); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("old token", e)
	}
	login = accountTestLogin(t, s, "alice", clock)
	accountTestEnroll(t, s, login, "alice", "recover")
	if e = use(login.Token, SeedPublish); !errors.Is(e, ErrHeld) {
		t.Fatal("hold bypass", e)
	}
	if e = use(login.Token, View); e != nil {
		t.Fatal("held read", e)
	}
}
