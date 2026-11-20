// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

type hostedBackupError struct {
	code  string
	cause error
}

func (e *hostedBackupError) Error() string           { return e.code }
func (e *hostedBackupError) Unwrap() error           { return e.cause }
func hostedBackupFailure(code string, e error) error { return &hostedBackupError{code, e} }

// HostedRecoveryOwner has no installed wallet, miner, stable or payment owner.
// It borrows only surviving independently audited authority/inventory/source.
type HostedRecoveryOwner struct {
	mu               sync.Mutex
	root, fixture    string
	accounts         *gymaccount.Store
	vault            *recoveryvault.HostedVault
	inventory        *wallet.HostedAllocationInventory
	archive          *PoolRecoveryArchive
	session          gymaccount.Session
	epoch            recoveryvault.EpochFence
	snapshot         HostedMissingWalletSnapshot
	life             context.Context
	cancel           context.CancelFunc
	prepared, closed bool
	operation        *poolBackupOperation
	closeErr         error
	once             sync.Once
}

func NewHostedRecoveryOwner(root string, accounts *gymaccount.Store, vault *recoveryvault.HostedVault, setup *HostedAllocationSetup, archive *PoolRecoveryArchive, fixture string, session gymaccount.Session, view string, scope HostedAllocationScope, epoch recoveryvault.EpochFence) (*HostedRecoveryOwner, error) {
	if !filepath.IsAbs(root) || accounts == nil || vault == nil || setup == nil || archive == nil || epoch == nil || session.Token == "" || session.Kind != "login" || !payoutNonzeroID(view) || ValidateHostedAllocationScope(scope) != nil || scope.CurrentBinding.VaultBinding.AccountID != session.Account.ID || scope.CurrentBinding.VaultBinding.AuthorityID != accounts.AuthorityID() || archive.bindings[fixture] != scope.CurrentBinding {
		return nil, ErrPoolRuntime
	}
	inv, e := setup.Inventory(fixture)
	if e != nil {
		return nil, e
	}
	b, e := HostedAllocationBinding(scope)
	if e != nil || !reflect.DeepEqual(b, inv.Binding()) {
		return nil, ErrPoolRuntime
	}
	life, cancel := context.WithCancel(context.Background())
	v := scope.CurrentBinding.VaultBinding
	return &HostedRecoveryOwner{root: root, fixture: fixture, accounts: accounts, vault: vault, inventory: inv, archive: archive, session: session, epoch: epoch, life: life, cancel: cancel, snapshot: HostedMissingWalletSnapshot{View: HostedRecoveryView{1, view, accounts.AuthorityID(), session.Account.ID, session.Account.Generation, fixture, v.RootPublicKey, v.GymPublicKey, v.GymID}, Allocation: scope, LocalWallet: "missing", Backup: HostedBackupSnapshot{State: "idle"}}}, nil
}
func (o *HostedRecoveryOwner) fence(ctx context.Context, cap gymaccount.Capability, op *poolBackupOperation, fn func(func() error) error) error {
	if ctx == nil || fn == nil {
		return gymaccount.ErrUnauthorized
	}
	called := false
	var inner error
	e := o.accounts.WithSession(ctx, o.session.Token, cap, func(a gymaccount.Account, recheck func() error) (e error) {
		defer func() { inner = e }()
		if called || recheck == nil || a.ID != o.session.Account.ID || a.Generation != o.session.Account.Generation {
			return gymaccount.ErrUnauthorized
		}
		called = true
		epochCalled := false
		var epochInner error
		outer := o.epoch(ctx, func(parentCheck func() error) (result error) {
			defer func() { epochInner = errors.Join(epochInner, result) }()
			if epochCalled || parentCheck == nil {
				return gymaccount.ErrUnauthorized
			}
			epochCalled = true
			check := func() error {
				if ctx.Err() != nil || o.life.Err() != nil {
					return gymaccount.ErrUnauthorized
				}
				if e := recheck(); e != nil {
					return e
				}
				if e := parentCheck(); e != nil {
					return e
				}
				if !o.mu.TryLock() {
					return wallet.ErrAllocationBusy
				}
				defer o.mu.Unlock()
				if o.closed || op != nil && o.operation != op {
					return gymaccount.ErrUnauthorized
				}
				return nil
			}
			if e := check(); e != nil {
				return e
			}
			if e := fn(check); e != nil {
				return e
			}
			return check()
		})
		if !epochCalled {
			return errors.Join(gymaccount.ErrUnauthorized, outer)
		}
		return errors.Join(outer, epochInner)
	})
	if !called {
		return errors.Join(gymaccount.ErrUnauthorized, e)
	}
	return errors.Join(e, inner)
}
func (o *HostedRecoveryOwner) Prepare(ctx context.Context) error {
	status, e := o.vault.Status(ctx, o.session.Token, o.epoch)
	if e != nil || status.Receipt == nil {
		return errors.Join(ErrPoolRuntime, e)
	}
	b := o.snapshot.Allocation.CurrentBinding
	if status.Receipt.Binding != b.VaultBinding || status.Receipt.RetentionID != b.RetentionID || o.session.Account.Enrollment == nil {
		return ErrPoolRuntime
	}
	if o.session.Account.RecoveryRequired {
		challenge, e := o.accounts.BeginChallenge(ctx, o.session.Token, "recover")
		if e != nil {
			return e
		}
		auth, e := o.vault.SignAccountChallenge(ctx, o.session.Token, challenge, o.epoch)
		if e != nil {
			return e
		}
		if _, _, e = o.accounts.CompleteChallenge(ctx, o.session.Token, challenge.ChallengeID, auth); e != nil {
			return e
		}
	}
	receipt, e := o.vault.EnsureEnrollment(ctx, o.session.Token, o.epoch)
	if e != nil || receipt.Binding != b.VaultBinding || receipt.RetentionID != b.RetentionID {
		return errors.Join(ErrPoolRuntime, e)
	}
	statusHead, e := o.inventory.Status(ctx, o.session.Token, wallet.HostedAllocationEpochFence(o.epoch))
	if e != nil {
		return e
	}
	return o.fence(ctx, gymaccount.View, nil, func(check func() error) error {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.snapshot.InventoryRevision = statusHead.Revision
		o.snapshot.ObservedAtMS = statusHead.ObservedAtMS
		o.prepared = true
		return nil
	})
}
func (o *HostedRecoveryOwner) Snapshot() HostedMissingWalletSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshot
}
func (o *HostedRecoveryOwner) begin(ctx context.Context, verify bool) (*poolBackupOperation, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, wallet.ErrAllocationInvalid
	}
	duration, cap := 30*time.Second, gymaccount.RecoveryExport
	if verify {
		duration, cap = 10*time.Minute, gymaccount.View
	}
	work, cancel := context.WithTimeout(ctx, duration)
	stop := context.AfterFunc(o.life, cancel)
	op := &poolBackupOperation{ctx: work, cancel: func() { stop(); cancel() }, done: make(chan struct{}), verify: verify}
	e := o.fence(work, cap, nil, func(check func() error) error {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.prepared || o.operation != nil || o.closeErr != nil {
			return wallet.ErrAllocationBusy
		}
		o.operation = op
		o.snapshot.Backup = HostedBackupSnapshot{State: "running", Phase: "opening"}
		return nil
	})
	if e != nil {
		op.cancel()
		o.mu.Lock()
		if o.operation == op {
			o.operation = nil
			o.snapshot.Backup = HostedBackupSnapshot{State: "idle"}
		}
		o.mu.Unlock()
		return nil, e
	}
	return op, nil
}
func (o *HostedRecoveryOwner) phase(op *poolBackupOperation, phase string) error {
	return o.fence(op.ctx, gymaccount.View, op, func(check func() error) error {
		o.mu.Lock()
		o.snapshot.Backup.Phase = phase
		o.mu.Unlock()
		return nil
	})
}
func (o *HostedRecoveryOwner) finish(op *poolBackupOperation, code string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.operation == op {
		switch {
		case code == "backup_storage_failed" || code == "backup_export_uncertain":
			o.snapshot.Backup = HostedBackupSnapshot{State: "failed", Error: code}
		case op.ctx.Err() != nil:
			o.snapshot.Backup = HostedBackupSnapshot{State: "cancelled"}
		case code != "":
			o.snapshot.Backup = HostedBackupSnapshot{State: "failed", Error: code}
		case op.verify:
			o.snapshot.Backup = HostedBackupSnapshot{State: "succeeded", Phase: "complete"}
		default:
			o.snapshot.Backup = HostedBackupSnapshot{State: "idle"}
		}
		o.operation = nil
	}
	op.cancel()
	close(op.done)
}
func (o *HostedRecoveryOwner) protected() ([]string, error) {
	user, e := os.UserConfigDir()
	if e != nil {
		return nil, e
	}
	return []string{o.root, filepath.Join(user, "SLUG"), filepath.Dir(o.archive.r.config.NodeBinary)}, nil
}
func (o *HostedRecoveryOwner) cleanup(e error) {
	if e != nil {
		o.mu.Lock()
		o.closeErr = errors.Join(o.closeErr, e)
		o.mu.Unlock()
	}
}
func (o *HostedRecoveryOwner) Export(ctx context.Context, pin recoveryfile.Pin) (resultErr error) {
	if recoveryfile.ValidatePin(pin, recoveryfile.CreateMode) != nil {
		return recoveryfile.ErrInvalid
	}
	op, e := o.begin(ctx, false)
	if e != nil {
		return e
	}
	code := "backup_unavailable"
	defer func() { o.finish(op, code) }()
	protected, e := o.protected()
	if e != nil {
		return e
	}
	destination, e := recoveryvault.OpenCurrentDestination(op.ctx, recoveryvault.CurrentDestinationPin{Version: 1, Path: pin.Path, ParentDevice: pin.Parent.Device, ParentInode: pin.Parent.Inode}, protected)
	if e != nil {
		return e
	}
	cleanup := func(e error) {
		if e == nil {
			return
		}
		o.cleanup(e)
		if code != "backup_export_uncertain" {
			code = "backup_storage_failed"
		}
		resultErr = errors.Join(resultErr, e)
	}
	defer func() { cleanup(destination.Close()) }()
	head, e := o.inventory.CaptureRecoveryHead(op.ctx, o.session.Token, wallet.HostedAllocationEpochFence(o.epoch))
	if e != nil {
		return e
	}
	prepared, e := o.vault.PrepareInventoryExport(op.ctx, o.session.Token, head, destination, o.epoch)
	if e != nil {
		return e
	}
	defer func() { cleanup(prepared.Close()) }()
	e = prepared.Publish(op.ctx)
	switch {
	case errors.Is(e, recoveryvault.ErrExportUncertain):
		code = "backup_export_uncertain"
	case errors.Is(e, recoveryvault.ErrCleanup):
		code = "backup_storage_failed"
	case e == nil:
		code = ""
	}
	return e
}
func (o *HostedRecoveryOwner) Verify(ctx context.Context, pin recoveryfile.Pin) error {
	if recoveryfile.ValidatePin(pin, recoveryfile.ReadMode) != nil {
		return recoveryfile.ErrInvalid
	}
	op, e := o.begin(ctx, true)
	if e != nil {
		return e
	}
	protected, e := o.protected()
	if e != nil {
		o.finish(op, "backup_unavailable")
		return e
	}
	file, e := recoveryfile.Open(pin, protected)
	if e != nil {
		o.finish(op, "backup_invalid")
		return hostedBackupFailure("backup_invalid", e)
	}
	go func() { o.finish(op, o.verify(op, file)) }()
	return nil
}
func (o *HostedRecoveryOwner) verify(op *poolBackupOperation, file *recoveryfile.Handle) (code string) {
	ctx := op.ctx
	raw, e := file.Read(ctx, func(fn func(func() error) error) error { return o.fence(ctx, gymaccount.View, op, fn) })
	ce := file.Close()
	defer clear(raw)
	if ce != nil {
		o.cleanup(ce)
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
	head, e := o.inventory.CaptureRecoveryHead(ctx, o.session.Token, wallet.HostedAllocationEpochFence(o.epoch))
	if e != nil {
		return "backup_unavailable"
	}
	if !reflect.DeepEqual(bundle.Manifest, head.Manifest()) {
		return "backup_mismatch"
	}
	if o.phase(op, "checking") != nil {
		return "backup_unavailable"
	}
	node, rpc, closeSource, e := o.archive.BeginSource(ctx)
	if e != nil {
		return "backup_unavailable"
	}
	defer func() {
		if e := closeSource(); e != nil {
			o.cleanup(e)
			code = "backup_storage_failed"
		}
	}()
	before, e := hostedArchiveTip(ctx, node, o.archive.r.source.Policy)
	if e != nil {
		return "backup_unavailable"
	}
	scratch, e := os.MkdirTemp(o.root, "hosted-recovery-verify-")
	if e != nil {
		return "backup_storage_failed"
	}
	restore := &poolBackupRestore{}
	defer func() {
		if e := cleanupPoolBackupScratch(scratch, restore.Close, os.RemoveAll); e != nil {
			o.cleanup(e)
			code = "backup_storage_failed"
		}
	}()
	if o.phase(op, "restoring") != nil {
		return "backup_unavailable"
	}
	if restore.Run(ctx, filepath.Join(scratch, "wallet"), o.fixture, bundle, rpc) != nil {
		return "backup_unavailable"
	}
	bundle.Mnemonic = ""
	restored := restore.runtime
	id := walletIdentity{head.Manifest().RootPublicKey, head.Manifest().GymPublicKey, head.Manifest().GymID}
	if restored.Identity() != id || restored.Attach(ctx, rpc) != nil {
		return "backup_mismatch"
	}
	if o.phase(op, "syncing") != nil {
		return "backup_unavailable"
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		tip, e := restored.poolBackupSyncedTip(ctx)
		if e == nil && tip == before {
			break
		}
		select {
		case <-ctx.Done():
			return "backup_unavailable"
		case <-ticker.C:
		}
	}
	state, e := restored.VerificationState(ctx)
	if e != nil {
		return "backup_unavailable"
	}
	_ = state // bounded public ledger inspected; no absent installed baseline is invented.
	if o.phase(op, "verifying") != nil {
		return "backup_unavailable"
	}
	initial, proof, e := backupSigningProof(before, id)
	if e != nil {
		return "backup_unavailable"
	}
	signer, e := restored.Signer(ctx, o.archive.r.source.Policy)
	if e != nil {
		return "backup_unavailable"
	}
	auth, e := signer(ctx, initial, proof)
	if e != nil || auth.PublicKey != id.GymPublicKey || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
		return "backup_mismatch"
	}
	after, e := hostedArchiveTip(ctx, node, o.archive.r.source.Policy)
	walletTip, we := restored.poolBackupSyncedTip(ctx)
	if e != nil || we != nil || after != before || walletTip != before {
		return "backup_unavailable"
	}
	if e := cleanupPoolBackupScratch(scratch, restore.Close, os.RemoveAll); e != nil {
		o.cleanup(e)
		return "backup_storage_failed"
	}
	if e := closeSource(); e != nil {
		o.cleanup(e)
		return "backup_storage_failed"
	}
	lease, e := o.inventory.HoldRecoveryHead(ctx, head)
	if e != nil {
		return "backup_unavailable"
	}
	defer lease.Close()
	work, release, e := lease.Acquire(ctx)
	if e != nil {
		return "backup_unavailable"
	}
	defer release()
	if o.fence(work, gymaccount.View, op, func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		return check()
	}) != nil {
		return "backup_unavailable"
	}
	return ""
}
func hostedArchiveTip(ctx context.Context, node *prospectnode.Client, policy prospectnode.Policy) (localTip, error) {
	v, e := node.Resolve(ctx, policy)
	if e != nil || v.ParentHeight > math.MaxInt32 {
		return localTip{}, ErrPoolRuntime
	}
	return localTip{Hash: v.ParentHash, Height: int32(v.ParentHeight)}, nil
}
func (o *HostedRecoveryOwner) CancelBackup(ctx context.Context) error {
	if e := o.fence(ctx, gymaccount.View, nil, func(check func() error) error { return check() }); e != nil {
		return e
	}
	o.mu.Lock()
	op := o.operation
	if op != nil {
		op.cancel()
	}
	o.mu.Unlock()
	if op != nil {
		<-op.done
	}
	return o.fence(ctx, gymaccount.View, nil, func(check func() error) error { return check() })
}
func (o *HostedRecoveryOwner) Close() error {
	if o == nil {
		return nil
	}
	o.once.Do(func() {
		o.mu.Lock()
		o.closed = true
		o.cancel()
		op := o.operation
		if op != nil {
			op.cancel()
		}
		o.mu.Unlock()
		if op != nil {
			<-op.done
		}
	})
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.closeErr
}
