// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net"

	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolschedule"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

const MaxSetupBytes = 262144

// Setup is independently selected public operator configuration. It never
// carries credentials, private paths, or evidence authorizing a payment.
type Setup struct {
	Version               uint32                       `json:"version"`
	RuntimeID             string                       `json:"runtime_id"`
	Authority             registry.PublicManifestPin   `json:"authority"`
	OwnershipStoreID      string                       `json:"ownership_store_id"`
	Pool                  poolshare.Config             `json:"pool"`
	RoundLimits           poolround.Limits             `json:"round_limits"`
	Settlement            poolsettle.Config            `json:"settlement"`
	Schedule              poolschedule.Config          `json:"schedule"`
	Miners                poolminer.Config             `json:"miners"`
	WalletGuard           waddrmgr.PoolSettlementGuard `json:"wallet_guard"`
	Node                  poolsettlenode.Config        `json:"node"`
	Ingress               poolingress.Config           `json:"ingress"`
	MaintenanceIntervalMS int64                        `json:"maintenance_interval_ms"`
	UnlockLifetimeMS      int64                        `json:"unlock_lifetime_ms"`
	UnlockRenewalMS       int64                        `json:"unlock_renewal_ms"`
}

func identifier(s string) bool {
	if len(s) != 64 {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(b) == s && !bytes.Equal(b, make([]byte, 32))
}

func framedHash(label string, raw []byte) string { return framedHashVersion(label, raw, 1) }
func framedHashVersion(label string, raw []byte, version uint32) string {
	h := sha256.New()
	h.Write([]byte(label))
	h.Write([]byte{0, 'v', byte('0' + version), 0})
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

func (s Setup) walletBinding() wallet.PoolWalletBinding {
	return wallet.PoolWalletBinding{Version: s.Version, WalletOwnerID: s.WalletGuard.WalletOwnerID, OperatorProfileID: s.WalletGuard.OperatorProfileID, Source: s.Pool, Settlement: s.Settlement, Guard: s.WalletGuard}
}

func (s Setup) minerBinding(profile registry.OperatorProfile) poolminer.Binding {
	return poolminer.Binding{Profile: profile, Source: s.Pool, Settlement: s.Settlement}
}

// ValidateSetup is side-effect-free. Constructors repeat the original concrete
// owners' audits; this validation and its digest cannot replace them.
func ValidateSetup(s Setup) error { return validateSetupVersion(s, 1) }
func validateSetupVersion(s Setup, version uint32) error {
	if (version != 1 && version != 2) || s.Version != version || !identifier(s.RuntimeID) || !identifier(s.OwnershipStoreID) {
		return ErrInvalid
	}
	p, e := registry.NewOperatorProfile(s.Authority)
	if e != nil || p.Pin().Source != s.Pool.AuthorityPin {
		return ErrInvalid
	}
	minerValidate := poolminer.ValidateConfig
	walletValidate := wallet.ValidatePoolWalletBinding
	ingressValidate := poolingress.ValidateConfig
	if version == 2 {
		minerValidate = poolminer.ValidateDailyConfig
		walletValidate = wallet.ValidateDailyPoolWalletBinding
		ingressValidate = poolingress.ValidateDailyConfig
	}
	if s.Miners.RuntimeID != s.RuntimeID || minerValidate(s.Miners, s.minerBinding(p)) != nil {
		return ErrInvalid
	}
	if walletValidate(s.walletBinding()) != nil || s.WalletGuard.OperatorProfileID != p.ID() {
		return ErrInvalid
	}
	if poolsettlenode.ValidateConfig(s.Node) != nil || s.Node.Network != s.Authority.Source.Network || s.Node.Genesis != s.Authority.Source.Genesis || s.Node.PoolPublicKey != s.WalletGuard.FundingPublicKey || s.Node.PoolScript != s.WalletGuard.FundingScript {
		return ErrInvalid
	}
	l := s.RoundLimits
	maxBytes := uint64(1 << 30)
	if version == 2 {
		maxBytes = 8 << 30
	}
	if l.Version != version || l.MaxRounds != uint64(s.Settlement.MaxRounds) || l.MaxRounds < uint64(s.Pool.MaxWorks) || l.MaxBytes < 4096 || l.MaxBytes > maxBytes {
		return ErrInvalid
	}
	c := s.Schedule
	if c.Version != version || !identifier(c.SchedulerID) || c.RuntimeID != s.RuntimeID || c.DailyUTCMinute > 1439 || c.RetryMS < 1000 || c.RetryMS > 3600000 {
		return ErrInvalid
	}
	raw, e := json.Marshal(struct {
		Config poolsettle.Config `json:"config"`
		Source poolshare.Config  `json:"source"`
	}{s.Settlement, s.Pool})
	binding := framedHash("SLUG pool settlement binding", raw)
	if version == 2 {
		binding, e = poolschedule.DailySettlementConfigBinding(s.Settlement, s.Pool)
	}
	if e != nil || c.SettlementBindingID != binding {
		return ErrInvalid
	}
	if ingressValidate(s.Ingress) != nil || s.Ingress.PoolURL == s.Ingress.AdmissionURL || s.Ingress.PoolURL == s.Ingress.OwnershipURL || s.Ingress.AdmissionURL == s.Ingress.OwnershipURL {
		return ErrInvalid
	}
	_, port, e := net.SplitHostPort(s.Ingress.Listen)
	if e != nil || port == "0" {
		return ErrInvalid
	}
	if s.MaintenanceIntervalMS < 1000 || s.MaintenanceIntervalMS > 60000 || s.UnlockLifetimeMS < 60000 || s.UnlockLifetimeMS > 86400000 || s.UnlockRenewalMS < 1000 || s.UnlockRenewalMS > 3600000 || s.UnlockRenewalMS > s.UnlockLifetimeMS/2 {
		return ErrInvalid
	}
	return nil
}

func MarshalSetup(s Setup) ([]byte, error) { return marshalSetupVersion(s, 1) }
func marshalSetupVersion(s Setup, version uint32) ([]byte, error) {
	if e := validateSetupVersion(s, version); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(s)
	if e != nil || len(raw) > MaxSetupBytes {
		return nil, ErrInvalid
	}
	return raw, nil
}

func ParseSetup(raw []byte) (Setup, error) { return parseSetupVersion(raw, 1) }
func parseSetupVersion(raw []byte, version uint32) (Setup, error) {
	var s Setup
	if len(raw) == 0 || len(raw) > MaxSetupBytes {
		return s, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		return Setup{}, ErrInvalid
	}
	canonical, e := marshalSetupVersion(s, version)
	if e != nil || !bytes.Equal(canonical, raw) {
		return Setup{}, ErrInvalid
	}
	return s, nil
}

func SetupID(s Setup) (string, error) { return setupIDVersion(s, 1) }
func setupIDVersion(s Setup, version uint32) (string, error) {
	raw, e := marshalSetupVersion(s, version)
	if e != nil {
		return "", e
	}
	return framedHashVersion("SLUG pool operator setup", raw, version), nil
}

type readyRecord struct {
	Version   uint32 `json:"version"`
	RuntimeID string `json:"runtime_id"`
	SetupID   string `json:"setup_id"`
}

func readyBytes(s Setup) ([]byte, error) { return readyBytesVersion(s, 1) }
func readyBytesVersion(s Setup, version uint32) ([]byte, error) {
	id, e := setupIDVersion(s, version)
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(readyRecord{version, s.RuntimeID, id})
	return append(raw, '\n'), e
}
