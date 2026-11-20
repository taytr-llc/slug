package gymaccount

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIdentityLookupReadsExactRecordsWithoutAuthorityOrMutation(t *testing.T) {
	s, clock, _ := identityTestStore(t)
	ctx := context.Background()
	if _, err := s.GetIdentityAccount(ctx, "fixture:local", "alice"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing configured account not distinguished", err)
	}
	login := identityTestLogin(t, s, clock, "alice", 1)
	accountTestEnroll(t, s, login, "alice", "enroll")
	a := accountTestRead(t, s, login.Token)
	before := s.meta
	clock.Add(1)
	got, err := s.GetIdentityAccount(ctx, "fixture:local", "alice")
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Fatal("wrong retained account", err)
	}
	got.Enrollment.GymID = strings.Repeat("0", 64)
	if again, err := s.GetIdentityAccount(ctx, "fixture:local", "alice"); err != nil || !reflect.DeepEqual(again, a) {
		t.Fatal("lookup leaked mutable account storage", err)
	}
	if _, err := s.GetIdentityAccount(ctx, "fixture:local", "unconfigured"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal("unconfigured principal treated as a configured absent account")
	}
	event := strings.Repeat("b", 64)
	if _, err := s.GetResetBarrier(ctx, "fixture:local", "alice", event); !errors.Is(err, ErrNotFound) {
		t.Fatal("absent exact barrier not distinguished", err)
	}
	if !reflect.DeepEqual(s.meta, before) {
		t.Fatal("read-only lookup mutated clock or counters")
	}
	reset := identityTestReset(clock, "alice", "b", "begin", 1)
	barrier, _, err := s.BeginReset(ctx, reset)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetResetBarrier(ctx, "fixture:local", "alice", event); err != nil || got != barrier {
		t.Fatal("pending exact barrier missing", err)
	}
	if _, err := s.GetResetBarrier(ctx, "fixture:local", "alice", barrier.EventID); !errors.Is(err, ErrNotFound) {
		t.Fatal("derived barrier hash substituted for original event", err)
	}
	if err := s.WithSession(ctx, login.Token, View, func(Account, func() error) error { return nil }); err == nil {
		t.Fatal("lookup restored revoked authority")
	}
	reset.Stage = "complete"
	complete, _, err := s.CompleteReset(ctx, reset)
	if err != nil {
		t.Fatal(err)
	}
	before = s.meta
	clock.Add(1000)
	if got, err := s.GetResetBarrier(ctx, "fixture:local", "alice", event); err != nil || got != complete {
		t.Fatal("completed exact barrier changed", err)
	}
	if !reflect.DeepEqual(s.meta, before) {
		t.Fatal("historical barrier lookup changed the hold or clock")
	}
}

func TestIdentityLookupRejectsLegacyClosedAndCancelled(t *testing.T) {
	legacy, _, _ := accountTestStore(t)
	if _, err := legacy.GetIdentityAccount(context.Background(), "fixture:local", "alice"); !errors.Is(err, ErrForbidden) {
		t.Fatal("legacy authority exposed schema-2 lookup", err)
	}
	s, _, _ := identityTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetIdentityAccount(ctx, "fixture:local", "alice"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled lookup accepted", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIdentityAccount(context.Background(), "fixture:local", "alice"); !errors.Is(err, ErrClosed) {
		t.Fatal("closed lookup accepted", err)
	}
}
