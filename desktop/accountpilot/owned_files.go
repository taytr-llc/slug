// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

const ownedInitName = "SLUG-OWNED-GYM-INIT.json"
const ownedReadyName = "SLUG-OWNED-GYM-READY.json"
const ownedControlLimit = 65536

var ownedStages = [...]string{"authority", "coordinator", "driver", "vault", "runtime", "fixture-zero256", "fixture-incrementing256", "league"}
var ownedStageDirs = [...]string{"authority", "coordinator", "fixture-provider", "hosted-vault", "shared", "", "", ""}

type ownedGymInit struct {
	Version uint32                  `json:"version"`
	Mode    string                  `json:"mode"`
	Hosted  HostedInit              `json:"hosted"`
	Source  pilot.OwnedSourceConfig `json:"source"`
	Wallets [2]pilot.OwnedWalletPin `json:"wallets"`
}
type ownedGymReady struct {
	Version         uint32                          `json:"version"`
	InitSHA256      string                          `json:"init_sha256"`
	SourceBindingID string                          `json:"source_binding_id"`
	Fixtures        [2]recoveryvault.CurrentBinding `json:"fixtures"`
	LeagueSHA256    string                          `json:"league_sha256"`
}

func newOwnedInit(options pilot.PoolOptions, verifier string) (ownedGymInit, error) {
	var out ownedGymInit
	ids := make([]string, 14)
	for i := range ids {
		var e error
		ids[i], e = hostedRandomID(rand.Reader)
		if e != nil {
			return out, e
		}
	}
	pc, e := hostedProviderConfig(ids[0])
	if e != nil {
		return out, e
	}
	h := HostedInit{Version: 1, Mode: pilot.HostedPoolMode, DescriptorID: options.CandidateDescriptorID, AuthorityID: ids[0], CoordinatorID: ids[1], HostedVaultID: ids[2], ProviderDriverID: hostedDriverID, WrapperID: hostedWrapperID, VerifierConfigHash: verifier, IdentityConfigSHA256: hostedCanonicalHash(hostedIdentityConfig(verifier)), ProviderConfigSHA256: hostedCanonicalHash(pc), VaultConfigSHA256: hostedCanonicalHash(hostedVaultConfig()), Wallets: []WalletOwnerPin{{"zero256", ids[3]}, {"incrementing256", ids[4]}}}
	source, e := registry.NewPublicManifestPin(registry.ManifestAuthorityPin{Version: 5, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, AuthorityID: ids[5], DescriptorID: options.CandidateDescriptorID}, ids[6])
	if e != nil {
		return out, e
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return out, e
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if e = listener.Close(); e != nil {
		return out, e
	}
	out = ownedGymInit{Version: 1, Mode: pilot.OwnedGymMode, Hosted: h, Source: pilot.OwnedSourceConfig{Version: 1, Ownership: source, Store: fighterownership.StoreIdentity{StoreID: ids[7]}, PublicReadPort: uint16(port)}, Wallets: [2]pilot.OwnedWalletPin{{Fixture: "zero256", WalletOwnerID: ids[3], InventoryID: ids[8], CarrierJournalID: ids[9], WithdrawalJournalID: ids[10]}, {Fixture: "incrementing256", WalletOwnerID: ids[4], InventoryID: ids[11], CarrierJournalID: ids[12], WithdrawalJournalID: ids[13]}}}
	return out, nil
}
func validOwnedInit(v ownedGymInit, options pilot.PoolOptions) bool {
	if v.Version != 1 || v.Mode != pilot.OwnedGymMode || !validHostedInit(v.Hosted) || v.Hosted.DescriptorID != options.CandidateDescriptorID || pilot.ValidateOwnedGymSource(v.Source) != nil || v.Source.Ownership.Source.DescriptorID != options.CandidateDescriptorID {
		return false
	}
	ids := map[string]bool{}
	all := []string{v.Hosted.AuthorityID, v.Hosted.CoordinatorID, v.Hosted.HostedVaultID, v.Source.Ownership.Source.AuthorityID, v.Source.Ownership.PublicationID, v.Source.Store.StoreID}
	for i, p := range v.Wallets {
		if p.Fixture != hostedFixtures[i] || p.WalletOwnerID != v.Hosted.Wallets[i].WalletOwnerID {
			return false
		}
		all = append(all, p.WalletOwnerID, p.InventoryID, p.CarrierJournalID, p.WithdrawalJournalID)
	}
	for _, id := range all {
		if !hostedID(id) || ids[id] {
			return false
		}
		ids[id] = true
	}
	return true
}
func ownedRootInventory(root string) error {
	allowed := map[string]bool{"instance-lock": true, ownedInitName: true, ownedReadyName: true, "setup": true, "authority": true, "coordinator": true, "fixture-provider": true, "hosted-vault": true, "shared": true}
	entries, e := hostedDirectoryEntries(root, len(allowed))
	if e != nil {
		return e
	}
	for _, de := range entries {
		if !allowed[de.Name()] {
			return pilot.ErrManifestInitialization
		}
		if de.Name() != ownedInitName && de.Name() != ownedReadyName && !de.IsDir() {
			return pilot.ErrManifestInitialization
		}
	}
	return nil
}
func openOwnedRoot(root string) (*leveldb.DB, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\t") {
		return nil, pilot.ErrManifestInitialization
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(root))
	if e != nil || parent != filepath.Dir(root) {
		return nil, pilot.ErrManifestInitialization
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"SLUG-ACCOUNTS.json", "SLUG-POOL-ACCOUNTS.json", "SLUG-HOSTED-POOL-INIT.json", "SLUG-HOSTED-ALLOCATION-V1.json", "SLUG-PILOT.json", "SLUG-MANIFEST-ACCOUNTS.json", "SLUG-POOL-RUNTIME.json"} {
			if _, e := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(e) {
				return nil, pilot.ErrManifestInitialization
			}
		}
		if dir != root {
			if _, e := os.Lstat(filepath.Join(dir, ownedInitName)); !os.IsNotExist(e) {
				return nil, pilot.ErrManifestInitialization
			}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	if e = ownedMakeDirectory(root); e != nil {
		return nil, e
	}
	if e = ownedRootInventory(root); e != nil {
		return nil, e
	}
	path := filepath.Join(root, "instance-lock")
	if i, e := os.Lstat(path); e == nil && (!i.IsDir() || i.Mode()&os.ModeSymlink != 0) {
		return nil, pilot.ErrManifestInitialization
	} else if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	return leveldb.OpenFile(path, nil)
}
func ownedWrite(path string, v any) error {
	var existing any
	// Read into the exact concrete caller type so unknown/duplicate/order fields
	// remain rejected by the inherited canonical control reader.
	target := reflect.New(reflect.TypeOf(v)).Interface()
	found, e := readHostedControl(path, ownedControlLimit, target)
	if e != nil {
		return e
	}
	if found {
		existing = reflect.ValueOf(target).Elem().Interface()
		if !reflect.DeepEqual(existing, v) {
			return pilot.ErrManifestInitialization
		}
		d, e := os.Open(filepath.Dir(path))
		if e != nil {
			return e
		}
		return errors.Join(d.Sync(), d.Close())
	}
	return writeHostedControl(path, v, ownedControlLimit)
}
func readOwnedPrefix(root string, init ownedGymInit) ([]SetupReceipt, error) {
	if e := ownedRootInventory(root); e != nil {
		return nil, e
	}
	path := filepath.Join(root, "setup")
	exists, e := hostedComponentExists(path)
	if e != nil {
		return nil, e
	}
	prefix := []SetupReceipt{}
	if exists {
		rows, e := hostedDirectoryEntries(path, len(ownedStages))
		if e != nil {
			return nil, e
		}
		names := map[string]bool{}
		for _, row := range rows {
			if row.IsDir() {
				return nil, pilot.ErrManifestInitialization
			}
			names[row.Name()] = true
		}
		gap := false
		for _, stage := range ownedStages {
			name := stage + ".json"
			if !names[name] {
				gap = true
				continue
			}
			if gap {
				return nil, pilot.ErrManifestInitialization
			}
			delete(names, name)
			var v SetupReceipt
			found, e := readHostedControl(filepath.Join(path, name), hostedStageMax, &v)
			if e != nil || !found || v.Version != 1 || v.Stage != stage || v.InitSHA256 != hostedCanonicalHash(init) || !hostedHash(v.IdentitySHA256) {
				return nil, pilot.ErrManifestInitialization
			}
			prefix = append(prefix, v)
		}
		if len(names) != 0 {
			return nil, pilot.ErrManifestInitialization
		}
	}
	for i, dir := range ownedStageDirs {
		if dir == "" {
			continue
		}
		found, e := hostedComponentExists(filepath.Join(root, dir))
		if e != nil {
			return nil, e
		}
		if i < len(prefix) && !found || i > len(prefix) && found {
			return nil, pilot.ErrManifestInitialization
		}
	}
	return prefix, nil
}
func (c *OwnedGymController) stage(name, id string) error {
	index := -1
	for i, s := range ownedStages {
		if s == name {
			index = i
		}
	}
	if index < 0 || !hostedHash(id) || index > len(c.prefix) {
		return pilot.ErrManifestInitialization
	}
	v := SetupReceipt{Version: 1, Stage: name, InitSHA256: hostedCanonicalHash(c.init), IdentitySHA256: id}
	if index < len(c.prefix) {
		if c.prefix[index] != v {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	if c.hadReady {
		return pilot.ErrManifestInitialization
	}
	dir := filepath.Join(c.config.DataDir, "setup")
	if e := ownedMakeDirectory(dir); e != nil {
		return e
	}
	if e := ownedWrite(filepath.Join(dir, name+".json"), v); e != nil {
		return e
	}
	c.prefix = append(c.prefix, v)
	return nil
}
func (c *OwnedGymController) component(name string, index int) (string, bool, error) {
	path := filepath.Join(c.config.DataDir, name)
	exists, e := hostedComponentExists(path)
	if e != nil {
		return path, false, e
	}
	if !exists && (c.hadReady || index < len(c.prefix)) {
		return path, false, pilot.ErrManifestInitialization
	}
	return path, exists, nil
}

func ownedMakeDirectory(path string) error {
	e := os.Mkdir(path, 0700)
	if os.IsExist(e) {
		if !realDirectory(path) {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	if e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	return errors.Join(d.Sync(), d.Close())
}
