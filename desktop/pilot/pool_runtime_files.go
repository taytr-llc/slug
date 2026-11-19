// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

const poolRuntimeMarkerName = "SLUG-POOL-RUNTIME.json"
const poolRuntimeSourceName = "SLUG-POOL-SOURCE.json"
const poolRuntimeCoreName = "SLUG-POOL-CORE.json"
const poolRuntimeReadyName = "SLUG-POOL-READY.json"
const poolRuntimeGymName = "SLUG-POOL-GYM.json"
const poolRuntimeGymReadyName = "SLUG-POOL-GYM-READY.json"
const poolRuntimeMarkerLimit = 32 << 10

type poolRuntimeMarker struct {
	Application        string `json:"application"`
	Version            uint32 `json:"version"`
	AccountAuthorityID string `json:"account_authority_id"`
	Network            uint32 `json:"network"`
	Genesis            string `json:"genesis"`
	ActivationHeight   uint32 `json:"activation_height"`
	RecoveryProfile    string `json:"recovery_profile"`
	DescriptorID       string `json:"descriptor_id"`
	PrimaryTXIndex     bool   `json:"primary_txindex"`
	PoolPort           uint16 `json:"pool_port"`
	AdmissionPort      uint16 `json:"admission_port"`
}

func (m poolRuntimeMarker) valid(authority string) bool {
	return m.validFor(authority, manifestDescriptorID)
}
func (m poolRuntimeMarker) validFor(authority, descriptorID string) bool {
	options, err := ResolvePoolOptions(PoolOptions{CandidateDescriptorID: descriptorID})
	if err != nil || options.CandidateDescriptorID != descriptorID {
		return false
	}
	return m.Application == "slug-shared-pool-runtime" && m.Version == 1 && validAccountID(authority) && m.AccountAuthorityID == authority && m.Network == policy().Network && m.Genesis == genesis && m.ActivationHeight == chaincfg.PhaseAActivationHeight && m.RecoveryProfile == recovery.ProfileID && m.DescriptorID == descriptorID && m.PrimaryTXIndex && m.PoolPort != 0 && m.AdmissionPort != 0 && m.PoolPort != m.AdmissionPort
}
func poolRuntimeOrigin(port uint16) string { return "http://127.0.0.1:" + strconv.Itoa(int(port)) }

func poolRuntimePath(root string) error {
	if e := manifestManagedRoot(root); e != nil {
		return e
	}
	if filepath.Base(root) == "LocalManifestPilot" {
		return ErrPoolRuntime
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(root))
	if e != nil || parent != filepath.Dir(root) {
		return ErrPoolRuntime
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		for _, marker := range []string{"SLUG-MANIFEST-PILOT.json", "SLUG-MANIFEST-ACCOUNTS.json", poolRuntimeMarkerName, "SLUG-POOL-ACCOUNTS.json"} {
			_, e := os.Lstat(filepath.Join(dir, marker))
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return ErrPoolRuntime
			}
			if marker == poolRuntimeMarkerName && dir == root {
				continue
			}
			if marker == "SLUG-POOL-ACCOUNTS.json" && dir == parent && filepath.Base(root) == "shared" {
				continue
			}
			return ErrPoolRuntime
		}
		if dir == filepath.Dir(dir) {
			return nil
		}
	}
}

func (m poolRuntimeMarker) source(pin registry.ManifestAuthorityPin) poolshare.Config {
	return poolshare.Config{Version: 1, AuthorityPin: pin, Policy: policy(), PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: target, MaxWorks: 256, MaxSubmissions: 4096, MaxBytes: 256 << 20}
}

type poolRuntimeCore struct {
	Version uint32 `json:"version"`
	Binding string `json:"source_binding_id"`
}
type poolRuntimeReady struct {
	Version    uint32                 `json:"version"`
	Binding    string                 `json:"source_binding_id"`
	Recipients []PoolRecipientBinding `json:"recipients"`
}
type poolRuntimeGym struct {
	Version            uint32 `json:"version"`
	AccountAuthorityID string `json:"account_authority_id"`
	AccountID          string `json:"account_id"`
	Fixture            string `json:"fixture"`
}

func poolRuntimeExists(path string) (bool, error) {
	i, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil || i.Mode()&os.ModeSymlink != 0 {
		return false, ErrPoolRuntime
	}
	return true, nil
}

