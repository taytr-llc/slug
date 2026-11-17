// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

const Version2 uint32 = 2
const MaxManifestConfigSize = 128 << 10

// ManifestConfig pins one candidate authority, transport trust policy and gym.
// It contains no credential and never upgrades an existing version-1 outbox.
type ManifestConfig struct {
	Version          uint32                        `json:"version"`
	Policy           prospectnode.Policy           `json:"policy"`
	PublicKey        string                        `json:"public_key"`
	PayoutAddress    string                        `json:"payout_address"`
	AuthorityOrigin  string                        `json:"authority_origin"`
	AuthorityPin     registry.ManifestAuthorityPin `json:"authority_pin"`
	TransportTrustID string                        `json:"transport_trust_id"`
	MaxRecords       uint64                        `json:"max_records"`
	MaxBytes         uint64                        `json:"max_bytes"`
}

// ManifestAuthority must preserve its declared identity for its entire lifetime.
// A token-refresh adapter checks each new client's transport and registry pins.
type ManifestAuthority interface {
	ProtocolVersion() uint32
	AuthorityOrigin() string
	AuthorityPin() registry.ManifestAuthorityPin
	TransportTrustID() string
	IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error)
	SubmitManifest(context.Context, prospectjob.ManifestJob, prospect.Proof, gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error)
}

// ManifestEntry retains public conditional authority evidence, not a portable
// issuer signature or current ownership/canonicality claim.
type ManifestEntry struct {
	Version        uint32                                   `json:"version"`
	Sequence       uint64                                   `json:"sequence"`
	WorkHash       string                                   `json:"work_hash"`
	Job            prospectjob.ManifestJob                  `json:"job"`
	Candidate      prospectwork.Candidate                   `json:"candidate"`
	BlockWin       bool                                     `json:"block_win"`
	ProspectWin    bool                                     `json:"prospect_win"`
	Authorization  *gymauth.Authorization                   `json:"authorization,omitempty"`
	Receipt        *prospectservice.ManifestAdmissionResult `json:"receipt,omitempty"`
	BlockState     string                                   `json:"block_state"`
	AdmissionState string                                   `json:"admission_state"`
	LastError      string                                   `json:"last_error,omitempty"`
	NextAttemptMS  int64                                    `json:"next_attempt_ms"`
	RetryCount     uint32                                   `json:"retry_count"`
}

// ManifestScan is durable before the first hash. Its cursor is stored separately
// so a nonce attempt cannot rewrite the full immutable block template.
type ManifestScan struct {
	Version    uint32                  `json:"version"`
	Job        prospectjob.ManifestJob `json:"job"`
	Template   prospectwork.Template   `json:"template"`
	Candidate  prospectwork.Candidate  `json:"candidate"`
	ExtraNonce uint64                  `json:"extra_nonce"`
	NextNonce  uint64                  `json:"next_nonce"`
}
