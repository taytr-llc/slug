// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectwork"
)

func poolManifestConfig(c PoolConfig) ManifestConfig {
	return ManifestConfig{Version: Version2, Policy: c.Source.Policy, PublicKey: c.PublicKey, PayoutAddress: c.Source.PayoutAddress, AuthorityOrigin: c.AdmissionOrigin, AuthorityPin: c.Source.AuthorityPin, TransportTrustID: c.AdmissionTrustID, MaxRecords: c.MaxRecords, MaxBytes: c.MaxBytes}
}

func ValidatePoolConfig(c PoolConfig) (PoolConfig, error) {
	fail := errors.New("invalid pool worker configuration")
	if c.Version != PoolVersion1 || c.MaxRecords == 0 || c.MaxRecords > 4096 || c.MaxBytes < 64<<20 || c.MaxBytes > 1<<30 || !manifestHashShape(c.TransportTrustID) {
		return PoolConfig{}, fail
	}
	m, err := ValidateManifestConfig(poolManifestConfig(c))
	if err != nil || m.PublicKey != c.PublicKey || m.AuthorityOrigin != c.AdmissionOrigin || m.PayoutAddress != c.Source.PayoutAddress {
		return PoolConfig{}, fail
	}
	// Public dummy token performs syntax validation only; no request is made.
	p, err := poolservice.ValidateClientConfig(poolservice.ClientConfig{Version: 1, URL: c.AuthorityOrigin, Token: strings.Repeat("00", 32), PublicKey: c.PublicKey, Source: c.Source})
	if err != nil || p.URL != c.AuthorityOrigin {
		return PoolConfig{}, fail
	}
	return c, nil
}

func validatePoolScan(s PoolScan, c PoolConfig) error {
	if s.Version != PoolVersion1 || s.NextNonce > 1<<32 {
		return ErrCorrupt
	}
	if _, err := poolservice.PrepareWork(c.Source, c.PublicKey, s.Work); err != nil {
		return ErrCorrupt
	}
	return nil
}

