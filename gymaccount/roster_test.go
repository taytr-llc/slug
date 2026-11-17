// Copyright (c) 2026 The SLUG developers. ISC license.
package gymaccount

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRosterManageEnrollmentHoldAndCredentialBoundary(t *testing.T) {
	s, clock, _ := accountTestStore(t)
	ctx := context.Background()
	login := accountTestLogin(t, s, "alice", clock)
	calls := 0
	use := func(token string) error {
		return s.WithSession(ctx, token, RosterManage, func(a Account, check func() error) error { calls++; return check() })
	}
	if e := use(login.Token); !errors.Is(e, ErrForbidden) || calls != 0 {
		t.Fatal("unenrolled roster", e, calls)
	}
	accountTestEnroll(t, s, login, "alice", "enroll")
	if e := use(login.Token); e != nil || calls != 1 {
		t.Fatal("enrolled roster", e)
	}
	mining, e := s.MintMining(ctx, login.Token)
	if e != nil {
		t.Fatal(e)
	}
	if e = use(mining.Token); !errors.Is(e, ErrForbidden) || calls != 1 {
		t.Fatal("mining token elevated", e)
	}
	reset := accountTestPrincipal("alice", clock)
	reset.RecoveryVerified = true
	reset.RecoveryEventID = "fixture-roster-reset"
	if _, inserted, e := s.ApplyReset(ctx, reset); e != nil || !inserted {
		t.Fatal(e)
	}
	if e = use(login.Token); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("old session retained roster", e)
	}
	recovered := accountTestLogin(t, s, "alice", clock)
	if e = use(recovered.Token); !errors.Is(e, ErrForbidden) {
		t.Fatal("recovery proof skipped", e)
	}
	accountTestEnroll(t, s, recovered, "alice", "recover")
	if e = use(recovered.Token); e != nil {
		t.Fatal("hold blocked reversible roster", e)
	}
	a := accountTestRead(t, s, recovered.Token)
	if a.HoldUntilMS <= clock.Load() {
		t.Fatal("test not held")
	}
	for _, cap := range []Capability{RecoveryExport, Withdrawal, FighterTransfer, RecoveryAuthority, MarketplacePurchase, MarketplaceListing} {
		e = s.WithSession(ctx, recovered.Token, cap, func(Account, func() error) error { t.Error("restricted callback entered"); return nil })
		if !errors.Is(e, ErrHeld) {
			t.Fatal("enumerated hold weakened", cap, e)
		}
	}
	if e = s.Logout(ctx, recovered.Token); e != nil {
		t.Fatal(e)
	}
	if e = use(recovered.Token); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("logout retained roster", e)
	}
}

func TestRosterManageContendedLeaseHonorsContextBeforeGateRelease(t *testing.T) {
	s, clock, _ := accountTestStore(t)
	login := accountTestLogin(t, s, "alice", clock)
	accountTestEnroll(t, s, login, "alice", "enroll")
	entered, release := make(chan struct{}), make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- s.WithSession(context.Background(), login.Token, View, func(Account, func() error) error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		returned <- s.WithSession(ctx, login.Token, RosterManage, func(Account, func() error) error { t.Error("expired queued callback executed"); return nil })
	}()
	select {
	case e := <-returned:
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Error(e)
		}
	case <-time.After(2 * time.Second):
		close(release)
		<-held
		t.Fatal("context acquisition waited for holder")
	}
	// The holder is deliberately still blocked; this demonstrates actual
	// cancellable acquisition, not a wrapper returning with a leaked goroutine.
	select {
	case <-held:
		t.Fatal("holder unexpectedly released")
	default:
	}
	close(release)
	if e := <-held; e != nil {
		t.Fatal(e)
	}
	if e := s.WithSession(context.Background(), login.Token, RosterManage, func(_ Account, check func() error) error { return check() }); e != nil {
		t.Fatal(e)
	}
}
