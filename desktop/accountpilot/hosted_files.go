// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

const hostedInitName = "SLUG-HOSTED-POOL-INIT.json"
const hostedReadyName = "SLUG-HOSTED-POOL-READY.json"
const hostedMarkerMax = 32768
const hostedStageMax = 8192

var hostedStages = [...]string{"authority", "coordinator", "driver", "vault", "runtime", "fixture-zero256", "fixture-incrementing256", "current"}
var hostedStageDirectories = [...]string{"authority", "coordinator", "fixture-provider", "hosted-vault", "shared", "", "", "current-recovery"}

type WalletOwnerPin struct {
	Fixture       string `json:"fixture"`
	WalletOwnerID string `json:"wallet_owner_id"`
}
type HostedInit struct {
	Version              uint32           `json:"version"`
	Mode                 string           `json:"mode"`
	DescriptorID         string           `json:"descriptor_id"`
	AuthorityID          string           `json:"authority_id"`
	CoordinatorID        string           `json:"coordinator_id"`
	HostedVaultID        string           `json:"hosted_vault_id"`
	ProviderDriverID     string           `json:"provider_driver_id"`
	WrapperID            string           `json:"wrapper_id"`
	VerifierConfigHash   string           `json:"verifier_config_hash"`
	IdentityConfigSHA256 string           `json:"identity_config_sha256"`
	ProviderConfigSHA256 string           `json:"provider_config_sha256"`
	VaultConfigSHA256    string           `json:"vault_config_sha256"`
	Wallets              []WalletOwnerPin `json:"wallets"`
}
type SetupReceipt struct {
	Version        uint32 `json:"version"`
	Stage          string `json:"stage"`
	InitSHA256     string `json:"init_sha256"`
	IdentitySHA256 string `json:"identity_sha256"`
}
type HostedFixtureReady struct {
	Fixture        string                       `json:"fixture"`
	CurrentBinding recoveryvault.CurrentBinding `json:"current_binding"`
}
type HostedReady struct {
	Version         uint32               `json:"version"`
	InitSHA256      string               `json:"init_sha256"`
	AuthorityID     string               `json:"authority_id"`
	CoordinatorID   string               `json:"coordinator_id"`
	HostedVaultID   string               `json:"hosted_vault_id"`
	SourceBindingID string               `json:"source_binding_id"`
	Fixtures        []HostedFixtureReady `json:"fixtures"`
}

