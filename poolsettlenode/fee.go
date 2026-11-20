// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
	"math/big"
	"strconv"
	"strings"
)

// estimatefee's actual return type is mempool.BtcPerKilobyte. Parse exact coins
// per 1,000 virtual bytes, ceil to atoms, and never use binary floating point.
func estimatedRate(raw json.RawMessage, cap int64) (int64, error) {
	if len(raw) == 0 || len(raw) > 64 || !decimalAmount.Match(raw) || cap < 1 || cap > 1000000000 {
		return 0, ErrUnavailable
	}
	if i := strings.IndexAny(string(raw), "eE"); i >= 0 {
		e, err := strconv.ParseInt(string(raw[i+1:]), 10, 32)
		if err != nil || e < -32 || e > 32 {
			return 0, ErrUnavailable
		}
	}
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || r.Sign() <= 0 {
		return 0, ErrUnavailable
	}
	r.Mul(r, big.NewRat(100000000, 1))
	n := new(big.Int).Add(r.Num(), new(big.Int).Sub(r.Denom(), big.NewInt(1)))
	n.Quo(n, r.Denom())
	if !n.IsInt64() || n.Int64() < 1 || n.Int64() > cap {
		return 0, ErrUnavailable
	}
	return n.Int64(), nil
}
func conservativeVSize(inputs, payments int) (int64, error) {
	if inputs < 1 || inputs > poolsettle.MaxInputs || payments < 1 || payments > 256 {
		return 0, ErrInvalid
	}
	outputs := payments + 1 // quoted change may be absent; never underquote it
	base := int64(4 + wire.VarIntSerializeSize(uint64(inputs)) + 41*inputs + wire.VarIntSerializeSize(uint64(outputs)) + 31*outputs + 4)
	weight := 4*base + 2 + 109*int64(inputs)
	return (weight + 3) / 4, nil
}
func quotedAtoms(rate, size, cap int64) (int64, error) {
	if rate < 1 || rate > 1000000000 || size < 1 || size > poolsettle.MaxTransactionBytes || cap < 1 {
		return 0, ErrInvalid
	}
	atoms := (rate*size + 999) / 1000
	if atoms > cap {
		return 0, ErrUnavailable
	}
	return atoms, nil
}
func (b *Client) EstimateFee(ctx context.Context, coins []poolsettle.Coin, payments []poolsettle.Payment, change string) (int64, error) {
	work, end, err := b.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer end()
	size, err := conservativeVSize(len(coins), len(payments))
	if err != nil || change != b.config.PoolScript {
		return 0, ErrInvalid
	}
	seen := map[string]bool{}
	for _, coin := range coins {
		if !b.validCoin(coin) {
			return 0, ErrInvalid
		}
		key := coin.TxID + ":" + strconv.FormatUint(uint64(coin.Index), 10)
		if seen[key] {
			return 0, ErrInvalid
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for i, p := range payments {
		raw, err := hex.DecodeString(p.Script)
		if err != nil || len(raw) != 22 || raw[0] != 0 || raw[1] != 20 || hex.EncodeToString(raw) != p.Script || p.Script == change || !hash(p.GymID) || p.Atoms < poolsettle.MinimumAtoms || p.Atoms > poolsettle.MaxMoney || seen[p.Script] || i > 0 && payments[i-1].GymID >= p.GymID {
			return 0, ErrInvalid
		}
		seen[p.Script] = true
	}
	before, err := b.snapshot(work)
	if err != nil {
		return 0, err
	}
	rate := b.config.Fee.AtomsPerKVB
	if b.config.Fee.Mode == "node" {
		var raw json.RawMessage
		if b.call(work, "estimatefee", []any{b.config.Fee.ConfirmationTarget}, &raw) != nil {
			return 0, ErrUnavailable
		}
		rate, err = estimatedRate(raw, b.config.Fee.MaxAtomsPerKVB)
		if err != nil {
			return 0, err
		}
	}
	fee, err := quotedAtoms(rate, size, b.config.Fee.MaxFeeAtoms)
	if err != nil {
		return 0, err
	}
	if err = b.unchanged(work, before); err != nil {
		return 0, err
	}
	if err = b.check(work); err != nil {
		return 0, err
	}
	return fee, nil
}
