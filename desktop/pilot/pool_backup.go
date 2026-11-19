// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recovery"
)

type poolBackupOperation struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	verify bool
}

// This fence is the child's immutable original login/generation, never a
// freshly minted mining token. Its callback contains only bounded local work.
func (c *PoolChild) poolBackupFence(ctx context.Context, capability gymaccount.Capability, op *poolBackupOperation) recoveryfile.Fence {
	return func(fn func(func() error) error) error {
		called := false
		var operationErr error
		err := c.account.Lease(ctx, capability, func(account gymaccount.Account, recheck func() error) (result error) {
			defer func() { operationErr = result }()
			if called || recheck == nil || account.ID != c.AccountID() || account.Generation != c.account.Manifest.Generation {
				return gymaccount.ErrUnauthorized
			}
			called = true
			check := func() error {
				if ctx.Err() != nil || c.ctx.Err() != nil {
					return gymaccount.ErrUnauthorized
				}
				if err := recheck(); err != nil {
					return err
				}
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.closed || !c.prepared || op != nil && c.backup != op {
					return gymaccount.ErrUnauthorized
				}
				return nil
			}
			if err := check(); err != nil {
				return err
			}
			if err := fn(check); err != nil {
				return err
			}
			return check()
		})
		if !called {
			return errors.Join(gymaccount.ErrUnauthorized, err)
		}
		return errors.Join(err, operationErr)
	}
}

func (c *PoolChild) beginPoolBackup(parent context.Context, verify bool) (*poolBackupOperation, error) {
	if parent == nil || parent.Err() != nil {
		return nil, gymstable.ErrInvalid
	}
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	duration := 30 * time.Second
	capability := gymaccount.RecoveryExport
	if verify {
		duration, capability = 10*time.Minute, gymaccount.View
	}
	ctx, cancel := context.WithTimeout(parent, duration)
	stop := context.AfterFunc(c.ctx, cancel)
	op := &poolBackupOperation{ctx: ctx, cancel: func() { stop(); cancel() }, done: make(chan struct{}), verify: verify}
	err := c.poolBackupFence(ctx, capability, nil)(func(check func() error) error {
		if err := check(); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.backup != nil || c.run != nil || c.snapshot.State != "stopped" && c.snapshot.State != "failed" || c.backupCloseErr != nil {
			return gymstable.ErrBusy
		}
		c.backup = op
		c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "running", "opening", ""
		return nil
	})
	if err != nil {
		op.cancel()
		c.mu.Lock()
		if c.backup == op {
			c.backup = nil
			c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "idle", "", ""
		}
		c.mu.Unlock()
		return nil, err
	}
	// Start cannot run while stopMu is held, and the active operation prevents
	// later refreshes. Drain the prior refresh only after releasing authority.
	c.refreshMu.Lock()
	c.refreshMu.Unlock()
	// Work dispatch shares stopMu, so no new roster/entry/event task can race
	// this pause. Join the old serial lane outside every account lease.
	if s := c.stable; s != nil {
		s.stableMu.Lock()
		s.paused, s.needSync = true, false
		s.cancelTasksLocked()
		s.stableMu.Unlock()
		s.stopStoppedLoop()
		s.laneGate <- struct{}{}
		<-s.laneGate
	}
	return op, nil
}

func (c *PoolChild) poolBackupPhase(op *poolBackupOperation, phase string) error {
	return c.publish(op.ctx, func() error {
		if c.backup != op {
			return gymaccount.ErrUnauthorized
		}
		c.snapshot.BackupPhase = phase
		return nil
	})
}

func (c *PoolChild) finishPoolBackup(op *poolBackupOperation, code string) {
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()
	if code == "backup_storage_failed" {
		c.mu.Lock()
		c.backupCloseErr = errors.Join(c.backupCloseErr, errors.New("pool backup cleanup failed"))
		c.mu.Unlock()
	}
	_ = c.publish(ctx, func() error {
		if c.backup != op {
			return gymaccount.ErrUnauthorized
		}
		switch {
		case code == "backup_storage_failed" || code == "backup_export_uncertain":
			c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
		case errors.Is(op.ctx.Err(), context.Canceled):
			c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "cancelled", "", ""
		case code != "":
			c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
		case op.verify:
			c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "succeeded", "complete", ""
		default:
			c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "idle", "", ""
		}
		return nil
	})
	op.cancel()
	c.mu.Lock()
	if c.backup == op {
		c.backup = nil
	}
	close(op.done)
	c.mu.Unlock()
}

func (c *PoolChild) cancelPoolBackup() {
	c.mu.Lock()
	op := c.backup
	if op != nil {
		op.cancel()
	}
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
}

// CancelPoolBackup joins cleanup; its completion never restarts a worker.
func (c *PoolChild) CancelPoolBackup() {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	c.cancelPoolBackup()
}

