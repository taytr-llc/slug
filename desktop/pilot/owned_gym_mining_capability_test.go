// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospect"
)

// This adapter uses the real account store's separate Mining capability. It
// does not substitute a permissive callback for the original credential.
type ownedRealMiningAuthority struct {
	store     *gymaccount.Store
	accountID string
	calls     atomic.Uint32
	entered   atomic.Uint32
}

func (a *ownedRealMiningAuthority) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	a.calls.Add(1)
	return a.store.WithSession(ctx, token, gymaccount.Mining, func(account gymaccount.Account, check func() error) error {
		a.entered.Add(1)
		if account.ID != a.accountID || account.Enrollment == nil {
			return gymaccount.ErrForbidden
		}
		return fn(account.Enrollment.GymPublicKey, check)
	})
}

func TestOwnedGymChildRealLoginAndMiningCapabilities(t *testing.T) {
	h := newOwnedWalletTestFixture(t) // Real schema2 account enrollment and fixture vault.
	ctx := context.Background()
	login := h.sessions["zero256"]
	var account gymaccount.Account
	if e := h.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		account = a
		return check()
	}); e != nil || account.Enrollment == nil {
		t.Fatal("original enrolled login View", e)
	}
	entered := false
	if e := h.accounts.WithSession(ctx, login.Token, gymaccount.Mining, func(gymaccount.Account, func() error) error {
		entered = true
		return nil
	}); !errors.Is(e, gymaccount.ErrForbidden) || entered {
		t.Fatal("login token acquired Mining", e, entered)
	}
	mining, e := h.accounts.MintMining(ctx, login.Token)
	if e != nil {
		t.Fatal(e)
	}
	if e = h.accounts.WithSession(ctx, mining.Token, gymaccount.Mining, func(a gymaccount.Account, check func() error) error {
		if a.ID != account.ID {
			t.Fatal("mining credential changed account")
		}
		return check()
	}); e != nil {
		t.Fatal("separately minted Mining credential", e)
	}

	c, _ := ownedChildFenceFixture(t)
	c.view.AccountAuthorityID, c.view.AccountID, c.view.Generation = h.accounts.AuthorityID(), account.ID, account.Generation
	var leases atomic.Uint32
	workerFenceDone := make(chan struct{})
	c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		n := leases.Add(1)
		e := h.accounts.WithSession(ctx, login.Token, cap, fn)
		if n == 3 { // begin, run publication, then denied-worker final publication
			close(workerFenceDone)
		}
		return e
	}
	if e = c.Start(ctx, c.view.ViewID); e != nil {
		t.Fatal("original login View did not publish Start", e)
	}
	// No worker resources are supplied in this finite fence test. Start is not
	// successful mining; the independently authorized whole-mode actual proves
	// that separately. The admitted worker must fail and join without work.
	select {
	case <-workerFenceDone:
	case <-time.After(time.Second):
		t.Fatal("denied worker did not finish its original View fence")
	}
	if e = c.Stop(ctx, c.view.ViewID); e != nil {
		t.Fatal("denied worker join", e)
	}
	if s := c.Snapshot(); s.State != "failed" || s.Attempts != 0 {
		t.Fatal("View Start claimed successful work without mining resources", s.State, s.Attempts)
	}

	// Exercise the actual signer leaf with a valid original View and a real
	// retained fixture vault, but invalid mining credentials. The minimally
	// framed proof is never a signed/accepted proof: credential refusal must
	// happen before the vault can inspect or sign it.
	authorizer := &ownedRealMiningAuthority{store: h.accounts, accountID: account.ID}
	manifest := ManifestAccountConfig{Account: AccountConfig{Authorizer: authorizer}, ViewID: c.view.ViewID, Generation: account.Generation}
	manifest.Account.AccountID = account.ID
	manifest.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return h.accounts.WithSession(ctx, login.Token, gymaccount.View, fn)
	}
	leaf := &OwnedGymChild{view: c.view, scope: OwnedGymScope{PoolSource: h.config.Source}, wallet: &OwnedWalletSession{setup: &OwnedWalletSetup{vault: h.vault}, epoch: h.epoch}, authority: &PoolSessionAuthority{account: manifest}}
	p := h.config.Source.Policy
	initial := prospect.Context{Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion, ProspectTarget: p.ProspectTarget}
	proof := prospect.Proof{Version: prospect.Version2}
	leaf.account.Manifest.Account.Authorizer = authorizer
	leaf.account.Manifest.Account.MiningToken = func(context.Context) (string, error) { return login.Token, nil }
	signer, e := leaf.signer(ctx)
	if e != nil {
		t.Fatal("original View unexpectedly lost", e)
	}
	if authorization, e := signer(ctx, initial, proof); !errors.Is(e, gymaccount.ErrUnauthorized) || authorization.PublicKey != "" || authorization.Signature != "" {
		t.Fatal("login-as-Mining produced authorization", e)
	}
	if authorizer.calls.Load() != 1 || authorizer.entered.Load() != 0 {
		t.Fatal("signer skipped real Mining credential refusal", authorizer.calls.Load(), authorizer.entered.Load())
	}
	if e = h.accounts.Logout(ctx, login.Token); e != nil {
		t.Fatal(e)
	}
	if e = h.accounts.WithSession(ctx, mining.Token, gymaccount.Mining, func(gymaccount.Account, func() error) error {
		t.Error("revoked parent retained Mining authorization")
		return nil
	}); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("original login logout did not revoke derived Mining", e)
	}
	if e = c.Start(ctx, c.view.ViewID); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("revoked original View started worker", e)
	}
}
