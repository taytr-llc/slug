//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package accountpilot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

func providerIntegrationPilot(t *testing.T, config pilot.Config, store *gymaccount.Store, session gymaccount.Session, vault *recoveryvault.Vault, fixture string, existing bool, signed *atomic.Uint64) *Controller {
	t.Helper()
	c := &Controller{config: config, store: store, state: "signing_in", fixture: fixture, session: session}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.MkdirAll(filepath.Join(config.DataDir, "gyms"), 0700); err != nil {
		t.Fatal(err)
	}
	child, err := pilot.NewAccount(pilot.Config{DataDir: filepath.Join(config.DataDir, "gyms", session.Account.ID), NodeBinary: config.NodeBinary}, pilot.AccountConfig{
		Fixture: fixture, AccountID: session.Account.ID, AuthorityID: store.AuthorityID(), RequireExisting: existing,
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

func providerIntegrationRestore(t *testing.T, root, path, fixture string) {
	t.Helper()
	ctx := context.Background()
	expected := recoveryReferences(t)[fixture]
	bundle := fixtureBundle(t, path, expected)
	loader := wallet.NewLoader(&chaincfg.MainNetParams, filepath.Join(root, "fresh-restore-"+fixture), true, 5*time.Second, 20)
	password := []byte("PUBLIC-PROVIDER-FIXTURE-RESTORE-ONLY")
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
	identity, err := restored.GymIdentity(ctx)
	if err != nil || identity.GymID != expected.Export.Manifest.GymID || identity.PublicKey != expected.Export.Manifest.GymPublicKey {
		t.Fatal("provider export restore changed gym", err)
	}
	exported, err := restored.ExportRecovery(ctx)
	if err != nil || !reflect.DeepEqual(exported.Manifest, bundle.Manifest) {
		t.Fatal("provider file-only restore changed root or manifest", err)
	}
	reference := accountReferences(t)[fixture]
	auth, err := restored.SignAccountChallenge(ctx, reference.Challenge.AuthorityID, reference.Challenge)
	if err != nil || auth != reference.Authorization {
		t.Fatal("provider restored signer differs from independent vector", err)
	}
	if err = loader.UnloadWallet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderCoordinatorVaultMiningNormalPoW(t *testing.T) {
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
		root, err = os.MkdirTemp(artifacts, "provider-normal-")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("public fixture provider artifacts", root)
	}
	config := pilot.Config{DataDir: filepath.Join(root, "ProviderFixture"), NodeBinary: node}
	if err := os.Mkdir(config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, sign := identityTestVerifier(t)
	verifier, err := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if err != nil {
		t.Fatal(err)
	}
	accountConfig := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: verifier.ConfigDigest(), MaxLoginAttempts: 256, Accounts: gymaccount.Config{Version: 1, MaxSessions: 512, MaxChallenges: 256, MaxEvents: 2048, Principals: []gymaccount.FixturePrincipal{{Issuer: identityTestIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: identityTestIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}}}
	accountPath := filepath.Join(config.DataDir, "authority")
	store, err := gymaccount.CreateIdentity(accountPath, accountConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); checkAccountListenersClosed(t, config.DataDir) })
	providerConfig := gymprovider.Config{Version: 1, Provider: gymprovider.ProviderFixture, Issuer: gymprovider.FixtureIssuer, TenantID: "fixture-tenant", ClientID: "fixture-client", ConnectionID: "fixture-database", AuthorityID: store.AuthorityID(), VerifierConfigHash: verifier.ConfigDigest(), MaxLoginFlows: 128, MaxResetFlows: 16}
	coordinatorPath := filepath.Join(config.DataDir, "coordinator")
	coordinator, err := gymprovider.Create(ctx, coordinatorPath, providerConfig, store, verifier)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { coordinator.Close() })
	providerPath := filepath.Join(config.DataDir, "provider-state.json")
	provider := newDurableProviderFixture(t, providerPath, providerConfig, sign, false)
	wrapper := new(vaultIntegrationWrapper)
	vaultConfig := recoveryvault.Config{Version: 1, WrapperID: wrapper.ID(), RecoveryKeyRef: "fixture-recovery-v1", GymKeyRef: "fixture-gym-v1", AllowedRecoveryKeyRefs: []string{"fixture-recovery-v1", "fixture-recovery-v2"}, AllowedGymKeyRefs: []string{"fixture-gym-v1", "fixture-gym-v2"}, MaxOperations: 128, Policy: prospectnode.Policy{Version: 1, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}}
	vaultPath := filepath.Join(config.DataDir, "vault")
	vault, err := recoveryvault.Create(ctx, vaultPath, vaultConfig, store, wrapper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vault.Close() })
	reopen := func() {
		t.Helper()
		store, err = gymaccount.OpenIdentity(accountPath, accountConfig)
		if err != nil {
			t.Fatal(err)
		}
		coordinator, err = gymprovider.Open(ctx, coordinatorPath, providerConfig, store, verifier)
		if err != nil {
			t.Fatal(err)
		}
		vault, err = recoveryvault.Open(ctx, vaultPath, vaultConfig, store, wrapper)
		if err != nil {
			t.Fatal(err)
		}
		provider = newDurableProviderFixture(t, providerPath, providerConfig, sign, true)
	}
	results := make([]map[string]any, 0, 2)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		session, origin := provider.login(ctx, coordinator, fixture)
		retained, inserted, e := vault.ProvisionFixture(ctx, session.Token)
		if e != nil || !inserted {
			t.Fatal("coordinated provision", e)
		}
		if enrolled, e := vault.EnsureEnrollment(ctx, session.Token); e != nil || !reflect.DeepEqual(enrolled, retained) {
			t.Fatal("coordinated enrollment changed wallet", e)
		}
		expected := recoveryReferences(t)[fixture]
		backup := filepath.Join(root, fixture+"-recovery.json")
		if e = vault.ExportRecoveryFile(ctx, session.Token, backup); e != nil {
			t.Fatal(e)
		}
		providerIntegrationRestore(t, root, backup, fixture)
		var signed atomic.Uint64
		c := providerIntegrationPilot(t, config, store, session, vault, fixture, false, &signed)
		assertAccountIdentity(t, c.Snapshot(), expected)
		if e = c.Start(); e != nil {
			t.Fatal(e)
		}
		waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool { return s.Height >= 2 && s.ProspectCount >= 3 })
		c.Stop()
		before := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
		first, e := c.Receipt(before.Prospects[0].Sequence)
		if e != nil {
			t.Fatal(e)
		}
		other := "incrementing256"
		if fixture == other {
			other = "zero256"
		}
		otherSession, _ := provider.login(ctx, coordinator, other)
		oldFlow, e := coordinator.IssueLogin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		request, e := coordinator.IssueReset(ctx)
		if e != nil {
			t.Fatal(e)
		}
		op := provider.verifiedReset(request, fixture, session.Account.ID)
		if e = provider.commit(op.ID, gymprovider.ResetResult{}); e == nil || provider.data.Epochs[fixture] != 1 {
			t.Fatal("provider changed credentials without an acknowledged barrier")
		}
		pending, e := provider.begin(ctx, coordinator, op.ID)
		if e != nil || !pending.CanCommit || pending.Phase != "pending" || pending.Barrier.EventID == request.EventID {
			t.Fatal("coordinated begin or raw/principal event distinction", e)
		}
		if e = store.WithSession(ctx, session.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); e == nil {
			t.Fatal("begin left the old account session usable")
		}
		if e = store.WithSession(ctx, otherSession.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); e != nil {
			t.Fatal("reset revoked the other account", e)
		}
		badPermit := pending
		badPermit.ProviderOperationID = strings.Repeat("ff", 32)
		if e = provider.commit(op.ID, badPermit); e == nil || provider.data.Epochs[fixture] != 1 {
			t.Fatal("provider accepted a permit for another operation")
		}
		recoveryBefore, gymBefore, _ := wrapper.counts()
		if e = vault.ExportRecoveryFile(ctx, session.Token, filepath.Join(root, fixture+"-pending-denied.json")); e == nil {
			t.Fatal("pending reset exported recovery")
		}
		// Simulate abandonment after the verified barrier, before any provider
		// credential mutation. Restart all three independent durable stores.
		if e = errors.Join(coordinator.Close(), vault.Close(), c.Close()); e != nil {
			t.Fatal(e)
		}
		reopen()
		if got := provider.data.Operations[op.ID]; !reflect.DeepEqual(got, op) || got.Commits != 0 {
			t.Fatal("abandoned provider operation lost its immutable binding")
		}
		replayed, e := provider.begin(ctx, coordinator, op.ID)
		if e != nil || !reflect.DeepEqual(replayed, pending) {
			t.Fatal("resumed begin changed its original barrier", e)
		}
		if e = provider.commit(op.ID, replayed); e != nil {
			t.Fatal(e)
		}
		// Lost provider completion callback: reopen its own state and consume the
		// same pending acknowledgement again. Credential mutation stays once.
		provider = newDurableProviderFixture(t, providerPath, providerConfig, sign, true)
		if e = provider.commit(op.ID, replayed); e != nil || provider.data.Operations[op.ID].Commits != 1 {
			t.Fatal("provider replay changed credentials twice", e)
		}
		completed, e := provider.complete(ctx, coordinator, op.ID)
		if e != nil || completed.CanCommit {
			t.Fatal("coordinated completion", e)
		}
		historical, e := provider.begin(ctx, coordinator, op.ID)
		if e != nil || !reflect.DeepEqual(historical, completed) || provider.commit(op.ID, historical) == nil {
			t.Fatal("completed begin became a credential mutation permit", e)
		}
		if _, e = coordinator.FinishLogin(ctx, oldFlow.ID, provider.loginEvidence(oldFlow, origin.ID, "reuse")); e == nil {
			t.Fatal("pre-reset flow accepted stale original authentication")
		}
		stale, e := coordinator.IssueLogin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = coordinator.FinishLogin(ctx, stale.ID, provider.loginEvidence(stale, origin.ID, "reuse")); e == nil {
			t.Fatal("new transport silently upgraded the original authentication epoch")
		}
		recovered, _ := provider.login(ctx, coordinator, fixture)
		if recovered.Account.ID != session.Account.ID || !recovered.Account.RecoveryRequired || recovered.Account.HoldUntilMS != completed.Barrier.HoldUntilMS {
			t.Fatal("coordinated reset changed wallet account or hold")
		}
		if restored, e := vault.EnsureEnrollment(ctx, recovered.Token); e != nil || !reflect.DeepEqual(restored, retained) {
			t.Fatal("held gym recovery changed retained root", e)
		}
		deniedPath := filepath.Join(root, fixture+"-held-denied.json")
		if e = vault.ExportRecoveryFile(ctx, recovered.Token, deniedPath); !errors.Is(e, gymaccount.ErrHeld) {
			t.Fatal("held export not refused", e)
		}
		if _, _, e = vault.Rewrap(ctx, recovered.Token, "fixture-recovery-v2", "fixture-gym-v2"); !errors.Is(e, gymaccount.ErrHeld) {
			t.Fatal("held recovery-authority change not refused", e)
		}
		if _, e = os.Stat(deniedPath); !os.IsNotExist(e) {
			t.Fatal("held export created a file")
		}
		c = providerIntegrationPilot(t, config, store, recovered, vault, fixture, true, &signed)
		assertAccountIdentity(t, c.Snapshot(), expected)
		if e = c.Start(); e != nil {
			t.Fatal(e)
		}
		waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool {
			return s.Height > before.Height && s.ProspectCount >= before.ProspectCount+3
		})
		c.Stop()
		after := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
		unchanged, e := c.Receipt(first.Sequence)
		if e != nil || !reflect.DeepEqual(unchanged, first) {
			t.Fatal("provider recovery changed original fighter birth", e)
		}
		recoveryAfter, gymAfter, _ := wrapper.counts()
		if recoveryAfter != recoveryBefore || gymAfter <= gymBefore || signed.Load() < after.ProspectCount {
			t.Fatal("held mining did not use only vault gym signatures")
		}
		if e = errors.Join(coordinator.Close(), vault.Close(), c.Close()); e != nil {
			t.Fatal(e)
		}
		reopen()
		final, _ := provider.login(ctx, coordinator, fixture)
		status, e := vault.Status(ctx, final.Token)
		if e != nil || status.Receipt == nil || !reflect.DeepEqual(*status.Receipt, retained) || final.Account.RecoveryRequired || final.Account.HoldUntilMS != completed.Barrier.HoldUntilMS {
			t.Fatal("completed recovery changed after restart", e)
		}
		exact, e := provider.complete(ctx, coordinator, op.ID)
		if e != nil || !reflect.DeepEqual(exact, completed) || provider.data.Operations[op.ID].Commits != 1 {
			t.Fatal("completion replay changed the hold or credential epoch", e)
		}
		results = append(results, map[string]any{"fixture": fixture, "same_root_and_gym": true, "actual_export_fresh_restore": true, "abandoned_begin_resumed": true, "provider_credential_mutations": provider.data.Operations[op.ID].Commits, "historical_begin_cannot_commit": true, "stale_original_auth_rejected": true, "other_account_isolated": true, "held_recovery_unwraps": recoveryAfter - recoveryBefore, "held_gym_unwraps": gymAfter - gymBefore, "historical_birth_preserved": true, "height": after.Height, "receipts": after.ProspectCount})
	}
	if err = errors.Join(coordinator.Close(), vault.Close(), store.Close()); err != nil {
		t.Fatal(err)
	}
	checkAccountListenersClosed(t, config.DataDir)
	raw, _ := json.MarshalIndent(map[string]any{"status": "passed", "fixture_only": true, "real_provider_claim": false, "accounts": results}, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "result.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
