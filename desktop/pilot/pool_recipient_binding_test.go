// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type prbSessions struct{ h *prbHarness }

func (s *prbSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	h := s.h
	h.t.Helper()
	if h.viewActive || h.sessionActive {
		h.t.Fatal("nested recipient account lease")
	}
	h.sessionActive = true
	defer func() { h.sessionActive = false }()
	h.seenTokens = append(h.seenTokens, token)
	if !h.tokens[token] {
		return gymaccount.ErrUnauthorized
	}
	check := func() error {
		if !h.tokens[token] {
			return gymaccount.ErrUnauthorized
		}
		return ctx.Err()
	}
	switch h.sessionMode {
	case "omit":
		return nil
	case "swallow":
		_ = fn(strings.Repeat("0", 66), check)
		return nil
	case "double":
		_ = fn(h.config.Identity.GymPublicKey, check)
		_ = fn(h.config.Identity.GymPublicKey, check)
		return nil
	default:
		return fn(h.config.Identity.GymPublicKey, check)
	}
}

type prbWalletState struct {
	allocated   bool
	allocations int
}
type prbWallet struct {
	h                          *prbHarness
	state                      *prbWalletState
	want                       walletPoolRecipient
	identity                   walletIdentity
	ensureCalls, snapshotCalls int
	expected                   []bool
	uncertain                  bool
	synced                     bool
	ensureHook                 func() error
	snapshotHook               func() error
}

func (w *prbWallet) Identity() walletIdentity { return w.identity }
func (w *prbWallet) EnsurePoolRecipient(_ context.Context, expected *walletPoolRecipient) (walletPoolRecipient, error) {
	w.h.t.Helper()
	w.ensureCalls++
	w.expected = append(w.expected, expected != nil)
	if w.h.viewActive || w.h.sessionActive {
		w.h.t.Fatal("recipient wallet allocation held account lease")
	}
	if err := readPoolRecipientMarker(filepath.Join(w.h.root, poolRecipientIntentName), recipientMarker(w.h.binding, "initializing")); err != nil {
		w.h.t.Fatal("wallet allocation preceded exact immutable intent", err)
	}
	if w.uncertain {
		return walletPoolRecipient{}, ErrPoolRecipientUncertain
	}
	if expected != nil && (*expected != w.want || !w.state.allocated) {
		return walletPoolRecipient{}, ErrPoolRecipient
	}
	if !w.state.allocated {
		w.state.allocated = true
		w.state.allocations++
	}
	if w.ensureHook != nil {
		if err := w.ensureHook(); err != nil {
			w.uncertain = errors.Is(err, ErrPoolRecipientUncertain)
			return walletPoolRecipient{}, err
		}
	}
	return w.want, nil
}
func (w *prbWallet) Snapshot(context.Context) (walletView, error) {
	w.h.t.Helper()
	w.snapshotCalls++
	if w.h.viewActive || w.h.sessionActive {
		w.h.t.Fatal("wallet synchronization held account lease")
	}
	if w.snapshotHook != nil {
		if err := w.snapshotHook(); err != nil {
			return walletView{}, err
		}
	}
	return walletView{Synced: w.synced}, nil
}

type prbHarness struct {
	t                         *testing.T
	root                      string
	config                    PoolRecipientConfig
	binding                   PoolRecipientBinding
	account                   ManifestAccountConfig
	wallet                    *prbWallet
	token                     string
	tokens                    map[string]bool
	seenTokens                []string
	tokenFetches, viewCalls   int
	viewValid                 bool
	viewGeneration            uint64
	viewActive, sessionActive bool
	viewMode, sessionMode     string
}

