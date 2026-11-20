// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

// HostedPoolPolicy exposes only the fixed candidate source policy needed to pin
// the hosted vault before the runtime is opened. It is not configurable.
func HostedPoolPolicy() prospectnode.Policy { return policy() }

type hostedPoolResources struct {
	allocation *HostedAllocationSetup
	vault      *recoveryvault.HostedVault
	current    *recoveryvault.CurrentStore
	owners     map[string]string
	bindings   map[string]recoveryvault.CurrentBinding
	// One accounted 16MiB recovery operation across all child generations; no queue.
	recoveryPermit chan struct{}
	failedWallet   *walletRuntime
	failure        error
}

// HostedPoolSession is a concrete, runtime-bound original login/parent epoch.
// Its private fields prevent a generic signer or caller-populated credential
// adapter from selecting an alternate signing authority.
type HostedPoolSession struct {
	runtime *PoolRuntime
	login   gymaccount.Session
	epoch   recoveryvault.EpochFence
}

func NewHostedPoolRuntime(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, vault *recoveryvault.HostedVault, zeroOwner, incrementingOwner string) (*PoolRuntime, error) {
	if ctx == nil || vault == nil || accounts == nil || !validAccountID(zeroOwner) || !validAccountID(incrementingOwner) || zeroOwner == incrementingOwner {
		return nil, ErrPoolRuntime
	}
	info, e := vault.Info(ctx)
	if e != nil || info.Authority.Version != 2 || info.Authority.AuthorityID != accounts.AuthorityID() || info.Config.Policy != policy() {
		return nil, ErrPoolRuntime
	}
	r, e := NewPoolRuntimeWithOptions(ctx, config, accounts, options)
	if e != nil {
		return nil, e
	}
	if r.source.Policy != info.Config.Policy {
		_ = r.Close()
		return nil, ErrPoolRuntime
	}
	r.hosted = &hostedPoolResources{vault: vault, owners: map[string]string{"zero256": zeroOwner, "incrementing256": incrementingOwner}, bindings: make(map[string]recoveryvault.CurrentBinding), recoveryPermit: make(chan struct{}, 1)}
	return r, nil
}
func (r *PoolRuntime) HostedSession(login gymaccount.Session, epoch recoveryvault.EpochFence) (*HostedPoolSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.hosted == nil || r.hosted.current == nil || r.hosted.failure != nil || epoch == nil || login.Kind != "login" || login.Token == "" || !validAccountID(login.Account.ID) {
		return nil, ErrPoolRuntime
	}
	found := false
	for _, b := range r.hosted.bindings {
		if b.VaultBinding.AccountID == login.Account.ID {
			found = true
		}
	}
	if !found {
		return nil, ErrPoolRuntime
	}
	return &HostedPoolSession{r, login, epoch}, nil
}
func (r *PoolRuntime) HostedBinding(fixture string) (recoveryvault.CurrentBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hosted == nil || r.closed || r.hosted.failure != nil {
		return recoveryvault.CurrentBinding{}, ErrPoolRuntime
	}
	b, ok := r.hosted.bindings[fixture]
	if !ok {
		return b, ErrPoolRuntime
	}
	return b, nil
}
func (r *PoolRuntime) AttachHostedCurrent(store *recoveryvault.CurrentStore, bindings []recoveryvault.CurrentBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.hosted == nil || store == nil || len(bindings) != 2 || r.hosted.failure != nil {
		return ErrPoolRuntime
	}
	if r.hosted.current != nil && r.hosted.current != store {
		return ErrPoolRuntime
	}
	if bindings[0] != r.hosted.bindings["zero256"] || bindings[1] != r.hosted.bindings["incrementing256"] {
		return ErrPoolRuntime
	}
	r.hosted.current = store
	return nil
}
func (r *PoolRuntime) ProvisionHostedFixture(ctx context.Context, fixture string, login gymaccount.Session, epoch recoveryvault.EpochFence) (PoolRecipientBinding, error) {
	r.mu.Lock()
	available := !r.closed && r.hosted != nil && r.hosted.failure == nil
	r.mu.Unlock()
	if !available || epoch == nil {
		return PoolRecipientBinding{}, ErrPoolRuntime
	}
	return r.provisionFixture(ctx, fixture, login, epoch)
}
func (r *PoolRuntime) hostedReceipt(ctx context.Context, login gymaccount.Session, epoch recoveryvault.EpochFence, identity gymaccount.Identity) (recoveryvault.Receipt, error) {
	r.mu.Lock()
	h := r.hosted
	available := h != nil && h.failure == nil
	r.mu.Unlock()
	if !available {
		return recoveryvault.Receipt{}, ErrPoolRuntime
	}
	status, e := h.vault.Status(ctx, login.Token, epoch)
	if errors.Is(e, recoveryvault.ErrNotFound) {
		if login.Account.Enrollment != nil {
			return recoveryvault.Receipt{}, ErrPoolRuntime
		}
		receipt, _, e := h.vault.ProvisionFixture(ctx, login.Token, epoch)
		return receipt, e
	}
	if e != nil {
		return recoveryvault.Receipt{}, e
	}
	if status.Receipt == nil {
		if login.Account.Enrollment != nil {
			return recoveryvault.Receipt{}, ErrPoolRuntime
		}
		receipt, _, e := h.vault.ProvisionFixture(ctx, login.Token, epoch)
		return receipt, e
	}
	b := status.Receipt.Binding
	if b.AccountID != login.Account.ID || b.AuthorityID != r.accounts.AuthorityID() || b.RootPublicKey != identity.RootPublicKey || b.GymPublicKey != identity.GymPublicKey || b.GymID != identity.GymID || b.Profile != identity.Profile || b.Network != identity.Network || b.Genesis != identity.Genesis {
		return recoveryvault.Receipt{}, ErrPoolRuntime
	}
	return *status.Receipt, nil
}
func (r *PoolRuntime) hostedCurrentBinding(ctx context.Context, fixture string, login gymaccount.Session, epoch recoveryvault.EpochFence) (recoveryvault.CurrentBinding, error) {
	receipt, e := r.hosted.vault.EnsureEnrollment(ctx, login.Token, epoch)
	if e != nil {
		return recoveryvault.CurrentBinding{}, e
	}
	info, e := r.hosted.vault.Info(ctx)
	if e != nil {
		return recoveryvault.CurrentBinding{}, e
	}
	source, e := poolservice.BindingID(r.source)
	if e != nil {
		return recoveryvault.CurrentBinding{}, e
	}
	b := recoveryvault.CurrentBinding{Version: 1, HostedVaultID: info.HostedVaultID, VaultBinding: receipt.Binding, RetentionID: receipt.RetentionID, WalletOwnerID: r.hosted.owners[fixture], SourceBindingID: source}
	if !validAccountID(b.WalletOwnerID) {
		return recoveryvault.CurrentBinding{}, ErrPoolRuntime
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.hosted.bindings[fixture]; ok && !reflect.DeepEqual(old, b) {
		return recoveryvault.CurrentBinding{}, ErrPoolRuntime
	}
	return b, nil
}

// A failed close remains owned; the runtime and parent refuse replacement.
func (r *PoolRuntime) retainHostedWallet(w *walletRuntime, e error) {
	if r.hosted == nil || e == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hosted.failedWallet = w
	r.hosted.failure = errors.Join(r.hosted.failure, e)
}

// Reserved for a single original child; no account lease spans this permit.
type hostedRecoveryRelease struct {
	once sync.Once
	h    *hostedPoolResources
}

func (h *hostedPoolResources) acquireRecovery() (func(), error) {
	select {
	case h.recoveryPermit <- struct{}{}:
		release := &hostedRecoveryRelease{h: h}
		return func() { release.once.Do(func() { <-h.recoveryPermit }) }, nil
	default:
		return nil, ErrPoolRuntimeBusy
	}
}

// NewHostedRecoveryPoolRuntime borrows independently initialized inventories;
// their parent owner outlives this runtime and all children.
func NewHostedRecoveryPoolRuntime(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, vault *recoveryvault.HostedVault, zeroOwner, incrementingOwner string, allocation *HostedAllocationSetup) (*PoolRuntime, error) {
	if allocation == nil || allocation.accounts != accounts {
		return nil, ErrPoolRuntime
	}
	r, e := NewHostedPoolRuntime(ctx, config, accounts, options, vault, zeroOwner, incrementingOwner)
	if e != nil {
		return nil, e
	}
	r.hosted.allocation = allocation
	return r, nil
}
