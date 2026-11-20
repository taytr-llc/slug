// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

const allocationInitName = "SLUG-HOSTED-ALLOCATION-INIT.json"
const allocationReadyName = "SLUG-HOSTED-ALLOCATION-READY.json"

type HostedInventoryPin struct {
	Fixture     string `json:"fixture"`
	InventoryID string `json:"inventory_id"`
}
type hostedAllocationInit struct {
	Version     uint32               `json:"version"`
	GuardSHA256 string               `json:"guard_sha256"`
	Inventories []HostedInventoryPin `json:"inventories"`
}
type hostedAllocationIntent struct {
	Version          uint32                   `json:"version"`
	Fixture          string                   `json:"fixture"`
	GuardSHA256      string                   `json:"guard_sha256"`
	PoolAllocationID string                   `json:"pool_allocation_id"`
	Binding          wallet.AllocationBinding `json:"binding"`
}
type hostedAllocationReadyFixture struct {
	Fixture          string `json:"fixture"`
	InventoryID      string `json:"inventory_id"`
	BindingID        string `json:"binding_id"`
	BindingSHA256    string `json:"binding_sha256"`
	PoolAllocationID string `json:"pool_allocation_id"`
}
type hostedAllocationReady struct {
	Version         uint32                         `json:"version"`
	GuardSHA256     string                         `json:"guard_sha256"`
	SourceBindingID string                         `json:"source_binding_id"`
	Fixtures        []hostedAllocationReadyFixture `json:"fixtures"`
}

// HostedAllocationSetup owns the two independently pinned inventories. Runtimes
// and children borrow it; the parent closes it after all borrowers have joined.
type HostedAllocationSetup struct {
	mu          sync.Mutex
	root        string
	accounts    *gymaccount.Store
	init        hostedAllocationInit
	ready       *hostedAllocationReady
	intents     map[string]hostedAllocationIntent
	inventories map[string]*wallet.HostedAllocationInventory
	established map[string]bool
	closed      bool
	closeErr    error
}

