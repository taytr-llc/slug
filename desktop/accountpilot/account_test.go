// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package accountpilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymaccount"
)

type accountReference struct {
	Name               string                         `json:"name"`
	FixtureName        string                         `json:"fixture_name"`
	TestOnlyPrivateKey string                         `json:"test_only_private_key"`
	Challenge          gymaccount.EnrollmentChallenge `json:"challenge"`
	Authorization      gymaccount.Authorization       `json:"authorization"`
}

func accountReferences(t *testing.T) map[string]accountReference {
	t.Helper()
	raw, err := os.ReadFile("../../gymaccount/testdata/enrollment_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		FixtureOnly bool               `json:"fixture_only"`
		Cases       []accountReference `json:"cases"`
	}
	if json.Unmarshal(raw, &file) != nil || !file.FixtureOnly || len(file.Cases) != 4 {
		t.Fatal("independent public enrollment fixtures unavailable")
	}
	result := make(map[string]accountReference)
	for _, entry := range file.Cases {
		if entry.Challenge.Action == "enroll" {
			result[entry.FixtureName] = entry
		}
	}
	return result
}

func enrolledFixtureSession(t *testing.T, store *gymaccount.Store, fixture string) (gymaccount.Session, gymaccount.Session) {
	t.Helper()
	principal, err := verifyFixture(context.Background(), fixture, false)
	if err != nil {
		t.Fatal(err)
	}
	login, err := store.Login(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := store.BeginChallenge(context.Background(), login.Token, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	reference := accountReferences(t)[fixture]
	raw, err := hex.DecodeString(reference.TestOnlyPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := btcec.PrivKeyFromBytes(raw)
	defer key.Zero()
	authorization, err := gymaccount.SignChallenge(challenge, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.CompleteChallenge(context.Background(), login.Token, challenge.ChallengeID, authorization); err != nil {
		t.Fatal(err)
	}
	mining, err := store.MintMining(context.Background(), login.Token)
	if err != nil {
		t.Fatal(err)
	}
	return login, mining
}

// The generic authority supports multiple credentials. Each desktop child's
// registry must additionally pin the account, even when two sessions from the
// same shared account store are simultaneously valid.
func TestMiningAuthorizerBindsChildAccount(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, aliceMining := enrolledFixtureSession(t, store, "zero256")
	bob, bobMining := enrolledFixtureSession(t, store, "incrementing256")
	if alice.Account.ID == bob.Account.ID {
		t.Fatal("distinct fixture principals acquired one account")
	}
	authorizer := miningAuthorizer{store: store, accountID: alice.Account.ID}
	called := false
	if err := authorizer.WithSession(context.Background(), bobMining.Token, func(string, func() error) error {
		called = true
		return nil
	}); !errors.Is(err, gymaccount.ErrForbidden) || called {
		t.Fatal("another valid account entered the child registry authority")
	}
	expected := accountReferences(t)["zero256"].Challenge.GymPublicKey
	if err := authorizer.WithSession(context.Background(), aliceMining.Token, func(key string, recheck func() error) error {
		if key != expected {
			t.Fatal("authorized child supplied another gym key")
		}
		called = true
		return recheck()
	}); err != nil || !called {
		t.Fatalf("own account could not enter child authority: %v", err)
	}
}

func TestAccountPilotProviderCannotSelectPrincipalOrResetAssurance(t *testing.T) {
	provider := fixtureProvider{}
	for _, input := range []string{"", "zero256", "SLUG-PUBLIC-FIXTURE-LOGIN-C", `{"subject":"zero256","recovery_verified":true}`} {
		if _, err := provider.Verify(context.Background(), input); err == nil {
			t.Fatal("unverified caller data became a fixture principal")
		}
	}
	principal, err := provider.Verify(context.Background(), "SLUG-PUBLIC-FIXTURE-LOGIN-A")
	if err != nil || principal.Issuer != fixtureIssuer || principal.Subject != "zero256" || principal.RecoveryVerified || principal.RecoveryEventID != "" {
		t.Fatal("login fixture silently granted reset assurance")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Verify(ctx, "SLUG-PUBLIC-FIXTURE-LOGIN-A"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled fixture authentication succeeded")
	}
}

func TestAccountPilotStoreRefusesForeignOrMissingEstablishedData(t *testing.T) {
	root := filepath.Join(t.TempDir(), "accounts")
	store, err := openStore(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := store.AuthorityID()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(root)
	if err != nil || reopened.AuthorityID() != identity {
		t.Fatal("account restart substituted authority identity")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "authority"), filepath.Join(root, "retained-authority")); err != nil {
		t.Fatal(err)
	}
	if replacement, err := openStore(root); err == nil {
		replacement.Close()
		t.Fatal("missing established authority was recreated")
	}
	if _, err := os.Stat(filepath.Join(root, "authority")); !os.IsNotExist(err) {
		t.Fatal("failed open changed missing established state")
	}
	foreign := filepath.Join(t.TempDir(), "old-pilot")
	if err := os.Mkdir(foreign, 0700); err != nil {
		t.Fatal(err)
	}
	marker := []byte(`{"version":2,"mode":"local-wallet-pilot"}`)
	path := filepath.Join(foreign, "SLUG-PILOT.json")
	if err := os.WriteFile(path, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if adopted, err := openStore(foreign); err == nil {
		adopted.Close()
		t.Fatal("old standalone pilot namespace was adopted")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(marker) {
		t.Fatal("foreign pilot data changed on rejection")
	}
}

func TestMiningAuthorizerRechecksRevocationAndCancellation(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	login, mining := enrolledFixtureSession(t, store, "zero256")
	authorizer := miningAuthorizer{store: store, accountID: login.Account.ID}
	ctx, cancel := context.WithCancel(context.Background())
	err = authorizer.WithSession(ctx, mining.Token, func(_ string, recheck func() error) error {
		cancel()
		return recheck()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("canceled child authorization retained permission")
	}
	if err := store.Logout(context.Background(), login.Token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	called := false
	if err := authorizer.WithSession(ctx, mining.Token, func(string, func() error) error { called = true; return nil }); err == nil || called {
		t.Fatal("revoked parent login retained child mining permission")
	}
}
