// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

func (s *HostedAllocationSetup) walletGuards(fixture, account, owner, source string) (waddrmgr.HostedAllocationGuard, *waddrmgr.HostedAllocationGuard, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.Pin(fixture)
	if e != nil || s.closed {
		return waddrmgr.HostedAllocationGuard{}, nil, false, ErrPoolRuntime
	}
	f, e := gymaccount.FixtureIdentity(fixture)
	if e != nil || !validAccountID(account) || !validAccountID(owner) || !validAccountID(source) {
		return waddrmgr.HostedAllocationGuard{}, nil, false, ErrPoolRuntime
	}
	g := waddrmgr.HostedAllocationGuard{Version: 1, State: "locked", InventoryID: p.InventoryID, AuthorityID: s.accounts.AuthorityID(), AccountID: account, WalletOwnerID: owner, SourceBindingID: source, InitialManifestSHA256: allocationJSONHash(hostedInitialManifest(f.RootPublicKey, f.GymPublicKey, f.GymID))}
	i, found := s.intents[fixture]
	established := s.ready != nil || s.established[fixture]
	if !found {
		if established {
			return g, nil, false, ErrManifestInitialization
		}
		return g, nil, true, nil
	}
	b := i.Binding
	if b.AccountID != account || b.WalletOwnerID != owner || b.SourceBindingID != source || b.InitialManifest.RootPublicKey != f.RootPublicKey {
		return g, nil, false, ErrManifestInitialization
	}
	bound := g
	bound.State = "bound"
	bound.BindingID, e = wallet.AllocationBindingID(b)
	if e != nil {
		return g, nil, false, e
	}
	return g, &bound, !established, nil
}

// Both expected guards are derived from retained initialization metadata. A
// failed exact-bound open may try only the exact locked predecessor while setup
// is unfinished, and only after the loader has joined a typed guard conflict.
func openHostedAllocationWallet(ctx context.Context, dir, fixture string, create bool, locked waddrmgr.HostedAllocationGuard, bound *waddrmgr.HostedAllocationGuard, allowLocked bool) (*walletRuntime, error) {
	f, e := gymaccount.FixtureIdentity(fixture)
	if e != nil || wire.TestPoW || ctx == nil {
		return nil, ErrPoolRuntime
	}
	if e = prepareWalletDirectory(dir, create); e != nil {
		return nil, e
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 0)
	var w *wallet.Wallet
	guard := locked
	if create {
		if bound != nil || !allowLocked {
			return nil, ErrManifestInitialization
		}
		var entropy [32]byte
		if fixture == "incrementing256" {
			for i := range entropy {
				entropy[i] = byte(i)
			}
		}
		w, e = loader.CreateHostedAllocationWallet(ctx, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), entropy, chaincfg.MainNetParams.GenesisBlock.Header.Timestamp, guard)
		clear(entropy[:])
	} else {
		if bound != nil {
			guard = *bound
		} else if !allowLocked {
			return nil, ErrManifestInitialization
		}
		w, e = loader.OpenHostedAllocationWallet(ctx, []byte(fixtureWalletPublicPass), guard)
		var cleanup *wallet.HostedAllocationCleanupError
		if e != nil && bound != nil && allowLocked && !errors.As(e, &cleanup) && errors.Is(e, waddrmgr.ErrHostedAllocationConflict) {
			guard = locked
			w, e = loader.OpenHostedAllocationWallet(ctx, []byte(fixtureWalletPublicPass), guard)
		}
	}
	if e != nil {
		var cleanup *wallet.HostedAllocationCleanupError
		if errors.As(e, &cleanup) {
			return &walletRuntime{loader: loader, closed: true, closeErr: cleanup}, e
		}
		return nil, e
	}
	r := &walletRuntime{loader: loader, wallet: w, allocationGuard: &guard}
	g, e := w.HostedAllocationIdentity(ctx, guard)
	if e == nil && (g.Version != 1 || g.Profile != f.Profile || g.Network != f.Network || g.Genesis != f.Genesis || g.PublicKey != f.GymPublicKey || g.GymID != f.GymID) {
		e = ErrManifestInitialization
	}
	if e != nil {
		closeErr := r.Close()
		return r, errors.Join(e, closeErr)
	}
	r.identity = walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
	return r, nil
}

type allocatedRecipientWallet struct {
	wallet      *walletRuntime
	allocator   *wallet.HostedAllocator
	operationID string
}

func (w *allocatedRecipientWallet) Identity() walletIdentity { return w.wallet.Identity() }
func (w *allocatedRecipientWallet) Snapshot(ctx context.Context) (walletView, error) {
	return w.wallet.Snapshot(ctx)
}
func (w *allocatedRecipientWallet) EnsurePoolRecipient(ctx context.Context, expected *walletPoolRecipient) (walletPoolRecipient, error) {
	want, e := expectedPoolRecipient(w.wallet.identity)
	if e != nil || expected != nil && *expected != want {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	ack, e := w.allocator.Resolve(ctx, w.operationID)
	if errors.Is(e, wallet.ErrAllocationNotFound) {
		if expected != nil {
			return walletPoolRecipient{}, ErrPoolRecipient
		}
		plan, e := w.allocator.PlanRecipient(ctx, w.operationID)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		reservation, e := w.allocator.Reserve(ctx, plan)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		receipt, e := w.allocator.Consume(ctx, reservation)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		ack, e = w.allocator.Acknowledge(ctx, receipt)
		if e != nil {
			return walletPoolRecipient{}, e
		}
	} else if errors.Is(e, wallet.ErrAllocationResumeRequired) {
		reservation, e := w.allocator.Resume(ctx, w.operationID)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		receipt, e := w.allocator.Consume(ctx, reservation)
		if e != nil {
			return walletPoolRecipient{}, e
		}
		ack, e = w.allocator.Acknowledge(ctx, receipt)
		if e != nil {
			return walletPoolRecipient{}, e
		}
	} else if e != nil {
		return walletPoolRecipient{}, e
	}
	final, e := w.allocator.Finalize(ctx, ack)
	if e != nil {
		return walletPoolRecipient{}, e
	}
	a, e := final.Details()
	if e != nil {
		return walletPoolRecipient{}, e
	}
	if a.Address != want.Payout.Address || a.Script != want.Script || a.Purpose != 84 || a.Coin != 0 || a.Account != 0 || a.Branch != 0 || a.Index != 0 {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	return want, nil
}