func (c *PoolChild) poolBackupProtected() ([]string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, errors.New("managed backup roots unavailable")
	}
	root := filepath.Dir(filepath.Dir(c.resources.Config.DataDir))
	protected := []string{c.resources.Config.DataDir, root, filepath.Join(config, "SLUG"), filepath.Dir(c.resources.Config.NodeBinary)}
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("application backup roots unavailable")
	}
	for _, file := range []string{executable, c.resources.Config.NodeBinary} {
		for parent := filepath.Dir(file); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if strings.HasSuffix(strings.ToLower(parent), ".app") {
				protected = append(protected, parent)
				break
			}
		}
	}
	return protected, nil
}

func poolBackupBundleMatches(bundle recovery.Export, identity walletIdentity) bool {
	if recovery.ValidateExport(bundle) != nil || bundle.Version != recovery.Version1 || bundle.Manifest.Profile != recovery.ProfileID ||
		bundle.Manifest.RootPublicKey != identity.RootPublicKey || bundle.Manifest.GymPublicKey != identity.GymPublicKey || bundle.Manifest.GymID != identity.GymID {
		return false
	}
	for _, account := range bundle.Manifest.Accounts {
		if account.Purpose == 84 && account.Coin == 0 && account.Account == 0 {
			return account.NextExternal > 0
		}
	}
	return false
}

// ExportPoolRecovery borrows the already-open wallet. Only the exact original
// RecoveryExport fence may publish its bytes to the native-selected file.
func (c *PoolChild) ExportPoolRecovery(parent context.Context, pin recoveryfile.Pin) (resultErr error) {
	if pin.Mode != recoveryfile.CreateMode {
		return recoveryfile.ErrInvalid
	}
	op, err := c.beginPoolBackup(parent, false)
	if err != nil {
		return err
	}
	code := "backup_unavailable"
	defer func() { c.finishPoolBackup(op, code) }()
	protected, err := c.poolBackupProtected()
	if err != nil {
		return err
	}
	file, err := recoveryfile.Open(pin, protected)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			if code != "backup_export_uncertain" {
				code = "backup_storage_failed"
			}
			c.mu.Lock()
			c.backupCloseErr = errors.Join(c.backupCloseErr, errors.New("pool backup file cleanup failed"))
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, err)
		}
	}()
	export := c.backupExport
	if export == nil {
		export = c.wallet.ExportRecovery
	}
	bundle, err := export(op.ctx)
	if err != nil {
		return errors.New("pool recovery export unavailable")
	}
	defer func() { bundle.Mnemonic = "" }()
	if !poolBackupBundleMatches(bundle, c.wallet.Identity()) {
		code = "backup_mismatch"
		return errors.New(code)
	}
	raw, err := recovery.MarshalExport(bundle)
	bundle.Mnemonic = ""
	defer clear(raw)
	if err != nil {
		return errors.New("pool recovery export unavailable")
	}
	err = file.Publish(op.ctx, raw, c.poolBackupFence(op.ctx, gymaccount.RecoveryExport, op))
	if errors.Is(err, recoveryfile.ErrCleanup) {
		c.mu.Lock()
		c.backupCloseErr = errors.Join(c.backupCloseErr, errors.New("pool backup file cleanup failed"))
		c.mu.Unlock()
	}
	switch {
	case errors.Is(err, recoveryfile.ErrOutcomeUnknown):
		code = "backup_export_uncertain"
	case errors.Is(err, recoveryfile.ErrCleanup):
		code = "backup_storage_failed"
	case err == nil:
		code = ""
	}
	return err
}

// VerifyPoolBackup dispatches a fresh file-only restoration. No wallet or node
// is reopened over the original database, and this operation never spends.
func (c *PoolChild) VerifyPoolBackup(parent context.Context, pin recoveryfile.Pin) error {
	if pin.Mode != recoveryfile.ReadMode {
		return recoveryfile.ErrInvalid
	}
	op, err := c.beginPoolBackup(parent, true)
	if err != nil {
		return err
	}
	protected, err := c.poolBackupProtected()
	if err != nil {
		c.finishPoolBackup(op, "backup_unavailable")
		return err
	}
	// Native retains its descriptors until this dispatch returns. Acquire our
	// own exact pinned descriptors before that handoff, not in the goroutine.
	file, err := recoveryfile.Open(pin, protected)
	if err != nil {
		c.finishPoolBackup(op, "backup_invalid")
		return err
	}
	go func() { c.finishPoolBackup(op, c.verifyPoolBackup(op, file)) }()
	return nil
}

func (c *PoolChild) poolBackupTip(ctx context.Context) (localTip, error) {
	observation, err := c.resources.Node.Resolve(ctx, c.resources.Source.Policy)
	if err != nil || observation.ParentHeight > math.MaxInt32 {
		return localTip{}, errors.New("shared backup tip unavailable")
	}
	return localTip{Hash: observation.ParentHash, Height: int32(observation.ParentHeight)}, nil
}

