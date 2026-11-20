// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
)

const capacityStateBytes uint64 = 64 << 20
const capacityReportReserve uint64 = 256*(4*4096+2048) + 4096

// CapacityReservation is a conservative logical-journal bound for one finite
// original source. It is not a payout promise or a continuous-service horizon.
// Physical filesystem/LevelDB capacity remains separate and can stop intake
// earlier. Nothing here changes the immutable Setup or its existing limits.
type CapacityReservation struct {
	Submissions       uint64 `json:"submissions"`
	MaxBatches        uint64 `json:"max_batches"`
	StateBytes        uint64 `json:"state_bytes"`
	RoundBytes        uint64 `json:"round_bytes"`
	LargestRoundBytes uint64 `json:"largest_round_bytes"`
}

type capacityInputs struct {
	SourceMax    uint64 `json:"source_max"`
	Rounds       uint64 `json:"rounds"`
	RoundBytes   uint64 `json:"round_bytes"`
	CreditRounds uint64 `json:"credit_rounds"`
	Batches      uint64 `json:"batches"`
	Gyms         uint64 `json:"gyms"`
}

// CapacityEnvelope validates the full selected Setup, then derives the largest
// finite row ceiling fitting every existing round/batch/byte constraint. A zero
// ceiling is valid and means recovery-only; it never invents a larger journal.
func CapacityEnvelope(s Setup) (CapacityReservation, error) {
	if err := ValidateSetup(s); err != nil {
		return CapacityReservation{}, err
	}
	return capacityEnvelope(capacityInputs{s.Pool.MaxSubmissions, s.RoundLimits.MaxRounds, s.RoundLimits.MaxBytes, uint64(s.Settlement.MaxRounds), uint64(s.Settlement.MaxBatches), uint64(len(s.Settlement.Destinations))})
}

func capacityEnvelope(in capacityInputs) (CapacityReservation, error) {
	if in.SourceMax == 0 || in.SourceMax > 4096 || in.Rounds == 0 || in.Rounds > 256 || in.RoundBytes < 4096 || in.RoundBytes > 1<<30 || in.CreditRounds == 0 || in.CreditRounds > 256 || in.Batches == 0 || in.Batches > 366 || in.Gyms == 0 || in.Gyms > 64 {
		return CapacityReservation{}, ErrInvalid
	}
	max := min(in.SourceMax, in.Rounds, in.CreditRounds, uint64(256), (in.Batches+1)/2)
	// lcm(1..w)*100 bounds every surviving fractional denominator for a
	// partition whose largest window has at most w accepted shares.
	denominators := make([]*big.Int, max+1)
	lcm := big.NewInt(1)
	for w := uint64(1); w <= max; w++ {
		v := new(big.Int).SetUint64(w)
		gcd := new(big.Int).GCD(nil, nil, lcm, v)
		lcm.Mul(new(big.Int).Quo(lcm, gcd), v)
		denominators[w] = new(big.Int).Mul(lcm, big.NewInt(100))
	}
	var best CapacityReservation
	for n := uint64(1); n <= max; n++ {
		bound := capacityBound(n, in.Gyms, denominators)
		if bound.StateBytes <= capacityStateBytes-capacityReportReserve && bound.RoundBytes <= in.RoundBytes && bound.LargestRoundBytes <= 2<<20 {
			best = bound
		}
	}
	return best, nil
}

// The field shapes below deliberately track the bounded V1 journal grammar,
// independently checked against the frozen Python arithmetic. Commas are
// overcounted. No financial state or expected transaction is generated here.
func capacityBound(n, gyms uint64, denominators []*big.Int) CapacityReservation {
	h := strings.Repeat("f", 64)
	script := "0014" + strings.Repeat("f", 40)
	money := int64(21000000 * 100000000)
	size := func(v any) uint64 { raw, _ := json.Marshal(v); return uint64(len(raw)) }
	coin := map[string]any{"txid": h, "index": uint64(4294967295), "atoms": money, "script": script, "coinbase": false, "block_hash": h, "block_height": 2147483647, "claims": []any{}}
	coinBytes := size(coin)
	creditBytes := size(map[string]any{"round_hash": h, "round_digest": h, "coin": coin})
	paymentBytes := size(map[string]any{"gym_id": h, "script": script, "atoms": money})
	batchBytes := size(map[string]any{"day": strings.Repeat("9", 20), "created_at_ms": int64(9223372036854775807), "inputs": []any{}, "payments": []any{}, "fee_atoms": money, "change": nil, "txid": h, "raw": "", "state": "confirmed"})
	allocationBytes := size(map[string]any{"gym_id": h, "shares": n, "numerator": strconv.FormatUint(uint64(money)*n, 10), "denominator": strconv.FormatUint(n, 10)})
	roundBytes := size(map[string]any{"version": 1, "sequence": n, "block_hash": h, "block_height": 2147483647, "first_accepted": n, "last_accepted": n, "share_keys": []any{}, "pool_target": h, "payout_address": strings.Repeat("p", 128), "reward_atoms": money, "allocations": []any{}, "winner": nil})
	out := CapacityReservation{Submissions: n, MaxBatches: 2*n - 1, LargestRoundBytes: roundBytes + 65536 + n*67 + n*(allocationBytes+1)}
	for r := uint64(1); r <= n; r++ {
		denom := denominators[n-r+1]
		numerator := new(big.Int).Mul(big.NewInt(money), denom)
		claimBytes := size(map[string]any{"round_hash": h, "round_height": 2147483647, "gym_id": h, "amount": map[string]string{"numerator": numerator.String(), "denominator": denom.String()}})
		claims, batches := n+r, 2*r-1
		inputs, copies := r+batches, 2*claims*r
		payees := min(gyms, n)
		signedBytes := 16*batches + 150*inputs + 31*(payees+1)*batches
		state := uint64(65536) + capacityReportReserve + r*(creditBytes+1) + claims*(claimBytes+1) + batches*(batchBytes+1) + (inputs+batches)*(coinBytes+1) + copies*(claimBytes+1) + batches*payees*(paymentBytes+1) + 2*signedBytes
		out.StateBytes = max(out.StateBytes, state)
		out.RoundBytes = max(out.RoundBytes, r*(roundBytes+65536+4096)+n*67+n*(allocationBytes+1))
	}
	return out
}
