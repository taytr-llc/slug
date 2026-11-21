// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/pooltxshape"
	"github.com/taytr-llc/slug/wire/v2"
)

const minimumChangeAtoms int64 = 294
const maxFractionDigits = 4096

func hexBytes(s string, n int) ([]byte, bool) {
	if len(s) != 2*n || s != strings.ToLower(s) {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	return b, err == nil
}

func nativeScript(s string) ([]byte, bool) {
	b, ok := hexBytes(s, 22)
	return b, ok && b[0] == 0 && b[1] == 20
}

func validateConfig(c Config) error {
	if c.Version != Version1 || c.MaxRounds == 0 || c.MaxRounds > 256 || c.MaxBatches == 0 || c.MaxBatches > 366 || len(c.Destinations) == 0 || len(c.Destinations) > 256 {
		return ErrConfig
	}
	if _, ok := nativeScript(c.ChangeScript); !ok {
		return ErrConfig
	}
	seen := map[string]bool{c.ChangeScript: true}
	for i, d := range c.Destinations {
		if _, ok := hexBytes(d.GymID, 32); !ok || i > 0 && d.GymID <= c.Destinations[i-1].GymID {
			return ErrConfig
		}
		if _, ok := nativeScript(d.Script); !ok || seen[d.Script] {
			return ErrConfig
		}
		seen[d.Script] = true
	}
	return nil
}

func frac(r *big.Rat) Fraction {
	return Fraction{Numerator: r.Num().String(), Denominator: r.Denom().String()}
}

func rat(f Fraction) (*big.Rat, error) {
	if len(f.Numerator) == 0 || len(f.Denominator) == 0 || len(f.Numerator) > maxFractionDigits || len(f.Denominator) > maxFractionDigits {
		return nil, ErrCorrupt
	}
	n, okN := new(big.Int).SetString(f.Numerator, 10)
	d, okD := new(big.Int).SetString(f.Denominator, 10)
	if !okN || !okD || n.Sign() < 0 || d.Sign() <= 0 || n.String() != f.Numerator || d.String() != f.Denominator {
		return nil, ErrCorrupt
	}
	r := new(big.Rat).SetFrac(n, d)
	if frac(r) != f {
		return nil, ErrCorrupt
	}
	return r, nil
}

func sameValue(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}

// deriveCredit performs exact arithmetic on an already verified concrete
// round. It does not authenticate imported records or independently prove PoW.
func deriveCredit(record poolround.Record) (Credit, error) {
	r := record.Round
	o := record.Confirmation
	if r.Version != poolround.Version1 || r.Sequence == 0 || r.RewardAtoms < 0 || r.RewardAtoms > MaxMoney || r.FirstAccepted == 0 || r.LastAccepted < r.FirstAccepted || r.LastAccepted-r.FirstAccepted >= 4096 || uint64(len(r.ShareKeys)) != r.LastAccepted-r.FirstAccepted+1 || len(r.Allocations) == 0 || len(r.Allocations) > 256 || r.Winner.Proof == nil || o == nil || !o.Canonical || o.BlockHash != r.BlockHash || o.BlockHeight != r.BlockHeight || o.Descendants < 60 || o.TipHeight < r.BlockHeight || o.TipHeight-r.BlockHeight != o.Descendants {
		return Credit{}, ErrSource
	}
	if _, ok := hexBytes(r.BlockHash, 32); !ok {
		return Credit{}, ErrSource
	}
	if _, ok := hexBytes(o.TipHash, 32); !ok {
		return Credit{}, ErrSource
	}
	proof := r.Winner.Proof
	if proof.Header != r.Winner.Share.Header || r.Winner.Share.WorkHash != r.BlockHash || r.Winner.RewardAtoms != r.RewardAtoms || len(proof.Coinbase) == 0 || len(proof.Coinbase) > 32768 {
		return Credit{}, ErrSource
	}
	raw, err := hex.DecodeString(proof.Coinbase)
	if err != nil || hex.EncodeToString(raw) != proof.Coinbase {
		return Credit{}, ErrSource
	}
	reader := bytes.NewReader(raw)
	tx := new(wire.MsgTx)
	if tx.DeserializeNoWitness(reader) != nil || reader.Len() != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) == 0 || tx.HasWitness() || tx.TxIn[0].PreviousOutPoint.Hash.String() != strings.Repeat("0", 64) || tx.TxIn[0].PreviousOutPoint.Index != ^uint32(0) {
		return Credit{}, ErrSource
	}
	var encoded bytes.Buffer
	if tx.SerializeNoWitness(&encoded) != nil || !bytes.Equal(encoded.Bytes(), raw) || tx.TxOut[0].Value != r.RewardAtoms {
		return Credit{}, ErrSource
	}
	script := hex.EncodeToString(tx.TxOut[0].PkScript)
	if _, ok := nativeScript(script); !ok {
		return Credit{}, ErrSource
	}
	for _, output := range tx.TxOut[1:] {
		if output.Value != 0 {
			return Credit{}, ErrSource
		}
	}
	shares := uint64(0)
	for i, a := range r.Allocations {
		if _, ok := hexBytes(a.GymID, 32); !ok || a.Shares == 0 || a.Shares > 4096-shares || i > 0 && a.GymID <= r.Allocations[i-1].GymID {
			return Credit{}, ErrSource
		}
		shares += a.Shares
	}
	if shares != r.LastAccepted-r.FirstAccepted+1 {
		return Credit{}, ErrSource
	}
	claims := []Claim{{RoundHash: r.BlockHash, RoundHeight: r.BlockHeight, Amount: frac(new(big.Rat).SetFrac(big.NewInt(r.RewardAtoms), big.NewInt(100)))}}
	for _, a := range r.Allocations {
		gross, err := rat(Fraction{a.Numerator, a.Denominator})
		expected := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(r.RewardAtoms), new(big.Int).SetUint64(a.Shares)), new(big.Int).SetUint64(shares))
		if err != nil || gross.Cmp(expected) != 0 {
			return Credit{}, ErrSource
		}
		net := new(big.Rat).Mul(gross, big.NewRat(99, 100))
		claims = append(claims, Claim{RoundHash: r.BlockHash, RoundHeight: r.BlockHeight, GymID: a.GymID, Amount: frac(net)})
	}
	data, err := json.Marshal(r)
	if err != nil {
		return Credit{}, ErrSource
	}
	digest := sha256.Sum256(append([]byte("SLUG pool settlement round\x00v1\x00"), data...))
	coin := Coin{TxID: tx.TxHash().String(), Index: 0, Atoms: r.RewardAtoms, Script: script, Coinbase: true, BlockHash: r.BlockHash, BlockHeight: r.BlockHeight, Claims: claims}
	return Credit{RoundHash: r.BlockHash, RoundDigest: hex.EncodeToString(digest[:]), Coin: coin}, nil
}

