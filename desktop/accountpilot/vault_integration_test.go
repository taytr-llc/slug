//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package accountpilot

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

// These KEKs are the published oracle's byte ranges 00..1f,20..3f,40..5f,
// 60..7f. This test-only provider does not read or create any operator secret.
type vaultIntegrationWrapper struct {
	mu        sync.Mutex
	counter   uint64
	unwraps   map[string]uint64
	deniedGym uint64
	denyGym   bool
}

func (*vaultIntegrationWrapper) ID() string { return "public-fixture-wrapper-v1" }

func vaultIntegrationKEK(ref string) ([32]byte, error) {
	var key [32]byte
	base := 0
	switch ref {
	case "fixture-recovery-v1":
		base = 0
	case "fixture-recovery-v2":
		base = 32
	case "fixture-gym-v1":
		base = 64
	case "fixture-gym-v2":
		base = 96
	default:
		return key, errors.New("unknown public fixture wrapping key")
	}
	for i := range key {
		key[i] = byte(base + i)
	}
	return key, nil
}

func (w *vaultIntegrationWrapper) Wrap(ctx context.Context, ref string, dek [32]byte, aad []byte) ([]byte, error) {
	defer clear(dek[:])
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := vaultIntegrationKEK(ref)
	if err != nil {
		return nil, err
	}
	defer clear(key[:])
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	w.counter++
	counter := w.counter
	w.mu.Unlock()
	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint64(nonce, counter)
	nonce[11] = 29
	return aead.Seal(nonce, nonce, dek[:], aad), nil
}

func (w *vaultIntegrationWrapper) Unwrap(ctx context.Context, ref string, blob, aad []byte) ([32]byte, error) {
	var result [32]byte
	w.mu.Lock()
	if w.unwraps == nil {
		w.unwraps = make(map[string]uint64)
	}
	w.unwraps[ref]++
	deny := w.denyGym && strings.HasPrefix(ref, "fixture-gym-")
	if deny {
		w.deniedGym++
	}
	w.mu.Unlock()
	if deny {
		return result, errors.New("fixture wrapping provider unavailable")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(blob) != 60 {
		return result, errors.New("invalid fixture wrapped payload")
	}
	key, err := vaultIntegrationKEK(ref)
	if err != nil {
		return result, err
	}
	defer clear(key[:])
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return result, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return result, err
	}
	plain, err := aead.Open(nil, blob[:12], blob[12:], aad)
	if err != nil {
		return result, err
	}
	defer clear(plain)
	copy(result[:], plain)
	return result, nil
}

func (w *vaultIntegrationWrapper) counts() (recovery, gym, denied uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ref, count := range w.unwraps {
		if strings.HasPrefix(ref, "fixture-recovery-") {
			recovery += count
		} else {
			gym += count
		}
	}
	return recovery, gym, w.deniedGym
}
func (w *vaultIntegrationWrapper) failGym(fail bool) { w.mu.Lock(); w.denyGym = fail; w.mu.Unlock() }

func vaultIntegrationPilot(t *testing.T, config pilot.Config, store *gymaccount.Store, session gymaccount.Session, vault *recoveryvault.Vault, existing bool, signed *atomic.Uint64) *Controller {
	t.Helper()
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
		Fixture: "zero256", AccountID: session.Account.ID, AuthorityID: store.AuthorityID(), RequireExisting: existing,
		Authorizer: miningAuthorizer{store, session.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, session.Account.ID) },
		AdmissionSigner: func(ctx context.Context, token string, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
			auth, e := vault.SignAdmission(ctx, token, initial, proof)
			if e == nil {
				signed.Add(1)
			}
			return auth, e
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.child = child
	if err = child.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.state = "signed_in"
	return c
}

