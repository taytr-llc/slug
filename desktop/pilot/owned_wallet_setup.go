// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

const ownedWalletIntentName = "SLUG-OWNED-WALLET-INTENT.json"
const ownedWalletReadyName = "SLUG-OWNED-WALLET-READY.json"
const ownedWalletControlLimit = 16384

type OwnedWalletPin struct {
	Fixture             string `json:"fixture"`
	WalletOwnerID       string `json:"wallet_owner_id"`
	InventoryID         string `json:"inventory_id"`
	CarrierJournalID    string `json:"carrier_journal_id"`
	WithdrawalJournalID string `json:"withdrawal_journal_id"`
}
type OwnedWalletFixture struct {
	Pin             OwnedWalletPin
	ChildRoot       string
	RequireExisting bool
}
type OwnedWalletConfig struct {
	Source    poolshare.Config
	Ownership fighterownership.Pin
	Fixtures  []OwnedWalletFixture
}
type ownedWalletIntent struct {
	Version             uint32                    `json:"version"`
	Pin                 OwnedWalletPin            `json:"pin"`
	Source              poolshare.Config          `json:"source"`
	SourceBindingID     string                    `json:"source_binding_id"`
	Ownership           fighterownership.Pin      `json:"ownership"`
	OwnershipPinID      string                    `json:"ownership_pin_id"`
	PoolAllocationID    string                    `json:"pool_allocation_id"`
	Binding             wallet.AllocationBinding2 `json:"binding"`
	BindingID           string                    `json:"binding_id"`
	Carrier             fightercarrier.Identity   `json:"carrier"`
	CarrierBindingID    string                    `json:"carrier_binding_id"`
	Withdrawal          walletwithdraw.Identity   `json:"withdrawal"`
	WithdrawalBindingID string                    `json:"withdrawal_binding_id"`
	InitialManifest     recovery.Manifest         `json:"initial_manifest"`
}
type ownedWalletReady struct {
	Version             uint32               `json:"version"`
	IntentSHA256        string               `json:"intent_sha256"`
	BindingID           string               `json:"binding_id"`
	CarrierBindingID    string               `json:"carrier_binding_id"`
	WithdrawalBindingID string               `json:"withdrawal_binding_id"`
	Recipient           PoolRecipientBinding `json:"recipient"`
}
type ownedWalletEntry struct {
	config     OwnedWalletFixture
	intent     *ownedWalletIntent
	ready      *ownedWalletReady
	inventory  *wallet.HostedAllocationInventory2
	carrier    *fightercarrier.Journal
	withdrawal *walletwithdraw.Journal
	session    *OwnedWalletSession
	opening    bool
}

// OwnedWalletSetup owns only the explicitly selected schema14 route. The root
// has persisted every supplied identity before this owner can create anything.
type OwnedWalletSetup struct {
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	accounts              *gymaccount.Store
	vault                 *recoveryvault.HostedVault
	config                OwnedWalletConfig
	sourceID, ownershipID string
	entries               map[string]*ownedWalletEntry
	closing               bool
	closeDone             chan struct{}
	closeErr              error
	active                sync.WaitGroup
}

