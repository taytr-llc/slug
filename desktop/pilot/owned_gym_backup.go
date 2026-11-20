// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func (c *OwnedGymChild) backupFence(ctx context.Context, cap gymaccount.Capability, op *poolBackupOperation) recoveryfile.Fence {
	return func(fn func(func() error) error) error {
		return c.capabilityFence(ctx, cap, func(check func() error) error {
			current := func() error {
				if e := check(); e != nil {
					return e
				}
				if !c.mu.TryLock() {
					return gymstable.ErrBusy
				}
				defer c.mu.Unlock()
				if op != nil && c.backup != op {
					return gymaccount.ErrUnauthorized
				}
				return nil
			}
			if e := current(); e != nil {
				return e
			}
			if e := fn(current); e != nil {
				return e
			}
			return current()
		})
	}
}
func (c *OwnedGymChild) beginBackup(ctx context.Context, view string, verify bool) (*poolBackupOperation, error) {
	work, done, e := c.begin(ctx, view)
	if e != nil {
		return nil, e
	}
	defer done()
	if !c.wallet.opMu.TryLock() {
		return nil, gymstable.ErrBusy
	}
	duration, cap := 30*time.Second, gymaccount.RecoveryExport
	if verify {
		duration, cap = 10*time.Minute, gymaccount.View
	}
	opctx, cancel := context.WithTimeout(ctx, duration)
	stop := context.AfterFunc(c.life, cancel)
	op := &poolBackupOperation{ctx: opctx, cancel: func() { stop(); cancel() }, done: make(chan struct{}), verify: verify}
	e = c.backupFence(work, cap, nil)(func(check func() error) error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if c.run != nil || c.backup != nil || c.backupCloseErr != nil || !stableOne(c.snapshot.State, "stopped", "failed") {
			return gymstable.ErrBusy
		}
		c.backup = op
		c.snapshot.Backup = HostedBackupSnapshot{State: "running", Phase: "opening"}
		return nil
	})
	if e != nil {
		op.cancel()
		c.mu.Lock()
		if c.backup == op {
			c.backup = nil
			c.snapshot.Backup = HostedBackupSnapshot{State: "idle"}
		}
		c.mu.Unlock()
		c.wallet.opMu.Unlock()
		return nil, e
	}
	return op, nil
}
func (c *OwnedGymChild) backupPhase(op *poolBackupOperation, phase string) error {
	return c.backupFence(op.ctx, gymaccount.View, op)(func(check func() error) error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		c.snapshot.Backup.Phase = phase
		return nil
	})
}

// The cleanup error retains the exact failed destination/file owner. Closing
// has already joined its work, but failure cannot silently free its namespace.
type ownedBackupCleanupError struct {
	owner io.Closer
	cause error
}

func (e *ownedBackupCleanupError) Error() string { return "owned backup cleanup failed" }
func (e *ownedBackupCleanupError) Unwrap() error { return e.cause }
func (c *OwnedGymChild) retainBackupCleanup(owner io.Closer, e error) {
	if e != nil {
		c.backupCleanup(&ownedBackupCleanupError{owner: owner, cause: e})
	}
}