func allocationJSONHash(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func allocationControl(path string, max int64, out any) (bool, error) {
	i, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil || !i.Mode().IsRegular() || i.Mode()&os.ModeSymlink != 0 || i.Size() < 2 || i.Size() > max {
		return false, ErrManifestInitialization
	}
	if readManifestJSON(path, max, out) != nil {
		return false, ErrManifestInitialization
	}
	if readPoolRuntimeRecord(path, out) != nil {
		return false, ErrManifestInitialization
	}
	return true, nil
}
func allocationDirectory(root string, allowed map[string]bool) error {
	f, e := os.Open(root)
	if e != nil {
		return e
	}
	entries, e := f.ReadDir(len(allowed) + 1)
	ce := f.Close()
	if e != nil && !errors.Is(e, io.EOF) {
		return errors.Join(e, ce)
	}
	if ce != nil || len(entries) > len(allowed) {
		return ErrManifestInitialization
	}
	for _, v := range entries {
		if !allowed[v.Name()] || v.Type()&os.ModeSymlink != 0 {
			return ErrManifestInitialization
		}
	}
	return nil
}
func OpenHostedAllocationSetup(ctx context.Context, root string, accounts *gymaccount.Store, guardHash string, pins []HostedInventoryPin, established bool, completed []string) (*HostedAllocationSetup, error) {
	if ctx == nil || ctx.Err() != nil || accounts == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || !payoutNonzeroID(guardHash) || len(pins) != 2 || pins[0].Fixture != "zero256" || pins[1].Fixture != "incrementing256" || !payoutNonzeroID(pins[0].InventoryID) || !payoutNonzeroID(pins[1].InventoryID) || pins[0].InventoryID == pins[1].InventoryID {
		return nil, ErrManifestInitialization
	}
	exists, e := poolRuntimeExists(root)
	if e != nil {
		return nil, e
	}
	if !exists {
		if established {
			return nil, ErrManifestInitialization
		}
		if e = os.Mkdir(root, 0700); e != nil {
			return nil, e
		}
		if e = syncDirectory(filepath.Dir(root)); e != nil {
			return nil, e
		}
	}
	if e = plainChild(root, true); e != nil {
		return nil, e
	}
	if e = allocationDirectory(root, map[string]bool{allocationInitName: true, allocationReadyName: true, "setup": true, "zero256": true, "incrementing256": true}); e != nil {
		return nil, e
	}
	s := &HostedAllocationSetup{root: root, accounts: accounts, init: hostedAllocationInit{1, guardHash, append([]HostedInventoryPin(nil), pins...)}, intents: make(map[string]hostedAllocationIntent), inventories: make(map[string]*wallet.HostedAllocationInventory), established: make(map[string]bool)}
	for _, f := range completed {
		if !stableOne(f, "zero256", "incrementing256") {
			return nil, ErrManifestInitialization
		}
		s.established[f] = true
	}
	var init hostedAllocationInit
	found, e := allocationControl(filepath.Join(root, allocationInitName), 16384, &init)
	if e != nil {
		return nil, e
	}
	if found {
		if !reflect.DeepEqual(init, s.init) {
			return nil, ErrManifestInitialization
		}
	} else {
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 0 || established || len(completed) != 0 {
			return nil, ErrManifestInitialization
		}
		if e = writePoolRuntimeRecord(filepath.Join(root, allocationInitName), s.init); e != nil {
			return nil, e
		}
	}
	var ready hostedAllocationReady
	hasReady, e := allocationControl(filepath.Join(root, allocationReadyName), 16384, &ready)
	if e != nil {
		return nil, e
	}
	if hasReady {
		if ready.Version != 1 || ready.GuardSHA256 != guardHash || !payoutNonzeroID(ready.SourceBindingID) || len(ready.Fixtures) != 2 {
			return nil, ErrManifestInitialization
		}
		s.ready = &ready
	}
	if established && !hasReady {
		return nil, ErrManifestInitialization
	}
	setup := filepath.Join(root, "setup")
	hasSetup, e := poolRuntimeExists(setup)
	if e != nil {
		return nil, e
	}
	if hasSetup {
		if e = plainChild(setup, true); e != nil {
			return nil, e
		}
		if e = allocationDirectory(setup, map[string]bool{"zero256.json": true, "incrementing256.json": true}); e != nil {
			return nil, e
		}
	}
	for i, p := range pins {
		var intent hostedAllocationIntent
		found, e := allocationControl(filepath.Join(setup, p.Fixture+".json"), 8192, &intent)
		if e != nil {
			return nil, e
		}
		if !found {
			if s.ready != nil || s.established[p.Fixture] {
				return nil, ErrManifestInitialization
			}
			if has, e := poolRuntimeExists(filepath.Join(root, p.Fixture)); e != nil || has {
				return nil, ErrManifestInitialization
			}
			continue
		}
		id, e := wallet.AllocationBindingID(intent.Binding)
		if e != nil || intent.Version != 1 || intent.Fixture != p.Fixture || intent.GuardSHA256 != guardHash || !payoutNonzeroID(intent.PoolAllocationID) || intent.Binding.InventoryID != p.InventoryID || intent.Binding.AuthorityID != accounts.AuthorityID() {
			return nil, ErrManifestInitialization
		}
		if s.ready != nil {
			expected := hostedAllocationReadyFixture{p.Fixture, p.InventoryID, id, allocationJSONHash(intent.Binding), intent.PoolAllocationID}
			if ready.Fixtures[i] != expected || ready.SourceBindingID != intent.Binding.SourceBindingID {
				return nil, ErrManifestInitialization
			}
		}
		s.intents[p.Fixture] = intent
	}
	return s, nil
}
func (s *HostedAllocationSetup) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.ready != nil
}
func (s *HostedAllocationSetup) Pin(fixture string) (HostedInventoryPin, error) {
	for _, p := range s.init.Inventories {
		if p.Fixture == fixture {
			return p, nil
		}
	}
	return HostedInventoryPin{}, ErrManifestInitialization
}
func (s *HostedAllocationSetup) ExpectedScope(fixture string) (HostedAllocationScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent, ok := s.intents[fixture]
	if s.closed || !ok {
		return HostedAllocationScope{}, ErrManifestInitialization
	}
	return hostedScopeForBinding(intent.Binding)
}
func hostedScopeForBinding(b wallet.AllocationBinding) (HostedAllocationScope, error) {
	id, e := wallet.AllocationBindingID(b)
	if e != nil {
		return HostedAllocationScope{}, e
	}
	m := b.InitialManifest
	v := recoveryvault.Binding{Version: 1, AuthorityID: b.AuthorityID, AccountID: b.AccountID, PrincipalID: b.PrincipalID, ProvisionID: b.ProvisionID, Network: m.Network, Genesis: m.Genesis, Profile: m.Profile, RootPublicKey: m.RootPublicKey, GymPublicKey: m.GymPublicKey, GymID: m.GymID}
	return HostedAllocationScope{Version: 1, InventoryID: b.InventoryID, InventoryBindingID: id, CurrentBinding: recoveryvault.CurrentBinding{Version: 1, HostedVaultID: b.HostedVaultID, VaultBinding: v, RetentionID: b.RetentionID, WalletOwnerID: b.WalletOwnerID, SourceBindingID: b.SourceBindingID}}, nil
}
func (s *HostedAllocationSetup) ensureInventory(ctx context.Context, fixture string, current recoveryvault.CurrentBinding, locked bool) (*wallet.HostedAllocationInventory, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", ErrPoolRuntime
	}
	p, e := s.Pin(fixture)
	if e != nil {
		return nil, "", e
	}
	// Reconstruct the binding independently of an existing intent or store.
	scope := HostedAllocationScope{Version: 1, InventoryID: p.InventoryID, CurrentBinding: current}
	v := current.VaultBinding
	manifest := hostedInitialManifest(v.RootPublicKey, v.GymPublicKey, v.GymID)
	b := wallet.AllocationBinding{Version: 1, InventoryID: p.InventoryID, AuthorityID: v.AuthorityID, AccountID: v.AccountID, PrincipalID: v.PrincipalID, ProvisionID: v.ProvisionID, HostedVaultID: current.HostedVaultID, RetentionID: current.RetentionID, WalletOwnerID: current.WalletOwnerID, SourceBindingID: current.SourceBindingID, ManagerSchema: 13, MaxAllocations: 1024, MaxBytes: 4194304, MaxPlanAttempts: 128, MaxBranchIndex: 4096, InitialManifest: manifest}
	scope.InventoryBindingID, e = wallet.AllocationBindingID(b)
	if e != nil {
		return nil, "", e
	}
	if ValidateHostedAllocationScope(scope) != nil {
		return nil, "", ErrManifestInitialization
	}
	intent, has := s.intents[fixture]
	if has {
		if !reflect.DeepEqual(intent.Binding, b) {
			return nil, "", ErrManifestInitialization
		}
	} else {
		if s.ready != nil || s.established[fixture] || !locked {
			return nil, "", ErrManifestInitialization
		}
		var random [32]byte
		if _, e = rand.Read(random[:]); e != nil {
			return nil, "", e
		}
		id := hex.EncodeToString(random[:])
		if !payoutNonzeroID(id) {
			return nil, "", ErrManifestInitialization
		}
		intent = hostedAllocationIntent{1, fixture, s.init.GuardSHA256, id, b}
		dir := filepath.Join(s.root, "setup")
		if e = os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
			return nil, "", e
		}
		if e = plainChild(dir, true); e != nil {
			return nil, "", e
		}
		if e = syncDirectory(s.root); e != nil {
			return nil, "", e
		}
		if e = writePoolRuntimeRecord(filepath.Join(dir, fixture+".json"), intent); e != nil {
			return nil, "", e
		}
		s.intents[fixture] = intent
	}
	if inv := s.inventories[fixture]; inv != nil {
		return inv, intent.PoolAllocationID, nil
	}
	path := filepath.Join(s.root, fixture)
	var inv *wallet.HostedAllocationInventory
	if !locked || s.ready != nil || s.established[fixture] {
		inv, e = wallet.OpenHostedAllocationInventory(ctx, path, s.accounts, b)
	} else {
		inv, e = wallet.ResumeHostedAllocationInventory(ctx, path, s.accounts, b)
	}
	if e != nil {
		return nil, "", e
	}
	s.inventories[fixture] = inv
	return inv, intent.PoolAllocationID, nil
}
func (s *HostedAllocationSetup) Inventory(fixture string) (*wallet.HostedAllocationInventory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.inventories[fixture] == nil {
		return nil, ErrPoolRuntime
	}
	return s.inventories[fixture], nil
}
func (s *HostedAllocationSetup) OpenEstablished(ctx context.Context) error {
	for _, p := range s.init.Inventories {
		scope, e := s.ExpectedScope(p.Fixture)
		if e != nil {
			return e
		}
		if _, _, e = s.ensureInventory(ctx, p.Fixture, scope.CurrentBinding, false); e != nil {
			return e
		}
	}
	return nil
}
func (s *HostedAllocationSetup) PublishReady(source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !payoutNonzeroID(source) {
		return ErrManifestInitialization
	}
	r := hostedAllocationReady{Version: 1, GuardSHA256: s.init.GuardSHA256, SourceBindingID: source}
	for _, p := range s.init.Inventories {
		i, ok := s.intents[p.Fixture]
		if !ok || i.Binding.SourceBindingID != source || s.inventories[p.Fixture] == nil {
			return ErrManifestInitialization
		}
		id, e := wallet.AllocationBindingID(i.Binding)
		if e != nil {
			return e
		}
		r.Fixtures = append(r.Fixtures, hostedAllocationReadyFixture{p.Fixture, p.InventoryID, id, allocationJSONHash(i.Binding), i.PoolAllocationID})
	}
	if s.ready != nil {
		if !reflect.DeepEqual(*s.ready, r) {
			return ErrManifestInitialization
		}
		return nil
	}
	if e := writePoolRuntimeRecord(filepath.Join(s.root, allocationReadyName), r); e != nil {
		return e
	}
	s.ready = &r
	return nil
}
func (s *HostedAllocationSetup) Close() error {
	s.mu.Lock()
	s.closed = true
	values := make([]*wallet.HostedAllocationInventory, 0, 2)
	for _, p := range s.init.Inventories {
		if inv := s.inventories[p.Fixture]; inv != nil {
			values = append(values, inv)
		}
	}
	s.mu.Unlock()
	var e error
	for _, inv := range values {
		e = errors.Join(e, inv.Close())
	}
	s.mu.Lock()
	s.closeErr = errors.Join(s.closeErr, e)
	e = s.closeErr
	s.mu.Unlock()
	return e
}
