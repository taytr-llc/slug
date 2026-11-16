//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package accountpilot

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

const identityTestIssuer = "https://identity.slug.invalid"

// This key is the public RFC7515 Appendix A.2 test fixture. Never generate or
// load an operator key. The independent Python corpus checks its exact RFC JWS.
func identityTestVerifier(t *testing.T) (*gymidentity.Verifier, func(string, map[string]any) string) {
	t.Helper()
	raw, err := os.ReadFile("../../gymidentity/testdata/rfc7515_rsa.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		FixtureOnly  bool `json:"fixture_only"`
		N, E, D, Kid string
	}
	if json.Unmarshal(raw, &f) != nil || !f.FixtureOnly || f.Kid != "rfc7515-a2" {
		t.Fatal("published RSA fixture unavailable")
	}
	number := func(s string) *big.Int {
		b, e := base64.RawURLEncoding.DecodeString(s)
		if e != nil {
			t.Fatal("invalid public RSA fixture")
		}
		return new(big.Int).SetBytes(b)
	}
	key := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: number(f.N), E: int(number(f.E).Int64())}, D: number(f.D)}
	jwks, _ := json.Marshal(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": f.Kid, "alg": "RS256", "use": "sig", "n": f.N, "e": f.E}}})
	verifier, err := gymidentity.NewVerifier(gymidentity.Config{Version: 1, Issuer: identityTestIssuer, Audience: "slug-desktop-fixture", ResetAudience: "slug-reset-fixture", EpochClaim: "slug_auth_epoch", PublicJWKS: jwks})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(typ string, claims map[string]any) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": f.Kid, "typ": typ})
		payload, e := json.Marshal(claims)
		if e != nil {
			t.Fatal(e)
		}
		encoded := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
		digest := sha256.Sum256([]byte(encoded))
		signature, e := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
		if e != nil {
			t.Fatal("public fixture signing failed")
		}
		return encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	return verifier, sign
}

