// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

const manifestDescriptorID = "c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303"
const manifestMarkerName = "SLUG-MANIFEST-PILOT.json"
const manifestReadyName = "SLUG-MANIFEST-PILOT-READY.json"
const manifestChainsName = "SLUG-MANIFEST-PILOT-CHAINS.json"
const manifestMaxSafe = uint64(1<<53 - 1)

type manifestIdentity struct {
	Version            uint32                        `json:"version"`
	Mode               string                        `json:"mode"`
	Phase              string                        `json:"phase"`
	AccountAuthorityID string                        `json:"account_authority_id"`
	AccountID          string                        `json:"account_id"`
	Fixture            string                        `json:"fixture"`
	Network            uint32                        `json:"network"`
	Genesis            string                        `json:"genesis"`
	ActivationHeight   uint32                        `json:"activation_height"`
	RecoveryProfile    string                        `json:"recovery_profile"`
	AuthorityPort      uint16                        `json:"authority_port"`
	DescriptorID       string                        `json:"descriptor_id"`
	Identity           walletIdentity                `json:"identity"`
	Payout             walletPayout                  `json:"payout"`
	AuthorityPin       registry.ManifestAuthorityPin `json:"authority_pin"`
	TransportTrustID   string                        `json:"transport_trust_id"`
}

func (m manifestIdentity) origin() string {
	return "http://127.0.0.1:" + strconv.Itoa(int(m.AuthorityPort))
}
func (m manifestIdentity) workerConfig() prospectworker.ManifestConfig {
	return prospectworker.ManifestConfig{Version: 2, Policy: policy(), PublicKey: m.Identity.GymPublicKey, PayoutAddress: m.Payout.Address, AuthorityOrigin: m.origin(), AuthorityPin: m.AuthorityPin, TransportTrustID: m.TransportTrustID, MaxRecords: 1000, MaxBytes: 64 << 20}
}
func (m manifestIdentity) validReadyWallet() bool {
	f, e := gymaccount.FixtureIdentity(m.Fixture)
	return e == nil && m.Identity == (walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}) && m.Payout.Address != "" && m.Payout.Purpose == 44 && m.Payout.Coin == 0 && m.Payout.Account == 0 && m.Payout.Branch == 0 && m.Payout.Index == 0
}
func (m manifestIdentity) valid() bool {
	if m.Version != 1 || m.Mode != ManifestMode || !validAccountID(m.AccountAuthorityID) || !validAccountID(m.AccountID) || m.AuthorityPort == 0 || m.Network != recovery.Network || m.Genesis != genesis || m.ActivationHeight != chaincfg.PhaseAActivationHeight || m.RecoveryProfile != recovery.ProfileID || m.DescriptorID != manifestDescriptorID {
		return false
	}
	if _, e := gymaccount.FixtureIdentity(m.Fixture); e != nil {
		return false
	}
	if _, e := fighterdescriptor.ResolveCandidate(m.DescriptorID); e != nil {
		return false
	}
	switch m.Phase {
	case "new", "initializing":
		return m.Identity == (walletIdentity{}) && m.Payout == (walletPayout{}) && m.AuthorityPin == (registry.ManifestAuthorityPin{}) && m.TransportTrustID == ""
	case "ready":
		p := m.AuthorityPin
		if !m.validReadyWallet() || p.Version != 5 || p.Network != m.Network || p.Genesis != m.Genesis || p.DescriptorID != m.DescriptorID || !validAccountID(p.AuthorityID) {
			return false
		}
		trust, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: m.origin(), Token: fixtureToken, AuthorityPin: p})
		if e != nil || trust != m.TransportTrustID {
			return false
		}
		_, e = prospectworker.ValidateManifestConfig(m.workerConfig())
		return e == nil
	}
	return false
}
func readManifestJSON(path string, limit int64, v any) error {
	selected, e := os.Lstat(path)
	if e != nil || !selected.Mode().IsRegular() || selected.Size() > limit {
		return ErrManifestInitialization
	}
	f, e := os.Open(path)
	if e != nil {
		return ErrManifestInitialization
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !os.SameFile(selected, info) || !info.Mode().IsRegular() || info.Size() > limit {
		return ErrManifestInitialization
	}
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(raw)) > limit {
		return ErrManifestInitialization
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrManifestInitialization
	}
	canonical, e := json.Marshal(v)
	if e != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return ErrManifestInitialization
	}
	return nil
}
func manifestBindingMatches(m manifestIdentity, a ManifestAccountConfig) bool {
	return m.AccountID == a.Account.AccountID && m.AccountAuthorityID == a.Account.AuthorityID && m.Fixture == a.Account.Fixture
}
func manifestSameIntent(a, b manifestIdentity) bool {
	a.Phase = "initializing"
	a.Identity = walletIdentity{}
	a.Payout = walletPayout{}
	a.AuthorityPin = registry.ManifestAuthorityPin{}
	a.TransportTrustID = ""
	b.Phase = "initializing"
	b.Identity = walletIdentity{}
	b.Payout = walletPayout{}
	b.AuthorityPin = registry.ManifestAuthorityPin{}
	b.TransportTrustID = ""
	return a == b
}
func checkManifestChains(root string, m manifestIdentity) error {
	var saved manifestIdentity
	if readManifestJSON(filepath.Join(root, manifestChainsName), 8192, &saved) != nil || saved != m {
		return ErrManifestInitialization
	}
	return requireChainFiles(root)
}
func markManifestChains(root string, m manifestIdentity) error {
	if e := requireChainFiles(root); e != nil {
		return e
	}
	if _, e := os.Lstat(filepath.Join(root, manifestChainsName)); !os.IsNotExist(e) {
		return checkManifestChains(root, m)
	}
	return atomicJSON(filepath.Join(root, manifestChainsName), m)
}