func newPRBHarness(t *testing.T) *prbHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("recipient bindings require the normal candidate profile")
	}
	identity, err := gymaccount.FixtureIdentity("zero256")
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	p := policy()
	source := poolshare.Config{Version: 1, AuthorityPin: registry.ManifestAuthorityPin{Version: 5, Network: p.Network, Genesis: p.Genesis, AuthorityID: strings.Repeat("a", 64), DescriptorID: descriptor}, Policy: p, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 32, MaxBytes: 1 << 20}
	h := &prbHarness{t: t, root: t.TempDir(), token: strings.Repeat("1", 64), viewValid: true, viewGeneration: 1, tokens: map[string]bool{strings.Repeat("1", 64): true, strings.Repeat("2", 64): true}}
	h.config = PoolRecipientConfig{Version: 1, AccountAuthorityID: strings.Repeat("c", 64), AccountID: strings.Repeat("b", 64), Fixture: "zero256", Identity: identity, Source: source}
	h.binding, _, err = poolRecipientExpected(h.config)
	if err != nil {
		t.Fatal(err)
	}
	want := poolRecipientOracle(t, "zero256")
	if h.binding.Recipient != poolPublicRecipient(want) {
		t.Fatal("binding differs from independent recovery destination")
	}
	h.wallet = &prbWallet{h: h, state: &prbWalletState{}, want: want, identity: walletIdentity{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}, synced: true}
	h.account = ManifestAccountConfig{Account: AccountConfig{Fixture: "zero256", AccountID: h.config.AccountID, AuthorityID: h.config.AccountAuthorityID, Authorizer: &prbSessions{h}, MiningToken: func(context.Context) (string, error) {
		h.tokenFetches++
		if h.viewActive || h.sessionActive {
			t.Fatal("token refresh ran under another account lease")
		}
		return h.token, nil
	}}, ViewID: strings.Repeat("d", 64), Generation: 1}
	h.account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if h.viewActive || h.sessionActive {
			t.Fatal("nested recipient view lease")
		}
		h.viewActive = true
		defer func() { h.viewActive = false }()
		h.viewCalls++
		check := func() error {
			if !h.viewValid {
				return gymaccount.ErrUnauthorized
			}
			return ctx.Err()
		}
		account := gymaccount.Account{ID: h.config.AccountID, Generation: h.viewGeneration, Enrollment: &identity}
		switch h.viewMode {
		case "omit":
			return nil
		case "swallow":
			account.Generation++
			_ = fn(account, check)
			return nil
		case "double":
			_ = fn(account, check)
			_ = fn(account, check)
			return nil
		default:
			if err := check(); err != nil {
				return err
			}
			return fn(account, check)
		}
	}
	return h
}

func (h *prbHarness) ensure(create bool) (PoolRecipientBinding, error) {
	return ensurePoolRecipientBinding(context.Background(), h.root, h.config, h.account, h.wallet, create)
}
func (h *prbHarness) absentReady() {
	h.t.Helper()
	if _, err := os.Lstat(filepath.Join(h.root, poolRecipientReadyName)); !os.IsNotExist(err) {
		h.t.Fatal("recipient became ready before successful synchronization and final authorization", err)
	}
}

