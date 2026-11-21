// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolschedule"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/waddrmgr"
)

const MaxProvisionPolicyBytes = 65536
const provisionScope = "phase-a-finite-operator"

var (
	ErrProvisionInvalid          = errors.New("invalid operator provisioning selection")
	ErrProvisionIncompleteWallet = errors.New("operator provisioning wallet incomplete; preserve original files")
	ErrProvisionIncomplete       = errors.New("operator provisioning incomplete; preserve original files")
)

type ProvisionMining struct {
	DNAVersion     uint32 `json:"dna_version"`
	ProspectTarget string `json:"prospect_target"`
	PoolTarget     string `json:"pool_target"`
}
type ProvisionLimits struct {
	MaxWorks       uint64 `json:"max_works"`
	MaxSubmissions uint64 `json:"max_submissions"`
	PoolMaxBytes   uint64 `json:"pool_max_bytes"`
	MaxRounds      uint32 `json:"max_rounds"`
	MaxBatches     uint32 `json:"max_batches"`
	RoundMaxBytes  uint64 `json:"round_max_bytes"`
}
type ProvisionEnrollment struct {
	PublicKey   string `json:"public_key"`
	PayeeScript string `json:"payee_script"`
}
type ProvisionSchedule struct {
	DailyUTCMinute uint32 `json:"daily_utc_minute"`
	RetryMS        int64  `json:"retry_ms"`
}
type ProvisionTransport struct {
	Listen       string `json:"listen"`
	Origin       string `json:"origin"`
	PoolURL      string `json:"pool_url"`
	AdmissionURL string `json:"admission_url"`
	OwnershipURL string `json:"ownership_url"`
}
type ProvisionPolicy struct {
	Version               uint32                   `json:"version"`
	Scope                 string                   `json:"scope"`
	RequestID             string                   `json:"request_id"`
	Network               uint32                   `json:"network"`
	Genesis               string                   `json:"genesis"`
	DescriptorID          string                   `json:"descriptor_id"`
	CheckpointPublicKey   string                   `json:"checkpoint_public_key"`
	Mining                ProvisionMining          `json:"mining"`
	Limits                ProvisionLimits          `json:"limits"`
	Enrollments           []ProvisionEnrollment    `json:"enrollments"`
	Schedule              ProvisionSchedule        `json:"schedule"`
	Fee                   poolsettlenode.FeePolicy `json:"fee"`
	Transport             ProvisionTransport       `json:"transport"`
	MaintenanceIntervalMS int64                    `json:"maintenance_interval_ms"`
	UnlockLifetimeMS      int64                    `json:"unlock_lifetime_ms"`
	UnlockRenewalMS       int64                    `json:"unlock_renewal_ms"`
}

// Public projections confer no original-file or custody authority.
type ProvisionPublicMaterial struct {
	Version          uint32 `json:"version"`
	CreatedAtMS      int64  `json:"created_at_ms"`
	RootPublicKey    string `json:"root_public_key"`
	FundingPublicKey string `json:"funding_public_key"`
	AdmissionKey     string `json:"admission_key"`
	PublicationKey   string `json:"publication_key"`
	CutoffKey        string `json:"cutoff_key"`
}
type ProvisionCertificate struct {
	Version           uint32 `json:"version"`
	CertificateSHA256 string `json:"certificate_sha256"`
	CertificateSPKI   string `json:"certificate_spki"`
}