func coinKey(c Coin) string { return c.TxID + ":" + new(big.Int).SetUint64(uint64(c.Index)).String() }

func validCoin(c Coin, allowUnbound bool) error {
	if c.Atoms < 0 || c.Atoms > MaxMoney || len(c.Claims) == 0 || len(c.Claims) > 256*257 {
		return ErrCorrupt
	}
	if _, ok := nativeScript(c.Script); !ok {
		return ErrCorrupt
	}
	if _, ok := hexBytes(c.TxID, 32); !ok && !(allowUnbound && c.TxID == "") {
		return ErrCorrupt
	}
	if c.Coinbase {
		if _, ok := hexBytes(c.BlockHash, 32); !ok || c.Index != 0 {
			return ErrCorrupt
		}
	} else if c.BlockHash != "" || c.BlockHeight != 0 {
		return ErrCorrupt
	}
	total := new(big.Rat)
	seen := make(map[string]bool)
	for _, claim := range c.Claims {
		if claim.RoundHeight == 0 || claim.RoundHeight > 2147483647 {
			return ErrCorrupt
		}
		if _, ok := hexBytes(claim.RoundHash, 32); !ok {
			return ErrCorrupt
		}
		if claim.GymID != "" {
			if _, ok := hexBytes(claim.GymID, 32); !ok {
				return ErrCorrupt
			}
		}
		key := claim.RoundHash + claim.GymID
		amount, err := rat(claim.Amount)
		if err != nil || seen[key] {
			return ErrCorrupt
		}
		seen[key] = true
		total.Add(total, amount)
	}
	if total.Cmp(new(big.Rat).SetInt64(c.Atoms)) != 0 {
		return ErrCorrupt
	}
	return nil
}

// plan preserves the input/claim order when charging exact per-round claims.
// Caller-owned concrete credits, not this arithmetic function, grant authority.
func plan(coins []Coin, config Config, fee int64) ([]Payment, *Coin, string, error) {
	return planProfile(coins, config, fee, false)
}

func planDaily(coins []Coin, config Config, fee int64) ([]Payment, *Coin, string, error) {
	return planProfile(coins, config, fee, true)
}