// Control hashes cover compact JSON without the marker's one trailing LF.
func hostedCanonicalHash(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validHostedInit(v HostedInit) bool {
	options, err := pilot.ResolvePoolOptions(pilot.PoolOptions{CandidateDescriptorID: v.DescriptorID})
	if err != nil || options.CandidateDescriptorID != v.DescriptorID || v.Version != 1 || v.Mode != pilot.HostedPoolMode || v.ProviderDriverID != hostedDriverID || v.WrapperID != hostedWrapperID || len(v.Wallets) != 2 {
		return false
	}
	for _, id := range []string{v.AuthorityID, v.CoordinatorID, v.HostedVaultID, v.Wallets[0].WalletOwnerID, v.Wallets[1].WalletOwnerID} {
		if !hostedID(id) {
			return false
		}
	}
	for _, h := range []string{v.VerifierConfigHash, v.IdentityConfigSHA256, v.ProviderConfigSHA256, v.VaultConfigSHA256} {
		if !hostedHash(h) {
			return false
		}
	}
	return v.Wallets[0].Fixture == "zero256" && v.Wallets[1].Fixture == "incrementing256" && v.Wallets[0].WalletOwnerID != v.Wallets[1].WalletOwnerID
}
func validHostedReady(v HostedReady) bool {
	if v.Version != 1 || !hostedHash(v.InitSHA256) || len(v.Fixtures) != 2 {
		return false
	}
	for _, id := range []string{v.AuthorityID, v.CoordinatorID, v.HostedVaultID, v.SourceBindingID} {
		if !hostedID(id) {
			return false
		}
	}
	for i, name := range []string{"zero256", "incrementing256"} {
		f := v.Fixtures[i]
		b := f.CurrentBinding
		a := b.VaultBinding
		identity, err := gymaccount.FixtureIdentity(name)
		principal, pe := gymaccount.PrincipalID(gymprovider.FixtureIssuer, name)
		if err != nil || pe != nil || f.Fixture != name || b.Version != 1 || b.HostedVaultID != v.HostedVaultID || b.SourceBindingID != v.SourceBindingID || !hostedID(b.RetentionID) || !hostedID(b.WalletOwnerID) || a.Version != 1 || a.AuthorityID != v.AuthorityID || !hostedID(a.AccountID) || !hostedID(a.ProvisionID) || a.PrincipalID != principal || a.Network != gymaccount.FixtureNetwork || a.Genesis != gymaccount.FixtureGenesis || a.Profile != gymaccount.FixtureProfile || a.RootPublicKey != identity.RootPublicKey || a.GymPublicKey != identity.GymPublicKey || a.GymID != identity.GymID {
			return false
		}
	}
	a, b := v.Fixtures[0].CurrentBinding, v.Fixtures[1].CurrentBinding
	return a.VaultBinding.AccountID != b.VaultBinding.AccountID && a.WalletOwnerID != b.WalletOwnerID
}
func hostedReadyMatches(init HostedInit, ready HostedReady) bool {
	if !validHostedInit(init) || !validHostedReady(ready) || ready.InitSHA256 != hostedCanonicalHash(init) || ready.AuthorityID != init.AuthorityID || ready.CoordinatorID != init.CoordinatorID || ready.HostedVaultID != init.HostedVaultID {
		return false
	}
	for i := range init.Wallets {
		if ready.Fixtures[i].CurrentBinding.WalletOwnerID != init.Wallets[i].WalletOwnerID {
			return false
		}
	}
	return true
}

// readHostedControl distinguishes absence from corrupt/unreadable data. Strict
// reencoding checks required/duplicate/aliased fields and every nested order.
func readHostedControl(path string, maximum int64, value any) (bool, error) {
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > maximum {
		return false, pilot.ErrManifestInitialization
	}
	f, err := os.Open(path)
	if err != nil {
		return false, pilot.ErrManifestInitialization
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() {
		_ = f.Close()
		return false, pilot.ErrManifestInitialization
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, maximum+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || int64(len(raw)) > maximum || !utf8.Valid(raw) || json.Unmarshal(raw, value) != nil {
		return false, pilot.ErrManifestInitialization
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return false, pilot.ErrManifestInitialization
	}
	return true, nil
}
func readHostedInit(root string) (HostedInit, bool, error) {
	var v HostedInit
	found, err := readHostedControl(filepath.Join(root, hostedInitName), hostedMarkerMax, &v)
	if err == nil && found && !validHostedInit(v) {
		err = pilot.ErrManifestInitialization
	}
	return v, found, err
}
func readHostedReady(root string) (HostedReady, bool, error) {
	var v HostedReady
	found, err := readHostedControl(filepath.Join(root, hostedReadyName), hostedMarkerMax, &v)
	if err == nil && found && !validHostedReady(v) {
		err = pilot.ErrManifestInitialization
	}
	return v, found, err
}

func hostedDirectoryEntries(path string, maximum int) ([]os.DirEntry, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, pilot.ErrManifestInitialization
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, pilot.ErrManifestInitialization
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, pilot.ErrManifestInitialization
	}
	entries, readErr := f.ReadDir(maximum + 1)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > maximum {
		return nil, pilot.ErrManifestInitialization
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, pilot.ErrManifestInitialization
		}
	}
	return entries, nil
}

// Fixed preimages keep field order independent of map iteration.
func hostedInitialStageHash(init HostedInit, stage string) string {
	switch stage {
	case "authority":
		return hostedCanonicalHash(struct {
			AuthorityID          string `json:"authority_id"`
			IdentityConfigSHA256 string `json:"identity_config_sha256"`
		}{init.AuthorityID, init.IdentityConfigSHA256})
	case "coordinator":
		return hostedCanonicalHash(struct {
			CoordinatorID        string `json:"coordinator_id"`
			ProviderConfigSHA256 string `json:"provider_config_sha256"`
		}{init.CoordinatorID, init.ProviderConfigSHA256})
	case "driver":
		return hostedCanonicalHash(struct {
			ProviderDriverID     string `json:"provider_driver_id"`
			CoordinatorID        string `json:"coordinator_id"`
			ProviderConfigSHA256 string `json:"provider_config_sha256"`
		}{init.ProviderDriverID, init.CoordinatorID, init.ProviderConfigSHA256})
	case "vault":
		return hostedCanonicalHash(struct {
			HostedVaultID     string `json:"hosted_vault_id"`
			AuthorityID       string `json:"authority_id"`
			VaultConfigSHA256 string `json:"vault_config_sha256"`
		}{init.HostedVaultID, init.AuthorityID, init.VaultConfigSHA256})
	}
	return ""
}
func hostedReadyStageHash(init HostedInit, ready HostedReady, stage string) string {
	if h := hostedInitialStageHash(init, stage); h != "" {
		return h
	}
	switch stage {
	case "runtime":
		return hostedCanonicalHash(struct {
			SourceBindingID string `json:"source_binding_id"`
			DescriptorID    string `json:"descriptor_id"`
			AuthorityID     string `json:"authority_id"`
		}{ready.SourceBindingID, init.DescriptorID, init.AuthorityID})
	case "fixture-zero256":
		return hostedCanonicalHash(ready.Fixtures[0].CurrentBinding)
	case "fixture-incrementing256":
		return hostedCanonicalHash(ready.Fixtures[1].CurrentBinding)
	case "current":
		return hostedCanonicalHash(struct {
			Version  uint32                         `json:"version"`
			Bindings []recoveryvault.CurrentBinding `json:"bindings"`
		}{1, []recoveryvault.CurrentBinding{ready.Fixtures[0].CurrentBinding, ready.Fixtures[1].CurrentBinding}})
	}
	return ""
}

// readHostedSetupPrefix proves only the outer order and pins, never the inner
// store's validity. The constructor still audits each concrete store and checks
// its exact expected identity before accepting or recovering its receipt.
func readHostedSetupPrefix(root string, init HostedInit) ([]SetupReceipt, error) {
	return readHostedSetupPrefixMode(root, init, false)
}
func readHostedSetupPrefixMode(root string, init HostedInit, allocation bool) ([]SetupReceipt, error) {
	if !validHostedInit(init) {
		return nil, pilot.ErrManifestInitialization
	}
	limit := 11
	if allocation {
		limit += 2
	}
	entries, err := hostedDirectoryEntries(root, limit)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{"instance-lock": true, hostedInitName: true, hostedReadyName: true, "setup": true}
	if allocation {
		known[hostedAllocationGuardName] = true
		known["allocation-recovery"] = true
	}
	for _, name := range hostedStageDirectories {
		if name != "" {
			known[name] = true
		}
	}
	for _, entry := range entries {
		if !known[entry.Name()] {
			return nil, pilot.ErrManifestInitialization
		}
	}
	setup := filepath.Join(root, "setup")
	names := map[string]bool{}
	if _, err = os.Lstat(setup); !os.IsNotExist(err) {
		rows, e := hostedDirectoryEntries(setup, len(hostedStages))
		if e != nil {
			return nil, e
		}
		for _, entry := range rows {
			if entry.IsDir() {
				return nil, pilot.ErrManifestInitialization
			}
			names[entry.Name()] = true
		}
	}
	prefix := []SetupReceipt{}
	gap := false
	initHash := hostedCanonicalHash(init)
	for _, stage := range hostedStages {
		name := stage + ".json"
		if !names[name] {
			gap = true
			continue
		}
		delete(names, name)
		if gap {
			return nil, pilot.ErrManifestInitialization
		}
		var receipt SetupReceipt
		found, e := readHostedControl(filepath.Join(setup, name), hostedStageMax, &receipt)
		if e != nil || !found || receipt.Version != 1 || receipt.Stage != stage || receipt.InitSHA256 != initHash || !hostedHash(receipt.IdentitySHA256) {
			return nil, pilot.ErrManifestInitialization
		}
		if expected := hostedInitialStageHash(init, stage); expected != "" && receipt.IdentitySHA256 != expected {
			return nil, pilot.ErrManifestInitialization
		}
		prefix = append(prefix, receipt)
	}
	if len(names) != 0 {
		return nil, pilot.ErrManifestInitialization
	}
	for i, dir := range hostedStageDirectories {
		if dir == "" {
			continue
		}
		info, e := os.Lstat(filepath.Join(root, dir))
		if os.IsNotExist(e) {
			if i < len(prefix) {
				return nil, pilot.ErrManifestInitialization
			}
			continue
		}
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || i > len(prefix) {
			return nil, pilot.ErrManifestInitialization
		}
	}
	ready, exists, err := readHostedReady(root)
	if err != nil {
		return nil, err
	}
	if exists {
		if len(prefix) != len(hostedStages) || !hostedReadyMatches(init, ready) {
			return nil, pilot.ErrManifestInitialization
		}
		for _, receipt := range prefix {
			if receipt.IdentitySHA256 != hostedReadyStageHash(init, ready, receipt.Stage) {
				return nil, pilot.ErrManifestInitialization
			}
		}
	}
	return prefix, nil
}

// Exclusive link publication never replaces an established marker. The parent
// descriptor stays open through sync and identity checks. A failed publication
// is retained for exact reopen/audit, never repaired by selecting new IDs.
func writeHostedControl(path string, value any, maximum int) (resultErr error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw)+1 > maximum {
		return pilot.ErrManifestInitialization
	}
	raw = append(raw, '\n')
	parent, name := filepath.Dir(path), filepath.Base(path)
	before, err := os.Lstat(parent)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return pilot.ErrManifestInitialization
	}
	dir, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, dir.Close()) }()
	fd, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, fd.Close()) }()
	opened, err := fd.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return pilot.ErrManifestInitialization
	}
	var id [16]byte
	if _, err = io.ReadFull(rand.Reader, id[:]); err != nil {
		return err
	}
	temp := ".hosted-control-" + hex.EncodeToString(id[:])
	f, err := dir.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if e := dir.Remove(temp); e != nil && !os.IsNotExist(e) {
			resultErr = errors.Join(resultErr, e)
		}
	}()
	n, we := f.Write(raw)
	if we == nil && n != len(raw) {
		we = io.ErrShortWrite
	}
	err = errors.Join(we, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err = dir.Link(temp, name); err != nil {
		return err
	}
	if err = dir.Remove(temp); err != nil {
		return err
	}
	if err = fd.Sync(); err != nil {
		return err
	}
	after, err := os.Lstat(parent)
	if err != nil || !os.SameFile(before, after) {
		return pilot.ErrManifestInitialization
	}
	return nil
}
func writeHostedInit(root string, init HostedInit) error {
	if !validHostedInit(init) {
		return pilot.ErrManifestInitialization
	}
	if err := writeHostedControl(filepath.Join(root, hostedInitName), init, hostedMarkerMax); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(root))
	if err != nil {
		return err
	}
	return errors.Join(parent.Sync(), parent.Close())
}
func writeHostedSetupReceipt(root string, receipt SetupReceipt) error {
	return writeHostedSetupReceiptMode(root, receipt, false)
}
func writeHostedSetupReceiptMode(root string, receipt SetupReceipt, allocation bool) error {
	init, found, err := readHostedInit(root)
	if err != nil || !found {
		return pilot.ErrManifestInitialization
	}
	prefix, err := readHostedSetupPrefixMode(root, init, allocation)
	if err != nil {
		return err
	}
	if len(prefix) >= len(hostedStages) || receipt.Version != 1 || receipt.Stage != hostedStages[len(prefix)] || receipt.InitSHA256 != hostedCanonicalHash(init) || !hostedHash(receipt.IdentitySHA256) {
		return pilot.ErrManifestInitialization
	}
	if expected := hostedInitialStageHash(init, receipt.Stage); expected != "" && expected != receipt.IdentitySHA256 {
		return pilot.ErrManifestInitialization
	}
	if dir := hostedStageDirectories[len(prefix)]; dir != "" && !realDirectory(filepath.Join(root, dir)) {
		return pilot.ErrManifestInitialization
	}
	setup := filepath.Join(root, "setup")
	if err = os.Mkdir(setup, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if !realDirectory(setup) {
		return pilot.ErrManifestInitialization
	}
	if err = writeHostedControl(filepath.Join(setup, receipt.Stage+".json"), receipt, hostedStageMax); err != nil {
		return err
	}
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func writeHostedReady(root string, ready HostedReady) error {
	return writeHostedReadyMode(root, ready, false)
}
func writeHostedReadyMode(root string, ready HostedReady, allocation bool) error {
	init, found, err := readHostedInit(root)
	if err != nil || !found || !hostedReadyMatches(init, ready) {
		return pilot.ErrManifestInitialization
	}
	prefix, err := readHostedSetupPrefixMode(root, init, allocation)
	if err != nil || len(prefix) != len(hostedStages) {
		return pilot.ErrManifestInitialization
	}
	for _, receipt := range prefix {
		if receipt.IdentitySHA256 != hostedReadyStageHash(init, ready, receipt.Stage) {
			return pilot.ErrManifestInitialization
		}
	}
	return writeHostedControl(filepath.Join(root, hostedReadyName), ready, hostedMarkerMax)
}

func hostedRootPath(root string) error { return hostedRootPathMode(root, false) }
func hostedRootPathMode(root string, allocation bool) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\t") {
		return pilot.ErrManifestInitialization
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil || parent != filepath.Dir(root) {
		return pilot.ErrManifestInitialization
	}
	names := []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot", "LocalPoolPilot"}
	if allocation {
		names = append(names, "LocalHostedPoolPilot")
	} else {
		names = append(names, "LocalHostedRecoveryPilot")
	}
	for _, name := range names {
		paths := []string{filepath.Join(parent, name)}
		if config, e := os.UserConfigDir(); e == nil {
			paths = append(paths, filepath.Join(config, "SLUG", name))
		}
		for _, path := range paths {
			if real, e := filepath.EvalSymlinks(path); e == nil {
				path = real
			}
			if manifestContains(path, root) || manifestContains(root, path) {
				return pilot.ErrManifestInitialization
			}
		}
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"SLUG-ACCOUNTS.json", "SLUG-PILOT.json", "SLUG-MANIFEST-ACCOUNTS.json", "SLUG-MANIFEST-PILOT.json", "SLUG-POOL-ACCOUNTS.json", "SLUG-POOL-RUNTIME.json", hostedInitName, hostedReadyName, hostedAllocationGuardName} {
			if dir == root && (name == hostedInitName || name == hostedReadyName || allocation && name == hostedAllocationGuardName) {
				continue
			}
			if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				return pilot.ErrManifestInitialization
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return nil
}

// openHostedRoot acquires only the instance lock. The caller must validate its
// compiled configuration hashes and open each dependency under INIT's pins.
func openHostedRoot(root string, selection pilot.PoolOptions) (*leveldb.DB, error) {
	selection, err := pilot.ResolvePoolOptions(selection)
	if err != nil {
		return nil, err
	}
	if err = hostedRootPath(root); err != nil {
		return nil, err
	}
	if err = os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	check := func() (HostedInit, bool, error) {
		init, exists, e := readHostedInit(root)
		if e != nil {
			return init, exists, e
		}
		if exists {
			if init.DescriptorID != selection.CandidateDescriptorID {
				return init, true, pilot.ErrManifestInitialization
			}
			_, e = readHostedSetupPrefix(root, init)
			return init, true, e
		}
		entries, e := hostedDirectoryEntries(root, 1)
		if e != nil {
			return init, false, e
		}
		for _, entry := range entries {
			if entry.Name() != "instance-lock" || !entry.IsDir() {
				return init, false, pilot.ErrManifestInitialization
			}
		}
		return init, false, nil
	}
	before, exists, err := check()
	if err != nil {
		return nil, err
	}
	lock, err := leveldb.OpenFile(filepath.Join(root, "instance-lock"), nil)
	if err != nil {
		return nil, pilot.ErrManifestInitialization
	}
	after, still, err := check()
	if err != nil || exists != still || !reflect.DeepEqual(before, after) {
		return nil, errors.Join(pilot.ErrManifestInitialization, lock.Close())
	}
	return lock, nil
}
