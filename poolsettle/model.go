// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/taytr-llc/slug/poolround"
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
	if validateConfig(config) != nil || fee < 0 || fee > MaxMoney || len(coins) > MaxInputs {
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
	if validateConfig(s.Config) != nil || len(s.Credits) > int(s.Config.MaxRounds) || len(s.Batches) > int(s.Config.MaxBatches) {
		return nil, ErrCorrupt
	}
	coins := make([]Coin, 0, len(s.Credits)+1)
	seenRoots, seenCoins := make(map[string]bool), make(map[string]bool)
	for _, credit := range s.Credits {
		coin := credit.Coin
		if validCoin(coin, false) != nil || !coin.Coinbase || coin.BlockHash != credit.RoundHash || coin.Script != s.Config.ChangeScript || seenRoots[credit.RoundHash] || seenCoins[coinKey(coin)] {
			return nil, ErrCorrupt
		}
		if _, ok := hexBytes(credit.RoundDigest, 32); !ok {
			return nil, ErrCorrupt
		}
		fee, net := new(big.Rat), new(big.Rat)
		for _, claim := range coin.Claims {
			if claim.RoundHash != credit.RoundHash || claim.RoundHeight != coin.BlockHeight {
				return nil, ErrCorrupt
			}
			amount, _ := rat(claim.Amount)
			if claim.GymID == "" {
				fee.Add(fee, amount)
			} else {
				net.Add(net, amount)
			}
		}
		if fee.Cmp(new(big.Rat).SetFrac(big.NewInt(coin.Atoms), big.NewInt(100))) != 0 || net.Cmp(new(big.Rat).Mul(new(big.Rat).SetInt64(coin.Atoms), big.NewRat(99, 100))) != 0 {
			return nil, ErrCorrupt
		}
		seenRoots[credit.RoundHash], seenCoins[coinKey(coin)] = true, true
		if coin.Atoms != 0 {
			coins = append(coins, coin)
		}
	}
	lastDay := ""
	lastTime := int64(0)
	seenTx := make(map[string]bool)
	for batchIndex, batch := range s.Batches {
		if batch.Day <= lastDay || batch.CreatedAtMS <= lastTime || batch.CreatedAtMS > s.LastClockMS || batch.Day != time.UnixMilli(batch.CreatedAtMS).UTC().Format("2006-01-02") || batch.State != "prepared" && batch.State != "broadcast" && batch.State != "confirmed" || batchIndex < len(s.Batches)-1 && batch.State != "confirmed" || len(batch.Inputs) == 0 {
			return nil, ErrCorrupt
		}
		lastDay, lastTime = batch.Day, batch.CreatedAtMS
		remaining := make([]Coin, 0, len(coins))
		next := 0
		for _, coin := range coins {
			if next < len(batch.Inputs) && coinKey(coin) == coinKey(batch.Inputs[next]) {
				if !sameValue(coin, batch.Inputs[next]) {
					return nil, ErrCorrupt
				}
				next++
			} else {
				remaining = append(remaining, coin)
			}
		}
		if next != len(batch.Inputs) {
			return nil, ErrCorrupt
		}
		payments, change, reason, err := plan(batch.Inputs, s.Config, batch.FeeAtoms)
		if err != nil || reason != "" || !sameValue(payments, batch.Payments) {
			return nil, ErrCorrupt
		}
		if change != nil {
			change.TxID = batch.TxID
		}
		if !sameValue(change, batch.Change) || len(batch.Raw) == 0 || len(batch.Raw) > 2*MaxTransactionBytes {
			return nil, ErrCorrupt
		}
		raw, err := hex.DecodeString(batch.Raw)
		if err != nil || hex.EncodeToString(raw) != batch.Raw {
			return nil, ErrCorrupt
		}
		txid, err := validateSigned(batch.Inputs, batch.Payments, batch.Change, batch.FeeAtoms, raw)
		if err != nil || txid != batch.TxID || seenTx[txid] {
			return nil, ErrCorrupt
		}
		seenTx[txid] = true
		coins = remaining
		if change != nil {
			if seenCoins[coinKey(*change)] {
				return nil, ErrCorrupt
			}
			seenCoins[coinKey(*change)] = true
			coins = append([]Coin{*change}, coins...)
		}
	}
	return coins, nil
}
