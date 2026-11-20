// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

const poolRecipientIntentName = "SLUG-POOL-RECIPIENT.json"
const poolRecipientReadyName = "SLUG-POOL-RECIPIENT-READY.json"
const poolRecipientMarkerLimit = 8192

// PoolRecipientConfig pins public fixture/account/source identity. It contains
// neither session credentials nor a caller-selected payment address.
type PoolRecipientConfig struct {
	Version            uint32              `json:"version"`
	AccountAuthorityID string              `json:"account_authority_id"`
	AccountID          string              `json:"account_id"`
	Fixture            string              `json:"fixture"`
	Identity           gymaccount.Identity `json:"identity"`
	Source             poolshare.Config    `json:"source"`
}

type PoolRecipient struct {
	Address string `json:"address"`
	Script  string `json:"script"`
	Purpose uint32 `json:"purpose"`
	Coin    uint32 `json:"coin"`
	Account uint32 `json:"account"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}

// PoolRecipientBinding is public destination evidence from an owned fixture
// wallet, not a transport credential or authority to change settlement payees.
type PoolRecipientBinding struct {
	Version         uint32              `json:"version"`
	Config          PoolRecipientConfig `json:"config"`
	SourceBindingID string              `json:"source_binding_id"`
	Recipient       PoolRecipient       `json:"recipient"`
}

type poolRecipientMarker struct {
	Application string               `json:"application"`
	Version     uint32               `json:"version"`
	Phase       string               `json:"phase"`
	Binding     PoolRecipientBinding `json:"binding"`
}
type poolRecipientWallet interface {
	Identity() walletIdentity
	EnsurePoolRecipient(context.Context, *walletPoolRecipient) (walletPoolRecipient, error)
	Snapshot(context.Context) (walletView, error)
}

func poolPublicRecipient(r walletPoolRecipient) PoolRecipient {
	p := r.Payout
	return PoolRecipient{Address: p.Address, Script: r.Script, Purpose: p.Purpose, Coin: p.Coin, Account: p.Account, Branch: p.Branch, Index: p.Index}
}
func poolRecipientExpected(c PoolRecipientConfig) (PoolRecipientBinding, walletPoolRecipient, error) {
	f, e := gymaccount.FixtureIdentity(c.Fixture)
	if e != nil || c.Version != 1 || c.Identity != f || !validAccountID(c.AccountAuthorityID) || !validAccountID(c.AccountID) || c.AccountAuthorityID == c.Source.AuthorityPin.AuthorityID || c.Source.AuthorityPin.Network != f.Network || c.Source.AuthorityPin.Genesis != f.Genesis {
		return PoolRecipientBinding{}, walletPoolRecipient{}, ErrPoolRecipient
	}
	id, e := poolservice.BindingID(c.Source)
	if e != nil {
		return PoolRecipientBinding{}, walletPoolRecipient{}, ErrPoolRecipient
	}
	r, e := expectedPoolRecipient(walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID})
	if e != nil {
		return PoolRecipientBinding{}, walletPoolRecipient{}, e
	}
	return PoolRecipientBinding{Version: 1, Config: c, SourceBindingID: id, Recipient: poolPublicRecipient(r)}, r, nil
}

// ValidatePoolRecipientBinding checks public shape/identity only. The installer
// obtains ownership and derivation evidence from its concrete wallet; reading
// arbitrary JSON is never a substitute for that step or original-view access.
func ValidatePoolRecipientBinding(b PoolRecipientBinding) error {
	want, _, e := poolRecipientExpected(b.Config)
	if e != nil || b != want {
		return ErrPoolRecipient
	}
	return nil
}

func recipientMarker(binding PoolRecipientBinding, phase string) poolRecipientMarker {
	return poolRecipientMarker{Application: "slug-pool-recipient", Version: 1, Phase: phase, Binding: binding}
}
func readPoolRecipientMarker(path string, expected poolRecipientMarker) error {
	var got poolRecipientMarker
	if readManifestJSON(path, poolRecipientMarkerLimit, &got) != nil || got != expected {
		return ErrPoolRecipient
	}
	return nil
}

// A previous writer can publish complete bytes and still fail directory Sync.
// Reading those bytes alone cannot finish that uncertain durability boundary.
func confirmPoolRecipientMarker(path string, expected poolRecipientMarker) error {
	if err := readPoolRecipientMarker(path, expected); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return ErrPoolRecipientUncertain
	}
	return nil
}

// Link publication is atomic and exclusive. It never overwrites a different
// intent or READY record. A failure after publication is an uncertain outcome;
// a later explicit call validates the exact existing record before proceeding.
func writePoolRecipientMarker(path string, marker poolRecipientMarker) error {
	raw, e := json.Marshal(marker)
	if e != nil || len(raw)+1 > poolRecipientMarkerLimit {
		return ErrPoolRecipient
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pool-recipient-")
	if e != nil {
		return ErrPoolRecipient
	}
	name := f.Name()
	defer os.Remove(name)
	e = f.Chmod(0600)
	if e == nil {
		_, e = f.Write(append(raw, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return ErrPoolRecipientUncertain
	}
	if e = os.Link(name, path); e != nil {
		if os.IsExist(e) {
			return confirmPoolRecipientMarker(path, marker)
		}
		return ErrPoolRecipientUncertain
	}
	if syncDirectory(filepath.Dir(path)) != nil {
		return ErrPoolRecipientUncertain
	}
	return nil
}

// recipientView runs only bounded local marker work under the original account
// view. Wallet RPC and synchronization always happen outside this lease.
func recipientView(ctx context.Context, c PoolRecipientConfig, a ManifestAccountConfig, fn func() error) error {
	called := false
	var result error
	err := a.ViewLease(ctx, func(account gymaccount.Account, recheck func() error) (e error) {
		defer func() { result = e }()
		if called {
			return ErrPoolRecipient
		}
		called = true
		if account.ID != c.AccountID || account.Generation != a.Generation || account.Enrollment == nil || *account.Enrollment != c.Identity || recheck == nil || recheck() != nil || ctx.Err() != nil {
			return ErrPoolRecipient
		}
		if e = fn(); e != nil {
			return e
		}
		if recheck() != nil || ctx.Err() != nil {
			return ErrPoolRecipientUncertain
		}
		return nil
	})
	if err != nil || result != nil || !called || ctx.Err() != nil {
		if errors.Is(err, ErrPoolRecipientUncertain) || errors.Is(result, ErrPoolRecipientUncertain) {
			return ErrPoolRecipientUncertain
		}
		return ErrPoolRecipient
	}
	return nil
}
func recipientToken(ctx context.Context, c PoolRecipientConfig, a ManifestAccountConfig, token string) error {
	called := false
	var result error
	err := a.Account.Authorizer.WithSession(ctx, token, func(key string, recheck func() error) (e error) {
		defer func() { result = e }()
		if called {
			return ErrPoolRecipient
		}
		called = true
		if key != c.Identity.GymPublicKey || recheck == nil || recheck() != nil || ctx.Err() != nil {
			return ErrPoolRecipient
		}
		return nil
	})
	if err != nil || result != nil || !called || ctx.Err() != nil {
		return ErrPoolRecipient
	}
	return nil
}

func ensurePoolRecipientBinding(ctx context.Context, root string, c PoolRecipientConfig, a ManifestAccountConfig, w poolRecipientWallet, create bool) (PoolRecipientBinding, error) {
	return ensurePoolRecipientBindingWithWrite(ctx, root, c, a, w, create, writePoolRecipientMarker)
}

// create permits only the first immutable intent. Established callers require
// its presence; no marker is inferred from an address, wallet or other marker.
// The owner serializes this operation with its wallet lifecycle and reopens
// that wallet after ErrPoolRecipientUncertain before recovering an allocation.
func ensurePoolRecipientBindingWithWrite(ctx context.Context, root string, c PoolRecipientConfig, a ManifestAccountConfig, w poolRecipientWallet, create bool, write func(string, poolRecipientMarker) error) (PoolRecipientBinding, error) {
	if wire.TestPoW || ctx == nil || w == nil || write == nil || a.ViewLease == nil || a.Account.Authorizer == nil || a.Account.MiningToken == nil || a.Generation == 0 || a.Generation > manifestMaxSafe || !validAccountID(a.ViewID) || a.Account.AccountID != c.AccountID || a.Account.AuthorityID != c.AccountAuthorityID || a.Account.Fixture != c.Fixture {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	if v := reflect.ValueOf(w); v.Kind() == reflect.Pointer && v.IsNil() {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	if v := reflect.ValueOf(a.Account.Authorizer); v.Kind() == reflect.Pointer && v.IsNil() {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	i, e := os.Lstat(root)
	if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	binding, want, e := poolRecipientExpected(c)
	if e != nil || w.Identity() != (walletIdentity{RootPublicKey: c.Identity.RootPublicKey, GymPublicKey: c.Identity.GymPublicKey, GymID: c.Identity.GymID}) {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	if e = recipientView(ctx, c, a, func() error { return nil }); e != nil {
		return PoolRecipientBinding{}, e
	}
	token, e := a.Account.MiningToken(ctx)
	if e != nil || recipientToken(ctx, c, a, token) != nil {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	intentPath, readyPath := filepath.Join(root, poolRecipientIntentName), filepath.Join(root, poolRecipientReadyName)
	intent, ready := recipientMarker(binding, "initializing"), recipientMarker(binding, "ready")
	hasReady := false
	e = recipientView(ctx, c, a, func() error {
		_, ie := os.Lstat(intentPath)
		_, re := os.Lstat(readyPath)
		if os.IsNotExist(ie) {
			if !create || !os.IsNotExist(re) {
				return ErrPoolRecipient
			}
			if e := write(intentPath, intent); e != nil {
				return e
			}
		} else if ie != nil {
			return ErrPoolRecipient
		} else if e := confirmPoolRecipientMarker(intentPath, intent); e != nil {
			return e
		}
		if re == nil {
			if e := confirmPoolRecipientMarker(readyPath, ready); e != nil {
				return e
			}
			hasReady = true
		} else if !os.IsNotExist(re) {
			return ErrPoolRecipient
		}
		return nil
	})
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	var expected *walletPoolRecipient
	if hasReady {
		expected = &want
	}
	actual, e := w.EnsurePoolRecipient(ctx, expected)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	if actual != want {
		return PoolRecipientBinding{}, ErrPoolRecipientUncertain
	}
	view, e := w.Snapshot(ctx)
	if e != nil || !view.Synced {
		return PoolRecipientBinding{}, ErrPoolRecipient
	}
	// Recheck the exact token used before the call, never fetch a replacement.
	if recipientToken(ctx, c, a, token) != nil {
		return PoolRecipientBinding{}, ErrPoolRecipientUncertain
	}
	e = recipientView(ctx, c, a, func() error {
		if e := confirmPoolRecipientMarker(intentPath, intent); e != nil {
			return e
		}
		if _, e := os.Lstat(readyPath); e == nil {
			return confirmPoolRecipientMarker(readyPath, ready)
		} else if !os.IsNotExist(e) {
			return ErrPoolRecipient
		}
		return write(readyPath, ready)
	})
	if e != nil {
		return PoolRecipientBinding{}, ErrPoolRecipientUncertain
	}
	return binding, nil
}