func (c *PoolChild) waitPoolBackupWallet(ctx context.Context, wallet *walletRuntime) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, err := wallet.Snapshot(ctx)
		tip, tipErr := c.poolBackupTip(ctx)
		// Height zero also needs the asynchronous rescan-finished notification.
		// A fresh wallet can already report genesis while its restore is pending.
		if err == nil && tipErr == nil && view.Height == tip.Height && view.Synced {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func comparePoolBackupState(expected, restored, resampled walletVerificationState, before, after localTip) string {
	// A fresh chain rescan does not replay every transaction already in the
	// mempool. Keep the full comparison, but defer while any observation has
	// unconfirmed history instead of calling a valid file mismatched.
	for _, state := range []walletVerificationState{expected, restored, resampled} {
		if state.View.Unconfirmed != 0 {
			return "backup_unavailable"
		}
		for _, output := range state.UTXOs {
			if output.Confirmations <= 0 {
				return "backup_unavailable"
			}
		}
		for _, transaction := range state.Transactions {
			if transaction.Confirmations <= 0 {
				return "backup_unavailable"
			}
		}
	}
	if before != after || !reflect.DeepEqual(expected, resampled) {
		return "backup_unavailable"
	}
	if !reflect.DeepEqual(expected, restored) {
		return "backup_mismatch"
	}
	return ""
}

// A failed close retains the scratch directory. Callers surface either failure
// even if cancellation happened concurrently; deletion never races a live DB.
func cleanupPoolBackupScratch(path string, closeWallet func() error, remove func(string) error) error {
	if closeWallet != nil {
		if err := closeWallet(); err != nil {
			return err
		}
	}
	return remove(path)
}

func (c *PoolChild) verifyPoolBackup(op *poolBackupOperation, file *recoveryfile.Handle) (code string) {
	ctx := op.ctx
	raw, readErr := file.Read(ctx, c.poolBackupFence(ctx, gymaccount.View, op))
	closeErr := file.Close()
	defer clear(raw)
	if closeErr != nil {
		return "backup_storage_failed"
	}
	if readErr != nil {
		return "backup_invalid"
	}
	bundle, err := recovery.ParseExport(raw)
	clear(raw)
	if err != nil {
		return "backup_invalid"
	}
	defer func() { bundle.Mnemonic = "" }()
	identity := c.wallet.Identity()
	if !poolBackupBundleMatches(bundle, identity) {
		return "backup_mismatch"
	}
	_, recipient, err := poolRecipientExpected(c.resources.Recipient.Config)
	if err != nil || c.resources.Recipient.Recipient != poolPublicRecipient(recipient) || c.poolBackupPhase(op, "checking") != nil {
		return "backup_unavailable"
	}
	if _, err = c.wallet.EnsurePoolRecipient(ctx, &recipient); err != nil || c.waitPoolBackupWallet(ctx, c.wallet) != nil {
		return "backup_unavailable"
	}
	tip, err := c.poolBackupTip(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	expected, err := c.wallet.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	if current, err := c.poolBackupTip(ctx); err != nil || current != tip {
		return "backup_unavailable"
	}
	if c.poolBackupPhase(op, "restoring") != nil {
		return "backup_unavailable"
	}
	scratch, err := os.MkdirTemp(c.resources.Config.DataDir, "pool-backup-verify-")
	if err != nil {
		return "backup_storage_failed"
	}
	var restored *walletRuntime
	defer func() {
		var closeWallet func() error
		if restored != nil {
			closeWallet = restored.Close
		}
		if cleanupPoolBackupScratch(scratch, closeWallet, os.RemoveAll) != nil {
			code = "backup_storage_failed"
		}
	}()
	restored, err = restoreAccountFixtureWallet(filepath.Join(scratch, "wallet"), c.account.Manifest.Account.Fixture, bundle)
	bundle.Mnemonic = ""
	if err != nil {
		return "backup_invalid"
	}
	if restored.Identity() != identity || restored.Attach(ctx, c.resources.RPC) != nil {
		return "backup_mismatch"
	}
	if _, err = restored.EnsurePoolRecipient(ctx, &recipient); err != nil {
		return "backup_mismatch"
	}
	if c.poolBackupPhase(op, "syncing") != nil || c.waitPoolBackupWallet(ctx, restored) != nil {
		return "backup_unavailable"
	}
	actual, err := restored.VerificationState(ctx)
	if err != nil || c.poolBackupPhase(op, "verifying") != nil {
		return "backup_unavailable"
	}
	initial, proof, err := backupSigningProof(tip, identity)
	if err != nil {
		return "backup_unavailable"
	}
	signer, err := restored.Signer(ctx, c.resources.Source.Policy)
	if err != nil {
		return "backup_unavailable"
	}
	authorization, err := signer(ctx, initial, proof)
	if err != nil || gymauth.VerifyAuthorization(initial, proof, authorization) != nil {
		return "backup_mismatch"
	}
	resampled, err := c.wallet.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	after, err := c.poolBackupTip(ctx)
	if err != nil || ctx.Err() != nil {
		return "backup_unavailable"
	}
	if code := comparePoolBackupState(expected, actual, resampled, tip, after); code != "" {
		return code
	}
	if c.publish(ctx, func() error {
		if c.backup != op {
			return gymaccount.ErrUnauthorized
		}
		c.snapshot.Height, c.snapshot.WalletHeight = uint64(actual.View.Height), uint64(actual.View.Height)
		c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = actual.View.Spendable, actual.View.Immature
		return nil
	}) != nil {
		return "backup_unavailable"
	}
	return ""
}