// poolCheck validates structure using the retained digest. It is deliberately
// not a proof verifier: delivery rehashes exact work before any external call.
func poolCheck(e PoolEntry, c PoolConfig) (prospectwork.ShareCheck, error) {
	w, err := poolservice.PrepareWork(c.Source, c.PublicKey, e.Work)
	if err != nil {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	digest, err := chainhash.NewHashFromStr(e.WorkHash)
	if err != nil || digest.String() != e.WorkHash {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	candidate := w.Candidate()
	header, err := hex.DecodeString(candidate.Proof.Header)
	if err != nil || len(header) != 80 {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	binary.LittleEndian.PutUint32(header[76:], e.Nonce)
	h := hex.EncodeToString(header)
	dup := sha256.Sum256(append([]byte("SLUG pool submission\x00v1\x00"), header...))
	target, _ := new(big.Int).SetString(c.Source.PoolTarget, 16)
	block, prospect := classifyManifest(*digest, e.Work.Job)
	check := prospectwork.ShareCheck{JobID: e.Work.Job.ID, GymID: e.Work.Job.GymID, Nonce: e.Nonce, Header: h, WorkHash: e.WorkHash, DuplicateKey: hex.EncodeToString(dup[:]), Pool: blockchain.HashToBig(digest).Cmp(target) <= 0, Prospect: prospect, Block: block}
	if check.Pool || check.Prospect || check.Block {
		candidate.Proof.Header = h
		candidate.BlockHex = h + candidate.BlockHex[160:]
		check.Candidate = &candidate
	}
	return check, nil
}

func validatePoolEntry(e PoolEntry, c PoolConfig) error {
	if e.Version != PoolVersion1 || e.Sequence == 0 || e.Sequence > c.MaxRecords || e.NextAttemptMS < 0 || e.RetryCount > 32 {
		return ErrCorrupt
	}
	check, err := poolCheck(e, c)
	if err != nil || check.Candidate == nil || e.DuplicateKey != check.DuplicateKey || e.PoolWin != check.Pool {
		return ErrCorrupt
	}
	if check.Block || check.Prospect {
		f := e.Fighter
		if f == nil || f.Sequence != e.Sequence || f.Job != e.Work.Job || f.WorkHash != e.WorkHash || !reflect.DeepEqual(f.Candidate, *check.Candidate) || f.BlockWin != check.Block || f.ProspectWin != check.Prospect || validateManifestEntry(*f, poolManifestConfig(c)) != nil {
			return ErrCorrupt
		}
	} else if e.Fighter != nil {
		return ErrCorrupt
	}
	switch e.PoolState {
	case "not-applicable":
		if check.Pool || check.Block || e.PoolReceipt != nil {
			return ErrCorrupt
		}
	case "pending", "accepted", "expired", "rejected":
		if !check.Pool && !check.Block {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	if e.PoolReceipt != nil {
		if !poolservice.SubmissionMatches(c.Source, e.Work, e.Nonce, check, *e.PoolReceipt) || (e.PoolReceipt.AcceptedSequence > 0) != (e.PoolState == "accepted") || e.PoolReceipt.AcceptedSequence == 0 && e.PoolState != "pending" {
			return ErrCorrupt
		}
	} else if e.PoolState == "accepted" {
		return ErrCorrupt
	}
	switch e.LastError {
	case "", "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "not_found", "unauthorized", "forbidden", "invalid_request", "too_large", "signer_unavailable", "clock", "stale":
	default:
		return ErrCorrupt
	}
	return nil
}

func poolDone(e PoolEntry) bool {
	return e.PoolState != "pending" && (e.Fighter == nil || manifestDone(*e.Fighter))
}
func poolImmutable(e PoolEntry) PoolEntry {
	e.PoolState, e.LastError = "", ""
	e.PoolReceipt = nil
	e.NextAttemptMS, e.RetryCount = 0, 0
	if e.Fighter != nil {
		f := manifestImmutable(*e.Fighter)
		e.Fighter = &f
	}
	return e
}
func poolTransition(a, b PoolEntry) bool {
	if a.RetryCount > b.RetryCount || !reflect.DeepEqual(poolImmutable(a), poolImmutable(b)) {
		return false
	}
	if a.PoolState != b.PoolState && !(a.PoolState == "pending" && (b.PoolState == "accepted" || b.PoolState == "expired" || b.PoolState == "rejected")) {
		return false
	}
	if a.PoolReceipt != nil {
		// Pending canonical observations may advance, but the original received
		// evidence and first accepted receipt can never be replaced.
		if b.PoolReceipt == nil {
			return false
		}
		if a.PoolReceipt.AcceptedSequence > 0 {
			if *a.PoolReceipt != *b.PoolReceipt {
				return false
			}
		} else {
			x, y := *a.PoolReceipt, *b.PoolReceipt
			x.AcceptedSequence, y.AcceptedSequence = 0, 0
			x.AcceptedAtMS, y.AcceptedAtMS = 0, 0
			x.Acceptance, y.Acceptance = "", ""
			x.BlockState, y.BlockState = "", ""
			x.BlockObservedAtMS, y.BlockObservedAtMS = 0, 0
			if x != y {
				return false
			}
		}
	}
	if a.Fighter != nil {
		x, y := *a.Fighter, *b.Fighter
		if x.Authorization != nil && !reflect.DeepEqual(x.Authorization, y.Authorization) || x.Receipt != nil && !reflect.DeepEqual(x.Receipt, y.Receipt) {
			return false
		}
		// An unsigned proof that expires must never invoke the signer later.
		if x.AdmissionState == "needs-signature" && y.AdmissionState == "expired" {
			x.AdmissionState = "expired"
		}
		if !manifestTransition(x, y) {
			return false
		}
	}
	return true
}

func poolEntryForCheck(s PoolScan, check prospectwork.ShareCheck) PoolEntry {
	e := PoolEntry{Version: PoolVersion1, Work: s.Work, Nonce: check.Nonce, WorkHash: check.WorkHash, DuplicateKey: check.DuplicateKey, PoolWin: check.Pool, PoolState: "not-applicable"}
	if check.Pool || check.Block {
		e.PoolState = "pending"
	}
	if check.Block || check.Prospect {
		f := &ManifestEntry{Version: Version2, WorkHash: check.WorkHash, Job: s.Work.Job, Candidate: *check.Candidate, BlockWin: check.Block, ProspectWin: check.Prospect, BlockState: "not-applicable", AdmissionState: "not-applicable"}
		if check.Block {
			f.BlockState = "pending"
		}
		if check.Prospect {
			f.AdmissionState = "needs-signature"
		}
		e.Fighter = f
	}
	return e
}
