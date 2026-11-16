// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAuthorityInfoPinnedPublicCopy(t *testing.T) {
	s, _, _ := identityTestStore(t)
	info, err := s.AuthorityInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != IdentityVersion || info.AuthorityID != s.AuthorityID() || info.VerifierConfigHash != identityTestConfig().VerifierConfigHash || info.Network != FixtureNetwork || info.Genesis != FixtureGenesis || info.Profile != FixtureProfile {
		t.Fatal("incorrect authority binding")
	}
	original := info
	info.VerifierConfigHash = "changed"
	again, err := s.AuthorityInfo(context.Background())
	if err != nil || again != original {
		t.Fatal("caller changed pinned info")
	}
	raw, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"subject", "principal", "session", "token", "mnemonic", "private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("unexpected authority data %s", secret)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.AuthorityInfo(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.Close()
	if _, err = s.AuthorityInfo(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal("closed authority returned trusted info", err)
	}
	legacy, _, _ := accountTestStore(t)
	if _, err = legacy.AuthorityInfo(context.Background()); !errors.Is(err, ErrForbidden) {
		t.Fatal("legacy authority relabelled", err)
	}
}

func TestEnrollmentIdentityBoundToAuthorizedAccount(t *testing.T) {
	s, clock, _ := identityTestStore(t)
	alice := identityTestLogin(t, s, clock, "alice", 1)
	bob := identityTestLogin(t, s, clock, "bob", 1)
	for _, item := range []struct {
		session Session
		fixture string
	}{{alice, "zero256"}, {bob, "incrementing256"}} {
		account, identity, err := s.EnrollmentIdentity(context.Background(), item.session.Token)
		expected, _ := FixtureIdentity(item.fixture)
		if err != nil || account.ID != item.session.Account.ID || identity != expected {
			t.Fatal("wrong account fixture tuple", err)
		}
	}
	accountTestEnroll(t, s, alice, "alice", "enroll")
	copied, _, err := s.EnrollmentIdentity(context.Background(), alice.Token)
	if err != nil {
		t.Fatal(err)
	}
	copied.Enrollment.GymID = "changed"
	again, _, err := s.EnrollmentIdentity(context.Background(), alice.Token)
	if err != nil || again.Enrollment.GymID == "changed" {
		t.Fatal("caller retained enrollment alias")
	}
	mining, err := s.MintMining(context.Background(), alice.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{mining.Token, strings.Repeat("0", 64)} {
		if _, _, err = s.EnrollmentIdentity(context.Background(), token); err == nil {
			t.Fatal("unauthorized identity lookup")
		}
	}
	reset := identityTestReset(clock, "alice", "d", "begin", 1)
	if _, _, err = s.BeginReset(context.Background(), reset); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnrollmentIdentity(context.Background(), alice.Token); err == nil {
		t.Fatal("pending lookup authorized")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = s.EnrollmentIdentity(cancelled, bob.Token); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestProvisioningIdentityFreshnessHoldAndFinalGuard(t *testing.T) {
	s, clock, _ := identityTestStore(t)
	ctx := context.Background()
	login := identityTestLogin(t, s, clock, "alice", 1)
	if _, _, err := s.ProvisioningIdentity(ctx, login.Token); err != nil {
		t.Fatal("initial unenrolled provisioning denied", err)
	}
	var escaped func() error
	err := s.WithProvisioning(ctx, login.Token, func(a Account, i Identity, check func() error) error {
		escaped = check
		clock.Add(FreshAuthenticationMS + 1000)
		return check()
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal("final provisioning freshness not checked", err)
	}
	if err = escaped(); !errors.Is(err, ErrClosed) {
		t.Fatal("escaped provisioning lease remained active", err)
	}
	if _, _, err = s.EnrollmentIdentity(ctx, login.Token); err != nil {
		t.Fatal("ordinary public identity lookup semantics changed", err)
	}
	reset := identityTestReset(clock, "alice", "e", "begin", 1)
	if _, _, err = s.BeginReset(ctx, reset); err != nil {
		t.Fatal(err)
	}
	completed, _, err := s.CompleteReset(ctx, identityRefreshReset(reset, clock, "complete"))
	if err != nil {
		t.Fatal(err)
	}
	recovered := identityTestLogin(t, s, clock, "alice", 2)
	if _, _, err = s.EnrollmentIdentity(ctx, recovered.Token); err != nil {
		t.Fatal("held gym enrollment disabled", err)
	}
	if _, _, err = s.ProvisioningIdentity(ctx, recovered.Token); !errors.Is(err, ErrHeld) {
		t.Fatal("held full material preparation authorized", err)
	}
	clock.Store(completed.HoldUntilMS)
	fresh := identityTestLogin(t, s, clock, "alice", 2)
	account, identity, err := s.ProvisioningIdentity(ctx, fresh.Token)
	expected, _ := FixtureIdentity("zero256")
	if err != nil || account.ID != login.Account.ID || identity != expected {
		t.Fatal("original unenrolled reservation cannot resume after hold", err)
	}
	legacy, legacyClock, _ := accountTestStore(t)
	old := accountTestLogin(t, legacy, "alice", legacyClock)
	before := legacy.meta.ClockMS
	legacyClock.Add(1)
	if _, _, err = legacy.ProvisioningIdentity(ctx, old.Token); !errors.Is(err, ErrForbidden) || legacy.meta.ClockMS != before {
		t.Fatal("legacy schema touched by new provisioning API", err)
	}
}