func TestPoolRecipientBindingPublishesIntentBeforeWalletAndReadyAfterRechecks(t *testing.T) {
	h := newPRBHarness(t)
	h.wallet.ensureHook = func() error { h.absentReady(); return nil }
	h.wallet.snapshotHook = func() error { h.absentReady(); return nil }
	write := func(path string, marker poolRecipientMarker) error {
		if !h.viewActive || h.sessionActive {
			t.Fatal("marker publication lacks original view lease")
		}
		if marker.Phase == "ready" && (h.wallet.ensureCalls != 1 || h.wallet.snapshotCalls != 1 || len(h.seenTokens) != 2 || h.viewCalls != 3) {
			t.Fatal("ready preceded wallet synchronization and original token/view recheck")
		}
		return writePoolRecipientMarker(path, marker)
	}
	got, err := ensurePoolRecipientBindingWithWrite(context.Background(), h.root, h.config, h.account, h.wallet, true, write)
	if err != nil || got != h.binding || ValidatePoolRecipientBinding(got) != nil {
		t.Fatal("recipient binding unavailable", err)
	}
	before := map[string][]byte{}
	for _, name := range []string{poolRecipientIntentName, poolRecipientReadyName} {
		path := filepath.Join(h.root, name)
		before[name], err = os.ReadFile(path)
		if err != nil || bytes.Contains(before[name], []byte(h.token)) {
			t.Fatal("marker missing or retained credential", err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("recipient marker permissions", err)
		}
	}
	h.wallet.ensureHook, h.wallet.snapshotHook = nil, nil
	got, err = h.ensure(false)
	if err != nil || got != h.binding || h.wallet.state.allocations != 1 || !reflect.DeepEqual(h.wallet.expected, []bool{false, true}) {
		t.Fatal("ready recovery allocated a new destination or omitted verify-only expected value", err)
	}
	for name, old := range before {
		raw, err := os.ReadFile(filepath.Join(h.root, name))
		if err != nil || !bytes.Equal(raw, old) {
			t.Fatal("immutable binding changed on verification", err)
		}
	}
}

func TestPoolRecipientBindingRejectsRebindingBeforeWalletIO(t *testing.T) {
	for _, kind := range []string{"source-target", "source-limit", "source-authority", "account", "account-authority", "fixture", "identity", "wallet-identity"} {
		t.Run(kind, func(t *testing.T) {
			h := newPRBHarness(t)
			if _, err := h.ensure(true); err != nil {
				t.Fatal(err)
			}
			c, a := h.config, h.account
			switch kind {
			case "source-target":
				c.Source.PoolTarget = strings.Repeat("e", 64)
			case "source-limit":
				c.Source.MaxWorks++
			case "source-authority":
				c.Source.AuthorityPin.AuthorityID = strings.Repeat("e", 64)
			case "account":
				c.AccountID, a.Account.AccountID = strings.Repeat("e", 64), strings.Repeat("e", 64)
			case "account-authority":
				c.AccountAuthorityID, a.Account.AuthorityID = strings.Repeat("e", 64), strings.Repeat("e", 64)
			case "fixture":
				c.Fixture, a.Account.Fixture = "incrementing256", "incrementing256"
			case "identity":
				c.Identity, _ = gymaccount.FixtureIdentity("incrementing256")
			case "wallet-identity":
				h.wallet.identity.GymID = strings.Repeat("e", 64)
			}
			calls := h.wallet.ensureCalls
			got, err := ensurePoolRecipientBinding(context.Background(), h.root, c, a, h.wallet, false)
			if err == nil || got != (PoolRecipientBinding{}) || h.wallet.ensureCalls != calls {
				t.Fatal("rebound identity reached wallet or published binding")
			}
		})
	}
}

func TestPoolRecipientBindingRequiresEstablishedIntentAndOwnedWallet(t *testing.T) {
	for _, kind := range []string{"missing-established", "ready-without-intent", "ready-without-wallet-allocation"} {
		t.Run(kind, func(t *testing.T) {
			h := newPRBHarness(t)
			create := false
			if kind != "missing-established" {
				if err := writePoolRecipientMarker(filepath.Join(h.root, poolRecipientReadyName), recipientMarker(h.binding, "ready")); err != nil {
					t.Fatal(err)
				}
				create = true
			}
			if kind == "ready-without-wallet-allocation" {
				if err := writePoolRecipientMarker(filepath.Join(h.root, poolRecipientIntentName), recipientMarker(h.binding, "initializing")); err != nil {
					t.Fatal(err)
				}
			}
			got, err := h.ensure(create)
			if err == nil || got != (PoolRecipientBinding{}) || h.wallet.state.allocations != 0 {
				t.Fatal("missing established component was silently recreated")
			}
			if kind == "ready-without-wallet-allocation" {
				if !reflect.DeepEqual(h.wallet.expected, []bool{true}) {
					t.Fatal("ready wallet check was allowed to allocate")
				}
			} else if h.wallet.ensureCalls != 0 {
				t.Fatal("invalid marker state reached wallet")
			}
		})
	}
}

func TestPoolRecipientBindingUncertainRecoveryKeepsOriginalDestination(t *testing.T) {
	for _, kind := range []string{"wallet-ack", "ready-ack", "intent-ack"} {
		t.Run(kind, func(t *testing.T) {
			h := newPRBHarness(t)
			write := writePoolRecipientMarker
			if kind == "wallet-ack" {
				h.wallet.ensureHook = func() error { return ErrPoolRecipientUncertain }
			} else {
				phase := "ready"
				if kind == "intent-ack" {
					phase = "initializing"
				}
				write = func(path string, marker poolRecipientMarker) error {
					if err := writePoolRecipientMarker(path, marker); err != nil {
						return err
					}
					if marker.Phase == phase {
						return ErrPoolRecipientUncertain
					}
					return nil
				}
			}
			got, err := ensurePoolRecipientBindingWithWrite(context.Background(), h.root, h.config, h.account, h.wallet, true, write)
			if !errors.Is(err, ErrPoolRecipientUncertain) || got != (PoolRecipientBinding{}) {
				t.Fatal("lost acknowledgment became success", err)
			}
			if kind == "intent-ack" && h.wallet.ensureCalls != 0 {
				t.Fatal("uncertain intent continued into wallet")
			}
			if kind != "ready-ack" {
				h.absentReady()
			}
			// Model the required wallet close/reopen, retaining its committed cursor.
			old := h.wallet
			h.wallet = &prbWallet{h: h, state: old.state, want: old.want, identity: old.identity, synced: true}
			got, err = h.ensure(false)
			if err != nil || got != h.binding || h.wallet.state.allocations != 1 {
				t.Fatal("uncertain recovery changed or reallocated destination", err)
			}
			if !reflect.DeepEqual(h.wallet.expected, []bool{kind == "ready-ack"}) {
				t.Fatal("recovery did not honor durable ready state")
			}
		})
	}
}

func TestPoolRecipientBindingRequiresSyncAndOriginalAuthorization(t *testing.T) {
	for _, kind := range []string{"unsynced", "snapshot-error", "reset", "view-switch", "replacement-token", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			h := newPRBHarness(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h.wallet.snapshotHook = func() error {
				switch kind {
				case "unsynced":
					h.wallet.synced = false
				case "snapshot-error":
					return errors.New("wallet offline")
				case "reset":
					h.viewGeneration++
				case "view-switch":
					h.viewValid = false
				case "replacement-token":
					h.tokens[h.token] = false
					h.token = strings.Repeat("2", 64)
				case "cancel":
					cancel()
				}
				return nil
			}
			got, err := ensurePoolRecipientBinding(ctx, h.root, h.config, h.account, h.wallet, true)
			if err == nil || got != (PoolRecipientBinding{}) {
				t.Fatal("lost sync/access published recipient")
			}
			h.absentReady()
			if h.tokenFetches != 1 {
				t.Fatal("operation fetched replacement token")
			}
			for _, token := range h.seenTokens {
				if token != strings.Repeat("1", 64) {
					t.Fatal("replacement token substituted during operation")
				}
			}
		})
	}
}

