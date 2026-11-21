// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"encoding/hex"
	"math/big"
	"sort"
	"time"
)

// Exact pre-extraction function snapshots: differential V1 compatibility only.
// They are not independent financial/transaction expected-value generators.
func replayLegacyLiveCoins(s state) ([]Coin, error) {
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

func replayLegacyMakeBalances(st state, available []Coin, reason string, reasons map[string]string) ([]Balance, error) {
	totals, ready := make(map[string]*big.Rat), make(map[string]*big.Rat)
	paid, reserved := make(map[string]int64), make(map[string]int64)
	dest := make(map[string]bool)
	for _, d := range st.Config.Destinations {
		dest[d.GymID] = true
		totals[d.GymID] = new(big.Rat)
	}
	for _, c := range st.Credits {
		for _, claim := range c.Coin.Claims {
			if claim.GymID == "" {
				continue
			}
			v, e := rat(claim.Amount)
			if e != nil {
				return nil, e
			}
			add(totals, claim.GymID, v)
		}
	}
	for _, b := range st.Batches {
		for _, p := range b.Payments {
			if b.State == "confirmed" {
				paid[p.GymID] += p.Atoms
			} else {
				reserved[p.GymID] += p.Atoms
			}
		}
	}
	for _, c := range available {
		for _, claim := range c.Claims {
			if claim.GymID == "" {
				continue
			}
			v, e := rat(claim.Amount)
			if e != nil {
				return nil, e
			}
			add(ready, claim.GymID, v)
		}
	}
	keys := make([]string, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Balance, 0, len(keys))
	for _, gym := range keys {
		balance := new(big.Rat).Sub(totals[gym], new(big.Rat).SetInt64(paid[gym]))
		if balance.Sign() < 0 {
			return nil, ErrCorrupt
		}
		avail := ready[gym]
		if avail == nil {
			avail = new(big.Rat)
		}
		unreserved := new(big.Rat).Sub(balance, new(big.Rat).SetInt64(reserved[gym]))
		if unreserved.Sign() < 0 || avail.Cmp(unreserved) > 0 {
			return nil, ErrCorrupt
		}
		why := ""
		switch {
		case reserved[gym] > 0:
			why = reason
			if why == "" {
				why = "payout-pending"
			}
		case balance.Sign() == 0:
		case !dest[gym]:
			why = "destination-unavailable"
		case reason != "" && reason != "below-minimum":
			why = reason
		case reasons[gym] != "":
			why = reasons[gym]
		case avail.Cmp(new(big.Rat).SetInt64(MinimumAtoms)) < 0:
			why = "below-minimum"
		}
		out = append(out, Balance{GymID: gym, Confirmed: frac(balance), Available: frac(avail), ReservedAtoms: reserved[gym], PaidAtoms: paid[gym], Reason: why})
	}
	return out, nil
}
