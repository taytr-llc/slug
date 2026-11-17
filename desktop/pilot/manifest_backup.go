// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slugwallet/recovery"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func (c *ManifestController) ExportRecoveryChecked(parent context.Context, path string, guard func() error) (resultErr error) {
	path, e := c.manifestExportPath(path, false)
	if e != nil {
		return errors.New("recovery export unavailable")
	}
	ctx, done, e := c.beginOperation(parent, "export", 30*time.Second)
	if e != nil {
		return e
	}
	defer c.finishOperation(done)
	w, e := c.openWallet(false)
	if e != nil {
		return errors.New("recovery export unavailable")
	}
	defer func() {
		if w.Close() != nil {
			c.noteShutdown("pilot wallet export close failed")
			resultErr = errors.New("recovery export close failed")
		}
	}()
	if w.Identity() != c.manifest.Identity {
		return errors.New("recovery export identity mismatch")
	}
	bundle, e := w.ExportRecovery(ctx)
	if e != nil {
		return errors.New("recovery export unavailable")
	}
	raw, e := recovery.MarshalExport(bundle)
	bundle.Mnemonic = ""
	defer clear(raw)
	if e != nil || ctx.Err() != nil || guard == nil || guard() != nil {
		return errors.New("recovery export authorization unavailable")
	}
	output := append(raw, '\n')
	defer clear(output)
	if manifestExclusiveFile(ctx, path, output, guard) != nil {
		return errors.New("recovery export unavailable")
	}
	return nil
}
func (c *ManifestController) VerifyBackup(parent context.Context, path string) error {
	path, e := c.manifestExportPath(path, true)
	if e != nil {
		return errors.New("backup verification unavailable")
	}
	ctx, done, e := c.beginOperation(parent, "backup", 10*time.Minute)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "running", "opening", ""
	c.mu.Unlock()
	go func() { defer c.finishOperation(done); c.backupResult(ctx, c.verifyBackup(ctx, path)) }()
	return nil
}
func (c *ManifestController) backupResult(ctx context.Context, code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case code == "backup_storage_failed":
		// Cancellation cannot hide a failed close or scratch-directory removal.
		c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
	case errors.Is(ctx.Err(), context.Canceled):
		c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "cancelled", "", ""
	case code != "":
		c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
	default:
		c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "succeeded", "complete", ""
	}
}

// The remover argument keeps filesystem-failure tests independent of platform
// permission behavior; production always supplies os.RemoveAll.
func (c *ManifestController) cleanupBackupScratch(path string, remove func(string) error) error {
	if err := remove(path); err != nil {
		c.noteShutdown("backup scratch cleanup failed")
		return errors.New("backup scratch cleanup failed")
	}
	return nil
}

func (c *ManifestController) backupPhase(phase string) {
	c.mu.Lock()
	c.snapshot.BackupPhase = phase
	c.mu.Unlock()
}

func (c *ManifestController) verifyBackup(ctx context.Context, path string) (code string) {
	bundle, err := readBackup(path)
	if err != nil {
		return "backup_invalid"
	}
	defer func() { bundle.Mnemonic = "" }()
	if bundle.Manifest.RootPublicKey != c.manifest.Identity.RootPublicKey || bundle.Manifest.GymPublicKey != c.manifest.Identity.GymPublicKey || bundle.Manifest.GymID != c.manifest.Identity.GymID {
		return "backup_mismatch"
	}
	if ctx.Err() != nil {
		return "backup_unavailable"
	}
	if checkManifestChains(c.config.DataDir, c.manifest) != nil {
		return "backup_storage_failed"
	}
	c.backupPhase("checking")
	nodes, err := c.openNodes(ctx)
	if err != nil {
		if errors.Is(err, errPilotNodeClose) {
			return "backup_storage_failed"
		}
		return "backup_unavailable"
	}
	defer func() {
		if nodes.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup node close failed")
		}
	}()
	original, err := c.openWallet(false)
	if err != nil {
		return "backup_unavailable"
	}
	originalClosed := false
	defer func() {
		if !originalClosed && original.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup original wallet close failed")
		}
	}()
	if original.Identity() != c.manifest.Identity || original.Attach(ctx, nodes.rpcConfig()) != nil {
		return "backup_mismatch"
	}
	if _, err := original.EnsurePayout(ctx, &c.manifest.Payout); err != nil {
		return "backup_mismatch"
	}
	if waitWallet(ctx, original, nodes.primary) != nil {
		return "backup_unavailable"
	}
	expected, err := original.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	tip, err := observedTip(ctx, nodes.primary)
	if err != nil {
		return "backup_unavailable"
	}
	if err := original.Close(); err != nil {
		originalClosed = true
		c.noteShutdown("backup original wallet close failed")
		return "backup_storage_failed"
	}
	originalClosed = true
	// The fresh restorer receives only the selected file's parsed bundle.
	// The original wallet has been closed; expectations above are public data.
	c.backupPhase("restoring")
	scratch, err := os.MkdirTemp(c.config.DataDir, "backup-verify-")
	if err != nil {
		return "backup_storage_failed"
	}
	defer func() {
		if c.cleanupBackupScratch(scratch, os.RemoveAll) != nil {
			code = "backup_storage_failed"
		}
	}()
	restored, err := c.restoreWallet(filepath.Join(scratch, "wallet"), bundle)
	bundle.Mnemonic = ""
	if err != nil {
		return "backup_invalid"
	}
	defer func() {
		if restored.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup restored wallet close failed")
		}
	}()
	if restored.Identity() != c.manifest.Identity || restored.Attach(ctx, nodes.rpcConfig()) != nil {
		return "backup_mismatch"
	}
	if _, err := restored.EnsurePayout(ctx, &c.manifest.Payout); err != nil {
		return "backup_mismatch"
	}
	c.backupPhase("syncing")
	if waitWallet(ctx, restored, nodes.primary) != nil {
		return "backup_unavailable"
	}
	actual, err := restored.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	if !reflect.DeepEqual(expected, actual) {
		return "backup_mismatch"
	}
	c.backupPhase("verifying")
	// A deliberately unmined, unpublished job tests only restored key control.
	// There is no work/eligibility claim, submission, admission or new receipt.
	initial, proof, err := backupSigningProof(tip, c.manifest.Identity)
	if err != nil {
		return "backup_unavailable"
	}
	signer, err := restored.Signer(ctx, policy())
	if err != nil {
		return "backup_unavailable"
	}
	authorization, err := signer(ctx, initial, proof)
	if err != nil || gymauth.VerifyAuthorization(initial, proof, authorization) != nil {
		return "backup_mismatch"
	}
	after, err := observedTip(ctx, nodes.primary)
	if err != nil || after != tip || ctx.Err() != nil {
		return "backup_unavailable"
	}
	c.mu.Lock()
	c.snapshot.Height, c.snapshot.WalletHeight = uint64(actual.View.Height), uint64(actual.View.Height)
	c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = actual.View.Spendable, actual.View.Immature
	c.mu.Unlock()
	return ""
}
