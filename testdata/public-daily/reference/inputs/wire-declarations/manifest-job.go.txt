// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectjob

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospect"
)

const (
	Version2            uint32 = 2
	MaxManifestJobSize         = 8192
	MaxManifestJobDepth        = 2
	manifestJobDomain          = "SLUG mining job\x00v2\x00"
)

// ManifestJob is a separately typed, candidate-only issuer assertion. Its
// authority namespace and descriptor linkage do not authenticate issuance,
// currentness, accepted birth or ownership. Legacy Job remains version 1.
type ManifestJob struct {
	Version      uint32           `json:"version"`
	RuleVersion  uint32           `json:"rule_version"`
	ID           string           `json:"id"`
	AuthorityID  string           `json:"authority_id"`
	DescriptorID string           `json:"descriptor_id"`
	Context      prospect.Context `json:"context"`
	PublicKey    string           `json:"public_key"`
	GymID        string           `json:"gym_id"`
	Challenge    string           `json:"challenge"`
	IssuedAtMS   int64            `json:"issued_at_ms"`
	ExpiresAtMS  int64            `json:"expires_at_ms"`
}

func normalizeManifestID(value string) (string, error) {
	if len(value) != 64 {
		return "", errors.New("manifest job identifier must be exactly 32 hexadecimal bytes")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("invalid manifest job identifier")
	}
	return strings.ToLower(value), nil
}

// NewManifest reconstructs a candidate job from public inputs. The caller is
// responsible for a trusted clock, unpredictable challenge, pinned authority
// and durable issuance; none are established by this deterministic operation.
func NewManifest(context prospect.Context, authorityID, descriptorID, publicKey, challengeHex string, issuedAtMS int64) (ManifestJob, error) {
	authorityID, err := normalizeManifestID(authorityID)
	if err != nil {
		return ManifestJob{}, err
	}
	descriptorID, err = normalizeManifestID(descriptorID)
	if err != nil {
		return ManifestJob{}, err
	}
	descriptor, err := fighterdescriptor.ResolveCandidate(descriptorID)
	if err != nil {
		return ManifestJob{}, err
	}
	if descriptor.DNAVersion != context.DNAVersion {
		return ManifestJob{}, errors.New("manifest descriptor and context DNA versions differ")
	}
	// Reuse unchanged v1 public fact validation and normalization. Its old ID
	// is discarded, never altered and passed to a v1 builder or validator.
	base, err := New(context, publicKey, challengeHex, issuedAtMS)
	if err != nil {
		return ManifestJob{}, err
	}
	job := ManifestJob{
		Version: Version2, RuleVersion: RuleVersion1, AuthorityID: authorityID, DescriptorID: descriptorID,
		Context: base.Context, PublicKey: base.PublicKey, GymID: base.GymID, Challenge: base.Challenge,
		IssuedAtMS: base.IssuedAtMS, ExpiresAtMS: base.ExpiresAtMS,
	}
	preimage := manifestPreimage(job)
	digest := sha256.Sum256(preimage)
	job.ID = hex.EncodeToString(digest[:])
	return job, nil
}

// manifestPreimage is used only with internally validated canonical fields.
// Each decoded hex field has a fixed width; timestamps are already positive.
func manifestPreimage(job ManifestJob) []byte {
	result := make([]byte, 0, 328)
	result = append(result, manifestJobDomain...)
	put32 := func(n uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], n)
		result = append(result, b[:]...)
	}
	put64 := func(n int64) {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(n))
		result = append(result, b[:]...)
	}
	putHex := func(s string) { b, _ := hex.DecodeString(s); result = append(result, b...) }
	put32(job.Version)
	put32(job.RuleVersion)
	putHex(job.AuthorityID)
	putHex(job.DescriptorID)
	c := job.Context
	put32(c.Version)
	put32(c.Network)
	putHex(c.Genesis)
	put32(c.DNAVersion)
	putHex(c.ProspectTarget)
	putHex(c.ParentHash)
	put32(c.ParentHeight)
	put32(c.ExpectedBits)
	put32(c.MinTimestamp)
	put32(c.MaxTimestamp)
	putHex(job.PublicKey)
	putHex(job.GymID)
	putHex(job.Challenge)
	put64(job.IssuedAtMS)
	put64(job.ExpiresAtMS)
	return result
}

// ValidateManifest requires exact equality to the supported reconstructed
// value. A mathematically consistent unknown descriptor is still refused.
func ValidateManifest(job ManifestJob) error {
	if job.Version != Version2 || job.RuleVersion != RuleVersion1 {
		return errors.New("unsupported manifest job or birth-rule version")
	}
	canonical, err := NewManifest(job.Context, job.AuthorityID, job.DescriptorID, job.PublicKey, job.Challenge, job.IssuedAtMS)
	if err != nil {
		return err
	}
	if canonical != job {
		return errors.New("noncanonical manifest job or immutable ID mismatch")
	}
	return nil
}

// ManifestCommitmentScript binds the validated candidate assertion into the
// existing proof-v2 format. It does not authenticate issuance or perform PoW.
func ManifestCommitmentScript(job ManifestJob) ([]byte, error) {
	if err := ValidateManifest(job); err != nil {
		return nil, err
	}
	return prospect.CommitmentScriptForJob(job.Context, job.GymID, job.ID)
}
