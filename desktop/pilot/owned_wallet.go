// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

type OwnedWalletSession struct {
	setup       *OwnedWalletSetup
	entry       *ownedWalletEntry
	runtime     *walletRuntime
	login       gymaccount.Session
	epoch       recoveryvault.EpochFence
	intent      ownedWalletIntent
	root        string
	carrier     *wallet.FighterCarrierOwner
	withdrawals *wallet.AllocatedWithdrawals2
	recipient   PoolRecipientBinding
	ready       atomic.Bool
	opMu        sync.Mutex
	closed      atomic.Bool
	closeOnce   sync.Once
	closeErr    error
}

func ownedWalletGuard(i ownedWalletIntent, bound bool) waddrmgr.HostedAllocationGuard2 {
	b := i.Binding
	g := waddrmgr.HostedAllocationGuard2{Version: 2, State: "locked", InventoryID: b.InventoryID, AuthorityID: b.AuthorityID, AccountID: b.AccountID, WalletOwnerID: b.WalletOwnerID, SourceBindingID: b.SourceBindingID, OwnershipPinID: b.OwnershipPinID, CarrierJournalID: b.CarrierJournalID, WithdrawalJournalID: b.WithdrawalJournalID, InitialManifestSHA256: allocationJSONHash(b.InitialManifest)}
	if bound {
		g.State = "bound"
		g.BindingID = i.BindingID
	}
	return g
}
func openOwnedWallet(ctx context.Context, root string, i ownedWalletIntent, ready bool) (*walletRuntime, error) {
	dir := filepath.Join(root, "wallet")
	exists, e := poolRuntimeExists(filepath.Join(dir, wallet.WalletDBName))
	if e != nil {
		return nil, e
	}
	if ready && !exists {
		return nil, ErrManifestInitialization
	}
	if e = prepareWalletDirectory(dir, !exists); e != nil {
		return nil, e
	}
	l := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 0)
	guard := ownedWalletGuard(i, false)
	var w *wallet.Wallet
	if !exists {
		var entropy [32]byte
		if i.Pin.Fixture == "incrementing256" {
			for n := range entropy {
				entropy[n] = byte(n)
			}
		}
		w, e = l.CreateHostedTransferWallet(ctx, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), entropy, chaincfg.MainNetParams.GenesisBlock.Header.Timestamp, guard)
		clear(entropy[:])
	} else {
		guard = ownedWalletGuard(i, true)
		w, e = l.OpenHostedTransferWallet(ctx, []byte(fixtureWalletPublicPass), guard)
		var cleanup *wallet.HostedAllocationCleanupError
		if e != nil && !ready && !errors.As(e, &cleanup) && errors.Is(e, waddrmgr.ErrHostedAllocationConflict) {
			guard = ownedWalletGuard(i, false)
			w, e = l.OpenHostedTransferWallet(ctx, []byte(fixtureWalletPublicPass), guard)
		}
	}
	if e != nil {
		var cleanup *wallet.HostedAllocationCleanupError
		if errors.As(e, &cleanup) {
			return &walletRuntime{loader: l, closed: true, closeErr: cleanup}, e
		}
		return nil, e
	}
	r := &walletRuntime{loader: l, wallet: w, allocationGuard2: &guard}
	got, e := w.HostedAllocationIdentity2(ctx, guard)
	f, fe := gymaccount.FixtureIdentity(i.Pin.Fixture)
	if e == nil && (fe != nil || got.Profile != f.Profile || got.Network != f.Network || got.Genesis != f.Genesis || got.PublicKey != f.GymPublicKey || got.GymID != f.GymID) {
		e = ErrManifestInitialization
	}
	if e != nil {
		return r, errors.Join(e, r.Close())
	}
	r.identity = walletIdentity{f.RootPublicKey, f.GymPublicKey, f.GymID}
	return r, nil
}
func (s *OwnedWalletSetup) OpenFixture(ctx context.Context, fixture string, login gymaccount.Session, epoch recoveryvault.EpochFence, current recoveryvault.CurrentBinding) (out *OwnedWalletSession, resultErr error) {
	if ctx == nil {
		return nil, ErrPoolRuntime
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	s.mu.Lock()
	entry := s.entries[fixture]
	if s.closing || s.closeErr != nil || s.ctx.Err() != nil || entry == nil || entry.opening || entry.session != nil {
		s.mu.Unlock()
		return nil, ErrPoolRuntimeBusy
	}
	entry.opening = true
	s.active.Add(1)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); entry.opening = false; s.mu.Unlock(); s.active.Done() }()
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()
	expected, e := s.expectedIntent(work, entry, login, epoch, current)
	if e != nil {
		return nil, e
	}
	root := filepath.Join(entry.config.ChildRoot, "owned-wallet")
	intent, ready, e := ownedWalletPrepareControls(root, expected, entry.config.RequireExisting)
	if e != nil {
		return nil, e
	}
	session := &OwnedWalletSession{setup: s, entry: entry, login: login, epoch: epoch, intent: intent, root: root}
	session.ready.Store(ready != nil)
	s.mu.Lock()
	entry.intent = &intent
	entry.ready = ready
	entry.session = session
	s.mu.Unlock()
	defer func() {
		if resultErr != nil {
			ce := session.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				out = session
			}
		}
	}()
	if e = s.openStores(work, entry, intent, ready != nil, root); e != nil {
		return nil, e
	}
	session.runtime, e = openOwnedWallet(work, root, intent, ready != nil)
	if e != nil {
		return nil, e
	}
	if session.runtime.allocationGuard2.State == "locked" {
		if e = entry.inventory.BindWallet(work, session.runtime.wallet, login.Token, wallet.HostedAllocationEpochFence(epoch)); e != nil {
			return nil, e
		}
		bound := ownedWalletGuard(intent, true)
		session.runtime.allocationGuard2 = &bound
	}
	session.runtime.allocation2, e = entry.inventory.Attach(work, session.runtime.wallet, login.Token, wallet.HostedAllocationEpochFence(epoch))
	if e != nil {
		return nil, e
	}
	a := session.runtime.allocation2
	session.carrier, e = a.OpenFighterCarrier(work, entry.carrier)
	if e != nil && !session.ready.Load() && errors.Is(e, fightercarrier.ErrCorrupt) {
		// Create audits exact unbound wallet state and an empty concrete journal;
		// it cannot repair or overwrite a malformed/already-bound namespace.
		session.carrier, e = a.CreateFighterCarrier(work, entry.carrier)
	}
	if e != nil {
		return nil, e
	}
	session.withdrawals, e = a.OpenWithdrawals(work, intent.WithdrawalBindingID, entry.withdrawal)
	if errors.Is(e, wallet.ErrWithdrawalNotFound) && !session.ready.Load() {
		session.withdrawals, e = a.CreateWithdrawals(work, intent.WithdrawalBindingID, entry.withdrawal)
	}
	if e != nil {
		return nil, e
	}
	if !session.ready.Load() {
		empty, e := session.withdrawals.Empty(work)
		if e != nil || !empty {
			return nil, ErrManifestInitialization
		}
	}
	if e = session.check(work); e != nil {
		return nil, e
	}
	return session, nil
}
func (s *OwnedWalletSetup) openStores(ctx context.Context, e *ownedWalletEntry, i ownedWalletIntent, ready bool, root string) error {
	var err error
	if e.inventory == nil {
		// A complete inventory marker is an existence boundary, never a source
		// of identity. Open always receives the independently reconstructed pin.
		// Resume is only the bounded empty-initialization path; recipient rows
		// may already exist before the outer wallet READY publication.
		marker, lookupErr := poolRuntimeExists(filepath.Join(root, "inventory", "SLUG-HOSTED-TRANSFER-ALLOCATION.json"))
		if lookupErr != nil {
			return lookupErr
		}
		if ready || marker {
			e.inventory, err = wallet.OpenHostedAllocationInventory2(ctx, filepath.Join(root, "inventory"), s.accounts, i.Binding)
		} else {
			e.inventory, err = wallet.ResumeHostedAllocationInventory2(ctx, filepath.Join(root, "inventory"), s.accounts, i.Binding)
		}
		if err != nil {
			return err
		}
	}
	if e.carrier == nil {
		if ready {
			e.carrier, err = fightercarrier.Open(s.ctx, filepath.Join(root, "carrier"), i.Carrier)
		} else {
			e.carrier, err = fightercarrier.Create(s.ctx, filepath.Join(root, "carrier"), i.Carrier)
		}
		if err != nil {
			return err
		}
	}
	if e.withdrawal == nil {
		if ready {
			e.withdrawal, err = walletwithdraw.Open(s.ctx, filepath.Join(root, "withdrawal"), i.Withdrawal)
		} else {
			e.withdrawal, err = walletwithdraw.Create(s.ctx, filepath.Join(root, "withdrawal"), i.Withdrawal)
		}
		if err != nil {
			return err
		}
	}
	if e.inventory.Binding().InventoryID != i.Binding.InventoryID || e.carrier.Identity() != i.Carrier || e.withdrawal.Identity() != i.Withdrawal {
		return ErrManifestInitialization
	}
	if !ready {
		co, err := e.carrier.TryAcquire(ctx)
		if err != nil {
			return err
		}
		cs, err := co.Snapshot()
		ce := co.Close()
		if err != nil || ce != nil || cs.Count != 0 {
			return errors.Join(ErrManifestInitialization, err, ce)
		}
		wo, err := e.withdrawal.TryAcquire(ctx)
		if err != nil {
			return err
		}
		ws, err := wo.Snapshot()
		ce = wo.Close()
		if err != nil || ce != nil || ws.Count != 0 {
			return errors.Join(ErrManifestInitialization, err, ce)
		}
	}
	return ctx.Err()
}
func (s *OwnedWalletSession) check(ctx context.Context) error {
	if s == nil || ctx == nil || s.closed.Load() || s.setup.ctx.Err() != nil {
		return ErrPoolRuntime
	}
	return s.setup.accounts.WithSession(ctx, s.login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.login.Account.ID || a.Generation != s.login.Account.Generation || a.PrincipalID != s.intent.Binding.PrincipalID {
			return gymaccount.ErrUnauthorized
		}
		return s.epoch(ctx, func(parent func() error) error {
			if parent == nil {
				return gymaccount.ErrUnauthorized
			}
			return errors.Join(check(), parent())
		})
	})
}
func (s *OwnedWalletSession) Allocator() *wallet.HostedAllocator2 {
	if s == nil || s.closed.Load() || !s.ready.Load() || s.runtime == nil {
		return nil
	}
	return s.runtime.allocation2
}
func (s *OwnedWalletSession) Carrier() *wallet.FighterCarrierOwner {
	if s == nil || s.closed.Load() || !s.ready.Load() {
		return nil
	}
	return s.carrier
}
func (s *OwnedWalletSession) CarrierJournal() *fightercarrier.Journal {
	if s == nil || s.closed.Load() || !s.ready.Load() {
		return nil
	}
	return s.entry.carrier
}
func (s *OwnedWalletSession) Withdrawals() *wallet.AllocatedWithdrawals2 {
	if s == nil || s.closed.Load() || !s.ready.Load() {
		return nil
	}
	return s.withdrawals
}
func (s *OwnedWalletSession) WithdrawalJournal() *walletwithdraw.Journal {
	if s == nil || s.closed.Load() || !s.ready.Load() {
		return nil
	}
	return s.entry.withdrawal
}
func (s *OwnedWalletSession) RecoveryHead(ctx context.Context) (*wallet.HostedRecoveryHead2, error) {
	if s == nil || !s.ready.Load() {
		return nil, ErrManifestInitialization
	}
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	return s.entry.inventory.CaptureRecoveryHead(ctx, s.login.Token, wallet.HostedAllocationEpochFence(s.epoch))
}
func (s *OwnedWalletSession) InitializeRecipient(ctx context.Context) (PoolRecipientBinding, error) {
	if s == nil || !s.opMu.TryLock() {
		return PoolRecipientBinding{}, ErrPoolRuntimeBusy
	}
	defer s.opMu.Unlock()
	if e := s.check(ctx); e != nil {
		return PoolRecipientBinding{}, e
	}
	f, e := gymaccount.FixtureIdentity(s.intent.Pin.Fixture)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	mining, e := s.setup.accounts.MintMining(ctx, s.login.Token)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	var nonce [32]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return PoolRecipientBinding{}, e
	}
	lease := func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return s.setup.accounts.WithSession(ctx, s.login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
			if a.ID != s.login.Account.ID || a.Generation != s.login.Account.Generation {
				return gymaccount.ErrUnauthorized
			}
			return s.epoch(ctx, func(parent func() error) error {
				if parent == nil {
					return gymaccount.ErrUnauthorized
				}
				guard := func() error { return errors.Join(check(), parent()) }
				if e := guard(); e != nil {
					return e
				}
				if e := fn(a, guard); e != nil {
					return e
				}
				return guard()
			})
		})
	}
	view := ManifestAccountConfig{Account: AccountConfig{Fixture: s.intent.Pin.Fixture, AccountID: s.login.Account.ID, AuthorityID: s.setup.accounts.AuthorityID(), RequireExisting: s.ready.Load(), Authorizer: poolRuntimeSessions{s.setup.accounts}, MiningToken: func(context.Context) (string, error) { return mining.Token, nil }}, ViewID: hex.EncodeToString(nonce[:]), Generation: s.login.Account.Generation, ViewLease: lease}
	cfg := PoolRecipientConfig{Version: 1, AccountAuthorityID: s.setup.accounts.AuthorityID(), AccountID: s.login.Account.ID, Fixture: s.intent.Pin.Fixture, Identity: f, Source: s.intent.Source}
	b, e := ensurePoolRecipientBinding(ctx, s.root, cfg, view, &ownedRecipientWallet{s}, !s.ready.Load())
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	if e = s.check(ctx); e != nil {
		return PoolRecipientBinding{}, e
	}
	s.recipient = b
	return b, nil
}
func (s *OwnedWalletSession) PublishReady(ctx context.Context) error {
	if s == nil || !s.opMu.TryLock() {
		return ErrPoolRuntimeBusy
	}
	defer s.opMu.Unlock()
	if e := s.check(ctx); e != nil {
		return e
	}
	if ValidatePoolRecipientBinding(s.recipient) != nil || s.carrier == nil || s.withdrawals == nil {
		return ErrManifestInitialization
	}
	ready := ownedWalletReady{1, allocationJSONHash(s.intent), s.intent.BindingID, s.intent.CarrierBindingID, s.intent.WithdrawalBindingID, s.recipient}
	if e := ownedWalletInventory(s.root); e != nil {
		return e
	}
	if e := writePoolRuntimeRecord(filepath.Join(s.root, ownedWalletReadyName), ready); e != nil {
		return e
	}
	if e := s.check(ctx); e != nil {
		return errors.Join(wallet.ErrAllocationOutcomeUnknown, e)
	}
	s.ready.Store(true)
	s.setup.mu.Lock()
	s.entry.ready = &ready
	s.setup.mu.Unlock()
	return nil
}
func (s *OwnedWalletSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.opMu.Lock()
		defer s.opMu.Unlock()
		if s.carrier != nil {
			s.closeErr = errors.Join(s.closeErr, s.carrier.Close())
		}
		if s.runtime != nil {
			s.closeErr = errors.Join(s.closeErr, s.runtime.Close())
		}
		s.setup.mu.Lock()
		if s.closeErr == nil && s.entry.session == s {
			s.entry.session = nil
		} else if s.closeErr != nil {
			s.setup.closeErr = errors.Join(s.setup.closeErr, s.closeErr)
		}
		s.setup.mu.Unlock()
	})
	return s.closeErr
}
func (s *OwnedWalletSetup) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closing {
		done := s.closeDone
		s.mu.Unlock()
		<-done
		return s.closeErr
	}
	s.closing = true
	s.cancel()
	s.mu.Unlock()
	s.active.Wait()
	var err error
	for _, fixture := range []string{"zero256", "incrementing256"} {
		e := s.entries[fixture]
		if e.session != nil {
			err = errors.Join(err, e.session.Close())
		}
	}
	// A failed session close keeps its exact database and supporting stores owned.
	if err == nil {
		for _, fixture := range []string{"zero256", "incrementing256"} {
			e := s.entries[fixture]
			if e.carrier != nil {
				err = errors.Join(err, e.carrier.Close())
			}
			if e.withdrawal != nil {
				err = errors.Join(err, e.withdrawal.Close())
			}
			if e.inventory != nil {
				err = errors.Join(err, e.inventory.Close())
			}
		}
	}
	s.mu.Lock()
	s.closeErr = errors.Join(s.closeErr, err)
	close(s.closeDone)
	s.mu.Unlock()
	return s.closeErr
}

