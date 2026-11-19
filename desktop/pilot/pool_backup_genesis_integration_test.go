//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
)

// A restored wallet must finish synchronizing even at an unmoving genesis.
// Only published fixture wallets and newly owned loopback nodes are used.
func TestPoolBackupActualGenesisSynchronization(t *testing.T) {
	binary, artifacts := os.Getenv("SLUG_POOL_NODE"), os.Getenv("SLUG_POOL_DESKTOP_ARTIFACTS")
	if binary == "" || artifacts == "" {
		t.Skip("explicit normal node and owned fixture artifact directory required")
	}
	root, err := os.MkdirTemp(artifacts, "genesis-observation-")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	accounts, err := gymaccount.Create(filepath.Join(root, "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 128, MaxChallenges: 64, MaxEvents: 256})
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	runtime, err := NewPoolRuntime(ctx, Config{DataDir: filepath.Join(root, "shared"), NodeBinary: binary}, accounts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	}()
	c := &PoolChild{resources: poolChildResources{Node: runtime.node, Source: runtime.source}}
	original, err := openAccountFixtureWallet(filepath.Join(root, "original"), "zero256", true)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	rpc := runtime.nodes.rpcConfig()
	if err := original.Attach(ctx, rpc); err != nil {
		t.Fatal(err)
	}
	recipient, err := original.EnsurePoolRecipient(ctx, nil)
	if err != nil || waitPoolChildWallet(ctx, original) != nil {
		t.Fatal("original wallet setup", err)
	}
	bundle, err := original.ExportRecovery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { bundle.Mnemonic = "" }()
	type observation struct {
		Expected, Early, Resampled, Ready                        walletView
		AccountsEqual, UTXOsEqual, HistoryEqual, SigningVerified bool
		EarlyCode, ReadyCode                                     string
	}
	results := []observation{}
	for i := 0; i < 3; i++ {
		expected, err := original.VerificationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tip, err := c.poolBackupTip(ctx)
		if err != nil || tip.Height != 0 {
			t.Fatal("genesis observation must not mine", err)
		}
		restored, err := restoreAccountFixtureWallet(filepath.Join(root, "restored-"+strconv.Itoa(i)), "zero256", bundle)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer restored.Close()
			if restored.Attach(ctx, rpc) != nil {
				t.Fatal("restored attachment")
			}
			if _, err := restored.EnsurePoolRecipient(ctx, &recipient); err != nil || c.waitPoolBackupWallet(ctx, restored) != nil {
				t.Fatal("restored initial readiness", err)
			}
			early, err := restored.VerificationState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !early.View.Synced || early.View.Height != tip.Height {
				t.Fatal("backup readiness returned before the restored genesis wallet synchronized")
			}
			initial, proof, err := backupSigningProof(tip, original.Identity())
			if err != nil {
				t.Fatal(err)
			}
			signer, err := restored.Signer(ctx, runtime.source.Policy)
			if err != nil {
				t.Fatal(err)
			}
			auth, err := signer(ctx, initial, proof)
			verified := err == nil && gymauth.VerifyAuthorization(initial, proof, auth) == nil
			resampled, err := original.VerificationState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			after, err := c.poolBackupTip(ctx)
			if err != nil || after != tip {
				t.Fatal("diagnostic chain changed", err)
			}
			if err := waitPoolChildWallet(ctx, restored); err != nil {
				t.Fatal("restored full synchronization", err)
			}
			ready, err := restored.VerificationState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			value := observation{Expected: expected.View, Early: early.View, Resampled: resampled.View, Ready: ready.View, AccountsEqual: reflect.DeepEqual(expected.Accounts, early.Accounts), UTXOsEqual: reflect.DeepEqual(expected.UTXOs, early.UTXOs), HistoryEqual: reflect.DeepEqual(expected.Transactions, early.Transactions), SigningVerified: verified, EarlyCode: comparePoolBackupState(expected, early, resampled, tip, after), ReadyCode: comparePoolBackupState(expected, ready, resampled, tip, after)}
			results = append(results, value)
			t.Logf("public genesis observation: %+v", value)
			if !verified || value.EarlyCode != "" || value.ReadyCode != "" {
				t.Fatal("fully synchronized genesis comparison or signing failed")
			}
		}()
		if results[len(results)-1].EarlyCode != "" {
			break
		}
	}
	raw, err := json.MarshalIndent(results, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(root, "public-observations.json"), raw, 0600) != nil {
		t.Fatal("write public diagnostic")
	}
}
