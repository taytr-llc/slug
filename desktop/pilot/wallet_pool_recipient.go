// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/walletdb"
)

var ErrPoolRecipient = errors.New("pool recipient unavailable")
var ErrPoolRecipientUncertain = errors.New("pool recipient allocation outcome uncertain; reopen wallet before retry")

// walletPoolRecipient is public payment metadata. It is separate from the
// established scope44 solo payout and grants no account or pool authority.
type walletPoolRecipient struct {
	Payout walletPayout `json:"payout"`
	Script string       `json:"script"`
}

// Values come from the independent recovery/testdata/recovery_v1.json corpus,
// SHA256 d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a.
// This pins only published profile-v1 fixtures, never a production derivation.
func expectedPoolRecipient(identity walletIdentity) (walletPoolRecipient, error) {
	for _, f := range []struct{ name, address, script string }{
		{"zero256", "slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme", "001416d630413cea75fd5ad88433cd815ae5ba1ba645"},
		{"incrementing256", "slug1qslk39wvggqa0vl8nd6jckaz54dw3vk45c8educ", "001487ed12b988403af67cf36ea58b7454ab5d165ab4"},
	} {
		known, err := gymaccount.FixtureIdentity(f.name)
		if err == nil && identity == (walletIdentity{RootPublicKey: known.RootPublicKey, GymPublicKey: known.GymPublicKey, GymID: known.GymID}) {
			return walletPoolRecipient{Payout: walletPayout{Address: f.address, Purpose: 84}, Script: f.script}, nil
		}
	}
	return walletPoolRecipient{}, ErrPoolRecipient
}

// poolRecipientCursor reads only public retained address inventory. Caller
// holds r.mu; the wallet database supplies its own consistent read transaction.
func (r *walletRuntime) poolRecipientCursor() (uint32, error) {
	var next uint32
	found := false
	err := walletdb.View(r.wallet.Database(), func(tx walletdb.ReadTx) error {
		accounts, err := waddrmgr.RecoveryAccounts(tx.ReadBucket([]byte("waddrmgr")))
		if err != nil {
			return err
		}
		for _, a := range accounts {
			if a.Scope == waddrmgr.KeyScopeBIP0084 {
				if found || a.NextExternal > recovery.MaxBranchIndex {
					return ErrPoolRecipient
				}
				next, found = a.NextExternal, true
			}
		}
		return nil
	})
	if err != nil || !found {
		return 0, ErrPoolRecipient
	}
	return next, nil
}

func (r *walletRuntime) verifyPoolRecipient(ctx context.Context, want walletPoolRecipient) error {
	var info btcjson.ValidateAddressWalletResult
	if r.rpc.call(ctx, "validateaddress", []any{want.Payout.Address}, &info) != nil || !info.IsValid || !info.IsMine || info.IsScript || info.Address != want.Payout.Address {
		return ErrPoolRecipient
	}
	decoded, err := address.DecodeAddress(want.Payout.Address, &chaincfg.MainNetParams)
	if err != nil || !decoded.IsForNet(&chaincfg.MainNetParams) {
		return ErrPoolRecipient
	}
	if _, ok := decoded.(*address.AddressWitnessPubKeyHash); !ok {
		return ErrPoolRecipient
	}
	script, err := txscript.PayToAddrScript(decoded)
	if err != nil || hex.EncodeToString(script) != want.Script {
		return ErrPoolRecipient
	}
	managed, err := r.wallet.AddressInfo(decoded)
	if err != nil {
		return ErrPoolRecipient
	}
	key, ok := managed.(waddrmgr.ManagedPubKeyAddress)
	if !ok || managed.Internal() {
		return ErrPoolRecipient
	}
	scope, path, ok := key.DerivationInfo()
	if !ok || scope != waddrmgr.KeyScopeBIP0084 || path.InternalAccount != 0 || path.Branch != 0 || path.Index != 0 {
		return ErrPoolRecipient
	}
	next, err := r.poolRecipientCursor()
	if err != nil || next == 0 || ctx.Err() != nil {
		return ErrPoolRecipient
	}
	return nil
}

// EnsurePoolRecipient establishes or recovers the frozen native84 index0
// destination. The owner persists account/source intent before passing nil,
// and separately waits for wallet synchronization before publishing readiness.
// A nonnil expected value requires an already-issued exact recipient.
//
// NewAddress commits before its notification/HTTP acknowledgement. Any failure
// after dispatch therefore requires Close and a fresh attachment before retry;
// that attachment also re-establishes watches for retained active addresses.
func (r *walletRuntime) EnsurePoolRecipient(ctx context.Context, expected *walletPoolRecipient) (walletPoolRecipient, error) {
	if r != nil && r.allocation != nil {
		return (&allocatedRecipientWallet{wallet: r, allocator: r.allocation, operationID: r.allocationRecipientID}).EnsurePoolRecipient(ctx, expected)
	}
	if r == nil || ctx == nil || wire.TestPoW {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.poolRecipientUncertain {
		return walletPoolRecipient{}, ErrPoolRecipientUncertain
	}
	if r.closed || r.rpc == nil || r.wallet == nil || ctx.Err() != nil || r.wallet.Manager.RecoveryVersion() != recovery.Version1 {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	want, err := expectedPoolRecipient(r.identity)
	if err != nil || expected != nil && *expected != want {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	next, err := r.poolRecipientCursor()
	if err != nil {
		return walletPoolRecipient{}, err
	}
	if next > 0 {
		if err := r.verifyPoolRecipient(ctx, want); err != nil {
			return walletPoolRecipient{}, err
		}
		return want, nil
	}
	if expected != nil {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	// A zero retained cursor authorizes one allocation only. The compiled
	// address, rather than the RPC response, remains the expected identity.
	var actual string
	r.poolRecipientUncertain = true
	if r.rpc.call(ctx, "getnewaddress", []any{"default", "bech32"}, &actual) != nil || actual != want.Payout.Address || r.verifyPoolRecipient(ctx, want) != nil {
		return walletPoolRecipient{}, ErrPoolRecipientUncertain
	}
	r.poolRecipientUncertain = false
	return want, nil
}