func vaultIntegrationRestore(t *testing.T, root, path string, expected recoveryReference) {
	t.Helper()
	bundle := fixtureBundle(t, path, expected)
	if !reflect.DeepEqual(bundle.Manifest.Accounts, recovery.DefaultAccounts()) {
		t.Fatal("vault changed immutable snapshot cursors")
	}
	// No local pilot wallet exists yet: the only restore input is the actual
	// authorized vault export, plus a different public fixture unlock password.
	loader := wallet.NewLoader(&chaincfg.MainNetParams, filepath.Join(root, "file-only-vault-restore"), true, 5*time.Second, 20)
	password := []byte("PUBLIC-VAULT-FRESH-RESTORE-ONLY")
	restored, err := loader.RestoreRecoveryWallet(password, password, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, loaded := loader.LoadedWallet(); loaded {
			if err := loader.UnloadWallet(); err != nil {
				t.Error(err)
			}
		}
	})
	if err = restored.Unlock(password, nil); err != nil {
		t.Fatal(err)
	}
	identity, err := restored.GymIdentity(context.Background())
	if err != nil || identity.GymID != expected.Export.Manifest.GymID || identity.PublicKey != expected.Export.Manifest.GymPublicKey {
		t.Fatal("file-only vault restore changed gym", err)
	}
	exported, err := restored.ExportRecovery(context.Background())
	if err != nil || exported.Manifest.RootPublicKey != expected.Export.Manifest.RootPublicKey || !reflect.DeepEqual(exported.Manifest, bundle.Manifest) {
		t.Fatal("file-only vault restore changed retained root/manifest", err)
	}
	reference := accountReferences(t)["zero256"]
	auth, err := restored.SignAccountChallenge(context.Background(), reference.Challenge.AuthorityID, reference.Challenge)
	if err != nil || auth != reference.Authorization {
		t.Fatal("vault restore differs from independent signing vector", err)
	}
	if err = loader.UnloadWallet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryVaultResetMiningNormalPoW(t *testing.T) {
	node := os.Getenv("SLUG_DESKTOP_NODE")
	if node == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an immutable normal-PoW node")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute immutable node required")
	}
	root := t.TempDir()
	if artifacts := os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"); artifacts != "" {
		var err error
		root, err = os.MkdirTemp(artifacts, "vault-normal-")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("public fixture vault artifacts", root)
	}
	config := pilot.Config{DataDir: filepath.Join(root, "VaultFixture"), NodeBinary: node}
	if err := os.Mkdir(config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	verifier, sign := identityTestVerifier(t)
	accountConfig := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: verifier.ConfigDigest(), MaxLoginAttempts: 256, Accounts: gymaccount.Config{Version: 1, MaxSessions: 512, MaxChallenges: 256, MaxEvents: 2048, Principals: []gymaccount.FixturePrincipal{{Issuer: identityTestIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: identityTestIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}}}
	accountPath := filepath.Join(config.DataDir, "authority")
	store, err := gymaccount.CreateIdentity(accountPath, accountConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); checkAccountListenersClosed(t, config.DataDir) })
	login := func(epoch uint64) gymaccount.Session {
		attempt, e := store.IssueLogin(ctx, identityTestIssuer)
		if e != nil {
			t.Fatal(e)
		}
		now := time.Now().Unix()
		token := sign("JWT", map[string]any{"iss": identityTestIssuer, "aud": "slug-desktop-fixture", "sub": "zero256", "nonce": attempt.Nonce, "iat": now, "exp": now + 120, "auth_time": now, "slug_auth_epoch": epoch})
		session, e := verifier.Login(ctx, store, attempt.ID, token)
		if e != nil {
			t.Fatal(e)
		}
		return session
	}
	session := login(1)
	wrapper := new(vaultIntegrationWrapper)
	vaultConfig := recoveryvault.Config{Version: 1, WrapperID: wrapper.ID(), RecoveryKeyRef: "fixture-recovery-v1", GymKeyRef: "fixture-gym-v1", AllowedRecoveryKeyRefs: []string{"fixture-recovery-v1", "fixture-recovery-v2"}, AllowedGymKeyRefs: []string{"fixture-gym-v1", "fixture-gym-v2"}, MaxOperations: 128, Policy: prospectnode.Policy{Version: 1, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}}
	vaultPath := filepath.Join(config.DataDir, "vault")
	vault, err := recoveryvault.Create(ctx, vaultPath, vaultConfig, store, wrapper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vault.Close(); err != nil {
			t.Error(err)
		}
	})
	retention, inserted, err := vault.ProvisionFixture(ctx, session.Token)
	if err != nil || !inserted {
		t.Fatal("vault provision", err)
	}
	again, inserted, err := vault.ProvisionFixture(ctx, session.Token)
	if err != nil || inserted || !reflect.DeepEqual(retention, again) {
		t.Fatal("provision retry replaced immutable retention", err)
	}
	enrolled, err := vault.EnsureEnrollment(ctx, session.Token)
	if err != nil || !reflect.DeepEqual(enrolled, retention) {
		t.Fatal("vault enrollment", err)
	}
	expected := recoveryReferences(t)["zero256"]
	if retention.Binding.RootPublicKey != expected.Export.Manifest.RootPublicKey || retention.Binding.GymID != expected.Export.Manifest.GymID {
		t.Fatal("vault provision differs from independent fixture")
	}
	backup := filepath.Join(root, "before-hold-recovery.json")
	if err = vault.ExportRecoveryFile(ctx, session.Token, backup); err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(beforeBytes)
	rotated, inserted, err := vault.Rewrap(ctx, session.Token, "fixture-recovery-v2", "fixture-gym-v2")
	if err != nil || !inserted || !reflect.DeepEqual(rotated, retention) {
		t.Fatal("rewrap changed retention", err)
	}
	rewrappedBackup := filepath.Join(root, "rewrapped-recovery.json")
	if err = vault.ExportRecoveryFile(ctx, session.Token, rewrappedBackup); err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(rewrappedBackup)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(afterBytes)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatal("rewrap changed actual recovery export")
	}
	vaultIntegrationRestore(t, root, rewrappedBackup, expected)
	if err = vault.Close(); err != nil {
		t.Fatal(err)
	}
	vault, err = recoveryvault.Open(ctx, vaultPath, vaultConfig, store, wrapper)
	if err != nil {
		t.Fatal(err)
	}
	status, err := vault.Status(ctx, session.Token)
	if err != nil || status.Receipt == nil || !reflect.DeepEqual(*status.Receipt, retention) || status.RecoveryWrapRevision != 2 || status.GymWrapRevision != 2 {
		t.Fatal("rewrapped vault reopen", err)
	}
	var signed atomic.Uint64
	c := vaultIntegrationPilot(t, config, store, session, vault, false, &signed)
	assertAccountIdentity(t, c.Snapshot(), expected)
	// A configured custody signer failure must leave the proof pending. The
	// already present local wallet must not silently sign in its place.
	wrapper.failGym(true)
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute)
	for {
		_, _, denied := wrapper.counts()
		if denied > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not reach configured vault signer")
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.Stop()
	deniedState := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	if deniedState.ProspectCount != 0 || signed.Load() != 0 {
		t.Fatal("vault signer failure fell back to local wallet")
	}
	wrapper.failGym(false)
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool { return s.Height >= 2 && s.ProspectCount >= 3 })
	c.Stop()
	before := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	first, err := c.Receipt(before.Prospects[0].Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if signed.Load() < before.ProspectCount {
		t.Fatal("worker receipts were not vault-authorized")
	}
	recoveryBeforeHold, gymBeforeHold, _ := wrapper.counts()
	verifiedAt := time.Now().Unix()
	eventID := strings.Repeat("29", 32)
	reset := func(stage string) gymaccount.VerifiedReset {
		now := time.Now().Unix()
		token := sign("slug-reset+jwt", map[string]any{"iss": identityTestIssuer, "aud": "slug-reset-fixture", "sub": "zero256", "iat": now, "exp": now + 120, "event_id": eventID, "stage": stage, "from_epoch": 1, "to_epoch": 2, "verified_at": verifiedAt})
		proof, e := verifier.VerifyReset(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		return proof
	}
	pending, inserted, err := store.BeginReset(ctx, reset("begin"))
	if err != nil || !inserted || pending.Status != "pending" {
		t.Fatal("vault reset barrier", err)
	}
	if err = vault.ExportRecoveryFile(ctx, session.Token, filepath.Join(root, "pending-export.json")); err == nil {
		t.Fatal("pending reset released recovery")
	}
	if err = vault.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = gymaccount.OpenIdentity(accountPath, accountConfig)
	if err != nil {
		t.Fatal(err)
	}
	vault, err = recoveryvault.Open(ctx, vaultPath, vaultConfig, store, wrapper)
	if err != nil {
		t.Fatal(err)
	}
	completed, inserted, err := store.CompleteReset(ctx, reset("complete"))
	if err != nil || !inserted {
		t.Fatal(err)
	}
	recovered := login(2)
	if recovered.Account.ID != session.Account.ID || !recovered.Account.RecoveryRequired {
		t.Fatal("reset changed account or skipped gym proof")
	}
	retained, err := vault.EnsureEnrollment(ctx, recovered.Token)
	if err != nil || !reflect.DeepEqual(retained, retention) {
		t.Fatal("held recovery replaced retained wallet", err)
	}
	deniedPath := filepath.Join(root, "held-export.json")
	if err = vault.ExportRecoveryFile(ctx, recovered.Token, deniedPath); !errors.Is(err, gymaccount.ErrHeld) {
		t.Fatalf("held full recovery export: %v", err)
	}
	if _, _, err = vault.Rewrap(ctx, recovered.Token, "fixture-recovery-v1", "fixture-gym-v1"); !errors.Is(err, gymaccount.ErrHeld) {
		t.Fatalf("held recovery authority rewrap: %v", err)
	}
	if _, err = os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatal("held export created a file")
	}
	recoveryAfterDenial, _, _ := wrapper.counts()
	if recoveryAfterDenial != recoveryBeforeHold {
		t.Fatal("hold recovery used full recovery envelope")
	}
	c = vaultIntegrationPilot(t, config, store, recovered, vault, true, &signed)
	held := c.Snapshot()
	assertAccountIdentity(t, held, expected)
	if !held.Account.Held || held.Account.CanExport || held.Account.HoldUntilMS != completed.HoldUntilMS {
		t.Fatal("same-wallet vault recovery lost hold")
	}
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool {
		return s.Height > before.Height && s.ProspectCount >= before.ProspectCount+3
	})
	c.Stop()
	after := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	historical, err := c.Receipt(first.Sequence)
	if err != nil || !reflect.DeepEqual(historical, first) {
		t.Fatal("vault reset changed historical birth", err)
	}
	recoveryAfterHold, gymAfterHold, _ := wrapper.counts()
	if recoveryAfterHold != recoveryBeforeHold || gymAfterHold <= gymBeforeHold {
		t.Fatal("held mining did not use only the gym envelope")
	}
	if err = vault.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = gymaccount.OpenIdentity(accountPath, accountConfig)
	if err != nil {
		t.Fatal(err)
	}
	vault, err = recoveryvault.Open(ctx, vaultPath, vaultConfig, store, wrapper)
	if err != nil {
		t.Fatal(err)
	}
	finalSession := login(2)
	finalStatus, err := vault.Status(ctx, finalSession.Token)
	if err != nil || finalStatus.Receipt == nil || !reflect.DeepEqual(*finalStatus.Receipt, retention) || finalSession.Account.HoldUntilMS != completed.HoldUntilMS || finalSession.Account.RecoveryRequired {
		t.Fatal("reopen changed retained binding or completed recovery", err)
	}
	if err = vault.Close(); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	checkAccountListenersClosed(t, config.DataDir)
	result := map[string]any{"status": "passed", "fixture_only": true, "same_root_and_gym": true, "idempotent_provisioning": true, "rewrap_export_unchanged": true, "file_only_restore_before_local_wallet_exists": true, "configured_signer_failure_no_fallback": true, "pending_reset_reopened": true, "held_export_and_rewrap_denied": true, "held_recovery_unwraps": recoveryAfterHold - recoveryBeforeHold, "held_gym_unwraps": gymAfterHold - gymBeforeHold, "vault_admission_signatures": signed.Load(), "historical_birth_preserved": true, "final_height": after.Height, "final_receipts": after.ProspectCount, "retention_id": retention.RetentionID}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "result.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