// Only the test coordinator uses this constructor. The shipped set27 account
// demo and schema1 directory remain unchanged; there is no external JWT input
// in the webview or production signup entry point in this set.
func identityTestPilot(t *testing.T, config pilot.Config, store *gymaccount.Store, session gymaccount.Session) *Controller {
	t.Helper()
	ctx := context.Background()
	c := &Controller{config: config, store: store, state: "signing_in", fixture: "zero256", session: session}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.MkdirAll(filepath.Join(config.DataDir, "gyms"), 0700); err != nil {
		t.Fatal(err)
	}
	child, err := pilot.NewAccount(pilot.Config{DataDir: filepath.Join(config.DataDir, "gyms", session.Account.ID), NodeBinary: config.NodeBinary}, pilot.AccountConfig{
		Fixture: "zero256", AccountID: session.Account.ID, AuthorityID: store.AuthorityID(), RequireExisting: session.Account.Enrollment != nil,
		Authorizer: miningAuthorizer{store, session.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, session.Account.ID) },
	})
	if err != nil {
		t.Fatal(err)
	}
	c.child = child
	if err := child.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if session.Account.Enrollment == nil || session.Account.RecoveryRequired {
		action := "enroll"
		if session.Account.RecoveryRequired {
			action = "recover"
		}
		challenge, err := store.BeginChallenge(ctx, session.Token, action)
		if err != nil {
			t.Fatal(err)
		}
		var signature gymaccount.Authorization
		err = store.WithSession(ctx, session.Token, gymaccount.Enrollment, func(account gymaccount.Account, recheck func() error) error {
			if account.ID != challenge.AccountID {
				return gymaccount.ErrForbidden
			}
			var e error
			signature, e = child.SignAccountChallenge(ctx, challenge)
			if e != nil {
				return e
			}
			return recheck()
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = store.CompleteChallenge(ctx, session.Token, challenge.ChallengeID, signature); err != nil {
			t.Fatal(err)
		}
	}
	c.state = "signed_in"
	return c
}

func TestIdentityAssertionResetRecoveryNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an immutable normal-PoW node")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("absolute immutable node required")
	}
	root := t.TempDir()
	if artifacts := os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"); artifacts != "" {
		var e error
		root, e = os.MkdirTemp(artifacts, "identity-normal-")
		if e != nil {
			t.Fatal(e)
		}
		t.Log("public fixture identity artifacts", root)
	}
	config := pilot.Config{DataDir: filepath.Join(root, "IdentityFixture"), NodeBinary: binary}
	if err := os.Mkdir(config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	verifier, sign := identityTestVerifier(t)
	authorityConfig := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: verifier.ConfigDigest(), MaxLoginAttempts: 256, Accounts: gymaccount.Config{Version: 1, MaxSessions: 512, MaxChallenges: 256, MaxEvents: 2048, Principals: []gymaccount.FixturePrincipal{{Issuer: identityTestIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: identityTestIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}}}
	authorityPath := filepath.Join(config.DataDir, "authority")
	store, err := gymaccount.CreateIdentity(authorityPath, authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); checkAccountListenersClosed(t, config.DataDir) })
	ctx := context.Background()
	loginToken := func(attempt gymaccount.LoginAttempt, epoch uint64) string {
		now := time.Now().Unix()
		return sign("JWT", map[string]any{"iss": identityTestIssuer, "aud": "slug-desktop-fixture", "sub": "zero256", "nonce": attempt.Nonce, "iat": now, "exp": now + 120, "auth_time": now, "slug_auth_epoch": epoch})
	}
	assertion := func(attempt gymaccount.LoginAttempt, epoch uint64) gymaccount.VerifiedLogin {
		verified, e := verifier.VerifyLogin(ctx, loginToken(attempt, epoch), attempt.Nonce)
		if e != nil {
			t.Fatal(e)
		}
		return verified
	}
	login := func(epoch uint64) gymaccount.Session {
		attempt, e := store.IssueLogin(ctx, identityTestIssuer)
		if e != nil {
			t.Fatal(e)
		}
		session, e := verifier.Login(ctx, store, attempt.ID, loginToken(attempt, epoch))
		if e != nil {
			t.Fatal(e)
		}
		return session
	}
	session := login(1)
	c := identityTestPilot(t, config, store, session)
	initial := c.Snapshot()
	if initial.Attempts != 0 || initial.Height != 0 || initial.Account.State != "signed_in" {
		t.Fatal("identity login mined implicitly")
	}
	expected := recoveryReferences(t)["zero256"]
	assertAccountIdentity(t, initial, expected)
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 60*time.Second, func(s pilot.Snapshot) bool { return s.Height >= 2 && s.ProspectCount >= 3 })
	c.Stop()
	before := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	receipt, err := c.Receipt(before.Prospects[0].Sequence)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "retained-recovery.json")
	if err = c.ExportRecovery(backup); err != nil {
		t.Fatal(err)
	}
	fixtureBundle(t, backup, expected)
	if err = c.VerifyBackup(backup); err != nil {
		t.Fatal(err)
	}
	restored := waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool { return s.BackupState == "succeeded" || s.BackupState == "failed" })
	if restored.BackupState != "succeeded" {
		t.Fatal("retained identity backup failed")
	}
	oldAttempt, err := store.IssueLogin(ctx, identityTestIssuer)
	if err != nil {
		t.Fatal(err)
	}
	oldAssertion := assertion(oldAttempt, 1)
	oldMining, err := store.MintMining(ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	verifiedAt := time.Now().Unix()
	eventID := strings.Repeat("28", 32)
	resetProof := func(stage string) gymaccount.VerifiedReset {
		now := time.Now().Unix()
		token := sign("slug-reset+jwt", map[string]any{"iss": identityTestIssuer, "aud": "slug-reset-fixture", "sub": "zero256", "iat": now, "exp": now + 120, "event_id": eventID, "stage": stage, "from_epoch": 1, "to_epoch": 2, "verified_at": verifiedAt})
		proof, e := verifier.VerifyReset(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		return proof
	}
	pending, inserted, err := store.BeginReset(ctx, resetProof("begin"))
	if err != nil || !inserted || pending.Status != "pending" {
		t.Fatal("verified reset barrier failed", err)
	}
	for _, credential := range []struct {
		token string
		cap   gymaccount.Capability
	}{{session.Token, gymaccount.View}, {oldMining.Token, gymaccount.Mining}} {
		if store.WithSession(ctx, credential.token, credential.cap, func(gymaccount.Account, func() error) error { return nil }) == nil {
			t.Fatal("pending reset retained old capability")
		}
	}
	if _, err = store.LoginWithAssertion(ctx, oldAttempt.ID, oldAssertion); err == nil {
		t.Fatal("old assertion crossed pending reset")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = gymaccount.OpenIdentity(authorityPath, authorityConfig)
	if err != nil {
		t.Fatal("pending authority reopen", err)
	}
	if _, err = store.LoginWithAssertion(ctx, oldAttempt.ID, oldAssertion); err == nil {
		t.Fatal("restart cleared reset barrier")
	}
	if _, err = store.Login(ctx, gymaccount.Principal{Issuer: identityTestIssuer, Subject: "zero256", AuthenticatedAtMS: time.Now().UnixMilli()}); err == nil {
		t.Fatal("legacy login bypassed schema2")
	}
	completed, inserted, err := store.CompleteReset(ctx, resetProof("complete"))
	if err != nil || !inserted || completed.Status != "completed" {
		t.Fatal("verified completion failed", err)
	}
	replay, inserted, err := store.CompleteReset(ctx, resetProof("complete"))
	if err != nil || inserted || replay != completed {
		t.Fatal("reset replay changed hold", err)
	}
	if _, err = store.LoginWithAssertion(ctx, oldAttempt.ID, oldAssertion); err == nil {
		t.Fatal("pre-reset authentication entered new generation")
	}
	freshAttempt, err := store.IssueLogin(ctx, identityTestIssuer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.LoginWithAssertion(ctx, freshAttempt.ID, assertion(freshAttempt, 1)); err == nil {
		t.Fatal("old authentication epoch gained access with fresh nonce")
	}
	recovered := login(2)
	if recovered.Account.ID != session.Account.ID || recovered.Account.Generation != session.Account.Generation+1 || !recovered.Account.RecoveryRequired {
		t.Fatal("reset replaced account or skipped original-key proof")
	}
	c = identityTestPilot(t, config, store, recovered)
	held := c.Snapshot()
	assertAccountIdentity(t, held, expected)
	if !held.Account.Held || held.Account.CanExport || held.Account.HoldUntilMS != completed.HoldUntilMS {
		t.Fatal("fixed hold missing after recovery")
	}
	denied := filepath.Join(root, "held-export.json")
	if c.ExportRecovery(denied) == nil {
		t.Fatal("identity reset allowed held export")
	}
	if _, err = os.Stat(denied); !os.IsNotExist(err) {
		t.Fatal("held export created a file")
	}
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 60*time.Second, func(s pilot.Snapshot) bool {
		return s.Height > before.Height && s.ProspectCount >= before.ProspectCount+3
	})
	c.Stop()
	after := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	retained, err := c.Receipt(receipt.Sequence)
	if err != nil || !reflect.DeepEqual(retained, receipt) {
		t.Fatal("provider reset changed historical birth")
	}
	if after.Account.HoldUntilMS != completed.HoldUntilMS {
		t.Fatal("mining changed reset hold")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = gymaccount.OpenIdentity(authorityPath, authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	c = identityTestPilot(t, config, store, login(2))
	final := c.Snapshot()
	assertAccountIdentity(t, final, expected)
	if final.Account.ID != session.Account.ID || final.Account.HoldUntilMS != completed.HoldUntilMS || final.ProspectCount != after.ProspectCount {
		t.Fatal("reopen lost identity, hold or receipts")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	checkAccountListenersClosed(t, config.DataDir)
	result := map[string]any{"status": "passed", "fixture_only": true, "same_wallet_and_gym": true, "signed_assertion_verified": true, "pending_barrier_survived_restart": true, "old_authentication_epoch_rejected": true, "legacy_login_refused": true, "same_event_hold_preserved": true, "retained_export_fresh_restore": true, "held_export_rejected": true, "mining_continued_during_hold": true, "historical_birth_preserved": true, "final_height": final.Height, "final_receipts": final.ProspectCount, "hold_until_ms": completed.HoldUntilMS, "verifier_config_hash": verifier.ConfigDigest()}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "result.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