func (c *OwnedGymChild) backupCleanup(e error) {
	if e != nil {
		c.mu.Lock()
		c.backupCloseErr = errors.Join(c.backupCloseErr, e)
		c.mu.Unlock()
	}
}
func (c *OwnedGymChild) finishBackup(op *poolBackupOperation, code string) {
	work, cancel := context.WithTimeout(c.life, 2*time.Second)
	defer cancel()
	_ = c.backupFence(work, gymaccount.View, op)(func(check func() error) error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		switch {
		case code == "backup_storage_failed" || code == "backup_export_uncertain":
			c.snapshot.Backup = HostedBackupSnapshot{State: "failed", Error: code}
		case errors.Is(op.ctx.Err(), context.Canceled):
			c.snapshot.Backup = HostedBackupSnapshot{State: "cancelled"}
		case code != "":
			c.snapshot.Backup = HostedBackupSnapshot{State: "failed", Error: code}
		case op.verify:
			c.snapshot.Backup = HostedBackupSnapshot{State: "succeeded", Phase: "complete"}
		default:
			c.snapshot.Backup = HostedBackupSnapshot{State: "idle"}
		}
		return nil
	})
	op.cancel()
	c.wallet.opMu.Unlock()
	c.mu.Lock()
	if c.backup == op {
		c.backup = nil
	}
	close(op.done)
	c.mu.Unlock()
}
func (c *OwnedGymChild) protectedBackupRoots() ([]string, error) {
	config, e := os.UserConfigDir()
	if e != nil {
		return nil, e
	}
	executable, e := os.Executable()
	if e != nil {
		return nil, e
	}
	protected := []string{filepath.Dir(c.runtime.config.DataDir), c.config.DataDir, filepath.Join(config, "SLUG"), filepath.Dir(c.config.NodeBinary)}
	for _, f := range []string{executable, c.config.NodeBinary} {
		for p := filepath.Dir(f); p != filepath.Dir(p); p = filepath.Dir(p) {
			if strings.HasSuffix(strings.ToLower(p), ".app") {
				protected = append(protected, p)
				break
			}
		}
	}
	return protected, nil
}
func (c *OwnedGymChild) ExportRecovery(ctx context.Context, view string, pin recoveryfile.Pin) (resultErr error) {
	if recoveryfile.ValidatePin(pin, recoveryfile.CreateMode) != nil {
		return recoveryfile.ErrInvalid
	}
	op, e := c.beginBackup(ctx, view, false)
	if e != nil {
		return e
	}
	code := "backup_unavailable"
	defer func() { c.finishBackup(op, code) }()
	protected, e := c.protectedBackupRoots()
	if e != nil {
		return e
	}
	destination, e := recoveryvault.OpenCurrentDestination(op.ctx, recoveryvault.CurrentDestinationPin{Version: 1, Path: pin.Path, ParentDevice: pin.Parent.Device, ParentInode: pin.Parent.Inode}, protected)
	if e != nil {
		return e
	}
	defer func() {
		if e := destination.Close(); e != nil {
			if code != "backup_export_uncertain" {
				code = "backup_storage_failed"
			}
			c.retainBackupCleanup(destination, e)
			resultErr = errors.Join(resultErr, e)
		}
	}()
	head, e := c.wallet.RecoveryHead(op.ctx)
	if e != nil {
		return e
	}
	prepared, e := c.wallet.setup.vault.PrepareInventoryExport2(op.ctx, c.wallet.login.Token, head, destination, c.wallet.epoch)
	if e != nil {
		return e
	}
	defer func() {
		if e := prepared.Close(); e != nil {
			if code != "backup_export_uncertain" {
				code = "backup_storage_failed"
			}
			c.retainBackupCleanup(prepared, e)
			resultErr = errors.Join(resultErr, e)
		}
	}()
	e = prepared.Publish(op.ctx)
	switch {
	case errors.Is(e, recoveryvault.ErrExportUncertain), errors.Is(e, recoveryvault.ErrOutcomeUnknown):
		code = "backup_export_uncertain"
	case errors.Is(e, recoveryvault.ErrCleanup):
		code = "backup_storage_failed"
		c.backupCleanup(e)
	case e == nil:
		code = ""
	}
	return e
}
func (c *OwnedGymChild) VerifyBackup(ctx context.Context, view string, pin recoveryfile.Pin) error {
	if recoveryfile.ValidatePin(pin, recoveryfile.ReadMode) != nil {
		return recoveryfile.ErrInvalid
	}
	op, e := c.beginBackup(ctx, view, true)
	if e != nil {
		return e
	}
	protected, e := c.protectedBackupRoots()
	if e != nil {
		c.finishBackup(op, "backup_unavailable")
		return e
	}
	file, e := recoveryfile.Open(pin, protected)
	if e != nil {
		code := "backup_invalid"
		var cleanup *recoveryfile.CleanupError
		if errors.As(e, &cleanup) {
			c.retainBackupCleanup(cleanup, e)
			code = "backup_storage_failed"
		}
		c.finishBackup(op, code)
		return e
	}
	go func() { c.finishBackup(op, c.verifyBackup(op, file)) }()
	return nil
}
func (c *OwnedGymChild) CancelBackup(ctx context.Context, view string) error {
	if c == nil || view != c.view.ViewID {
		return gymstable.ErrStaleView
	}
	if !c.opMu.TryLock() {
		return gymstable.ErrBusy
	}
	defer c.opMu.Unlock()
	var op *poolBackupOperation
	if e := c.fence(ctx, func() error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		op = c.backup
		return nil
	}); e != nil {
		return e
	}
	if op != nil {
		op.cancel()
		<-op.done
	}
	return c.fence(ctx, func() error { return nil })
}
func (c *OwnedGymChild) verifyBackup(op *poolBackupOperation, file *recoveryfile.Handle) (code string) {
	ctx := op.ctx
	raw, e := file.Read(ctx, c.backupFence(ctx, gymaccount.View, op))
	ce := file.Close()
	defer clear(raw)
	if ce != nil {
		c.retainBackupCleanup(file, ce)
		return "backup_storage_failed"
	}
	if e != nil {
		return "backup_invalid"
	}
	bundle, e := recovery.ParseExport(raw)
	clear(raw)
	if e != nil {
		return "backup_invalid"
	}
	defer func() { bundle.Mnemonic = "" }()
	head, e := c.wallet.RecoveryHead(ctx)
	if e != nil {
		return "backup_unavailable"
	}
	if !reflect.DeepEqual(bundle.Manifest, head.Manifest()) {
		return "backup_mismatch"
	}
	if c.backupPhase(op, "checking") != nil {
		return "backup_unavailable"
	}
	before, e := hostedArchiveTip(ctx, c.node, c.scope.PoolSource.Policy)
	if e != nil {
		return "backup_unavailable"
	}
	scratch, e := os.MkdirTemp(c.config.DataDir, "owned-backup-verify-")
	if e != nil {
		return "backup_storage_failed"
	}
	restore := &poolBackupRestore{}
	defer func() {
		if e := cleanupPoolBackupScratch(scratch, restore.Close, os.RemoveAll); e != nil {
			c.backupCleanup(e)
			code = "backup_storage_failed"
		}
	}()
	if c.backupPhase(op, "restoring") != nil || restore.Run(ctx, filepath.Join(scratch, "wallet"), c.view.Fixture, bundle, c.rpc) != nil {
		return "backup_unavailable"
	}
	bundle.Mnemonic = ""
	restored := restore.runtime
	identity := walletIdentity{RootPublicKey: c.view.RootPublicKey, GymPublicKey: c.view.GymPublicKey, GymID: c.view.GymID}
	if restored.Identity() != identity || restored.Attach(ctx, c.rpc) != nil {
		return "backup_mismatch"
	}
	if c.backupPhase(op, "syncing") != nil {
		return "backup_unavailable"
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		tip, e := restored.poolBackupSyncedTip(ctx)
		if e == nil && tip == before {
			break
		}
		select {
		case <-ctx.Done():
			return "backup_unavailable"
		case <-tick.C:
		}
	}
	if _, e = restored.VerificationState(ctx); e != nil {
		return "backup_unavailable"
	}
	if c.backupPhase(op, "verifying") != nil {
		return "backup_unavailable"
	}
	initial, proof, e := backupSigningProof(before, identity)
	if e != nil {
		return "backup_unavailable"
	}
	signer, e := restored.Signer(ctx, c.scope.PoolSource.Policy)
	if e != nil {
		return "backup_unavailable"
	}
	auth, e := signer(ctx, initial, proof)
	if e != nil || auth.PublicKey != identity.GymPublicKey || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
		return "backup_mismatch"
	}
	after, e := hostedArchiveTip(ctx, c.node, c.scope.PoolSource.Policy)
	tip, te := restored.poolBackupSyncedTip(ctx)
	if e != nil || te != nil || before != after || before != tip {
		return "backup_unavailable"
	}
	if e = cleanupPoolBackupScratch(scratch, restore.Close, os.RemoveAll); e != nil {
		c.backupCleanup(e)
		return "backup_storage_failed"
	}
	lease, e := head.Hold(ctx, c.wallet.login.Token)
	if e != nil {
		return "backup_unavailable"
	}
	defer lease.Close()
	work, release, e := lease.Acquire(ctx)
	if e != nil {
		return "backup_unavailable"
	}
	defer release()
	if c.backupFence(work, gymaccount.View, op)(func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		return check()
	}) != nil {
		return "backup_unavailable"
	}
	return ""
}
