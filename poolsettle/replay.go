// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"encoding/hex"
	"github.com/taytr-llc/slug/pooltxshape"
	"math"
	"math/big"
	"reflect"
	"time"
)

// settlementReplay is conditional arithmetic over caller-authenticated rows.
// It owns the live frontier and accounting projection, never historical Batch
// bodies or all-time duplicate sets. The caller must enforce its selected
// limits, complete namespace/count audit, and original source before use.
// Duplicate facts below come from that caller's retained indices, not DTOs.
// This private seam grants no source, funding, signing or storage authority.
type settlementReplay struct {
	daily          bool
	originalClaims uint64
	config         Config
	clock          int64
	expected       replayCounts
	seen           replayCounts
	coins          []Coin
	accounting     balanceProjection
	last           replayLastBatch
}

type replayCounts struct{ Credits, Batches uint64 }
type replayLastBatch struct {
	Day         string
	CreatedAtMS int64
	TxID, State string
}
type replayCreditCheck struct {
	Sequence                      uint64
	DuplicateRound, DuplicateCoin bool
}
type replayBatchCheck struct {
	Sequence                     uint64
	DuplicateTx, DuplicateChange bool
}

func newSettlementReplay(config Config, clock int64, expected replayCounts) (*settlementReplay, error) {
	if validateConfig(config) != nil {
		return nil, ErrCorrupt
	}
	return &settlementReplay{config: clone(config), clock: clock, expected: expected,
		coins: []Coin{}, accounting: newBalanceProjection(config)}, nil
}

func newDailySettlementReplay(config Config, clock int64, expected replayCounts) (*settlementReplay, error) {
	if validateDailyConfig(config) != nil || expected.Credits > uint64(config.MaxRounds) || expected.Batches > uint64(config.MaxBatches) {
		return nil, ErrCorrupt
	}
	owned := config
	owned.Destinations = append([]Destination(nil), config.Destinations...)
	return &settlementReplay{daily: true, config: owned, clock: clock, expected: expected, coins: []Coin{}, accounting: newBalanceProjection(config)}, nil
}
func (r *settlementReplay) equal(a, b any) bool {
	if r.daily {
		return reflect.DeepEqual(a, b)
	}
	return sameValue(a, b)
}
func (r *settlementReplay) ownedCoin(c Coin) Coin {
	if r.daily {
		c.Claims = append([]Claim(nil), c.Claims...)
		return c
	}
	return clone(c)
}

// addCredit may precede or follow a completed batch. Appending a new root after
// the existing frontier preserves the V1 order (new changes are prepended).
// A rejected transition leaves the accumulator unchanged. No row is published
// until the caller has also completed its own audit and complete() succeeds.
func (r *settlementReplay) addCredit(credit Credit, check replayCreditCheck) error {
	if r == nil || r.seen.Credits >= r.expected.Credits || check.Sequence != r.seen.Credits+1 {
		return ErrCorrupt
	}
	coin := credit.Coin
	if r.daily && uint64(len(coin.Claims)) > dailySigningClaims-r.originalClaims {
		return ErrCorrupt
	}
	if validCoin(coin, false) != nil || !coin.Coinbase || coin.BlockHash != credit.RoundHash || coin.Script != r.config.ChangeScript || check.DuplicateRound || check.DuplicateCoin {
		return ErrCorrupt
	}
	if _, ok := hexBytes(credit.RoundDigest, 32); !ok {
		return ErrCorrupt
	}
	fee, net := new(big.Rat), new(big.Rat)
	for _, claim := range coin.Claims {
		if claim.RoundHash != credit.RoundHash || claim.RoundHeight != coin.BlockHeight {
			return ErrCorrupt
		}
		amount, _ := rat(claim.Amount)
		if claim.GymID == "" {
			fee.Add(fee, amount)
		} else {
			net.Add(net, amount)
		}
	}
	if fee.Cmp(new(big.Rat).SetFrac(big.NewInt(coin.Atoms), big.NewInt(100))) != 0 || net.Cmp(new(big.Rat).Mul(new(big.Rat).SetInt64(coin.Atoms), big.NewRat(99, 100))) != 0 {
		return ErrCorrupt
	}
	owned := r.ownedCoin(coin)
	for _, claim := range owned.Claims {
		if claim.GymID == "" {
			continue
		}
		amount, _ := rat(claim.Amount)
		add(r.accounting.gross, claim.GymID, amount)
	}
	if owned.Atoms != 0 {
		r.coins = append(r.coins, owned)
	}
	r.seen.Credits++
	if r.daily {
		r.originalClaims += uint64(len(coin.Claims))
	}
	return nil
}