type ownedRecipientWallet struct{ s *OwnedWalletSession }

func (w *ownedRecipientWallet) Identity() walletIdentity { return w.s.runtime.Identity() }
func (w *ownedRecipientWallet) Snapshot(ctx context.Context) (walletView, error) {
	return w.s.runtime.Snapshot(ctx)
}
func (w *ownedRecipientWallet) EnsurePoolRecipient(ctx context.Context, expected *walletPoolRecipient) (walletPoolRecipient, error) {
	want, e := expectedPoolRecipient(w.s.runtime.identity)
	if e != nil || expected != nil && *expected != want {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	a := w.s.runtime.allocation2
	id := w.s.intent.PoolAllocationID
	ack, e := a.Resolve(ctx, id)
	if errors.Is(e, wallet.ErrAllocationNotFound) {
		if expected != nil {
			return walletPoolRecipient{}, ErrPoolRecipient
		}
		p, e := a.PlanRecipient(ctx, id)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		r, e := a.Reserve(ctx, p)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		receipt, e := a.Consume(ctx, r)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		ack, e = a.Acknowledge(ctx, receipt)
		if e != nil {
			return walletPoolRecipient{}, e
		}
	} else if errors.Is(e, wallet.ErrAllocationResumeRequired) {
		r, e := a.Resume(ctx, id)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		receipt, e := a.Consume(ctx, r)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		ack, e = a.Acknowledge(ctx, receipt)
		if e != nil {
			return walletPoolRecipient{}, e
		}
	} else if e != nil {
		return walletPoolRecipient{}, e
	}
	final, e := a.Finalize(ctx, ack)
	if e != nil {
		return walletPoolRecipient{}, e
	}
	address, e := final.Details()
	if e != nil {
		return walletPoolRecipient{}, e
	}
	if address.Address != want.Payout.Address || address.Script != want.Script || address.Purpose != 84 || address.Coin != 0 || address.Account != 0 || address.Branch != 0 || address.Index != 0 {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	return want, nil
}
