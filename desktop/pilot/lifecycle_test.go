//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slugwallet/recovery"
	walletcore "github.com/taytr-llc/slugwallet/wallet"
)

var structuralWalletFixture struct {
	sync.Once
	data []byte
	err  error
}

func installStructuralWallet(t *testing.T, dir string) {
	t.Helper()
	// A closed copy isolates every test while avoiding repeated expensive
	// password KDF creation unrelated to the filesystem-loss assertions.
	// Adapter/integration tests still exercise actual wallet creation/restore.
	structuralWalletFixture.Do(func() {
		template := filepath.Join(t.TempDir(), "template-wallet")
		wallet, err := openFixtureWallet(template, true)
		if err != nil {
			structuralWalletFixture.err = err
			return
		}
		if err := wallet.Close(); err != nil {
			structuralWalletFixture.err = err
			return
		}
		structuralWalletFixture.data, structuralWalletFixture.err = os.ReadFile(filepath.Join(template, walletcore.WalletDBName))
	})
	if structuralWalletFixture.err != nil {
		t.Fatal(structuralWalletFixture.err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, walletcore.WalletDBName), structuralWalletFixture.data, 0600); err != nil {
		t.Fatal(err)
	}
}

// This helper makes only a structural ready-state fixture. Its placeholder
// chain files are never claimed valid blocks or passed to a real node. Actual
// initialization/quit/reopen and payout registration are covered by the real
// normal-node integration gate.
func testReadyController(t *testing.T) (*Controller, Config) {
	t.Helper()
	c, config := testController(t)
	installStructuralWallet(t, filepath.Join(config.DataDir, "wallet"))
	c.manifest.Identity = publishedWalletIdentity
	c.manifest.Phase = "ready"
	// Independent zero256 oracle, recovery/testdata/recovery_v1.json.
	c.manifest.Payout = walletPayout{Address: "aiEhKC3LRX8PPG5haqRFUcFvj3pXpBts2P", Purpose: 44}
	for _, name := range []string{"primary-chain", "peer-chain"} {
		base := filepath.Join(config.DataDir, name, "mainnet", "blocks_ffldb")
		if err := os.MkdirAll(filepath.Join(base, "metadata"), 0700); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{filepath.Join(base, "metadata", "CURRENT"), filepath.Join(base, "000000000.fdb")} {
			if err := os.WriteFile(path, []byte("structural-fixture-only"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := c.openStores(true); err != nil {
		t.Fatal(err)
	}
	if err := c.closeStores(); err != nil {
		t.Fatal(err)
	}
	if err := markInitializedChains(config.DataDir, c.manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SLUG-PILOT.json", "SLUG-PILOT-READY.json"} {
		if err := atomicJSON(filepath.Join(config.DataDir, name), c.manifest); err != nil {
			t.Fatal(err)
		}
	}
	return c, config
}

func TestLegacyIdentityRejectedBeforeLockOrDirectoryMutation(t *testing.T) {
	config := testConfig(t)
	if err := os.Mkdir(config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := []byte(`{"version":1,"mode":"local-pilot","authority_port":12345}`)
	if err := os.WriteFile(filepath.Join(config.DataDir, "SLUG-PILOT.json"), marker, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.DataDir, "pre25-height62-evidence"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(config.DataDir)
	if c, err := New(config); err == nil {
		c.Close()
		t.Fatal("legacy identity adopted")
	}
	after, _ := os.ReadDir(config.DataDir)
	if len(before) != len(after) {
		t.Fatal("legacy rejection created a lock or other file")
	}
	raw, _ := os.ReadFile(filepath.Join(config.DataDir, "SLUG-PILOT.json"))
	if string(raw) != string(marker) {
		t.Fatal("legacy manifest changed")
	}
	if _, err := os.Stat(filepath.Join(config.DataDir, "instance-lock")); !os.IsNotExist(err) {
		t.Fatal("legacy instance lock opened")
	}
}

func TestIncompleteAndPreexistingChainCannotBeAdopted(t *testing.T) {
	for _, mode := range []string{"initializing", "existing-chain", "existing-wallet"} {
		t.Run(mode, func(t *testing.T) {
			c, config := testController(t)
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "initializing":
				m := c.manifest
				m.Phase = "initializing"
				if err := atomicJSON(filepath.Join(config.DataDir, "SLUG-PILOT.json"), m); err != nil {
					t.Fatal(err)
				}
			case "existing-chain":
				if err := os.WriteFile(filepath.Join(config.DataDir, "primary-chain", "old-block-data"), []byte("pre25"), 0600); err != nil {
					t.Fatal(err)
				}
			case "existing-wallet":
				if err := os.Mkdir(filepath.Join(config.DataDir, "wallet"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if opened, err := New(config); err == nil {
				opened.Close()
				t.Fatal("partial or old data adopted")
			}
		})
	}
}

func TestRecoveryExportUsesRetainedReopenedWalletAndProtectedPaths(t *testing.T) {
	c, config := testReadyController(t)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	path := filepath.Join(t.TempDir(), "public-fixture-backup.json")
	if err := reopened.ExportRecovery(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.ParseExport(raw)
	if err != nil || bundle.Manifest.GymID != publishedWalletIdentity.GymID {
		t.Fatal("retained export invalid", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("export mode is not private")
	}
	for _, protected := range []string{filepath.Join(config.DataDir, "SLUG-PILOT.json"), filepath.Join(config.DataDir, "wallet", walletcore.WalletDBName), filepath.Join(config.DataDir, "new-file.json")} {
		if err := reopened.ExportRecovery(protected); err == nil {
			t.Fatal("managed path allowed", protected)
		}
	}
	link := filepath.Join(t.TempDir(), "wallet-link")
	if err := os.Symlink(filepath.Join(config.DataDir, "wallet"), link); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ExportRecovery(filepath.Join(link, "replacement.json")); err == nil {
		t.Fatal("managed symlink parent escaped export guard")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ExportRecovery(path); err == nil {
		t.Fatal("closed controller exported")
	}
}

func TestBackupInvalidFileFailsAsynchronouslyWithoutStartingNodes(t *testing.T) {
	c, config := testReadyController(t)
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"mnemonic":"not a phrase"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyBackup(path); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("invalid backup did not terminate")
	}
	s := c.Snapshot()
	if s.State != "stopped" || s.BackupState != "failed" || s.BackupError != "backup_invalid" {
		t.Fatalf("invalid backup result %+v", s)
	}
	if _, err := os.Stat(filepath.Join(config.DataDir, "primary.conf")); !os.IsNotExist(err) {
		t.Fatal("invalid backup started a node")
	}
}

func TestBackupOperationExcludesMiningAndCloseJoins(t *testing.T) {
	c, _ := testReadyController(t)
	ctx, done, err := c.beginOperation("backup", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err == nil {
		t.Fatal("mining started during verification")
	}
	finished := make(chan struct{})
	go func() { <-ctx.Done(); close(finished); c.finishOperation(done) }()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("Close returned before verification joined")
	}
	if _, _, err := c.beginOperation("backup", time.Minute); err == nil {
		t.Fatal("closed controller started verification")
	}
}

func TestBackupStrictBoundsAndForeignFixture(t *testing.T) {
	c, config := testReadyController(t)
	other, err := recovery.NewExport([32]byte{1}, recovery.DefaultAccounts())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "foreign.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := c.verifyBackup(context.Background(), path); got != "backup_mismatch" {
		t.Fatal("foreign fixture not rejected", got)
	}
	before, _ := os.ReadDir(config.DataDir)
	if err := os.WriteFile(path, make([]byte, recovery.MaxExportSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyBackup(path); err == nil {
		t.Fatal("oversized backup accepted")
	}
	after, _ := os.ReadDir(config.DataDir)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("bad backup changed managed files")
	}
}

func TestCancelledBackupCleanupFailureIsReportedThroughClose(t *testing.T) {
	c, _ := testController(t)
	scratch := t.TempDir()
	retained := filepath.Join(scratch, "retained-fixture")
	if err := os.WriteFile(retained, []byte("preserve on cleanup failure"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := c.cleanupBackupScratch(scratch, func(path string) error {
		called = true
		if path != scratch {
			t.Fatal("cleanup targeted another directory")
		}
		return os.ErrPermission
	})
	if err == nil || !called {
		t.Fatal("injected cleanup failure was ignored")
	}
	c.backupResult(ctx, "backup_storage_failed")
	status := c.Snapshot()
	if status.BackupState != "failed" || status.BackupError != "backup_storage_failed" {
		t.Fatalf("cancellation hid cleanup failure: %+v", status)
	}
	if err := c.Close(); err == nil {
		t.Fatal("Close reported clean shutdown after cleanup failed")
	}
	if _, err := os.Stat(retained); err != nil {
		t.Fatal("failed cleanup erased retained data", err)
	}
}

func TestCustomPilotBackupProtectsEntireDefaultApplicationRoot(t *testing.T) {
	configRoot := t.TempDir()
	defaultPilot := filepath.Join(configRoot, "SLUG", "LocalWalletPilot")
	if err := os.MkdirAll(defaultPilot, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(defaultPilot, "SLUG-PILOT.json")
	original := []byte("another retained pilot")
	if err := os.WriteFile(manifest, original, 0600); err != nil {
		t.Fatal(err)
	}
	c := &Controller{config: Config{DataDir: filepath.Join(t.TempDir(), "custom-pilot"), NodeBinary: filepath.Join(t.TempDir(), "slugd")}}
	for _, existing := range []bool{false, true} {
		if _, err := c.backupPathUnderConfig(manifest, existing, configRoot); err == nil {
			t.Fatal("custom pilot could overwrite/read another default pilot")
		}
	}
	alias := filepath.Join(t.TempDir(), "managed-alias")
	if err := os.Symlink(defaultPilot, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := c.backupPathUnderConfig(filepath.Join(alias, "new.json"), false, configRoot); err == nil {
		t.Fatal("default application protection escaped via parent symlink")
	}
	external := filepath.Join(t.TempDir(), "fixture-backup.json")
	if _, err := c.backupPathUnderConfig(external, false, configRoot); err != nil {
		t.Fatal("unmanaged native export rejected", err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(original) {
		t.Fatal("default pilot evidence changed", err)
	}
}