func (r *settlementReplay) applyBatch(batch Batch, check replayBatchCheck) error {
	if r == nil || r.seen.Batches >= r.expected.Batches || check.Sequence != r.seen.Batches+1 {
		return ErrCorrupt
	}
	if batch.Day <= r.last.Day || batch.CreatedAtMS <= r.last.CreatedAtMS || batch.CreatedAtMS > r.clock || batch.Day != time.UnixMilli(batch.CreatedAtMS).UTC().Format("2006-01-02") || batch.State != "prepared" && batch.State != "broadcast" && batch.State != "confirmed" || check.Sequence < r.expected.Batches && batch.State != "confirmed" || r.seen.Batches > 0 && r.last.State != "confirmed" || len(batch.Inputs) == 0 {
		return ErrCorrupt
	}
	remaining := make([]Coin, 0, len(r.coins))
	next := 0
	for _, coin := range r.coins {
		if next < len(batch.Inputs) && coinKey(coin) == coinKey(batch.Inputs[next]) {
			if !r.equal(coin, batch.Inputs[next]) {
				return ErrCorrupt
			}
			next++
		} else {
			remaining = append(remaining, coin)
		}
	}
	if next != len(batch.Inputs) {
		return ErrCorrupt
	}
	payments, change, reason, err := planProfile(batch.Inputs, r.config, batch.FeeAtoms, r.daily)
	if err != nil || reason != "" || !r.equal(payments, batch.Payments) {
		return ErrCorrupt
	}
	if change != nil {
		change.TxID = batch.TxID
	}
	rawLimit := MaxTransactionBytes
	if r.daily {
		rawLimit = pooltxshape.MaxStandardWeight
	}
	if !r.equal(change, batch.Change) || len(batch.Raw) == 0 || len(batch.Raw) > 2*rawLimit {
		return ErrCorrupt
	}
	raw, err := hex.DecodeString(batch.Raw)
	if err != nil || hex.EncodeToString(raw) != batch.Raw {
		return ErrCorrupt
	}
	txid, err := validateSignedProfile(batch.Inputs, batch.Payments, batch.Change, batch.FeeAtoms, raw, r.daily)
	if err != nil || txid != batch.TxID || check.DuplicateTx {
		return ErrCorrupt
	}
	if change != nil && check.DuplicateChange {
		return ErrCorrupt
	}
	// Existing V1 bounds already keep these sums representable. The shared
	// accumulator also refuses arithmetic overflow without selecting new limits.
	tally := r.accounting.reserved
	if batch.State == "confirmed" {
		tally = r.accounting.paid
	}
	for _, p := range payments {
		if p.Atoms > math.MaxInt64-tally[p.GymID] {
			return ErrCorrupt
		}
	}
	for _, p := range payments {
		tally[p.GymID] += p.Atoms
	}
	r.coins = remaining
	if change != nil {
		r.coins = append([]Coin{r.ownedCoin(*change)}, r.coins...)
	}
	r.seen.Batches++
	r.last = replayLastBatch{batch.Day, batch.CreatedAtMS, batch.TxID, batch.State}
	return nil
}

func (r *settlementReplay) complete() error {
	if r == nil || r.seen != r.expected {
		return ErrCorrupt
	}
	return nil
}

// frontier is an owned conditional projection, including any reserved pending
// change. It is never a claim that the coins are currently spendable.
func (r *settlementReplay) frontier() []Coin {
	if r.daily {
		return dailyOwnedCoins(r.coins)
	}
	return clone(r.coins)
}

func (r *settlementReplay) balances(available []Coin, reason string, reasons map[string]string) ([]Balance, error) {
	if r.complete() != nil {
		return nil, ErrCorrupt
	}
	return projectBalances(r.config, r.accounting, available, reason, reasons)
}

type balanceProjection struct {
	gross          map[string]*big.Rat
	paid, reserved map[string]int64
}

func newBalanceProjection(config Config) balanceProjection {
	p := balanceProjection{gross: make(map[string]*big.Rat), paid: make(map[string]int64), reserved: make(map[string]int64)}
	for _, d := range config.Destinations {
		p.gross[d.GymID] = new(big.Rat)
	}
	return p
}
