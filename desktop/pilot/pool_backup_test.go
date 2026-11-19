// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recovery"
)

func poolBackupWait(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("backup fixture wait expired")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPoolBackupAdmissionDrainsRefreshWithoutLeaseAndRefusesStart(t *testing.T) {
	c, _, h, _ := poolChildLifecycleFixture(t)
	c.refreshMu.Lock()
	result := make(chan *poolBackupOperation, 1)
	failure := make(chan error, 1)
	go func() {
		op, err := c.beginPoolBackup(t.Context(), true)
		failure <- err
		result <- op
	}()
	poolBackupWait(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.backup != nil })
	if h.sessions.active.Load() {
		t.Fatal("backup retained account lease while refresh drained")
	}
	if err := h.account.ViewLease(t.Context(), func(_ gymaccount.Account, check func() error) error { return check() }); err != nil {
		t.Fatal("other account work blocked by refresh drain", err)
	}
	c.refreshMu.Unlock()
	if err := <-failure; err != nil {
		t.Fatal(err)
	}
	op := <-result
	if err := c.Start(); !errors.Is(err, gymstable.ErrBusy) {
		t.Fatal("Start admitted during exclusive backup", err)
	}
	if err := c.RefreshPoolBalance(t.Context()); !errors.Is(err, gymstable.ErrBusy) {
		t.Fatal("refresh admitted during backup", err)
	}
	c.finishPoolBackup(op, "")
	if got := c.Snapshot(); got.State != "stopped" || got.Attempts != 0 || got.BackupState != "succeeded" || got.BackupPhase != "complete" {
		t.Fatal("backup completion changed mining or verification state", got.State, got.BackupState)
	}
}

func TestPoolBackupCancelAndCloseJoinCleanup(t *testing.T) {
	for _, closeChild := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "close"}[closeChild], func(t *testing.T) {
			c, issuer, _, released := poolChildLifecycleFixture(t)
			op, err := c.beginPoolBackup(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			cleanup := make(chan struct{})
			go func() { <-op.ctx.Done(); <-cleanup; c.finishPoolBackup(op, "backup_unavailable") }()
			done := make(chan error, 1)
			go func() {
				if closeChild {
					done <- c.Close()
				} else {
					c.CancelPoolBackup()
					done <- nil
				}
			}()
			<-op.ctx.Done()
			select {
			case <-done:
				t.Fatal("operation returned before cleanup joined")
			case <-time.After(20 * time.Millisecond):
			}
			if released.Load() != 0 {
				t.Fatal("borrow released before backup cleanup")
			}
			close(cleanup)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if _, err := issuer.Info(); err != nil {
				t.Fatal("backup lifecycle closed borrowed parent issuer", err)
			}
			if !closeChild && c.Snapshot().BackupState != "cancelled" {
				t.Fatal("cancel result not retained")
			}
		})
	}
}

func poolBackupWalletFixture(t *testing.T) (*PoolChild, *poolDesktopHarness, string) {
	t.Helper()
	c, _, h, _ := poolChildLifecycleFixture(t)
	root := t.TempDir()
	c.resources.Config = Config{DataDir: filepath.Join(root, "owned", "gyms", c.AccountID()), NodeBinary: filepath.Join(root, "bin", "slugd")}
	if err := os.MkdirAll(c.resources.Config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(c.resources.Config.NodeBinary), 0700); err != nil {
		t.Fatal(err)
	}
	wallet, err := openAccountFixtureWallet(filepath.Join(c.resources.Config.DataDir, "wallet"), "zero256", true)
	if err != nil {
		t.Fatal(err)
	}
	c.wallet = wallet
	attachPoolRecipientRPC(t, wallet)
	if _, err := wallet.EnsurePoolRecipient(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	c.snapshot.BackupState = "idle"
	return c, h, filepath.Join(root, "recovery.json")
}

func TestPoolBackupExportUsesRetainedWalletAndOriginalCapability(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "reset_during_wallet_io"}[revoke], func(t *testing.T) {
			c, h, path := poolBackupWalletFixture(t)
			selection, err := recoveryfile.Capture(path, nil, recoveryfile.CreateMode)
			if err != nil {
				t.Fatal(err)
			}
			defer selection.Close()
			entered, release := make(chan struct{}), make(chan struct{})
			c.backupExport = func(ctx context.Context) (recovery.Export, error) {
				if h.sessions.active.Load() {
					t.Error("wallet export retained account lease")
				}
				close(entered)
				<-release
				return c.wallet.ExportRecovery(ctx)
			}
			done := make(chan error, 1)
			go func() { done <- c.ExportPoolRecovery(t.Context(), selection.Pin()) }()
			select {
			case <-entered:
			case err := <-done:
				t.Fatal("export failed before wallet work", err)
			case <-time.After(3 * time.Second):
				t.Fatal("export did not reach wallet work")
			}
			if revoke {
				h.mu.Lock()
				h.view = false
				h.mu.Unlock()
			}
			close(release)
			err = <-done
			if revoke {
				if err == nil {
					t.Fatal("revoked original view published export")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("revoked export created a destination")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := readBackup(path)
			defer func() { bundle.Mnemonic = "" }()
			if err != nil || !poolBackupBundleMatches(bundle, c.wallet.Identity()) {
				t.Fatal("published export lost native84 wallet metadata")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("recovery file permissions")
			}
			if c.wallet.closed || c.Snapshot().State != "stopped" || c.Snapshot().BackupState != "idle" {
				t.Fatal("export closed wallet, mined, or claimed restored verification")
			}
		})
	}
}