func OpenOwnedWalletSetup(ctx context.Context, accounts *gymaccount.Store, vault *recoveryvault.HostedVault, c OwnedWalletConfig) (*OwnedWalletSetup, error) {
	if ctx == nil || ctx.Err() != nil || wire.TestPoW || accounts == nil || vault == nil || len(c.Fixtures) != 2 {
		return nil, ErrPoolRuntime
	}
	sourceID, e := poolservice.BindingID(c.Source)
	if e != nil {
		return nil, e
	}
	ownership, e := fighterownership.ValidatePin(c.Ownership)
	if e != nil || ownership.Source != c.Source.AuthorityPin {
		return nil, ErrPoolRuntime
	}
	ownershipID, e := fighterownership.ID(ownership)
	if e != nil {
		return nil, e
	}
	info, e := vault.Info(ctx)
	if e != nil || info.Authority.AuthorityID != accounts.AuthorityID() || info.Config.Policy != c.Source.Policy {
		return nil, ErrPoolRuntime
	}
	entries := make(map[string]*ownedWalletEntry, 2)
	ids := make(map[string]bool, 10)
	for i, f := range c.Fixtures {
		want := "zero256"
		if i == 1 {
			want = "incrementing256"
		}
		if f.Pin.Fixture != want || !filepath.IsAbs(f.ChildRoot) || filepath.Clean(f.ChildRoot) != f.ChildRoot {
			return nil, ErrPoolRuntime
		}
		for _, id := range []string{f.Pin.WalletOwnerID, f.Pin.InventoryID, f.Pin.CarrierJournalID, f.Pin.WithdrawalJournalID} {
			if !payoutNonzeroID(id) || ids[id] {
				return nil, ErrPoolRuntime
			}
			ids[id] = true
		}
		if i > 0 && (f.ChildRoot == c.Fixtures[0].ChildRoot || withinDir(f.ChildRoot, c.Fixtures[0].ChildRoot) || withinDir(c.Fixtures[0].ChildRoot, f.ChildRoot)) {
			return nil, ErrPoolRuntime
		}
		if e = ownedWalletAncestors(f.ChildRoot); e != nil {
			return nil, e
		}
		entries[want] = &ownedWalletEntry{config: f}
	}
	c.Fixtures = append([]OwnedWalletFixture(nil), c.Fixtures...)
	c.Ownership = ownership
	life, cancel := context.WithCancel(ctx)
	return &OwnedWalletSetup{ctx: life, cancel: cancel, accounts: accounts, vault: vault, config: c, sourceID: sourceID, ownershipID: ownershipID, entries: entries, closeDone: make(chan struct{})}, nil
}
func withinDir(child, parent string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && rel != "." && rel != ".." && len(rel) > 0 && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func ownedWalletAncestors(path string) error {
	for p := path; ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return ErrManifestInitialization
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}
func ownedWalletInventory(root string) error {
	if withdrawalPrivatePath(root, true, false) != nil {
		return ErrManifestInitialization
	}
	allowed := map[string]bool{ownedWalletIntentName: true, ownedWalletReadyName: true, "wallet": true, "inventory": true, "carrier": true, "withdrawal": true, poolRecipientIntentName: true, poolRecipientReadyName: true}
	if e := allocationDirectory(root, allowed); e != nil {
		return e
	}
	for name := range allowed {
		path := filepath.Join(root, name)
		exists, e := poolRuntimeExists(path)
		if e != nil {
			return e
		}
		if !exists {
			continue
		}
		directory := name == "wallet" || name == "inventory" || name == "carrier" || name == "withdrawal"
		if e = withdrawalPrivatePath(path, directory, false); e != nil {
			return e
		}
	}
	return nil
}
func (s *OwnedWalletSetup) expectedIntent(ctx context.Context, e *ownedWalletEntry, login gymaccount.Session, epoch recoveryvault.EpochFence, current recoveryvault.CurrentBinding) (ownedWalletIntent, error) {
	var out ownedWalletIntent
	if epoch == nil || login.Kind != "login" || login.Token == "" || current.WalletOwnerID != e.config.Pin.WalletOwnerID || current.SourceBindingID != s.sourceID {
		return out, ErrPoolRuntime
	}
	f, err := gymaccount.FixtureIdentity(e.config.Pin.Fixture)
	if err != nil {
		return out, err
	}
	err = s.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != login.Account.ID || a.Generation != login.Account.Generation || a.PrincipalID != current.VaultBinding.PrincipalID || a.Enrollment == nil || *a.Enrollment != f || a.RecoveryRequired {
			return gymaccount.ErrUnauthorized
		}
		return epoch(ctx, func(parent func() error) error {
			if parent == nil {
				return gymaccount.ErrUnauthorized
			}
			return errors.Join(check(), parent())
		})
	})
	if err != nil {
		return out, err
	}
	status, err := s.vault.Status(ctx, login.Token, epoch)
	if err != nil || status.State != "enrolled" || status.Receipt == nil {
		return out, ErrPoolRuntime
	}
	info, err := s.vault.Info(ctx)
	if err != nil {
		return out, err
	}
	receipt := status.Receipt
	expected := recoveryvault.CurrentBinding{Version: 1, HostedVaultID: info.HostedVaultID, VaultBinding: receipt.Binding, RetentionID: receipt.RetentionID, WalletOwnerID: e.config.Pin.WalletOwnerID, SourceBindingID: s.sourceID}
	if current != expected || current.VaultBinding.AccountID != login.Account.ID || current.VaultBinding.AuthorityID != s.accounts.AuthorityID() || current.VaultBinding.RootPublicKey != f.RootPublicKey || current.VaultBinding.GymPublicKey != f.GymPublicKey || current.VaultBinding.GymID != f.GymID {
		return out, ErrManifestInitialization
	}
	m := hostedInitialManifest(f.RootPublicKey, f.GymPublicKey, f.GymID)
	if !reflect.DeepEqual(receipt.Manifest, m) {
		return out, ErrManifestInitialization
	}
	b := wallet.AllocationBinding2{Version: 2, InventoryID: e.config.Pin.InventoryID, AuthorityID: s.accounts.AuthorityID(), AccountID: login.Account.ID, PrincipalID: current.VaultBinding.PrincipalID, ProvisionID: current.VaultBinding.ProvisionID, HostedVaultID: current.HostedVaultID, RetentionID: current.RetentionID, WalletOwnerID: current.WalletOwnerID, SourceBindingID: s.sourceID, OwnershipPinID: s.ownershipID, CarrierJournalID: e.config.Pin.CarrierJournalID, WithdrawalJournalID: e.config.Pin.WithdrawalJournalID, ManagerSchema: 14, MaxAllocations: 1024, MaxBytes: 4194304, MaxPlanAttempts: 128, MaxBranchIndex: 4096, InitialManifest: m}
	bid, err := wallet.AllocationBindingID2(b)
	if err != nil {
		return out, err
	}
	cp := fightercarrier.Pin{Version: 1, OwnerMode: "hosted", ManagerSchema: 14, Network: f.Network, Genesis: f.Genesis, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, WalletOwnerID: current.WalletOwnerID, OwnershipPinID: s.ownershipID, AccountAuthorityID: s.accounts.AuthorityID(), AccountID: login.Account.ID, AllocationBindingID: bid}
	carrier := fightercarrier.Identity{JournalID: e.config.Pin.CarrierJournalID, Pin: cp}
	cid, err := fightercarrier.BindingID(carrier)
	if err != nil {
		return out, err
	}
	wp := walletwithdraw.Pin{Version: 1, Network: f.Network, Genesis: f.Genesis, AccountAuthorityID: s.accounts.AuthorityID(), AccountID: login.Account.ID, Fixture: e.config.Pin.Fixture, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, PoolSourceBindingID: s.sourceID}
	withdrawal := walletwithdraw.Identity{JournalID: e.config.Pin.WithdrawalJournalID, Pin: wp}
	wid, err := walletwithdraw.BindingID(withdrawal)
	if err != nil {
		return out, err
	}
	return ownedWalletIntent{Version: 1, Pin: e.config.Pin, Source: s.config.Source, SourceBindingID: s.sourceID, Ownership: s.config.Ownership, OwnershipPinID: s.ownershipID, Binding: b, BindingID: bid, Carrier: carrier, CarrierBindingID: cid, Withdrawal: withdrawal, WithdrawalBindingID: wid, InitialManifest: m}, nil
}
func ownedWalletPrepareControls(root string, expected ownedWalletIntent, established bool) (ownedWalletIntent, *ownedWalletReady, error) {
	var saved ownedWalletIntent
	var ready ownedWalletReady
	exists, e := poolRuntimeExists(root)
	if e != nil {
		return saved, nil, e
	}
	if !exists {
		if established {
			return saved, nil, ErrManifestInitialization
		}
		if e = os.Mkdir(root, 0700); e != nil {
			return saved, nil, e
		}
		if e = syncDirectory(filepath.Dir(root)); e != nil {
			return saved, nil, e
		}
	}
	if e = ownedWalletInventory(root); e != nil {
		return saved, nil, e
	}
	found, e := allocationControl(filepath.Join(root, ownedWalletIntentName), ownedWalletControlLimit, &saved)
	if e != nil {
		return saved, nil, e
	}
	hasReady, e := allocationControl(filepath.Join(root, ownedWalletReadyName), ownedWalletControlLimit, &ready)
	if e != nil {
		return saved, nil, e
	}
	if !found {
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 0 || hasReady || established {
			return saved, nil, ErrManifestInitialization
		}
		var nonce [32]byte
		if _, e = rand.Read(nonce[:]); e != nil {
			return saved, nil, e
		}
		expected.PoolAllocationID = hex.EncodeToString(nonce[:])
		if !payoutNonzeroID(expected.PoolAllocationID) {
			return saved, nil, ErrManifestInitialization
		}
		if e = writePoolRuntimeRecord(filepath.Join(root, ownedWalletIntentName), expected); e != nil {
			return saved, nil, e
		}
		saved = expected
	} else {
		if !payoutNonzeroID(saved.PoolAllocationID) {
			return saved, nil, ErrManifestInitialization
		}
		expected.PoolAllocationID = saved.PoolAllocationID
		if !reflect.DeepEqual(saved, expected) {
			return saved, nil, ErrManifestInitialization
		}
	}
	if established && !hasReady {
		return saved, nil, ErrManifestInitialization
	}
	if hasReady {
		f, e := gymaccount.FixtureIdentity(saved.Pin.Fixture)
		if e != nil {
			return saved, nil, e
		}
		recipient, _, e := poolRecipientExpected(PoolRecipientConfig{Version: 1, AccountAuthorityID: saved.Binding.AuthorityID, AccountID: saved.Binding.AccountID, Fixture: saved.Pin.Fixture, Identity: f, Source: saved.Source})
		if e != nil {
			return saved, nil, e
		}
		want := ownedWalletReady{1, allocationJSONHash(saved), saved.BindingID, saved.CarrierBindingID, saved.WithdrawalBindingID, recipient}
		if !reflect.DeepEqual(ready, want) {
			return saved, nil, ErrManifestInitialization
		}
		for _, name := range []string{"wallet", "inventory", "carrier", "withdrawal", poolRecipientIntentName, poolRecipientReadyName} {
			if ok, e := poolRuntimeExists(filepath.Join(root, name)); e != nil || !ok {
				return saved, nil, ErrManifestInitialization
			}
		}
		return saved, &ready, nil
	}
	return saved, nil, nil
}