func planProfile(coins []Coin, config Config, fee int64, daily bool) ([]Payment, *Coin, string, error) {
	configErr, inputLimit := validateConfig(config), MaxInputs
	if daily {
		configErr, inputLimit = validateDailyConfig(config), pooltxshape.MaxNativeInputs
	}
	if configErr != nil || fee < 0 || fee > MaxMoney || len(coins) > inputLimit {
		return nil, nil, "", ErrInvalid
	}
	destinations := make(map[string]string)
	for _, d := range config.Destinations {
		destinations[d.GymID] = d.Script
	}
	totals := make(map[string]*big.Rat)
	claims := make([]Claim, 0)
	amounts := make([]*big.Rat, 0)
	seenInputs := make(map[string]bool)
	seenClaims := make(map[string]bool)
	var inputAtoms int64
	for _, coin := range coins {
		if validCoin(coin, false) != nil || coin.Script != config.ChangeScript || coin.Atoms == 0 || coin.Atoms > MaxMoney-inputAtoms || seenInputs[coinKey(coin)] {
			return nil, nil, "", ErrCorrupt
		}
		seenInputs[coinKey(coin)] = true
		inputAtoms += coin.Atoms
		for _, claim := range coin.Claims {
			key := claim.RoundHash + claim.GymID
			if seenClaims[key] {
				return nil, nil, "", ErrCorrupt
			}
			seenClaims[key] = true
			if daily && len(seenClaims) > dailySigningClaims {
				return nil, nil, "", ErrCapacity
			}
			amount, _ := rat(claim.Amount)
			claims, amounts = append(claims, claim), append(amounts, amount)
			if totals[claim.GymID] == nil {
				totals[claim.GymID] = new(big.Rat)
			}
			totals[claim.GymID].Add(totals[claim.GymID], amount)
		}
	}
	payments := make([]Payment, 0)
	debits := make(map[string]*big.Rat)
	var paid int64
	for _, d := range config.Destinations {
		balance := totals[d.GymID]
		if balance == nil || balance.Cmp(new(big.Rat).SetInt64(MinimumAtoms)) < 0 {
			continue
		}
		whole := new(big.Int).Quo(balance.Num(), balance.Denom())
		if !whole.IsInt64() || whole.Int64() > MaxMoney-paid {
			return nil, nil, "", ErrCorrupt
		}
		atoms := whole.Int64()
		payments = append(payments, Payment{d.GymID, d.Script, atoms})
		paid += atoms
		debits[d.GymID] = new(big.Rat).SetInt64(atoms)
	}
	if len(payments) == 0 {
		for gym, balance := range totals {
			if gym != "" && destinations[gym] == "" && balance.Cmp(new(big.Rat).SetInt64(MinimumAtoms)) >= 0 {
				return nil, nil, "destination-unavailable", nil
			}
		}
		return nil, nil, "below-minimum", nil
	}
	feeBalance := totals[""]
	if feeBalance == nil || feeBalance.Cmp(new(big.Rat).SetInt64(fee)) < 0 {
		return nil, nil, "fee-budget", nil
	}
	debits[""] = new(big.Rat).SetInt64(fee)
	remaining := make([]Claim, 0, len(claims))
	for i, claim := range claims {
		amount := amounts[i]
		if debit := debits[claim.GymID]; debit != nil && debit.Sign() != 0 {
			if amount.Cmp(debit) <= 0 {
				debit.Sub(debit, amount)
				amount.SetInt64(0)
			} else {
				amount.Sub(amount, debit)
				debit.SetInt64(0)
			}
		}
		if amount.Sign() != 0 {
			claim.Amount = frac(amount)
			remaining = append(remaining, claim)
		}
	}
	for _, debit := range debits {
		if debit.Sign() != 0 {
			return nil, nil, "", ErrCorrupt
		}
	}
	changeAtoms := inputAtoms - paid - fee
	if changeAtoms < 0 {
		return nil, nil, "", ErrCorrupt
	}
	if changeAtoms == 0 {
		if len(remaining) != 0 {
			return nil, nil, "", ErrCorrupt
		}
		return payments, nil, "", nil
	}
	if changeAtoms < minimumChangeAtoms {
		return nil, nil, "change-dust", nil
	}
	change := &Coin{Index: uint32(len(payments)), Atoms: changeAtoms, Script: config.ChangeScript, Claims: remaining}
	if validCoin(*change, true) != nil {
		return nil, nil, "", ErrCorrupt
	}
	return payments, change, "", nil
}

// liveCoins replays every retained intent. A pending change is represented but
// is not spend authorization; the store blocks further batches until all prior
// transactions have fresh canonical observations.
func liveCoins(s state) ([]Coin, error) {
	// V1 keeps its exact selected limits and in-memory duplicate checks.
	if validateConfig(s.Config) != nil || len(s.Credits) > int(s.Config.MaxRounds) || len(s.Batches) > int(s.Config.MaxBatches) {
		return nil, ErrCorrupt
	}
	r, err := newSettlementReplay(s.Config, s.LastClockMS, replayCounts{uint64(len(s.Credits)), uint64(len(s.Batches))})
	if err != nil {
		return nil, err
	}
	seenRoots, seenCoins := make(map[string]bool), make(map[string]bool)
	for i, credit := range s.Credits {
		check := replayCreditCheck{uint64(i) + 1, seenRoots[credit.RoundHash], seenCoins[coinKey(credit.Coin)]}
		if err := r.addCredit(credit, check); err != nil {
			return nil, err
		}
		seenRoots[credit.RoundHash], seenCoins[coinKey(credit.Coin)] = true, true
	}
	seenTx := make(map[string]bool)
	for i, batch := range s.Batches {
		check := replayBatchCheck{Sequence: uint64(i) + 1, DuplicateTx: seenTx[batch.TxID]}
		if batch.Change != nil {
			check.DuplicateChange = seenCoins[coinKey(*batch.Change)]
		}
		if err := r.applyBatch(batch, check); err != nil {
			return nil, err
		}
		seenTx[batch.TxID] = true
		if batch.Change != nil {
			seenCoins[coinKey(*batch.Change)] = true
		}
	}
	if err := r.complete(); err != nil {
		return nil, err
	}
	return r.frontier(), nil
}
