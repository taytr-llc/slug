// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func validateHostedPoolChild(r poolChildResources, a StableAccountConfig) error {
	if r.Hosted == nil {
		if a.Hosted != nil {
			return ErrPoolRuntime
		}
		return nil
	}
	h := a.Hosted
	if h == nil || h.runtime == nil || h.runtime.hosted != r.Hosted || h.runtime.source != r.Source || h.login.Token == "" || h.login.Account.ID != a.Manifest.Account.AccountID || h.login.Account.Generation != a.Manifest.Generation || h.epoch == nil || r.Hosted.current == nil {
		return ErrPoolRuntime
	}
	b := r.HostedBinding
	if b.Version != 1 || b.VaultBinding.AccountID != h.login.Account.ID || b.VaultBinding.AuthorityID != a.Manifest.Account.AuthorityID || b.VaultBinding.GymID != r.Recipient.Config.Identity.GymID || b.WalletOwnerID != r.Hosted.owners[a.Manifest.Account.Fixture] {
		return ErrPoolRuntime
	}
	return nil
}
func (c *PoolChild) attachHostedCurrent(ctx context.Context) error {
	if c.resources.Hosted != nil && c.resources.Hosted.allocation != nil {
		if c.wallet == nil || c.wallet.allocation == nil {
			return ErrPoolRuntime
		}
		return nil
	}
	if c.resources.Hosted == nil {
		return nil
	}
	if validateHostedPoolChild(c.resources, c.account) != nil || c.wallet == nil {
		return ErrPoolRuntime
	}
	if !c.wallet.mu.TryLock() {
		return ErrPoolRuntimeBusy
	}
	installed := c.wallet.wallet
	available := !c.wallet.closed && installed != nil
	c.wallet.mu.Unlock()
	if !available {
		return ErrPoolRuntime
	}
	// Prepare exclusively owns stopMu and has not published any wallet operation.
	// Close cancels/joins Prepare before unloading this exact pointer. No wallet
	// or parent mutex is retained across the source's account/epoch fences.
	h := c.account.Hosted
	b := c.resources.HostedBinding
	source, e := c.resources.Hosted.current.AttachSource(ctx, h.login.Token, b, installed, h.epoch)
	if e != nil {
		return e
	}
	c.hostedCurrent = source
	return nil
}

// The concrete hosted adapter captures one original mining token per signature.
// Vault errors are final; the installed wallet is never a signer fallback.
func (c *PoolChild) hostedSigner(ctx context.Context) (prospectworker.Signer, error) {
	if c.resources.Hosted == nil || c.account.Hosted == nil || (c.hostedCurrent == nil && (c.wallet == nil || c.wallet.allocation == nil)) || c.authority.checkView(ctx) != nil {
		return nil, gymaccount.ErrUnauthorized
	}
	return func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		p := c.resources.Source.Policy
		if len(proof.MerkleBranch) > 20 || proof.Version != prospect.Version2 || initial.Network != p.Network || initial.DNAVersion != p.DNAVersion || !strings.EqualFold(initial.Genesis, p.Genesis) || !strings.EqualFold(initial.ProspectTarget, p.ProspectTarget) {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		proof.MerkleBranch = append([]string(nil), proof.MerkleBranch...)
		if c.authority.checkView(ctx) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		token, e := c.account.Manifest.Account.MiningToken(ctx)
		if e != nil {
			return gymauth.Authorization{}, e
		}
		lease, e := mininglease.Capture(ctx, c.account.Manifest.Account.Authorizer, token)
		if e != nil || lease.PublicKey() != c.resources.WorkerConfig.PublicKey {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		auth, e := c.resources.Hosted.vault.SignAdmission(ctx, token, initial, proof, c.account.Hosted.epoch)
		if e != nil {
			return gymauth.Authorization{}, e
		}
		if auth.PublicKey != c.resources.HostedBinding.VaultBinding.GymPublicKey || auth.PublicKey != c.resources.WorkerConfig.PublicKey || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		if lease.Check() != nil || c.authority.checkView(ctx) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		return auth, nil
	}, nil
}
func (c *PoolChild) exportHostedRecovery(op *poolBackupOperation, pin recoveryfile.Pin) (code string, resultErr error) {
	if c.resources.Hosted != nil && c.resources.Hosted.allocation != nil {
		return c.exportHostedInventoryRecovery(op, pin)
	}
	code = "backup_unavailable"
	if c.hostedCurrent == nil || c.resources.Hosted == nil || recoveryfile.ValidatePin(pin, recoveryfile.CreateMode) != nil {
		return code, ErrPoolRuntime
	}
	release, e := c.resources.Hosted.acquireRecovery()
	if e != nil {
		return code, e
	}
	defer release()
	protected, e := c.poolBackupProtected()
	if e != nil {
		return code, e
	}
	protected = append(protected, filepath.Dir(filepath.Dir(filepath.Dir(c.resources.Config.DataDir))))
	destination, e := recoveryvault.OpenCurrentDestination(op.ctx, recoveryvault.CurrentDestinationPin{Version: 1, Path: pin.Path, ParentDevice: pin.Parent.Device, ParentInode: pin.Parent.Inode}, protected)
	if e != nil {
		return code, e
	}
	defer func() {
		if e := destination.Close(); e != nil {
			if code != "backup_export_uncertain" {
				code = "backup_storage_failed"
			}
			c.mu.Lock()
			c.backupCloseErr = errors.Join(c.backupCloseErr, e)
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, e)
		}
	}()
	head, exists, e := c.hostedCurrent.CurrentCheckpoint(op.ctx)
	if e != nil {
		return code, e
	}
	revision := uint32(0)
	if exists {
		revision = head.Revision
	}
	if !c.wallet.mu.TryLock() {
		return code, ErrPoolRuntimeBusy
	}
	available := !c.wallet.closed && c.wallet.wallet != nil
	c.wallet.mu.Unlock()
	if !available {
		return code, ErrPoolRuntime
	}
	// beginPoolBackup has stopped mining and joined every prior wallet task. The
	// retained backup operation is joined by Stop/Close before wallet unload;
	// Capture can run its original account fences without this runtime mutex.
	capture, e := c.hostedCurrent.Capture(op.ctx)
	if e != nil {
		return code, e
	}
	checkpoint, _, e := c.hostedCurrent.Checkpoint(op.ctx, revision, capture)
	if e != nil {
		return code, e
	}
	prepared, e := c.hostedCurrent.PrepareExport(op.ctx, checkpoint.CaptureID, destination)
	if e != nil {
		return code, e
	}
	defer func() {
		if e := prepared.Close(); e != nil {
			if code != "backup_export_uncertain" {
				code = "backup_storage_failed"
			}
			c.mu.Lock()
			c.backupCloseErr = errors.Join(c.backupCloseErr, e)
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, e)
		}
	}()
	e = prepared.Publish(op.ctx)
	switch {
	case errors.Is(e, recoveryvault.ErrExportUncertain), errors.Is(e, recoveryvault.ErrOutcomeUnknown):
		code = "backup_export_uncertain"
	case errors.Is(e, recoveryvault.ErrCleanup):
		code = "backup_storage_failed"
		c.mu.Lock()
		c.backupCloseErr = errors.Join(c.backupCloseErr, e)
		c.mu.Unlock()
	case e == nil:
		code = ""
	}
	return code, e
}
