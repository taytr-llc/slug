// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Public test KEKs only; this wrapper is not a provider implementation or a
// source of expected ciphertext/signatures. All cryptographic expectations come
// from already-frozen independent inputs.
type ownedWalletTestWrapper struct{}

func (*ownedWalletTestWrapper) ID() string { return "owned-wallet-public-fixture" }
func ownedWalletTestAEAD(ref string) (cipher.AEAD, error) {
	var k [32]byte
	switch ref {
	case "recovery-fixture":
		k[0] = 1
	case "gym-fixture":
		k[0] = 2
	default:
		return nil, errors.New("unknown fixture key")
	}
	b, e := aes.NewCipher(k[:])
	clear(k[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(b)
}
func (w *ownedWalletTestWrapper) Wrap(ctx context.Context, ref string, dek [32]byte, aad []byte) ([]byte, error) {
	defer clear(dek[:])
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	a, e := ownedWalletTestAEAD(ref)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return a.Seal(nonce, nonce, dek[:], aad), nil
}
func (w *ownedWalletTestWrapper) Unwrap(ctx context.Context, ref string, blob, aad []byte) (out [32]byte, err error) {
	if e := ctx.Err(); e != nil {
		return out, e
	}
	a, e := ownedWalletTestAEAD(ref)
	if e != nil {
		return out, e
	}
	if len(blob) < a.NonceSize() {
		return out, errors.New("fixture envelope")
	}
	raw, e := a.Open(nil, blob[:a.NonceSize()], blob[a.NonceSize():], aad)
	if e != nil {
		return out, e
	}
	defer clear(raw)
	if len(raw) != 32 {
		return out, errors.New("fixture key size")
	}
	copy(out[:], raw)
	return out, nil
}

type ownedWalletTestFixture struct {
	config   OwnedWalletConfig
	accounts *gymaccount.Store
	vault    *recoveryvault.HostedVault
	sessions map[string]gymaccount.Session
	current  map[string]recoveryvault.CurrentBinding
	revoked  atomic.Bool
}

func (h *ownedWalletTestFixture) epoch(ctx context.Context, fn func(func() error) error) error {
	check := func() error {
		if h.revoked.Load() {
			return gymaccount.ErrUnauthorized
		}
		return ctx.Err()
	}
	if e := check(); e != nil {
		return e
	}
	return fn(check)
}
func newOwnedWalletTestFixture(t *testing.T) *ownedWalletTestFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("concrete wallet requires normal profile")
	}
	ctx := context.Background()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../fighterownership/testdata/reference/public-core-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "1145dede4fcfa453e8bb1cdffff01079f6cca1df3bbe810dc32868207292b507" {
		t.Fatal("independent ownership input changed")
	}
	var corpus struct {
		Positives []struct {
			Family string
			Value  json.RawMessage
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("public corpus")
	}
	var pin fighterownership.Pin
	for _, c := range corpus.Positives {
		if c.Family == "pin" {
			if e = json.Unmarshal(c.Value, &pin); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	if _, e = fighterownership.ValidatePin(pin); e != nil {
		t.Fatal(e)
	}
	source := poolRuntimeTest(t).source
	source.AuthorityPin = pin.Source
	h := &ownedWalletTestFixture{config: OwnedWalletConfig{Source: source, Ownership: pin}, sessions: make(map[string]gymaccount.Session), current: make(map[string]recoveryvault.CurrentBinding)}
	cfg := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: strings.Repeat("a", 64), MaxLoginAttempts: 16, Accounts: gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:owned-wallet", Subject: "zero256", Fixture: "zero256"}, {Issuer: "fixture:owned-wallet", Subject: "incrementing256", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 16, MaxEvents: 32}}
	h.accounts, e = gymaccount.CreateIdentity(filepath.Join(root, "accounts"), cfg)
	if e != nil {
		t.Fatal(e)
	}
	vc := recoveryvault.Config{Version: 1, WrapperID: "owned-wallet-public-fixture", RecoveryKeyRef: "recovery-fixture", GymKeyRef: "gym-fixture", AllowedRecoveryKeyRefs: []string{"recovery-fixture"}, AllowedGymKeyRefs: []string{"gym-fixture"}, MaxOperations: 16, Policy: source.Policy}
	h.vault, e = recoveryvault.CreateHosted(ctx, filepath.Join(root, "vault"), vc, h.accounts, &ownedWalletTestWrapper{}, fmt.Sprintf("%064x", 12345))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.vault.Close(); _ = h.accounts.Close() })
	sourceID, e := poolservice.BindingID(source)
	if e != nil {
		t.Fatal(e)
	}
	for n, name := range []string{"zero256", "incrementing256"} {
		attempt, e := h.accounts.IssueLogin(ctx, "fixture:owned-wallet")
		if e != nil {
			t.Fatal(e)
		}
		now := time.Now().UnixMilli()
		login, e := h.accounts.LoginWithAssertion(ctx, attempt.ID, gymaccount.VerifiedLogin{VerifierConfigHash: cfg.VerifierConfigHash, Issuer: "fixture:owned-wallet", Subject: name, Nonce: attempt.Nonce, AuthenticatedAtMS: now, IssuedAtMS: now, ExpiresAtMS: now + 300000, CredentialEpoch: 1})
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = h.vault.ProvisionFixture(ctx, login.Token, h.epoch); e != nil {
			t.Fatal(e)
		}
		challenge, e := h.accounts.BeginChallenge(ctx, login.Token, "enroll")
		if e != nil {
			t.Fatal(e)
		}
		auth, e := h.vault.SignAccountChallenge(ctx, login.Token, challenge, h.epoch)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = h.accounts.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil {
			t.Fatal(e)
		}
		receipt, e := h.vault.EnsureEnrollment(ctx, login.Token, h.epoch)
		if e != nil {
			t.Fatal(e)
		}
		info, e := h.vault.Info(ctx)
		if e != nil {
			t.Fatal(e)
		}
		id := func(k int) string { return fmt.Sprintf("%064x", 100+n*10+k) }
		p := OwnedWalletPin{Fixture: name, WalletOwnerID: id(1), InventoryID: id(2), CarrierJournalID: id(3), WithdrawalJournalID: id(4)}
		child := filepath.Join(root, name)
		if e = os.Mkdir(child, 0700); e != nil {
			t.Fatal(e)
		}
		h.config.Fixtures = append(h.config.Fixtures, OwnedWalletFixture{Pin: p, ChildRoot: child})
		h.sessions[name] = login
		h.current[name] = recoveryvault.CurrentBinding{Version: 1, HostedVaultID: info.HostedVaultID, VaultBinding: receipt.Binding, RetentionID: receipt.RetentionID, WalletOwnerID: p.WalletOwnerID, SourceBindingID: sourceID}
	}
	return h
}
func (h *ownedWalletTestFixture) open(t *testing.T) *OwnedWalletSetup {
	t.Helper()
	s, e := OpenOwnedWalletSetup(context.Background(), h.accounts, h.vault, h.config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s
}
func TestOwnedWalletFreshAndExactUnfinishedResume(t *testing.T) {
	h := newOwnedWalletTestFixture(t)
	s := h.open(t)
	ctx := context.Background()
	raws := make(map[string][]byte)
	ids := make(map[string]string)
	for _, f := range h.config.Fixtures {
		x, e := s.OpenFixture(ctx, f.Pin.Fixture, h.sessions[f.Pin.Fixture], h.epoch, h.current[f.Pin.Fixture])
		if e != nil {
			t.Fatal("fresh schema14 setup", e)
		}
		if x.runtime.allocationGuard2.State != "bound" {
			t.Fatal("successful Bind retained a locked expectation")
		}
		if x.Allocator() != nil || x.Carrier() != nil || x.Withdrawals() != nil {
			t.Fatal("unready setup exposed action owner")
		}
		ids[f.Pin.Fixture] = x.intent.PoolAllocationID
		// Stop after real atomic allocation/acknowledgment, before watch/READY. This
		// models lost recipient Finalize without claiming a successful node watch.
		a := x.runtime.allocation2
		p, e := a.PlanRecipient(ctx, x.intent.PoolAllocationID)
		if e != nil {
			t.Fatal(e)
		}
		r, e := a.Reserve(ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		receipt, e := a.Consume(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.Acknowledge(ctx, receipt); e != nil {
			t.Fatal(e)
		}
		raws[f.Pin.Fixture], e = os.ReadFile(filepath.Join(x.root, ownedWalletIntentName))
		if e != nil {
			t.Fatal(e)
		}
		if e = x.Close(); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	s = h.open(t)
	for _, f := range h.config.Fixtures {
		x, e := s.OpenFixture(ctx, f.Pin.Fixture, h.sessions[f.Pin.Fixture], h.epoch, h.current[f.Pin.Fixture])
		if e != nil {
			t.Fatal("exact unfinished resume", e)
		}
		if x.intent.PoolAllocationID != ids[f.Pin.Fixture] || x.runtime.allocationGuard2.State != "bound" {
			t.Fatal("resume changed guard/recipient ID")
		}
		got, e := os.ReadFile(filepath.Join(x.root, ownedWalletIntentName))
		if e != nil || !bytes.Equal(got, raws[f.Pin.Fixture]) {
			t.Fatal("immutable intent changed", e)
		}
		status, e := x.entry.inventory.Status(ctx, x.login.Token, wallet.HostedAllocationEpochFence(x.epoch))
		if e != nil || status.Allocations != 1 {
			t.Fatal("resumption duplicated allocation", e)
		}
		m, e := x.runtime.allocation2.CapturePublicRecovery(ctx)
		if e != nil || m.Accounts[2].NextExternal != 1 || m.Accounts[2].NextInternal != 0 {
			t.Fatal("recipient cursor changed", e)
		}
		if _, e = os.Stat(filepath.Join(x.root, ownedWalletReadyName)); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("unwatched setup fabricated READY")
		}
		if e = x.Close(); e != nil {
			t.Fatal(e)
		}
	}
	h.revoked.Store(true)
	if x, e := s.OpenFixture(ctx, "zero256", h.sessions["zero256"], h.epoch, h.current["zero256"]); e == nil || x != nil {
		t.Fatal("revoked original session reopened wallet")
	}
}
func TestOwnedWalletControlsRefuseForeignAndMissingState(t *testing.T) {
	h := newOwnedWalletTestFixture(t)
	s := h.open(t)
	ctx := context.Background()
	entry := s.entries["zero256"]
	want, e := s.expectedIntent(ctx, entry, h.sessions["zero256"], h.epoch, h.current["zero256"])
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"wrong-binding", "unknown-file", "symlink", "missing-ready", "missing-established-root"} {
		t.Run(name, func(t *testing.T) {
			parent, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			root := filepath.Join(parent, "owned-wallet")
			if name == "missing-established-root" {
				if _, _, e = ownedWalletPrepareControls(root, want, true); e == nil {
					t.Fatal("recreated established root")
				}
				return
			}
			first, _, e := ownedWalletPrepareControls(root, want, false)
			if e != nil {
				t.Fatal(e)
			}
			switch name {
			case "wrong-binding":
				want.Binding.WalletOwnerID = strings.Repeat("f", 64)
				defer func() { want.Binding.WalletOwnerID = entry.config.Pin.WalletOwnerID }()
			case "unknown-file":
				if e = os.WriteFile(filepath.Join(root, "unknown"), []byte("x"), 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				if e = os.Symlink(parent, filepath.Join(root, "inventory")); e != nil {
					t.Fatal(e)
				}
			case "missing-ready":
				if _, _, e = ownedWalletPrepareControls(root, want, true); e == nil {
					t.Fatal("missing established READY recreated")
				}
				return
			}
			if _, _, e = ownedWalletPrepareControls(root, want, false); e == nil {
				t.Fatal("accepted invalid setup", first.PoolAllocationID)
			}
		})
	}
	changed := h.config
	changed.Ownership.Source.AuthorityID = strings.Repeat("f", 64)
	if got, e := OpenOwnedWalletSetup(ctx, h.accounts, h.vault, changed); e == nil || got != nil {
		t.Fatal("same-network foreign ownership source accepted")
	}
	alias := h.config
	alias.Fixtures = append([]OwnedWalletFixture(nil), h.config.Fixtures...)
	alias.Fixtures[1].ChildRoot = alias.Fixtures[0].ChildRoot
	if got, e := OpenOwnedWalletSetup(ctx, h.accounts, h.vault, alias); e == nil || got != nil {
		t.Fatal("aliased fixture roots accepted")
	}
}

func TestOwnedWalletCheapProfileRefusesBeforeFilesystem(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	root := filepath.Join(t.TempDir(), "not-created")
	config := OwnedWalletConfig{Fixtures: []OwnedWalletFixture{{ChildRoot: root}, {ChildRoot: root + "-other"}}}
	// Deliberately uninitialized concrete owners are never dereferenced on the
	// unsupported profile; refusal must precede path, vault or account work.
	got, e := OpenOwnedWalletSetup(context.Background(), new(gymaccount.Store), new(recoveryvault.HostedVault), config)
	if got != nil || !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("cheap profile admitted owned setup", e)
	}
	if _, e = os.Lstat(root); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("cheap profile wrote setup", e)
	}
}
