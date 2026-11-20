// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Selected before the shared runtime reads a child directory. These identities
// are outer INIT inputs, never configuration discovered from a wallet marker.
type ownedGymRuntimeResources struct {
	pins    [2]OwnedWalletPin
	league  *OwnedLeague
	failure error
}
type ownedGymChildResources struct {
	Config        Config
	WorkerConfig  prospectworker.PoolConfig
	Recipient     PoolRecipientBinding
	Node          *prospectnode.Client
	RPC           prospectnode.RPCConfig
	Life          context.Context
	Release       func()
	RegisterClose func(func() error) error
}

func NewOwnedGymRuntime(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, expected OwnedSourceConfig, pins []OwnedWalletPin) (*PoolRuntime, error) {
	if len(pins) != 2 {
		return nil, ErrPoolRuntime
	}
	g := &ownedGymRuntimeResources{}
	ids := make(map[string]bool, 8)
	for i, p := range pins {
		fixture := "zero256"
		if i == 1 {
			fixture = "incrementing256"
		}
		if p.Fixture != fixture {
			return nil, ErrPoolRuntime
		}
		for _, id := range []string{p.WalletOwnerID, p.InventoryID, p.CarrierJournalID, p.WithdrawalJournalID} {
			if !payoutNonzeroID(id) || ids[id] {
				return nil, ErrPoolRuntime
			}
			ids[id] = true
		}
		g.pins[i] = p
	}
	return newPoolRuntimeSelected(ctx, config, accounts, options, &expected, g)
}

func (r *PoolRuntime) loadOwnedGymRecipients() error {
	r.gymAccounts = make(map[string]string, 2)
	dir := filepath.Join(r.config.DataDir, "gyms")
	if e := plainChild(dir, true); e != nil {
		return e
	}
	if e := os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) > 2 {
		return ErrPoolRuntime
	}
	for _, de := range entries {
		if !de.IsDir() || de.Type()&os.ModeSymlink != 0 {
			return ErrPoolRuntime
		}
		index := -1
		for i, p := range r.ownedGym.pins {
			if p.Fixture == de.Name() {
				index = i
			}
		}
		if index < 0 {
			return ErrPoolRuntime
		}
		child := filepath.Join(dir, de.Name())
		if e = allocationDirectory(child, map[string]bool{"owned-wallet": true, "SLUG-OWNED-WORKER-INTENT.json": true, "SLUG-OWNED-WORKER-READY.json": true, "owned-outbox": true}); e != nil {
			return e
		}
		root := filepath.Join(child, "owned-wallet")
		has, e := poolRuntimeExists(root)
		if e != nil {
			return e
		}
		if !has {
			continue
		}
		if e = ownedWalletInventory(root); e != nil {
			return e
		}
		intentExists, e := poolRuntimeExists(filepath.Join(root, ownedWalletIntentName))
		if e != nil {
			return e
		}
		if !intentExists {
			entries, e := os.ReadDir(root)
			if e != nil || len(entries) != 0 {
				return ErrManifestInitialization
			}
			continue
		}
		var intent ownedWalletIntent
		if readManifestJSON(filepath.Join(root, ownedWalletIntentName), ownedWalletControlLimit, &intent) != nil {
			return ErrManifestInitialization
		}
		pin := r.ownedGym.pins[index]
		sourceID, e := poolservice.BindingID(r.source)
		if e != nil {
			return e
		}
		if intent.Version != 1 || intent.Pin != pin || intent.Source != r.source || intent.SourceBindingID != sourceID || intent.Ownership != r.owned.config.Ownership || intent.Binding.AuthorityID != r.accounts.AuthorityID() || !payoutNonzeroID(intent.Binding.AccountID) {
			return ErrManifestInitialization
		}
		r.gymAccounts[pin.Fixture] = intent.Binding.AccountID
		has, e = poolRuntimeExists(filepath.Join(root, ownedWalletReadyName))
		if e != nil {
			return e
		}
		if !has {
			continue
		}
		var ready ownedWalletReady
		if readManifestJSON(filepath.Join(root, ownedWalletReadyName), ownedWalletControlLimit, &ready) != nil || ready.Version != 1 || ready.IntentSHA256 != allocationJSONHash(intent) || ready.BindingID != intent.BindingID || ready.CarrierBindingID != intent.CarrierBindingID || ready.WithdrawalBindingID != intent.WithdrawalBindingID {
			return ErrManifestInitialization
		}
		b := ready.Recipient
		if ValidatePoolRecipientBinding(b) != nil || b.Config.Source != r.source || b.Config.Fixture != pin.Fixture || b.Config.AccountID != intent.Binding.AccountID || b.Config.AccountAuthorityID != r.accounts.AuthorityID() {
			return ErrManifestInitialization
		}
		// READY is an established/open-only boundary for all paired stores.
		for _, path := range []string{filepath.Join("wallet", wallet.WalletDBName), "inventory", "carrier", "withdrawal", poolRecipientIntentName, poolRecipientReadyName} {
			found, e := poolRuntimeExists(filepath.Join(root, path))
			if e != nil || !found {
				return ErrManifestInitialization
			}
		}
		if readPoolRecipientMarker(filepath.Join(root, poolRecipientIntentName), recipientMarker(b, "initializing")) != nil || readPoolRecipientMarker(filepath.Join(root, poolRecipientReadyName), recipientMarker(b, "ready")) != nil {
			return ErrManifestInitialization
		}
		r.recipients[pin.Fixture] = b
	}
	return nil
}

