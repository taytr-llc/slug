// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"

	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

const (
	ManifestMode                 = "local-manifest-pilot"
	ManifestVersion       uint32 = 1
	MaxManifestProspects         = 8
	MaxManifestCacheBytes        = 1 << 20
	MaxManifestSequence   uint64 = 1000
)

var (
	ErrManifestProjection          = errors.New("projection_unavailable")
	ErrManifestProjectionCorrupt   = errors.New("projection_corrupt")
	ErrManifestProjectionCancelled = errors.New("projection_cancelled")
	ErrManifestProjectionRequired  = errors.New("projection_required")
	ErrManifestProspect            = errors.New("prospect_unavailable")
	ErrManifestPortrait            = errors.New("portrait_unavailable")
	ErrManifestReceipt             = errors.New("manifest_receipt_unavailable")
	ErrManifestInitialization      = errors.New("initialization_incomplete")
)

// ManifestViewLease captures one original login credential and view epoch. It
// must not substitute the current credential after a reset or account switch.
// The callback runs under the account View lease; never join a child from it.
type ManifestViewLease func(context.Context, func(gymaccount.Account, func() error) error) error

// ManifestAccountConfig is private dependency configuration, never a web DTO.
// Account.AdmissionSigner must be nil: this mode uses the retained fixture wallet.
type ManifestAccountConfig struct {
	Account    AccountConfig
	ViewID     string
	Generation uint64
	ViewLease  ManifestViewLease
}

type ManifestViewPin struct {
	Version            uint32                        `json:"version"`
	ViewID             string                        `json:"view_id"`
	AccountAuthorityID string                        `json:"account_authority_id"`
	AccountID          string                        `json:"account_id"`
	Generation         uint64                        `json:"generation"`
	AuthorityPin       registry.ManifestAuthorityPin `json:"authority_pin"`
	AuthorityOrigin    string                        `json:"authority_origin"`
	TransportTrustID   string                        `json:"transport_trust_id"`
}

type ManifestProspectSummary struct {
	Sequence        uint64                   `json:"sequence"`
	ID              string                   `json:"id"`
	WorkHash        string                   `json:"work_hash"`
	BornAtMS        int64                    `json:"born_at_ms"`
	AcceptedAtMS    int64                    `json:"accepted_at_ms"`
	Name            fightermanifest.Name     `json:"name"`
	Physical        fightermanifest.Physical `json:"physical"`
	Style           string                   `json:"style"`
	Rarity          string                   `json:"rarity"`
	TalentBudget    int                      `json:"talent_budget"`
	Attributes      fighter.Attributes       `json:"attributes"`
	DescriptorID    string                   `json:"descriptor_id"`
	ManifestDigest  string                   `json:"manifest_digest"`
	PortraitDigest  string                   `json:"portrait_digest"`
	EligibilityMode string                   `json:"eligibility_mode"`
}

type ManifestProjection struct {
	State           string                    `json:"state"`
	Error           string                    `json:"error"`
	Revision        uint64                    `json:"revision"`
	ThroughSequence uint64                    `json:"through_sequence"`
	HasOlder        bool                      `json:"has_older"`
	CompleteRecent  bool                      `json:"complete_recent"`
	Items           []ManifestProspectSummary `json:"items"`
}

// ManifestSnapshot deliberately has no legacy prospect/receipt union. View is
// the sole required nullable public field; an unavailable account has no view.
type ManifestSnapshot struct {
	Version         uint32             `json:"version"`
	Mode            string             `json:"mode"`
	State           string             `json:"state"`
	Attempts        uint64             `json:"attempts"`
	HashesPerSecond float64            `json:"hashes_per_second"`
	Height          uint64             `json:"height"`
	Pending         uint64             `json:"pending"`
	LastError       string             `json:"last_error"`
	RecoveryError   string             `json:"recovery_error"`
	WalletState     string             `json:"wallet_state"`
	WalletHeight    uint64             `json:"wallet_height"`
	PayoutAddress   string             `json:"payout_address"`
	GymID           string             `json:"gym_id"`
	GymPublicKey    string             `json:"gym_public_key"`
	SpendableAtoms  int64              `json:"spendable_atoms"`
	ImmatureAtoms   int64              `json:"immature_atoms"`
	BackupState     string             `json:"backup_state"`
	BackupPhase     string             `json:"backup_phase"`
	BackupError     string             `json:"backup_error"`
	Account         AccountSnapshot    `json:"account"`
	View            *ManifestViewPin   `json:"view"`
	AcceptedCount   uint64             `json:"accepted_count"`
	Projection      ManifestProjection `json:"projection"`
}

type ManifestPortrait struct {
	Version        uint32          `json:"version"`
	View           ManifestViewPin `json:"view"`
	Sequence       uint64          `json:"sequence"`
	ID             string          `json:"id"`
	ManifestDigest string          `json:"manifest_digest"`
	PortraitDigest string          `json:"portrait_digest"`
	SVG            string          `json:"svg"`
}

type ManifestReceipt struct {
	Version uint32                  `json:"version"`
	View    ManifestViewPin         `json:"view"`
	Record  registry.ManifestRecord `json:"record"`
}

// BlankManifestSnapshot contains no previous-account data and no nil list.
func BlankManifestSnapshot() ManifestSnapshot {
	return ManifestSnapshot{Version: ManifestVersion, Mode: ManifestMode,
		State: "stopped", WalletState: "closed", BackupState: "idle",
		Account:    AccountSnapshot{State: "signed_out"},
		Projection: ManifestProjection{State: "idle", Items: []ManifestProspectSummary{}}}
}
