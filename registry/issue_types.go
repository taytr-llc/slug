// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/taytr-llc/slug/btcec/v2"
)

const (
	IssueVersion1        uint32 = 1
	MaxIssueRequests     uint64 = 8192
	MaxIssueRowSize             = 4096
	issueApplication            = "slug-manifest-issue-recovery"
	maxIssueMetadataSize        = 2 * MaxManifestMetadataSize
)

var ErrIssueRequestNotFound = errors.New("manifest issue request not found")

// IssueProfile is immutable public correlation, never a store or signing
// capability. The operator also checks its complete original Base Setup2.
type IssueProfile struct {
	Version        uint32 `json:"version"`
	BaseSetupID    string `json:"base_setup_id"`
	SourceConfigID string `json:"source_config_id"`
	MaxRequests    uint64 `json:"max_requests"`
}

type IssueIntent struct {
	Version        uint32 `json:"version"`
	ProfileID      string `json:"profile_id"`
	RequestID      string `json:"request_id"`
	GymID          string `json:"gym_id"`
	PublicKey      string `json:"public_key"`
	TemplateSHA256 string `json:"template_sha256"`
}

type IssuerRequest struct {
	Version    uint32 `json:"version"`
	ProfileID  string `json:"profile_id"`
	RequestID  string `json:"request_id"`
	IntentID   string `json:"intent_id"`
	GymID      string `json:"gym_id"`
	PublicKey  string `json:"public_key"`
	State      string `json:"state"`
	JobID      string `json:"job_id"`
	SealedAtMS int64  `json:"sealed_at_ms"`
	Reason     string `json:"reason"`
}

func issueHash(label string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(label + "\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

func ValidateIssueProfile(p IssueProfile) error {
	if p.Version != IssueVersion1 || !publicHex(p.BaseSetupID, 32, true) || !publicHex(p.SourceConfigID, 32, true) || p.MaxRequests == 0 || p.MaxRequests > MaxIssueRequests {
		return ErrManifestAuthority
	}
	return nil
}

func IssueProfileID(p IssueProfile) (string, error) {
	if err := ValidateIssueProfile(p); err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 202 {
		return "", ErrManifestAuthority
	}
	return issueHash("SLUG pool issue recovery profile", raw), nil
}

func ValidateIssueIntent(i IssueIntent) error {
	if i.Version != IssueVersion1 || !publicHex(i.ProfileID, 32, true) || !publicHex(i.RequestID, 32, true) || !publicHex(i.GymID, 32, true) || !publicHex(i.TemplateSHA256, 32, true) || !publicHex(i.PublicKey, 33, false) {
		return ErrManifestAuthority
	}
	raw, _ := hex.DecodeString(i.PublicKey)
	key, err := btcec.ParsePubKey(raw)
	if err != nil || hex.EncodeToString(key.SerializeCompressed()) != i.PublicKey {
		return ErrManifestAuthority
	}
	return nil
}

func IssueIntentID(i IssueIntent) (string, error) {
	if err := ValidateIssueIntent(i); err != nil {
		return "", err
	}
	raw, err := json.Marshal(i)
	if err != nil || len(raw) > MaxIssueRowSize {
		return "", ErrManifestAuthority
	}
	return issueHash("SLUG pool issue recovery intent", raw), nil
}

// ValidateRecoverableIssueCapacity is a conservative, side-effect-free
// provisioning preflight. Runtime reserves actual outstanding publication rows.
func ValidateRecoverableIssueCapacity(limits PublicOwnershipLimits, maxRequests uint64) error {
	if limits != PublicOwnershipDefaultLimits() || maxRequests == 0 || maxRequests > MaxIssueRequests {
		return ErrPublicOwnership
	}
	const slot uint64 = 40 + MaxIssueRowSize + 40 + 32 + 68 + 8192
	const initial uint64 = uint64(len(metadataKey) + maxIssueMetadataSize + len(publicMetaKey) + MaxPublicSmallSize + len("ownership/guard") + MaxPublicSmallSize)
	// Both factors have already bounded domains; products fit uint64.
	required := initial + maxRequests*slot + uint64(limits.Admissions)*uint64(MaxPublicAdmissionSize+MaxPublicSmallSize)
	if required > uint64(limits.LogicalBytes) {
		return ErrPublicCapacity
	}
	return nil
}

func ValidateRecoverableOperatorProfile(p OperatorProfile, issue IssueProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := ValidateIssueProfile(issue); err != nil {
		return err
	}
	return ValidateRecoverableIssueCapacity(p.Pin().Limits, issue.MaxRequests)
}