func NewManifestAccount(config Config, a ManifestAccountConfig) (*ManifestController, error) {
	return newManifestAccount(config, a, false)
}

// validationOnly suppresses the existing READY-assisted marker promotion for the
// private root-upgrade preflight. It is never an authorization or IPC option.
func newManifestAccount(config Config, a ManifestAccountConfig, validationOnly bool) (*ManifestController, error) {
	if wire.TestPoW {
		return nil, errors.New("pilot requires production PoW")
	}
	if a.Account.AdmissionSigner != nil || a.ViewLease == nil || !validAccountID(a.ViewID) || a.Generation == 0 || a.Generation > manifestMaxSafe || !validAccountID(a.Account.AccountID) || !validAccountID(a.Account.AuthorityID) || a.Account.Authorizer == nil || a.Account.MiningToken == nil {
		return nil, errors.New("explicit manifest account authority is required")
	}
	if _, e := gymaccount.FixtureIdentity(a.Account.Fixture); e != nil {
		return nil, ErrManifestInitialization
	}
	if !filepath.IsAbs(config.DataDir) || !filepath.IsAbs(config.NodeBinary) || strings.ContainsAny(config.DataDir+config.NodeBinary, "\r\n\t\x00") {
		return nil, ErrManifestInitialization
	}
	config.DataDir = filepath.Clean(config.DataDir)
	config.NodeBinary = filepath.Clean(config.NodeBinary)
	if e := manifestManagedRoot(config.DataDir); e != nil {
		return nil, e
	}
	if info, e := os.Stat(config.NodeBinary); e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("bundled node binary is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.NodeBinary, "--version")
	var out limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if cmd.Run() != nil || strings.TrimSpace(out.String()) != "slugd version 0.27.0" {
		return nil, errors.New("normal SLUG node binary is required")
	}
	if e := os.Mkdir(config.DataDir, 0700); e != nil && !os.IsExist(e) {
		return nil, ErrManifestInitialization
	}
	if i, e := os.Lstat(config.DataDir); e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return nil, ErrManifestInitialization
	}
	marker := filepath.Join(config.DataDir, manifestMarkerName)
	if _, e := os.Lstat(marker); os.IsNotExist(e) {
		entries, e := os.ReadDir(config.DataDir)
		if e != nil {
			return nil, ErrManifestInitialization
		}
		for _, f := range entries {
			if f.Name() != "instance-lock" {
				return nil, ErrManifestInitialization
			}
		}
	} else if e != nil {
		return nil, ErrManifestInitialization
	}
	// Validate an existing marker before even opening/creating its lock store.
	if _, e := os.Lstat(marker); e == nil {
		var before manifestIdentity
		if readManifestJSON(marker, 8192, &before) != nil || !before.valid() || !manifestBindingMatches(before, a) {
			return nil, ErrManifestInitialization
		}
		if a.Account.RequireExisting && before.Phase != "ready" && before.Phase != "initializing" {
			return nil, ErrManifestInitialization
		}
	}
	if plainChild(filepath.Join(config.DataDir, "instance-lock"), true) != nil {
		return nil, ErrManifestInitialization
	}
	lock, e := leveldb.OpenFile(filepath.Join(config.DataDir, "instance-lock"), nil)
	if e != nil {
		return nil, errors.New("pilot is already open or unavailable")
	}
	keep := false
	defer func() {
		if !keep {
			_ = lock.Close()
		}
	}()
	var m manifestIdentity
	if _, e = os.Lstat(marker); os.IsNotExist(e) {
		if a.Account.RequireExisting {
			return nil, ErrManifestInitialization
		}
		entries, e := os.ReadDir(config.DataDir)
		if e != nil || len(entries) != 1 || entries[0].Name() != "instance-lock" {
			return nil, ErrManifestInitialization
		}
		port, e := unusedPort()
		if e != nil {
			return nil, e
		}
		m = manifestIdentity{Version: 1, Mode: ManifestMode, Phase: "new", AccountAuthorityID: a.Account.AuthorityID, AccountID: a.Account.AccountID, Fixture: a.Account.Fixture, Network: policy().Network, Genesis: genesis, ActivationHeight: chaincfg.PhaseAActivationHeight, RecoveryProfile: recovery.ProfileID, AuthorityPort: port, DescriptorID: manifestDescriptorID}
		if atomicJSON(marker, m) != nil {
			return nil, ErrManifestInitialization
		}
		for _, name := range []string{"primary-chain", "peer-chain"} {
			if os.Mkdir(filepath.Join(config.DataDir, name), 0700) != nil {
				return nil, ErrManifestInitialization
			}
		}
	} else if e != nil || readManifestJSON(marker, 8192, &m) != nil || !m.valid() || !manifestBindingMatches(m, a) {
		return nil, ErrManifestInitialization
	}
	promote := m.Phase == "initializing"
	if promote {
		var ready manifestIdentity
		if readManifestJSON(filepath.Join(config.DataDir, manifestReadyName), 8192, &ready) != nil || ready.Phase != "ready" || !ready.valid() || !manifestSameIntent(m, ready) || checkManifestChains(config.DataDir, ready) != nil {
			return nil, ErrManifestInitialization
		}
		// Validate every existing component below before promoting the primary marker.
		m = ready
	}
	if a.Account.RequireExisting && m.Phase != "ready" {
		return nil, ErrManifestInitialization
	}
	for _, name := range []string{"wallet", "manifest-authority", "manifest-outbox", "primary-chain", "peer-chain"} {
		if plainChild(filepath.Join(config.DataDir, name), true) != nil {
			return nil, ErrManifestInitialization
		}
	}
	c := &ManifestController{config: config, account: &a.Account, view: a, manifest: m, lock: lock, snapshot: BlankManifestSnapshot(), cache: make(map[uint64]*manifestCached)}
	if m.Phase == "new" {
		if c.checkNewData() != nil {
			return nil, ErrManifestInitialization
		}
	} else {
		var ready manifestIdentity
		if readManifestJSON(filepath.Join(config.DataDir, manifestReadyName), 8192, &ready) != nil || ready != m || checkManifestChains(config.DataDir, m) != nil {
			return nil, ErrManifestInitialization
		}
		w, e := c.openWallet(false)
		if e != nil {
			return nil, ErrManifestInitialization
		}
		id := w.Identity()
		ce := w.Close()
		if id != m.Identity || ce != nil {
			return nil, ErrManifestInitialization
		}
		if c.openStores(false) != nil {
			return nil, ErrManifestInitialization
		}
		info, e := c.store.Info()
		ce = c.closeStores()
		if e != nil || ce != nil {
			return nil, ErrManifestInitialization
		}
		c.snapshot.AcceptedCount = info.Records
		var status savedStatus
		if _, e := os.Lstat(filepath.Join(config.DataDir, "status.json")); e == nil {
			if readManifestJSON(filepath.Join(config.DataDir, "status.json"), 2048, &status) != nil || status.Attempts > manifestMaxSafe || status.Height > manifestMaxSafe || status.WalletHeight > manifestMaxSafe || status.SpendableAtoms < 0 || status.ImmatureAtoms < 0 || uint64(status.SpendableAtoms) > manifestMaxSafe || uint64(status.ImmatureAtoms) > manifestMaxSafe {
				return nil, ErrManifestInitialization
			}
			c.snapshot.Attempts, c.snapshot.Height, c.snapshot.WalletHeight = status.Attempts, status.Height, status.WalletHeight
			c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = status.SpendableAtoms, status.ImmatureAtoms
		} else if !os.IsNotExist(e) {
			return nil, ErrManifestInitialization
		}
		if promote && !validationOnly && atomicJSON(marker, m) != nil {
			return nil, ErrManifestInitialization
		}
	}
	c.snapshot.GymID, c.snapshot.GymPublicKey, c.snapshot.PayoutAddress = m.Identity.GymID, m.Identity.GymPublicKey, m.Payout.Address
	if m.Phase == "ready" {
		p := c.viewPin()
		c.snapshot.View = &p
	}
	keep = true
	return c, nil
}
func (c *ManifestController) checkNewData() error {
	entries, e := os.ReadDir(c.config.DataDir)
	if e != nil {
		return e
	}
	allowed := map[string]bool{"instance-lock": true, manifestMarkerName: true, "primary-chain": true, "peer-chain": true}
	for _, v := range entries {
		if !allowed[v.Name()] {
			return ErrManifestInitialization
		}
	}
	for _, n := range []string{"primary-chain", "peer-chain"} {
		p := filepath.Join(c.config.DataDir, n)
		i, e := os.Lstat(p)
		if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return ErrManifestInitialization
		}
		v, e := os.ReadDir(p)
		if e != nil || len(v) != 0 {
			return ErrManifestInitialization
		}
	}
	return nil
}
func (c *ManifestController) openStores(create bool) error {
	c.resources.Lock()
	defer c.resources.Unlock()
	var e error
	if create {
		c.store, e = registry.CreateManifestAuthority(filepath.Join(c.config.DataDir, "manifest-authority"), policy().Network, genesis, manifestDescriptorID)
	} else {
		c.store, e = registry.OpenManifestAuthority(filepath.Join(c.config.DataDir, "manifest-authority"), c.manifest.AuthorityPin, false)
	}
	if e != nil {
		return errors.New("manifest authority unavailable")
	}
	info, e := c.store.Info()
	if e != nil {
		_ = c.store.Close()
		c.store = nil
		return ErrManifestInitialization
	}
	if create {
		m := c.manifest
		m.AuthorityPin = info.Pin
		m.TransportTrustID, e = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: m.origin(), Token: fixtureToken, AuthorityPin: m.AuthorityPin})
		if e == nil && !m.valid() {
			e = ErrManifestInitialization
		}
		if e != nil {
			_ = c.store.Close()
			c.store = nil
			return e
		}
		c.mu.Lock()
		c.manifest = m
		c.mu.Unlock()
	}
	if info.Pin != c.manifest.AuthorityPin || info.Records > 1000 || info.Jobs > 1000 {
		_ = c.store.Close()
		c.store = nil
		return ErrManifestInitialization
	}
	path := filepath.Join(c.config.DataDir, "manifest-outbox")
	if create {
		c.box, e = prospectworker.CreateManifestOutbox(path, c.manifest.workerConfig())
	} else {
		c.box, e = prospectworker.OpenManifestOutbox(path, c.manifest.workerConfig())
	}
	if e != nil {
		_ = c.store.Close()
		c.store = nil
		return errors.New("manifest outbox unavailable")
	}
	var after, pending uint64
	for {
		entries, err := c.box.List(after, 1)
		if err != nil {
			_ = c.box.Close()
			c.box = nil
			_ = c.store.Close()
			c.store = nil
			return errors.New("manifest outbox unavailable")
		}
		if len(entries) == 0 {
			break
		}
		entry := entries[0]
		after = entry.Sequence
		blockDone := entry.BlockState == "not-applicable" || entry.BlockState == "canonical" || entry.BlockState == "rejected"
		admissionDone := entry.AdmissionState == "not-applicable" || entry.AdmissionState == "accepted" || entry.AdmissionState == "expired" || entry.AdmissionState == "rejected"
		if !blockDone || !admissionDone {
			pending++
		}
	}
	c.mu.Lock()
	c.snapshot.Pending = pending
	c.mu.Unlock()
	return nil
}
func (c *ManifestController) closeStores() error {
	c.resources.Lock()
	defer c.resources.Unlock()
	var e error
	if c.box != nil {
		e = errors.Join(e, c.box.Close())
		c.box = nil
	}
	if c.store != nil {
		e = errors.Join(e, c.store.Close())
		c.store = nil
	}
	if e != nil {
		return errors.New("pilot storage close failed")
	}
	return nil
}

// Refuse old managed roots in either containment direction; never adopt their
// data via a new constructor. The explicit account parent remains permitted.
func manifestManagedRoot(path string) error {
	for p := path; ; p = filepath.Dir(p) {
		for _, name := range []string{"SLUG-PILOT.json", "SLUG-ACCOUNTS.json"} {
			if _, e := os.Lstat(filepath.Join(p, name)); e == nil {
				return ErrManifestInitialization
			}
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	for _, base := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot"} {
		if filepath.Base(path) == base {
			return ErrManifestInitialization
		}
	}
	return nil
}
