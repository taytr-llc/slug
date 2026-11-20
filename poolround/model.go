// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
)

const maxAccepted = 4096
const maxMoney = int64(21000000 * 100000000)

func canonicalHash(value string, nonzero bool) bool {
	if len(value) != 64 || value != strings.ToLower(value) || nonzero && value == strings.Repeat("0", 64) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cloneEntry(e poolshare.AccountingEntry) poolshare.AccountingEntry {
	if e.Proof != nil {
		proof := *e.Proof
		if proof.MerkleBranch != nil {
			proof.MerkleBranch = append(make([]string, 0, len(proof.MerkleBranch)), proof.MerkleBranch...)
		}
		e.Proof = &proof
	}
	return e
}

// derive is conditional arithmetic over an immutable acceptance projection.
// Only snapshot's concrete source can authorize these inputs for storage.
func derive(config poolshare.Config, entries []poolshare.AccountingEntry) ([]Round, error) {
	if len(entries) > maxAccepted || !canonicalHash(config.PoolTarget, true) {
		return nil, ErrSourceChanged
	}
	rounds := make([]Round, 0)
	keys := make(map[string]bool)
	submissions := make(map[uint64]bool)
	winners := make(map[string]bool)
	counts := make(map[string]uint64)
	window := make([]string, 0)
	first := uint64(1)
	for i, e := range entries {
		r, j := e.Share, e.Job
		if r.Version != poolshare.Version1 || r.AcceptedSequence != uint64(i+1) || r.Sequence == 0 || submissions[r.Sequence] || !canonicalHash(r.DuplicateKey, false) || keys[r.DuplicateKey] || !canonicalHash(r.WorkHash, false) || !canonicalHash(r.GymID, false) || r.GymID != j.GymID || r.JobID != j.ID || r.PoolTarget != config.PoolTarget || r.BlockState != "" || r.BlockObservedAtMS != 0 || (!r.Pool && !r.Block) || r.AcceptedAtMS < r.ReceivedAtMS || r.ReceivedAtMS < j.IssuedAtMS || len(r.Header) != 160 {
			return nil, ErrSourceChanged
		}
		if _, err := hex.DecodeString(r.Header); err != nil {
			return nil, ErrSourceChanged
		}
		switch r.Acceptance {
		case "current-parent":
			if !r.Pool || r.AcceptedAtMS >= j.ExpiresAtMS {
				return nil, ErrSourceChanged
			}
		case "canonical-winning-block":
			if !r.Block || r.AcceptedAtMS >= j.ExpiresAtMS {
				return nil, ErrSourceChanged
			}
		case "canonical-late-winner":
			if !r.Block || r.AcceptedAtMS < j.ExpiresAtMS {
				return nil, ErrSourceChanged
			}
		default:
			return nil, ErrSourceChanged
		}
		keys[r.DuplicateKey] = true
		submissions[r.Sequence] = true
		counts[r.GymID]++
		window = append(window, r.DuplicateKey)
		if !r.Block {
			if e.Proof != nil || e.RewardAtoms != 0 || r.BlockHeight != 0 {
				return nil, ErrSourceChanged
			}
			continue
		}
		if winners[r.WorkHash] || j.Context.ParentHeight >= math.MaxInt32 || r.BlockHeight != j.Context.ParentHeight+1 || e.Proof == nil || e.Proof.Header != r.Header || e.Proof.GymID != r.GymID || e.Proof.JobID != r.JobID || e.RewardAtoms < 0 || e.RewardAtoms > maxMoney {
			return nil, ErrSourceChanged
		}
		proofBytes, err := json.Marshal(e.Proof)
		if err != nil || len(proofBytes) > prospect.MaxProofSize {
			return nil, ErrSourceChanged
		}
		winners[r.WorkHash] = true
		gyms := make([]string, 0, len(counts))
		for gym := range counts {
			gyms = append(gyms, gym)
		}
		sort.Strings(gyms)
		allocations := make([]Allocation, 0, len(gyms))
		for _, gym := range gyms {
			numerator := new(big.Int).Mul(big.NewInt(e.RewardAtoms), new(big.Int).SetUint64(counts[gym]))
			amount := new(big.Rat).SetFrac(numerator, new(big.Int).SetUint64(uint64(len(window))))
			allocations = append(allocations, Allocation{gym, counts[gym], amount.Num().String(), amount.Denom().String()})
		}
		rounds = append(rounds, Round{Version: Version1, Sequence: uint64(len(rounds)) + 1, BlockHash: r.WorkHash, BlockHeight: r.BlockHeight, FirstAccepted: first, LastAccepted: r.AcceptedSequence, ShareKeys: append([]string(nil), window...), PoolTarget: config.PoolTarget, PayoutAddress: config.PayoutAddress, RewardAtoms: e.RewardAtoms, Allocations: allocations, Winner: cloneEntry(e)})
		first = r.AcceptedSequence + 1
		counts = make(map[string]uint64)
		window = make([]string, 0)
	}
	return rounds, nil
}

func initialPrefix(config poolshare.Config) string {
	raw, _ := json.Marshal(config)
	h := sha256.New()
	h.Write([]byte("SLUG pool accepted prefix\x00v1\x00"))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

func extendPrefix(previous string, e poolshare.AccountingEntry) (string, error) {
	prior, err := hex.DecodeString(previous)
	if err != nil || len(prior) != 32 {
		return "", ErrCorrupt
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > maxEntrySize {
		return "", ErrSourceChanged
	}
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(raw)))
	h := sha256.New()
	h.Write(prior)
	h.Write(size[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validObservation(o prospectnode.PoolBlockObservation, r Round) bool {
	if o.BlockHash != r.BlockHash || o.BlockHeight != r.BlockHeight || !canonicalHash(o.TipHash, true) || o.TipHeight > math.MaxInt32 {
		return false
	}
	if o.Canonical {
		return o.TipHeight >= r.BlockHeight && o.Descendants == o.TipHeight-r.BlockHeight && (o.TipHeight != r.BlockHeight || o.TipHash == r.BlockHash)
	}
	return o.Descendants == 0 && (o.TipHeight != r.BlockHeight || o.TipHash != r.BlockHash)
}

func validRecord(record Record) bool {
	if record.Observation != nil && !validObservation(*record.Observation, record.Round) {
		return false
	}
	if record.Confirmation != nil {
		if record.Observation == nil || !validObservation(*record.Confirmation, record.Round) || !record.Confirmation.Canonical || record.Confirmation.Descendants < 60 {
			return false
		}
	}
	// A successful >=60 observation must have synchronized its first lock.
	return record.Observation == nil || !record.Observation.Canonical || record.Observation.Descendants < 60 || record.Confirmation != nil
}
