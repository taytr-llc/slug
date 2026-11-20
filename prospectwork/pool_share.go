// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

// ErrShareBusy means another adapter check owns the process-wide hash slot.
// Other callers of BlockHash are outside this adapter's resource bound.
var ErrShareBusy = errors.New("pool share hash slot is busy")

var shareHashSlot = make(chan struct{}, 1)

const poolSubmissionDomain = "SLUG pool submission\x00v1\x00"

// ManifestWork owns immutable manifest-bound work and three independent
// targets. It does not authenticate issuance, expiry, gym control or chain
// currentness. Only PrepareManifestWork creates a usable value.
type ManifestWork struct {
	prepared      bool
	job           prospectjob.ManifestJob
	candidate     Candidate
	header        [wire.MaxBlockHeaderPayload]byte
	poolTarget    string
	poolValue     [32]byte
	prospectValue [32]byte
	blockValue    [32]byte
}

// ShareCheck describes only the exact checked work and target comparisons.
// DuplicateKey is an identity for a future ledger, not duplicate prevention.
// Candidate is nil when none of the targets is met.
type ShareCheck struct {
	JobID        string
	GymID        string
	Nonce        uint32
	Header       string
	WorkHash     string
	DuplicateKey string
	Pool         bool
	Prospect     bool
	Block        bool
	Candidate    *Candidate
}

// PrepareManifestWork calls the unchanged manifest builder without hashing.
// The caller supplies separately trusted job/template assertions and its own
// payout configuration; the pool target never changes either existing target.
func PrepareManifestWork(job prospectjob.ManifestJob, template Template, payoutAddress string, extraNonce uint64, poolTarget string) (*ManifestWork, error) {
	if err := productionOnly(); err != nil {
		return nil, err
	}
	if len(poolTarget) != 64 || poolTarget != strings.ToLower(poolTarget) {
		return nil, errors.New("pool target must be 64 lowercase hexadecimal characters")
	}
	var poolValue [32]byte
	if _, err := hex.Decode(poolValue[:], []byte(poolTarget)); err != nil || poolValue == ([32]byte{}) {
		return nil, errors.New("pool target must be a nonzero 256-bit hexadecimal integer")
	}
	candidate, err := BuildManifest(job, template, payoutAddress, extraNonce)
	if err != nil {
		return nil, err
	}
	w := &ManifestWork{job: job, candidate: candidate, poolTarget: poolTarget, poolValue: poolValue}
	if len(candidate.Proof.Header) != hex.EncodedLen(len(w.header)) {
		return nil, errors.New("manifest candidate has an invalid header length")
	}
	if _, err := hex.Decode(w.header[:], []byte(candidate.Proof.Header)); err != nil {
		return nil, errors.New("manifest candidate has an invalid header")
	}
	if len(job.Context.ProspectTarget) != hex.EncodedLen(len(w.prospectValue)) {
		return nil, errors.New("manifest job has an invalid prospect target length")
	}
	if _, err := hex.Decode(w.prospectValue[:], []byte(job.Context.ProspectTarget)); err != nil {
		return nil, errors.New("manifest job has an invalid prospect target")
	}
	// BuildManifest has already checked the canonical, positive block target
	// against the production PowLimit, so this conversion fits exactly.
	blockchain.CompactToBig(job.Context.ExpectedBits).FillBytes(w.blockValue[:])
	w.prepared = true
	return w, nil
}

// Job returns the original preparation assertions, not authenticated issuance.
func (w *ManifestWork) Job() prospectjob.ManifestJob {
	if w == nil || !w.prepared {
		return prospectjob.ManifestJob{}
	}
	return w.job
}

// PoolTarget returns the separately retained pool target.
func (w *ManifestWork) PoolTarget() string {
	if w == nil || !w.prepared {
		return ""
	}
	return w.poolTarget
}

func copyShareCandidate(candidate Candidate) Candidate {
	if candidate.Proof.MerkleBranch != nil {
		branch := make([]string, len(candidate.Proof.MerkleBranch))
		copy(branch, candidate.Proof.MerkleBranch)
		candidate.Proof.MerkleBranch = branch
	}
	return candidate
}

// Candidate returns an independent copy of the nonce-zero preparation.
func (w *ManifestWork) Candidate() Candidate {
	if w == nil || !w.prepared {
		return Candidate{}
	}
	return copyShareCandidate(w.candidate)
}

// CheckNonce changes only the nonce in the retained header. The synchronous
// normal hash always finishes before return, including caller cancellation.
// Success establishes no admission, duplicate credit, birth or block acceptance.
func (w *ManifestWork) CheckNonce(ctx context.Context, nonce uint32) (ShareCheck, error) {
	if err := productionOnly(); err != nil {
		return ShareCheck{}, err
	}
	if ctx == nil {
		return ShareCheck{}, errors.New("pool share check requires a context")
	}
	if w == nil || !w.prepared {
		return ShareCheck{}, errors.New("pool share work is not prepared")
	}
	if err := ctx.Err(); err != nil {
		return ShareCheck{}, err
	}
	select {
	case shareHashSlot <- struct{}{}:
		defer func() { <-shareHashSlot }()
	default:
		if err := ctx.Err(); err != nil {
			return ShareCheck{}, err
		}
		return ShareCheck{}, ErrShareBusy
	}
	headerBytes := w.header
	binary.LittleEndian.PutUint32(headerBytes[76:80], nonce)
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(headerBytes[:])); err != nil {
		return ShareCheck{}, errors.New("invalid prepared pool share header")
	}
	if err := ctx.Err(); err != nil {
		return ShareCheck{}, err
	}
	workHash := header.BlockHash()
	if err := ctx.Err(); err != nil {
		return ShareCheck{}, err
	}
	value := blockchain.HashToBig(&workHash)
	meets := func(target [32]byte) bool {
		return value.Cmp(new(big.Int).SetBytes(target[:])) <= 0
	}
	var duplicateInput [len(poolSubmissionDomain) + wire.MaxBlockHeaderPayload]byte
	copy(duplicateInput[:], poolSubmissionDomain)
	copy(duplicateInput[len(poolSubmissionDomain):], headerBytes[:])
	duplicate := sha256.Sum256(duplicateInput[:])
	check := ShareCheck{
		JobID: w.job.ID, GymID: w.job.GymID, Nonce: nonce,
		Header: hex.EncodeToString(headerBytes[:]), WorkHash: workHash.String(),
		DuplicateKey: hex.EncodeToString(duplicate[:]),
		Pool:         meets(w.poolValue), Prospect: meets(w.prospectValue), Block: meets(w.blockValue),
	}
	if check.Pool || check.Prospect || check.Block {
		candidate := copyShareCandidate(w.candidate)
		candidate.Proof.Header = check.Header
		candidate.BlockHex = check.Header + candidate.BlockHex[hex.EncodedLen(len(headerBytes)):]
		check.Candidate = &candidate
	}
	if err := ctx.Err(); err != nil {
		return ShareCheck{}, err
	}
	return check, nil
}
