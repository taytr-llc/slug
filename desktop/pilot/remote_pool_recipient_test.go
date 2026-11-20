// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
)

type remoteRecipientMarkerState struct {
	info os.FileInfo
	raw  []byte
}

func remoteRecipientMarkers(t *testing.T, h *prbHarness) map[string]remoteRecipientMarkerState {
	t.Helper()
	out := make(map[string]remoteRecipientMarkerState)
	for _, name := range []string{poolRecipientIntentName, poolRecipientReadyName} {
		path := filepath.Join(h.root, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = remoteRecipientMarkerState{info: info, raw: raw}
	}
	return out
}

func remoteRecipientMarkersUnchanged(t *testing.T, h *prbHarness, before map[string]remoteRecipientMarkerState) {
	t.Helper()
	for name, old := range before {
		path := filepath.Join(h.root, name)
		info, err := os.Lstat(path)
		if err != nil || !os.SameFile(old.info, info) {
			t.Fatal("existing-only audit replaced a recipient marker", name, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, old.raw) {
			t.Fatal("existing-only audit changed a recipient marker", name, err)
		}
	}
}

func TestRemotePoolRecipientAuditExistingOnly(t *testing.T) {
	h := newPRBHarness(t)
	if _, err := h.ensure(true); err != nil {
		t.Fatal(err)
	}
	before := remoteRecipientMarkers(t, h)
	for range 2 {
		got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet)
		if err != nil || got != h.binding {
			t.Fatal("matching established recipient audit", err)
		}
	}
	if h.wallet.state.allocations != 1 || h.wallet.snapshotCalls != 3 || !reflect.DeepEqual(h.wallet.expected, []bool{false, true, true}) {
		t.Fatal("audit allocated or omitted exact expected recipient/synchronization")
	}
	remoteRecipientMarkersUnchanged(t, h, before)
}

func TestRemotePoolRecipientAuditMissingMarkerBeforeWallet(t *testing.T) {
	for _, name := range []string{poolRecipientIntentName, poolRecipientReadyName} {
		t.Run(name, func(t *testing.T) {
			h := newPRBHarness(t)
			if _, err := h.ensure(true); err != nil {
				t.Fatal(err)
			}
			before := remoteRecipientMarkers(t, h)
			if err := os.Remove(filepath.Join(h.root, name)); err != nil {
				t.Fatal(err)
			}
			delete(before, name)
			calls, snapshots, allocations := h.wallet.ensureCalls, h.wallet.snapshotCalls, h.wallet.state.allocations
			got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet)
			if !errors.Is(err, ErrPoolRecipient) || got != (PoolRecipientBinding{}) {
				t.Fatal("missing established marker admitted", err)
			}
			if h.wallet.ensureCalls != calls || h.wallet.snapshotCalls != snapshots || h.wallet.state.allocations != allocations {
				t.Fatal("missing established marker reached wallet")
			}
			if _, err := os.Lstat(filepath.Join(h.root, name)); !os.IsNotExist(err) {
				t.Fatal("audit repaired missing marker", err)
			}
			remoteRecipientMarkersUnchanged(t, h, before)
		})
	}
}

func TestRemotePoolRecipientAuditReadyLostDuringWallet(t *testing.T) {
	for _, phase := range []string{"recipient", "snapshot"} {
		t.Run(phase, func(t *testing.T) {
			h := newPRBHarness(t)
			if _, err := h.ensure(true); err != nil {
				t.Fatal(err)
			}
			before := remoteRecipientMarkers(t, h)
			delete(before, poolRecipientReadyName)
			remove := func() error { return os.Remove(filepath.Join(h.root, poolRecipientReadyName)) }
			if phase == "recipient" {
				h.wallet.ensureHook = remove
			} else {
				h.wallet.snapshotHook = remove
			}
			got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet)
			if !errors.Is(err, ErrPoolRecipientUncertain) || got != (PoolRecipientBinding{}) {
				t.Fatal("lost established READY disclosed an audit", err)
			}
			if h.wallet.state.allocations != 1 || !reflect.DeepEqual(h.wallet.expected, []bool{false, true}) {
				t.Fatal("audit allocated instead of requiring original recipient")
			}
			h.absentReady()
			remoteRecipientMarkersUnchanged(t, h, before)
		})
	}
}