// Immutable intent/publication only. Matching bytes after a lost directory
// sync are synchronized again; another value is never silently overwritten.
func writePoolRuntimeRecord(path string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || len(raw)+1 > poolRuntimeMarkerLimit {
		return ErrPoolRuntime
	}
	if exists, e := poolRuntimeExists(path); e != nil {
		return e
	} else if exists {
		var existing json.RawMessage
		if readManifestJSON(path, poolRuntimeMarkerLimit, &existing) != nil || !bytes.Equal(existing, raw) {
			return ErrPoolRuntime
		}
		return syncDirectory(filepath.Dir(path))
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pool-runtime-")
	if e != nil {
		return ErrPoolRuntime
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
		return ErrPoolRuntime
	}
	if e = os.Link(name, path); e != nil {
		if os.IsExist(e) {
			return writePoolRuntimeRecord(path, value)
		}
		return ErrPoolRuntime
	}
	return syncDirectory(filepath.Dir(path))
}

func readPoolRuntimeRecord(path string, expected any) error {
	var raw json.RawMessage
	if readManifestJSON(path, poolRuntimeMarkerLimit, &raw) != nil {
		return ErrPoolRuntime
	}
	want, e := json.Marshal(expected)
	if e != nil || !bytes.Equal(raw, want) {
		return ErrPoolRuntime
	}
	return nil
}

func poolRuntimeRootFiles(root string) error {
	allowed := map[string]bool{poolRuntimeMarkerName: true, poolRuntimeSourceName: true, poolRuntimeCoreName: true, poolRuntimeReadyName: true, "instance-lock": true, "issuer": true, "shares": true, "rounds": true, "settlement": true, "gyms": true, poolLeagueDirectory: true, poolPipelineDirectory: true,
		poolWithdrawalGuardName: true, poolWithdrawalDirectory: true,
		"primary-chain": true, "primary-logs": true, "primary.conf": true, "primary-process.log": true, "peer-chain": true, "peer-logs": true, "peer.conf": true, "peer-process.log": true}
	entries, e := os.ReadDir(root)
	if e != nil {
		return ErrPoolRuntime
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.Type()&os.ModeSymlink != 0 {
			return ErrPoolRuntime
		}
	}
	return nil
}

func (r *PoolRuntime) loadPoolRecipients() error {
	r.gymAccounts = make(map[string]string)
	dir := filepath.Join(r.config.DataDir, "gyms")
	if e := plainChild(dir, true); e != nil {
		return e
	}
	if e := os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) > 2 {
		return ErrPoolRuntime
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !validAccountID(entry.Name()) {
			return ErrPoolRuntime
		}
		root := filepath.Join(dir, entry.Name())
		var gym poolRuntimeGym
		if readManifestJSON(filepath.Join(root, poolRuntimeGymName), poolRuntimeMarkerLimit, &gym) != nil || gym.Version != 1 || gym.AccountID != entry.Name() || gym.AccountAuthorityID != r.marker.AccountAuthorityID || (gym.Fixture != "zero256" && gym.Fixture != "incrementing256") {
			return ErrPoolRuntime
		}
		if _, exists := r.gymAccounts[gym.Fixture]; exists {
			return ErrPoolRuntime
		}
		r.gymAccounts[gym.Fixture] = gym.AccountID
		var binding PoolRecipientBinding
		path := filepath.Join(root, poolRuntimeGymReadyName)
		exists, e := poolRuntimeExists(path)
		if e != nil {
			return e
		}
		if !exists {
			continue
		}
		if readManifestJSON(path, poolRuntimeMarkerLimit, &binding) != nil || ValidatePoolRecipientBinding(binding) != nil || binding.Config.AccountID != gym.AccountID || binding.Config.AccountAuthorityID != gym.AccountAuthorityID || binding.Config.Fixture != gym.Fixture || binding.Config.Source != r.source {
			return ErrPoolRuntime
		}
		if _, exists := r.recipients[gym.Fixture]; exists {
			return ErrPoolRuntime
		}
		if readPoolRecipientMarker(filepath.Join(root, poolRecipientIntentName), recipientMarker(binding, "initializing")) != nil || readPoolRecipientMarker(filepath.Join(root, poolRecipientReadyName), recipientMarker(binding, "ready")) != nil {
			return ErrPoolRuntime
		}
		if exists, e := poolRuntimeExists(filepath.Join(root, "wallet", wallet.WalletDBName)); e != nil || !exists {
			return ErrPoolRuntime
		}
		r.recipients[gym.Fixture] = binding
	}
	return nil
}

func (r *PoolRuntime) coreRecord() (poolRuntimeCore, error) {
	b, e := poolservice.BindingID(r.source)
	return poolRuntimeCore{1, b}, e
}