func TestPoolRecipientBindingRejectsBrokenLeaseCallbacks(t *testing.T) {
	for _, lane := range []string{"view", "session"} {
		for _, mode := range []string{"omit", "swallow", "double"} {
			t.Run(lane+"-"+mode, func(t *testing.T) {
				h := newPRBHarness(t)
				if lane == "view" {
					h.viewMode = mode
				} else {
					h.sessionMode = mode
				}
				got, err := h.ensure(true)
				if err == nil || got != (PoolRecipientBinding{}) || h.wallet.ensureCalls != 0 {
					t.Fatal("broken account callback escaped into wallet")
				}
				h.absentReady()
			})
		}
	}
}

func TestPoolRecipientBindingRejectsSymlinksAndNoncanonicalMarkers(t *testing.T) {
	for _, kind := range []string{"root-symlink", "intent-symlink", "ready-symlink", "intent-whitespace", "ready-whitespace", "duplicate-field"} {
		t.Run(kind, func(t *testing.T) {
			h := newPRBHarness(t)
			if _, err := h.ensure(true); err != nil {
				t.Fatal(err)
			}
			calls := h.wallet.ensureCalls
			name := poolRecipientIntentName
			if strings.HasPrefix(kind, "ready") {
				name = poolRecipientReadyName
			}
			path := filepath.Join(h.root, name)
			switch kind {
			case "root-symlink":
				link := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(h.root, link); err != nil {
					t.Fatal(err)
				}
				h.root = link
			case "intent-symlink", "ready-symlink":
				original := filepath.Join(h.root, "retained-marker")
				if err := os.Rename(path, original); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(original, path); err != nil {
					t.Fatal(err)
				}
			default:
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "duplicate-field" {
					raw = bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)
				} else {
					raw = append([]byte(" "), raw...)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := h.ensure(false)
			if err == nil || got != (PoolRecipientBinding{}) || h.wallet.ensureCalls != calls {
				t.Fatal("unsafe/noncanonical binding reached wallet")
			}
		})
	}
}
