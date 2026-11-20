// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"sort"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
)

func add(m map[string]*big.Rat, key string, v *big.Rat) {
	if m[key] == nil {
		m[key] = new(big.Rat)
	}
	m[key].Add(m[key], v)
}
func makeBalances(st state, available []Coin, reason string, reasons map[string]string) ([]Balance, error) {
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

func (s *Store) finish(ctx context.Context, st state, day, phase, reason string, available []Coin, reasons map[string]string) (Report, error) {
	balances, err := makeBalances(st, available, reason, reasons)
	if err != nil {
		return Report{}, err
	}
	now := s.now().UnixMilli()
	if now < st.LastClockMS {
		return Report{}, ErrClock
	}
	st.LastClockMS = now
	st.Report = Report{Day: day, ObservedAtMS: now, State: phase, Reason: reason, Balances: balances}
	if len(st.Batches) > 0 {
		st.Report.TxID = st.Batches[len(st.Batches)-1].TxID
	}
	if err = ctx.Err(); err != nil {
		return Report{}, err
	}
	if err = s.commit(st, false); err != nil {
		return Report{}, err
	}
	return clone(st.Report), nil
}

func (s *Store) inspect(ctx context.Context, coins []Coin) ([]Coin, map[string]string, error) {
	if len(coins) == 0 {
		return []Coin{}, map[string]string{}, nil
	}
	states, err := s.backend.Inspect(ctx, clone(coins))
	if err != nil {
		return nil, nil, err
	}
	if len(states) != len(coins) {
		return nil, nil, ErrBackend
	}
	ready := []Coin{}
	reasons := make(map[string]string)
	for i, f := range states {
		c := coins[i]
		if f.TxID != c.TxID || f.Index != c.Index {
			return nil, nil, ErrBackend
		}
		switch f.State {
		case "ready":
			ready = append(ready, c)
		case "immature", "awaiting-confirmation", "funding-conflict":
			for _, claim := range c.Claims {
				if claim.GymID != "" {
					old := reasons[claim.GymID]
					if old == "" || f.State == "funding-conflict" {
						reasons[claim.GymID] = f.State
					}
				}
			}
		default:
			return nil, nil, ErrBackend
		}
	}
	return ready, reasons, nil
}

// RunDaily is the operator's daily job. UTC calendar days are idempotency
// slots, not allocation windows. A deferred day may retry; once a transaction
// is prepared that day's slot is permanently occupied. A pending transaction
// blocks later batches and can only be recovered or rebroadcast byte-for-byte.
func (s *Store) RunDaily(ctx context.Context) (Report, error) {
	if s != nil && s.auditOnly {
		return Report{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer end()
	if err = s.verify(ctx); err != nil {
		return Report{}, err
	}
	st := clone(s.state)
	now := s.now()
	ms := now.UnixMilli()
	if ms <= 0 || ms < st.LastClockMS {
		return Report{}, ErrClock
	}
	st.LastClockMS = ms
	day := now.UTC().Format("2006-01-02")
	// Recheck every old transaction before spending any new funding. A later
	// loss of a historically confirmed payment is an incident, not permission
	// to debit miners again or substitute fresh rewards.
	for i := range st.Batches {
		b := &st.Batches[i]
		observation, e := s.backend.ObserveTransaction(ctx, b.TxID)
		if e != nil {
			return s.finish(ctx, st, day, "deferred", "transaction-unavailable", nil, nil)
		}
		if len(observation.Raw) > 0 {
			raw, e := hex.DecodeString(b.Raw)
			if e != nil || !bytes.Equal(raw, observation.Raw) {
				return s.finish(ctx, st, day, "deferred", "payout-conflict", nil, nil)
			}
			if observation.Confirmations > 0 {
				b.State = "confirmed"
				continue
			}
			if b.State == "confirmed" {
				return s.finish(ctx, st, day, "deferred", "payout-conflict", nil, nil)
			}
			b.State = "broadcast"
			return s.finish(ctx, st, day, "pending", "payout-pending", nil, nil)
		}
		if observation.Confirmations != 0 {
			return Report{}, ErrBackend
		}
		if b.State == "confirmed" {
			return s.finish(ctx, st, day, "deferred", "payout-conflict", nil, nil)
		}
		// Unknown does not free a reservation. Fresh input checks permit only
		// retransmission of this already journaled transaction.
		ready, _, e := s.inspect(ctx, b.Inputs)
		if e != nil {
			return s.finish(ctx, st, day, "pending", "funding-unavailable", nil, nil)
		}
		if len(ready) != len(b.Inputs) {
			return s.finish(ctx, st, day, "pending", "funding-conflict", nil, nil)
		}
		if e = s.sourceState(ctx, &st, false); e != nil {
			return Report{}, e
		}
		// Persist new confirmation observations and the clock before this RPC.
		if e = s.commit(st, false); e != nil {
			return Report{}, e
		}
		raw, _ := hex.DecodeString(b.Raw)
		txid, e := s.backend.Broadcast(ctx, raw)
		if e != nil || txid != b.TxID {
			return s.finish(ctx, st, day, "pending", "broadcast-unknown", nil, nil)
		}
		b.State = "broadcast"
		return s.finish(ctx, st, day, "pending", "payout-pending", nil, nil)
	}
	// Existing exact-byte recovery always precedes capacity for new credits.
	// A full journal must not strand an already reserved payment.
	if err = s.sourceState(ctx, &st, true); err != nil {
		if err == ErrCapacity {
			return s.finish(ctx, st, day, "deferred", "capacity", nil, nil)
		}
		return Report{}, err
	}
	coins, err := liveCoins(st)
	if err != nil {
		return Report{}, err
	}
	ready, reasons, err := s.inspect(ctx, coins)
	if err != nil {
		return s.finish(ctx, st, day, "deferred", "funding-unavailable", nil, nil)
	}
	if len(st.Batches) > 0 && st.Batches[len(st.Batches)-1].Day >= day {
		return s.finish(ctx, st, day, "complete", "daily-batch-complete", ready, reasons)
	}
	payments, _, reason, err := plan(ready, st.Config, 0)
	if err != nil {
		return Report{}, err
	}
	if len(payments) == 0 || reason != "" {
		return s.finish(ctx, st, day, "deferred", reason, ready, reasons)
	}
	fee, err := s.backend.EstimateFee(ctx, clone(ready), clone(payments), st.Config.ChangeScript)
	if err != nil {
		return s.finish(ctx, st, day, "deferred", "fee-unavailable", ready, reasons)
	}
	payments, change, reason, err := plan(ready, st.Config, fee)
	if err != nil {
		return Report{}, err
	}
	if reason != "" {
		return s.finish(ctx, st, day, "deferred", reason, ready, reasons)
	}
	if len(st.Batches) >= int(st.Config.MaxBatches) {
		return s.finish(ctx, st, day, "deferred", "capacity", ready, reasons)
	}
	unsigned, err := unsignedTransaction(ready, payments, change)
	if err != nil {
		return Report{}, err
	}
	raw, err := s.backend.Sign(ctx, clone(ready), append([]byte{}, unsigned...))
	if err != nil {
		return s.finish(ctx, st, day, "deferred", "signing-unavailable", ready, reasons)
	}
	txid, err := validateSigned(ready, payments, change, fee, raw)
	if err != nil {
		return Report{}, err
	}
	if change != nil {
		change.TxID = txid
		change.Index = uint32(len(payments))
	}
	// Slow signing cannot authorize stale inputs or changed source evidence.
	stillReady, _, err := s.inspect(ctx, ready)
	if err != nil || len(stillReady) != len(ready) {
		return s.finish(ctx, st, day, "deferred", "funding-changed", nil, nil)
	}
	if err = s.sourceState(ctx, &st, false); err != nil {
		return Report{}, err
	}
	st.Batches = append(st.Batches, Batch{Day: day, CreatedAtMS: ms, Inputs: ready, Payments: payments, FeeAtoms: fee, Change: change, TxID: txid, Raw: hex.EncodeToString(raw), State: "prepared"})
	// Every miner debit, fee debit, input reservation, change claim and signed
	// byte is committed together BEFORE the first possible network submission.
	balances, err := makeBalances(st, nil, "payout-pending", nil)
	if err != nil {
		return Report{}, err
	}
	st.Report = Report{Day: day, ObservedAtMS: ms, TxID: txid, State: "pending", Reason: "payout-pending", Balances: balances}
	if err = ctx.Err(); err != nil {
		return Report{}, err
	}
	if _, err = liveCoins(st); err != nil {
		return Report{}, err
	}
	commitMS := s.now().UnixMilli()
	if commitMS < st.LastClockMS {
		return Report{}, ErrClock
	}
	st.LastClockMS = commitMS
	// The daily slot belongs to the durable preparation time. Signing across
	// midnight must not permit two new batches in the same calendar day.
	day = time.UnixMilli(commitMS).UTC().Format("2006-01-02")
	st.Batches[len(st.Batches)-1].Day = day
	st.Batches[len(st.Batches)-1].CreatedAtMS = commitMS
	st.Report.Day, st.Report.ObservedAtMS = day, commitMS
	if err = s.commit(st, true); err != nil {
		return Report{}, err
	}
	result, e := s.backend.Broadcast(ctx, append([]byte{}, raw...))
	if e != nil || result != txid {
		return s.finish(ctx, st, day, "pending", "broadcast-unknown", nil, nil)
	}
	st.Batches[len(st.Batches)-1].State = "broadcast"
	return s.finish(ctx, st, day, "pending", "payout-pending", nil, nil)
}

// View returns the last settlement attempt, including explicit deferral reasons.
// Its timestamp identifies historical funding observations, not a fresh UTXO
// claim. Spending never consumes this report; RunDaily rechecks source and node.
func (s *Store) View(ctx context.Context) (Report, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer end()
	if err = s.verify(ctx); err != nil {
		return Report{}, err
	}
	return clone(s.state.Report), nil
}

// MinerBalance binds the balance view to the original live gym session. It
// exposes no other miner, funding input, destination list or signed transaction.
// The host transport/UI can display Reason directly with ObservedAtMS.
func (s *Store) MinerBalance(ctx context.Context, sessions prospectservice.SessionAuthorizer, token string) (Balance, time.Time, error) {
	if !nonnil(sessions) {
		return Balance{}, time.Time{}, ErrInvalid
	}
	end, e := s.begin(ctx)
	if e != nil {
		return Balance{}, time.Time{}, e
	}
	defer end()
	lease, e := mininglease.Capture(ctx, sessions, token)
	if e != nil {
		return Balance{}, time.Time{}, e
	}
	p := s.state.Source.Policy
	gym, e := gymauth.GymID(prospect.Context{Version: p.Version, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion}, lease.PublicKey())
	if e != nil {
		return Balance{}, time.Time{}, e
	}
	// Verification can traverse source proofs. Retain the settlement operation
	// permit, but never hold the global account lease over this slow work.
	if e = s.verify(ctx); e != nil {
		return Balance{}, time.Time{}, e
	}
	report := clone(s.state.Report)
	result := Balance{GymID: gym, Confirmed: Fraction{"0", "1"}, Available: Fraction{"0", "1"}}
	for _, b := range report.Balances {
		if b.GymID == gym {
			result = b
			break
		}
	}
	var observed time.Time
	if report.ObservedAtMS != 0 {
		observed = time.UnixMilli(report.ObservedAtMS)
	}
	// Only the original credential may disclose the prepared gym balance.
	// The copied report retains its historical observation time.
	if e = lease.Check(); e != nil {
		return Balance{}, time.Time{}, e
	}
	return result, observed, nil
}
