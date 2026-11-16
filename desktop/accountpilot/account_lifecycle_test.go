package accountpilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
)

func lifecycleStore(t *testing.T) (*gymaccount.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "accounts")
	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dir
}

func lifecycleSession(t *testing.T, store *gymaccount.Store, name string) gymaccount.Session {
	t.Helper()
	principal, err := verifyFixture(context.Background(), name, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Login(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func lifecycleEnroll(t *testing.T, store *gymaccount.Store, session gymaccount.Session, name string) {
	t.Helper()
	data, err := os.ReadFile("../../gymaccount/testdata/enrollment_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Name    string `json:"fixture_name"`
			Private string `json:"test_only_private_key"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures.Cases {
		if f.Name != name {
			continue
		}
		raw, err := hex.DecodeString(f.Private)
		if err != nil || len(raw) != 32 {
			t.Fatal("invalid public fixture key")
		}
		key, _ := btcec.PrivKeyFromBytes(raw)
		clear(raw)
		defer key.Zero()
		challenge, err := store.BeginChallenge(context.Background(), session.Token, "enroll")
		if err != nil {
			t.Fatal(err)
		}
		auth, err := gymaccount.SignChallenge(challenge, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.CompleteChallenge(context.Background(), session.Token, challenge.ChallengeID, auth); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("missing public fixture")
}

func TestAccountCleanupFailureSurvivesDroppedChild(t *testing.T) {
	store, dir := lifecycleStore(t)
	session := lifecycleSession(t, store, "zero256")
	c := &Controller{store: store, config: pilot.Config{DataDir: dir}}
	closed := false
	err := c.cleanupAccount(context.Background(), session.Token, func() error { closed = true; return errors.New("sensitive internal path") })
	if err == nil || !closed || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("cleanup failure lost or leaked")
	}
	if err := store.WithSession(context.Background(), session.Token, gymaccount.View, func(gymaccount.Account, func() error) error {
		t.Fatal("logged-out session reached callback")
		return nil
	}); !errors.Is(err, gymaccount.ErrUnauthorized) {
		t.Fatal("cleanup did not revoke session", err)
	}
	if err := c.Close(); err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("final close lost cleanup error")
	}
	if err := c.Close(); err == nil {
		t.Fatal("repeat close lost cleanup error")
	}
}

func TestAccountCanceledCleanupStillClosesChild(t *testing.T) {
	store, dir := lifecycleStore(t)
	session := lifecycleSession(t, store, "zero256")
	c := &Controller{store: store, config: pilot.Config{DataDir: dir}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed := false
	if err := c.cleanupAccount(ctx, session.Token, func() error { closed = true; return nil }); err == nil || !closed {
		t.Fatal("session cleanup cancellation prevented child drain")
	}
	if c.Close() == nil {
		t.Fatal("logout failure was forgotten")
	}
}

func TestAccountFailedProjectionHidesRetainedChild(t *testing.T) {
	// A failed reset can retain the child for explicit logout. The projection
	// must not touch that child/store or attach an account ID to failed state.
	c := &Controller{state: "failed", fixture: "zero256", lastError: "account_unavailable", child: new(pilot.Controller), session: gymaccount.Session{Token: strings.Repeat("01", 32), Account: gymaccount.Account{ID: strings.Repeat("02", 32)}}}
	s := c.Snapshot()
	if s.Account == nil || s.Account.State != "failed" || s.Account.Fixture != "zero256" || s.Account.LastError != "account_unavailable" || s.Account.ID != "" || s.PayoutAddress != "" {
		t.Fatal("failed account projection exposed retained child data")
	}
}

func TestAccountResetThenDetachOldSession(t *testing.T) {
	store, dir := lifecycleStore(t)
	session := lifecycleSession(t, store, "zero256")
	principal, err := verifyFixture(context.Background(), "zero256", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, inserted, err := store.ApplyReset(context.Background(), principal); err != nil || !inserted {
		t.Fatal("fixture reset failed", err)
	}
	c := &Controller{store: store, config: pilot.Config{DataDir: dir}, session: session}
	if err := c.detach(context.Background()); err != nil {
		t.Fatal("generation change prevented old session detach", err)
	}
	if c.session.Token != "" || c.Close() != nil {
		t.Fatal("reset cleanup did not finish cleanly")
	}
}

func TestAccountChildSessionMismatchDeniedBeforeAccess(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-chain child controller")
	}
	if runtime.GOOS == "windows" {
		t.Skip("test version executable uses POSIX shell; Windows runtime remains unvalidated")
	}
	store, dir := lifecycleStore(t)
	a := lifecycleSession(t, store, "zero256")
	b := lifecycleSession(t, store, "incrementing256")
	lifecycleEnroll(t, store, b, "incrementing256")
	node := filepath.Join(t.TempDir(), "version-only-node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\nprintf 'slugd version 0.27.0\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// NewAccount creates only a never-started identity and instance lock. This
	// test never starts a node, opens a wallet, hashes or mines.
	child, err := pilot.NewAccount(pilot.Config{DataDir: filepath.Join(dir, "gyms", a.Account.ID), NodeBinary: node}, pilot.AccountConfig{Fixture: "zero256", AccountID: a.Account.ID, AuthorityID: store.AuthorityID(), Authorizer: miningAuthorizer{store: store, accountID: a.Account.ID}, MiningToken: func(context.Context) (string, error) { return "", gymaccount.ErrUnauthorized }})
	if err != nil {
		t.Fatal(err)
	}
	c := &Controller{store: store, config: pilot.Config{DataDir: dir, NodeBinary: node}, child: child, session: b, state: "signed_in"}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, _, err := c.active(); err == nil {
		t.Fatal("child A accepted captured session B")
	}
	if got := c.Snapshot(); got.Account.State != "expired" || got.Account.ID != "" || got.PayoutAddress != "" {
		t.Fatal("snapshot mixed two account identities")
	}
	// Even if a stale local snapshot claims A, authorization of token B must
	// pin the actual store account again before accessing A's retained data.
	c.session.Account = a.Account
	if _, err := c.Receipt(1); !errors.Is(err, gymaccount.ErrForbidden) {
		t.Fatal("receipt did not reject actual token identity", err)
	}
	path := filepath.Join(t.TempDir(), "recovery.json")
	if err := c.ExportRecovery(path); !errors.Is(err, gymaccount.ErrForbidden) {
		t.Fatal("export did not reject actual token identity", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cross-account export created file")
	}
	if err := c.VerifyBackup(path); !errors.Is(err, gymaccount.ErrForbidden) {
		t.Fatal("backup did not reject actual token identity", err)
	}
	if got := c.Snapshot(); got.Account.State != "expired" || got.Account.ID != "" {
		t.Fatal("snapshot trusted stale captured account over token")
	}
	t.Run("failed-state-retains-valid-session", func(t *testing.T) {
		// Model a provider failure before ApplyReset: the original child and
		// login remain valid for cleanup, but may no longer perform actions.
		c.session, c.state = a, "failed"
		if err := store.WithSession(context.Background(), a.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); err != nil {
			t.Fatal("fixture must retain a valid session", err)
		}
		if got, token, err := c.active(); err == nil || got != nil || token != "" {
			t.Fatal("failed state exposed an operational child and token")
		}
		if err := c.Start(); err == nil || err.Error() != "account unavailable" {
			t.Fatal("failed state reached mining authorization", err)
		}
		if _, err := c.Receipt(1); err == nil || err.Error() != "account unavailable" {
			t.Fatal("failed state reached receipt storage", err)
		}
		if err := c.ExportRecovery(path); err == nil || err.Error() != "account unavailable" {
			t.Fatal("failed state reached export authorization", err)
		}
		if err := c.VerifyBackup(path); err == nil || err.Error() != "account unavailable" {
			t.Fatal("failed state reached backup verification", err)
		}
	})
}

func TestAccountPilotCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	dir := filepath.Join(t.TempDir(), "accounts")
	if c, err := New(pilot.Config{DataDir: dir, NodeBinary: filepath.Join(t.TempDir(), "node-not-opened")}); err == nil {
		_ = c.Close()
		t.Fatal("cheap desktop authority accepted")
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("cheap profile created account identity before refusing")
	}
}