// PrepareOwnedWallet borrows a concrete session, never closes it on success.
// Network/wallet work completes before the short original-session publication.
func (r *PoolRuntime) PrepareOwnedWallet(ctx context.Context, s *OwnedWalletSession) (PoolRecipientBinding, error) {
	if s == nil || s.setup == nil || s.runtime == nil || s.setup.accounts != r.accounts {
		return PoolRecipientBinding{}, ErrPoolRuntime
	}
	work, end, e := r.begin(ctx)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	defer end()
	r.mu.Lock()
	ok := r.ownedGym != nil && r.ownedGym.failure == nil && r.owned != nil && s.intent.Ownership == r.owned.config.Ownership && s.intent.Source == r.source
	pinOK := false
	if r.ownedGym != nil {
		for _, p := range r.ownedGym.pins {
			if p == s.intent.Pin {
				pinOK = true
			}
		}
	}
	for b := range r.borrows {
		if b.account == s.login.Account.ID {
			ok = false
		}
	}
	r.mu.Unlock()
	if !ok || !pinOK {
		return PoolRecipientBinding{}, ErrPoolRuntime
	}
	if e = s.check(work); e != nil {
		return PoolRecipientBinding{}, e
	}
	if e = s.runtime.Attach(work, r.nodes.rpcConfig()); e != nil {
		return PoolRecipientBinding{}, e
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := s.runtime.ownedStartupSynced(work)
		if err != nil {
			return PoolRecipientBinding{}, err
		}
		if ready {
			break
		}
		select {
		case <-work.Done():
			return PoolRecipientBinding{}, work.Err()
		case <-ticker.C:
		}
	}
	b, e := s.InitializeRecipient(work)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	if e = s.PublishReady(work); e != nil {
		return PoolRecipientBinding{}, e
	}
	e = r.accounts.WithSession(work, s.login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.login.Account.ID || a.Generation != s.login.Account.Generation {
			return gymaccount.ErrUnauthorized
		}
		return s.epoch(work, func(parent func() error) error {
			if check == nil || parent == nil {
				return gymaccount.ErrUnauthorized
			}
			if e := errors.Join(check(), parent()); e != nil {
				return e
			}
			if !r.mu.TryLock() {
				return ErrPoolRuntimeBusy
			}
			defer r.mu.Unlock()
			if r.closed || r.life.Err() != nil || r.ownedGym.failure != nil {
				return ErrPoolRuntime
			}
			if prior, found := r.recipients[s.intent.Pin.Fixture]; found && prior != b {
				return ErrManifestInitialization
			}
			r.recipients[s.intent.Pin.Fixture] = b
			r.gymAccounts[s.intent.Pin.Fixture] = a.ID
			r.verified[s.intent.Pin.Fixture] = true
			return errors.Join(check(), parent())
		})
	})
	return b, e
}
func (r *PoolRuntime) prepareOwnedLeague(ctx context.Context) error {
	if r.ownedGym == nil || r.owned == nil {
		return ErrPoolRuntime
	}
	expected, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.accounts.AuthorityID(), r.gymAccounts["zero256"], r.gymAccounts["incrementing256"])
	if e != nil {
		return e
	}
	if r.ownedGym.league != nil {
		if !reflect.DeepEqual(r.ownedGym.league.Config(), expected) {
			return ErrPoolRuntime
		}
		return nil
	}
	league, e := openOwnedLeague(ctx, r, expected)
	if league != nil {
		r.ownedGym.league = league
	}
	if e != nil && league != nil {
		r.ownedGym.failure = e
	}
	return e
}
func (r *PoolRuntime) OwnedLeague() (*OwnedLeague, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.ready || r.ownedGym == nil || r.ownedGym.failure != nil || r.ownedGym.league == nil {
		return nil, ErrPoolRuntime
	}
	return r.ownedGym.league, nil
}
func (r *PoolRuntime) ownedGymChildResources(s *OwnedWalletSession) (ownedGymChildResources, error) {
	if s == nil || s.setup == nil || s.setup.accounts != r.accounts || !s.ready.Load() || s.closed.Load() {
		return ownedGymChildResources{}, ErrPoolRuntime
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.ready || r.life.Err() != nil || r.ownedGym == nil || r.ownedGym.failure != nil || r.ownedGym.league == nil || !r.verified[s.intent.Pin.Fixture] || s.intent.Ownership != r.owned.config.Ownership || s.intent.Source != r.source {
		return ownedGymChildResources{}, ErrPoolRuntime
	}
	b, ok := r.recipients[s.intent.Pin.Fixture]
	if !ok || b != s.recipient || b.Config.AccountID != s.login.Account.ID {
		return ownedGymChildResources{}, ErrPoolRuntime
	}
	for old := range r.borrows {
		if old.account == s.login.Account.ID {
			return ownedGymChildResources{}, ErrPoolRuntimeBusy
		}
	}
	poolURL, admissionURL := poolRuntimeOrigin(r.marker.PoolPort), poolRuntimeOrigin(r.marker.AdmissionPort)
	pt, e := poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: poolURL, Token: fixtureToken, PublicKey: b.Config.Identity.GymPublicKey, Source: r.source})
	if e != nil {
		return ownedGymChildResources{}, e
	}
	at, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: admissionURL, Token: fixtureToken, AuthorityPin: r.source.AuthorityPin})
	if e != nil {
		return ownedGymChildResources{}, e
	}
	worker := prospectworker.PoolConfig{Version: 1, Source: r.source, PublicKey: b.Config.Identity.GymPublicKey, AuthorityOrigin: poolURL, TransportTrustID: pt, AdmissionOrigin: admissionURL, AdmissionTrustID: at, MaxRecords: 1000, MaxBytes: 256 << 20}
	if _, e = prospectworker.ValidatePoolConfig(worker); e != nil {
		return ownedGymChildResources{}, e
	}
	borrow := &poolRuntimeBorrow{account: s.login.Account.ID, done: make(chan struct{})}
	r.borrows[borrow] = true
	var once sync.Once
	release := func() {
		once.Do(func() { r.mu.Lock(); defer r.mu.Unlock(); delete(r.borrows, borrow); close(borrow.done) })
	}
	register := func(fn func() error) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed || !r.borrows[borrow] || borrow.close != nil || fn == nil {
			return ErrPoolRuntime
		}
		borrow.close = fn
		return nil
	}
	return ownedGymChildResources{Config: Config{DataDir: filepath.Join(r.config.DataDir, "gyms", s.intent.Pin.Fixture), NodeBinary: r.config.NodeBinary}, WorkerConfig: worker, Recipient: b, Node: r.node, RPC: r.nodes.rpcConfig(), Life: r.life, Release: release, RegisterClose: register}, nil
}

func (s *OwnedWalletSetup) CarrierIdentity(fixture string) (fightercarrier.Identity, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[fixture]
	if s.closing || s.closeErr != nil || s.ctx.Err() != nil || e == nil || e.intent == nil || e.ready == nil {
		return fightercarrier.Identity{}, "", ErrPoolRuntime
	}
	return e.intent.Carrier, e.intent.CarrierBindingID, nil
}
