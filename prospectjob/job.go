// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectjob

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"strings"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

const (
	// Version1 fixes the immutable mining-job byte format.
	Version1 uint32 = 1
	// RuleVersion1 is D68: issuance starts age and expiry has no grace period.
	RuleVersion1 uint32 = 1
	// LifetimeMS is exactly five minutes; the expiry instant is exclusive.
	LifetimeMS int64 = 300000
	// MaxJobSize bounds the complete JSON document including whitespace.
	MaxJobSize = 8192

	jobDomain = "SLUG mining job\x00v1\x00"
)

// Job is a canonical, immutable issuer assertion. ID commits every other field
// in the versioned binary encoding. PublicKey is a compressed secp256k1 key;
// all hexadecimal fields use lowercase. IssuedAtMS and ExpiresAtMS are positive
// Unix milliseconds asserted by the issuer, not authenticated mining times.
type Job struct {
	Version     uint32           `json:"version"`
	RuleVersion uint32           `json:"rule_version"`
	ID          string           `json:"id"`
	Context     prospect.Context `json:"context"`
	PublicKey   string           `json:"public_key"`
	GymID       string           `json:"gym_id"`
	Challenge   string           `json:"challenge"`
	IssuedAtMS  int64            `json:"issued_at_ms"`
	ExpiresAtMS int64            `json:"expires_at_ms"`
}

// New deterministically constructs a canonical job from public inputs. The
// caller, not this helper, is responsible for obtaining trusted context/time,
// generating an unpredictable challenge and durably recording issuance.
func New(context prospect.Context, publicKey, challengeHex string, issuedAtMS int64) (Job, error) {
	if issuedAtMS <= 0 || issuedAtMS > math.MaxInt64-LifetimeMS {
		return Job{}, errors.New("job issuance must be positive milliseconds with room for the five-minute lifetime")
	}
	if len(challengeHex) != 64 {
		return Job{}, errors.New("job challenge must contain exactly 64 hexadecimal characters")
	}
	challenge, err := hex.DecodeString(challengeHex)
	if err != nil {
		return Job{}, errors.New("invalid job challenge hexadecimal encoding")
	}
	gym, err := gymauth.GymID(context, publicKey)
	if err != nil {
		return Job{}, err
	}
	// The ordinary commitment helper validates the full context without PoW.
	if _, err := prospect.CommitmentScript(context, gym); err != nil {
		return Job{}, err
	}
	context.Genesis = strings.ToLower(context.Genesis)
	context.ProspectTarget = strings.ToLower(context.ProspectTarget)
	context.ParentHash = strings.ToLower(context.ParentHash)
	job := Job{
		Version: Version1, RuleVersion: RuleVersion1, Context: context,
		PublicKey: strings.ToLower(publicKey), GymID: gym,
		Challenge: hex.EncodeToString(challenge), IssuedAtMS: issuedAtMS,
		ExpiresAtMS: issuedAtMS + LifetimeMS,
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(jobDomain))
	put32 := func(value uint32) {
		var encoded [4]byte
		binary.LittleEndian.PutUint32(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
	put64 := func(value int64) {
		var encoded [8]byte
		binary.LittleEndian.PutUint64(encoded[:], uint64(value))
		_, _ = digest.Write(encoded[:])
	}
	putHex := func(value string) {
		// Every value has already passed its exact byte-shape validation.
		encoded, _ := hex.DecodeString(value)
		_, _ = digest.Write(encoded)
	}
	put32(job.Version)
	put32(job.RuleVersion)
	put32(context.Version)
	put32(context.Network)
	putHex(context.Genesis)
	put32(context.DNAVersion)
	putHex(context.ProspectTarget)
	putHex(context.ParentHash)
	put32(context.ParentHeight)
	put32(context.ExpectedBits)
	put32(context.MinTimestamp)
	put32(context.MaxTimestamp)
	putHex(job.PublicKey)
	putHex(job.GymID)
	_, _ = digest.Write(challenge)
	put64(job.IssuedAtMS)
	put64(job.ExpiresAtMS)
	job.ID = hex.EncodeToString(digest.Sum(nil))
	return job, nil
}

// Validate requires the exact canonical value, including the derived gym,
// fixed lifetime and ID. A structurally valid job is not proof of issuance by
// any authority; admission must find it in the trusted issuer's durable store.
func Validate(job Job) error {
	if job.Version != Version1 || job.RuleVersion != RuleVersion1 {
		return errors.New("unsupported job or birth-rule version")
	}
	canonical, err := New(job.Context, job.PublicKey, job.Challenge, job.IssuedAtMS)
	if err != nil {
		return err
	}
	if canonical != job {
		return errors.New("job is noncanonical or its immutable fields do not match its ID")
	}
	return nil
}

// CommitmentScript returns the proof-v2 script for this complete job assertion.
// It does not authenticate issuance or perform work verification.
func CommitmentScript(job Job) ([]byte, error) {
	if err := Validate(job); err != nil {
		return nil, err
	}
	return prospect.CommitmentScriptForJob(job.Context, job.GymID, job.ID)
}