func provisionTime(n int64) bool { return n >= 1 && n <= registry.MaxPublicTimestampMS }
func provisionPoint(s string) bool {
	if len(s) != 66 {
		return false
	}
	b, e := hex.DecodeString(s)
	if e != nil || hex.EncodeToString(b) != s {
		return false
	}
	p, e := btcec.ParsePubKey(b)
	return e == nil && bytes.Equal(b, p.SerializeCompressed())
}
func provisionScript(s string) bool {
	if len(s) != 44 || s[:4] != "0014" {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(b) == s
}
func provisionPolicyContext(p ProvisionPolicy) prospect.Context {
	return prospect.Context{Version: 1, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.Mining.DNAVersion}
}
func provisionIngress(p ProvisionPolicy, spki string) poolingress.Config {
	t := p.Transport
	return poolingress.Config{Version: p.Version, Listen: t.Listen, Origin: t.Origin, CertificateSPKI: spki, PoolURL: t.PoolURL, AdmissionURL: t.AdmissionURL, OwnershipURL: t.OwnershipURL}
}
func ValidateProvisionPolicy(p ProvisionPolicy) error { return validateProvisionPolicyVersion(p, 1) }
func validateProvisionPolicyVersion(p ProvisionPolicy, version uint32) error {
	if (version != 1 && version != 2) || p.Version != version || p.Scope != provisionScope || !identifier(p.RequestID) || p.Network != uint32(chaincfg.MainNetParams.Net) || p.Genesis != chaincfg.MainNetParams.GenesisHash.String() || p.CheckpointPublicKey != hex.EncodeToString(chaincfg.MainNetParams.CheckpointPublicKey) || !identifier(p.DescriptorID) || !identifier(p.Mining.ProspectTarget) || !identifier(p.Mining.PoolTarget) {
		return ErrProvisionInvalid
	}
	d, e := fighterdescriptor.ResolveCandidate(p.DescriptorID)
	if e != nil || d.DNAVersion != p.Mining.DNAVersion {
		return ErrProvisionInvalid
	}
	mining := prospectnode.Policy{Version: 1, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.Mining.DNAVersion, ProspectTarget: p.Mining.ProspectTarget}
	raw, _ := json.Marshal(mining)
	got, e := prospectnode.ParsePolicy(raw)
	if e != nil || got != mining {
		return ErrProvisionInvalid
	}
	l := p.Limits
	maxWorks, maxRows, maxBytes, maxRounds, maxBatches := uint64(256), uint64(4096), uint64(1<<30), uint32(256), uint32(366)
	ingressValidate := poolingress.ValidateConfig
	if version == 2 {
		maxWorks, maxRows, maxBytes, maxRounds, maxBatches = 8192, 16384, 8<<30, 16384, 32767
		ingressValidate = poolingress.ValidateDailyConfig
	}
	if l.MaxWorks < 1 || l.MaxWorks > maxWorks || l.MaxSubmissions < 1 || l.MaxSubmissions > maxRows || l.PoolMaxBytes < 4096 || l.PoolMaxBytes > maxBytes || l.MaxRounds < 1 || l.MaxRounds > maxRounds || uint64(l.MaxRounds) < l.MaxWorks || l.MaxBatches < 1 || l.MaxBatches > maxBatches || l.RoundMaxBytes < 4096 || l.RoundMaxBytes > maxBytes {
		return ErrProvisionInvalid
	}
	if len(p.Enrollments) < 1 || len(p.Enrollments) > 64 {
		return ErrProvisionInvalid
	}
	seen := map[string]bool{}
	last := ""
	for _, en := range p.Enrollments {
		if !provisionPoint(en.PublicKey) || !provisionScript(en.PayeeScript) || seen[en.PublicKey] || seen[en.PayeeScript] {
			return ErrProvisionInvalid
		}
		id, e := gymauth.GymID(provisionPolicyContext(p), en.PublicKey)
		if e != nil || !identifier(id) || id <= last {
			return ErrProvisionInvalid
		}
		last = id
		seen[en.PublicKey] = true
		seen[en.PayeeScript] = true
	}
	if p.Schedule.DailyUTCMinute > 1439 || p.Schedule.RetryMS < 1000 || p.Schedule.RetryMS > 3600000 || p.MaintenanceIntervalMS < 1000 || p.MaintenanceIntervalMS > 60000 || p.UnlockLifetimeMS < 60000 || p.UnlockLifetimeMS > 86400000 || p.UnlockRenewalMS < 1000 || p.UnlockRenewalMS > 3600000 || p.UnlockRenewalMS > p.UnlockLifetimeMS/2 {
		return ErrProvisionInvalid
	}
	f := p.Fee
	if f.MaxAtomsPerKVB < 1 || f.MaxAtomsPerKVB > 1000000000 || f.MaxFeeAtoms < 1 || f.MaxFeeAtoms > 2100000000000000 {
		return ErrProvisionInvalid
	}
	switch f.Mode {
	case "node":
		if f.ConfirmationTarget < 1 || f.ConfirmationTarget > 25 || f.AtomsPerKVB != 0 {
			return ErrProvisionInvalid
		}
	case "fixed":
		if f.ConfirmationTarget != 0 || f.AtomsPerKVB < 1 || f.AtomsPerKVB > f.MaxAtomsPerKVB {
			return ErrProvisionInvalid
		}
	default:
		return ErrProvisionInvalid
	}
	// Shape-only placeholder; it never leaves validation or becomes selected trust.
	if ingressValidate(provisionIngress(p, p.RequestID)) != nil || p.Transport.PoolURL == p.Transport.AdmissionURL || p.Transport.PoolURL == p.Transport.OwnershipURL || p.Transport.AdmissionURL == p.Transport.OwnershipURL {
		return ErrProvisionInvalid
	}
	// The reusable ingress unit-test grammar permits port0; durable Setup does not.
	if !provisionListen(p.Transport.Listen) {
		return ErrProvisionInvalid
	}
	raw, e = json.Marshal(p)
	if e != nil || len(raw) > MaxProvisionPolicyBytes {
		return ErrProvisionInvalid
	}
	return nil
}
func ProvisionPolicyID(p ProvisionPolicy) (string, error) { return provisionPolicyIDVersion(p, 1) }
func provisionPolicyIDVersion(p ProvisionPolicy, version uint32) (string, error) {
	if e := validateProvisionPolicyVersion(p, version); e != nil {
		return "", e
	}
	raw, _ := json.Marshal(p)
	return framedHashVersion("SLUG pool provision policy", raw, version), nil
}
func provisionNamespace(request, role string) string {
	return provisionNamespaceVersion(request, role, 1)
}
func provisionNamespaceVersion(request, role string, version uint32) string {
	raw, _ := hex.DecodeString(request)
	raw = append(raw, []byte(role)...)
	return framedHashVersion("SLUG pool provision identity", raw, version)
}
func ComposeProvisionSetup(p ProvisionPolicy, m ProvisionPublicMaterial, c ProvisionCertificate) (Setup, error) {
	return composeProvisionSetupVersion(p, m, c, 1)
}
func composeProvisionSetupVersion(p ProvisionPolicy, m ProvisionPublicMaterial, c ProvisionCertificate, version uint32) (Setup, error) {
	var s Setup
	if validateProvisionPolicyVersion(p, version) != nil || m.Version != 1 || !provisionTime(m.CreatedAtMS) || c.Version != 1 || !identifier(c.CertificateSHA256) || !identifier(c.CertificateSPKI) || !provisionPoint(m.FundingPublicKey) {
		return s, ErrProvisionInvalid
	}
	source := registry.ManifestAuthorityPin{Version: 5, Network: p.Network, Genesis: p.Genesis, AuthorityID: provisionNamespaceVersion(p.RequestID, "authority", version), DescriptorID: p.DescriptorID}
	pin := registry.PublicManifestPin{Version: 2, Source: source, PublicationID: provisionNamespaceVersion(p.RequestID, "publication", version), AdmissionKey: m.AdmissionKey, PublicationKey: m.PublicationKey, CutoffKey: m.CutoffKey, CheckpointKey: p.CheckpointPublicKey, RuleID: registry.PublicOwnershipRuleID(), ScanOrigin: registry.PublicChainPoint{Height: 0, Hash: p.Genesis}, Limits: registry.PublicOwnershipDefaultLimits()}
	profile, e := registry.NewOperatorProfile(pin)
	if e != nil {
		return s, ErrProvisionInvalid
	}
	point, _ := hex.DecodeString(m.FundingPublicKey)
	script := "0014" + hex.EncodeToString(address.Hash160(point))
	addr, e := address.NewAddressWitnessPubKeyHash(address.Hash160(point), &chaincfg.MainNetParams)
	if e != nil {
		return s, ErrProvisionInvalid
	}
	pool := poolshare.Config{Version: version, AuthorityPin: source, Policy: prospectnode.Policy{Version: 1, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.Mining.DNAVersion, ProspectTarget: p.Mining.ProspectTarget}, PayoutAddress: addr.EncodeAddress(), PoolTarget: p.Mining.PoolTarget, MaxWorks: p.Limits.MaxWorks, MaxSubmissions: p.Limits.MaxSubmissions, MaxBytes: p.Limits.PoolMaxBytes}
	settlement := poolsettle.Config{Version: version, ChangeScript: script, MaxRounds: p.Limits.MaxRounds, MaxBatches: p.Limits.MaxBatches}
	enrollments := make([]poolminer.Enrollment, 0, len(p.Enrollments))
	for _, en := range p.Enrollments {
		id, e := gymauth.GymID(provisionPolicyContext(p), en.PublicKey)
		if e != nil {
			return s, ErrProvisionInvalid
		}
		settlement.Destinations = append(settlement.Destinations, poolsettle.Destination{GymID: id, Script: en.PayeeScript})
		enrollments = append(enrollments, poolminer.Enrollment{PublicKey: en.PublicKey, GymID: id, PayeeScript: en.PayeeScript})
	}
	sourceDigest := poolservice.BindingID
	settlementDigest := poolsettle.ConfigBindingID
	if version == 2 {
		sourceDigest = poolshare.DailyConfigID
		settlementDigest = poolsettle.DailyConfigBindingID
	}
	sourceID, e := sourceDigest(pool)
	if e != nil {
		return s, ErrProvisionInvalid
	}
	configID, e := settlementDigest(settlement)
	if e != nil {
		return s, ErrProvisionInvalid
	}
	runtimeID := provisionNamespaceVersion(p.RequestID, "runtime", version)
	guard := waddrmgr.PoolSettlementGuard{Version: 1, ManagerSchema: 16, WalletOwnerID: provisionNamespaceVersion(p.RequestID, "wallet", version), OperatorProfileID: profile.ID(), SourceBindingID: sourceID, SettlementConfigID: configID, DerivationID: waddrmgr.PoolSettlementDerivation, Network: p.Network, Genesis: p.Genesis, RootPublicKey: m.RootPublicKey, FundingPublicKey: m.FundingPublicKey, FundingScript: script}
	binding, _ := json.Marshal(struct {
		Config poolsettle.Config `json:"config"`
		Source poolshare.Config  `json:"source"`
	}{settlement, pool})
	scheduleID := framedHash("SLUG pool settlement binding", binding)
	if version == 2 {
		scheduleID, e = poolschedule.DailySettlementConfigBinding(settlement, pool)
		if e != nil {
			return Setup{}, ErrProvisionInvalid
		}
	}
	s = Setup{Version: version, RuntimeID: runtimeID, Authority: pin, OwnershipStoreID: provisionNamespaceVersion(p.RequestID, "ownership", version), Pool: pool, RoundLimits: poolround.Limits{Version: version, MaxRounds: uint64(p.Limits.MaxRounds), MaxBytes: p.Limits.RoundMaxBytes}, Settlement: settlement, Schedule: poolschedule.Config{Version: version, SchedulerID: provisionNamespaceVersion(p.RequestID, "scheduler", version), RuntimeID: runtimeID, SettlementBindingID: scheduleID, DailyUTCMinute: p.Schedule.DailyUTCMinute, RetryMS: uint32(p.Schedule.RetryMS)}, Miners: poolminer.Config{Version: version, CredentialID: provisionNamespaceVersion(p.RequestID, "credentials", version), RuntimeID: runtimeID, OperatorProfileID: profile.ID(), SourceID: sourceID, SettlementConfigID: configID, Enrollments: enrollments}, WalletGuard: guard, Node: poolsettlenode.Config{Version: 1, Network: p.Network, Genesis: p.Genesis, PoolPublicKey: m.FundingPublicKey, PoolScript: script, Fee: p.Fee}, Ingress: provisionIngress(p, c.CertificateSPKI), MaintenanceIntervalMS: p.MaintenanceIntervalMS, UnlockLifetimeMS: p.UnlockLifetimeMS, UnlockRenewalMS: p.UnlockRenewalMS}
	if validateSetupVersion(s, version) != nil {
		return Setup{}, ErrProvisionInvalid
	}
	if version == 2 && dailyProvisionCapacity(p) != nil {
		return Setup{}, ErrProvisionInvalid
	}
	return s, nil
}