func TestRemotePoolRecipientAuditDoesNotReallocateMissingWalletRecipient(t *testing.T) {
	h := newPRBHarness(t)
	if _, err := h.ensure(true); err != nil {
		t.Fatal(err)
	}
	before := remoteRecipientMarkers(t, h)
	// Both exact markers still exist, but the wallet no longer has the recorded
	// destination. The existing-only expected value must make this a refusal.
	h.wallet.state.allocated = false
	got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet)
	if !errors.Is(err, ErrPoolRecipient) || got != (PoolRecipientBinding{}) || h.wallet.state.allocated || h.wallet.state.allocations != 1 {
		t.Fatal("markers recreated missing wallet recipient", err)
	}
	if !reflect.DeepEqual(h.wallet.expected, []bool{false, true}) || h.wallet.snapshotCalls != 1 {
		t.Fatal("missing wallet recipient did not refuse before snapshot")
	}
	remoteRecipientMarkersUnchanged(t, h, before)
}

// Real account capabilities are composed with the existing finite wallet
// control. This test grants no node readiness or real wallet allocation proof.
func TestRemotePoolRecipientAuditOriginalAccountFences(t *testing.T) {
	f := newOwnedWalletTestFixture(t)
	login := f.sessions["zero256"]
	mining, err := f.accounts.MintMining(t.Context(), login.Token)
	if err != nil {
		t.Fatal(err)
	}
	h := newPRBHarness(t)
	h.config.AccountAuthorityID, h.config.AccountID = f.accounts.AuthorityID(), login.Account.ID
	h.binding, _, err = poolRecipientExpected(h.config)
	if err != nil {
		t.Fatal(err)
	}
	h.account.Account.AccountID, h.account.Account.AuthorityID = h.config.AccountID, h.config.AccountAuthorityID
	h.account.Generation = login.Account.Generation
	h.account.Account.Authorizer = &ownedRealMiningAuthority{store: f.accounts, accountID: login.Account.ID}
	h.account.Account.MiningToken = func(context.Context) (string, error) {
		h.tokenFetches++
		return mining.Token, nil
	}
	h.account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if h.viewActive {
			t.Fatal("nested original View")
		}
		h.viewActive = true
		defer func() { h.viewActive = false }()
		return f.accounts.WithSession(ctx, login.Token, gymaccount.View, fn)
	}
	if _, err = h.ensure(true); err != nil {
		t.Fatal("establish under real original capabilities", err)
	}
	before := remoteRecipientMarkers(t, h)
	if got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet); err != nil || got != h.binding {
		t.Fatal("real original capability audit", err)
	}
	for _, kind := range []string{"cancelled", "stale-view"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a := h.account
			if kind == "cancelled" {
				cancel()
			} else {
				a.Generation++
			}
			calls := h.wallet.ensureCalls
			got, err := auditExistingPoolRecipientBinding(ctx, h.root, h.config, a, h.wallet)
			if err == nil || got != (PoolRecipientBinding{}) || h.wallet.ensureCalls != calls {
				t.Fatal("lost original View reached wallet", err)
			}
		})
	}
	// Logout of the original login during wallet work revokes its derived
	// Mining token. A new login/token must not be substituted on disclosure.
	h.wallet.snapshotHook = func() error { return f.accounts.Logout(t.Context(), login.Token) }
	fetches := h.tokenFetches
	got, err := auditExistingPoolRecipientBinding(t.Context(), h.root, h.config, h.account, h.wallet)
	if !errors.Is(err, ErrPoolRecipientUncertain) || got != (PoolRecipientBinding{}) || h.tokenFetches != fetches+1 || h.wallet.state.allocations != 1 {
		t.Fatal("revoked original authority disclosed or rebound recipient", err)
	}
	remoteRecipientMarkersUnchanged(t, h, before)
}