func TestPoolBackupHoldDeniesExportButAllowsVerification(t *testing.T) {
	c, _, _, _ := poolChildLifecycleFixture(t)
	old := c.account.Lease
	c.account.Lease = func(ctx context.Context, capability gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if capability == gymaccount.RecoveryExport {
			return gymaccount.ErrForbidden
		}
		return old(ctx, capability, fn)
	}
	if _, err := c.beginPoolBackup(t.Context(), false); !errors.Is(err, gymaccount.ErrForbidden) {
		t.Fatal("hold allowed export", err)
	}
	op, err := c.beginPoolBackup(t.Context(), true)
	if err != nil {
		t.Fatal("hold blocked existing View verification capability", err)
	}
	c.finishPoolBackup(op, "backup_invalid")
}

func TestPoolBackupComparisonDetectsSameTipChangesBeforeMismatch(t *testing.T) {
	before := walletVerificationState{Accounts: recovery.DefaultAccounts(), View: walletView{Synced: true, Height: 100, Spendable: 495000000}}
	tip := localTip{Hash: genesis, Height: 100}
	if code := comparePoolBackupState(before, before, before, tip, tip); code != "" {
		t.Fatal(code)
	}
	changed := before
	changed.View.Spendable = 100
	if code := comparePoolBackupState(before, changed, changed, tip, tip); code != "backup_unavailable" {
		t.Fatal("same-tip incoming payment mislabeled as invalid backup", code)
	}
	if code := comparePoolBackupState(before, changed, before, tip, tip); code != "backup_mismatch" {
		t.Fatal("stable original did not detect restored mismatch", code)
	}
	later := tip
	later.Height++
	if code := comparePoolBackupState(before, before, before, tip, later); code != "backup_unavailable" {
		t.Fatal("moving tip accepted", code)
	}
}

func TestPoolBackupComparisonDefersUnconfirmedState(t *testing.T) {
	confirmed := walletVerificationState{Accounts: recovery.DefaultAccounts(), View: walletView{Synced: true, Height: 100, Spendable: 495000000}}
	tip := localTip{Hash: genesis, Height: 100}
	for _, mode := range []string{"balance", "output", "history"} {
		t.Run(mode, func(t *testing.T) {
			pending := confirmed
			switch mode {
			case "balance":
				pending.View.Unconfirmed = 495000000
			case "output":
				pending.UTXOs = []btcjson.ListUnspentResult{{Confirmations: 0}}
			case "history":
				pending.Transactions = []btcjson.ListTransactionsResult{{Confirmations: 0}}
			}
			for _, input := range []struct{ expected, restored, resampled walletVerificationState }{
				{pending, confirmed, pending}, // Existing incoming mempool transaction is absent from fresh rescan.
				{confirmed, pending, confirmed},
				{confirmed, confirmed, pending},
				{pending, pending, pending},
			} {
				if code := comparePoolBackupState(input.expected, input.restored, input.resampled, tip, tip); code != "backup_unavailable" {
					t.Fatal("unconfirmed observation claimed success or mismatch", code)
				}
			}
		})
	}
	if code := comparePoolBackupState(confirmed, confirmed, confirmed, tip, tip); code != "" {
		t.Fatal("stable confirmed state no longer verifies", code)
	}
}

func TestPoolBackupBundleRequiresIssuedNative84Cursor(t *testing.T) {
	accounts := recovery.DefaultAccounts()
	for i := range accounts {
		if accounts[i].Purpose == 84 {
			accounts[i].NextExternal = 1
		}
	}
	bundle, err := recovery.NewExport([32]byte{}, accounts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { bundle.Mnemonic = "" }()
	if !poolBackupBundleMatches(bundle, publishedWalletIdentity) {
		t.Fatal("issued published native84 fixture refused")
	}
	for i := range bundle.Manifest.Accounts {
		if bundle.Manifest.Accounts[i].Purpose == 84 {
			bundle.Manifest.Accounts[i].NextExternal = 0
		}
	}
	if poolBackupBundleMatches(bundle, publishedWalletIdentity) {
		t.Fatal("missing native84 cursor permitted backup recovery")
	}
}

func TestPoolBackupCleanupFailureCannotHideBehindCancellation(t *testing.T) {
	var order []string
	if err := cleanupPoolBackupScratch("private-fixture", func() error { order = append(order, "close"); return errors.New("close failed") }, func(string) error { order = append(order, "remove"); return nil }); err == nil || !reflect.DeepEqual(order, []string{"close"}) {
		t.Fatal("removed a database after uncertain close")
	}
	order = nil
	if err := cleanupPoolBackupScratch("private-fixture", func() error { order = append(order, "close"); return nil }, func(string) error { order = append(order, "remove"); return errors.New("remove failed") }); err == nil || !reflect.DeepEqual(order, []string{"close", "remove"}) {
		t.Fatal("failed removal was hidden")
	}
	c, _, _, _ := poolChildLifecycleFixture(t)
	op, err := c.beginPoolBackup(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	op.cancel()
	c.finishPoolBackup(op, "backup_storage_failed")
	if got := c.Snapshot(); got.BackupState != "failed" || got.BackupError != "backup_storage_failed" {
		t.Fatal("cancellation hid cleanup failure", got.BackupState, got.BackupError)
	}
	if err := c.Close(); err == nil {
		t.Fatal("Close dropped prior cleanup failure")
	}
}
