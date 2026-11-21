// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/hex"
	"reflect"
	"time"

	"github.com/taytr-llc/slug/pooltxshape"
)

func (m *dailyMutation) sample() (int64, error) {
	ms := m.store.now().UnixMilli()
	if ms <= 0 || ms > 253402300799999 || ms < m.view.head.LastClockMS {
		return 0, ErrClock
	}
	m.view.head.LastClockMS = ms
	m.view.replay.clock = ms
	return ms, nil
}
func (m *dailyMutation) finish(ctx context.Context, day, phase, reason string, available []Coin, reasons map[string]string, recovery bool) (Report, error) {
	balances, err := m.view.replay.balances(available, reason, reasons)
	if err != nil {
		return Report{}, err
	}
	if recovery {
		for i := range balances {
			if balances[i].Reason == "below-minimum" {
				balances[i].Reason = ""
			}
		}
	}
	ms, err := m.sample()
	if err != nil {
		return Report{}, err
	}
	m.view.report = Report{Day: day, ObservedAtMS: ms, TxID: m.view.replay.last.TxID, State: phase, Reason: reason, Balances: balances}
	if err = m.commit(ctx); err != nil {
		return Report{}, err
	}
	return clone(m.view.report), nil
}

// inspectDaily authenticates the whole live frontier in ONE original backend
// observation. It does not combine independently sampled pages or choose a
// transaction subset. The daily node separately keeps its signing shape cap.
func (s *Store) inspectDaily(ctx context.Context, coins []Coin) ([]Coin, map[string]string, error) {
	if len(coins) == 0 {
		return []Coin{}, map[string]string{}, nil
	}
	if len(coins) > 16384 {
		return nil, nil, ErrCapacity
	}
	states, err := s.backend.Inspect(ctx, dailyOwnedCoins(coins))
	if err != nil {
		return nil, nil, err
	}
	if len(states) != len(coins) {
		return nil, nil, ErrBackend
	}
	ready := []Coin{}
	reasons := make(map[string]string)
	for i, f := range states {
		if err = ctx.Err(); err != nil {
			return nil, nil, err
		}
		c := coins[i]
		if f.TxID != c.TxID || f.Index != c.Index {
			return nil, nil, ErrBackend
		}
		switch f.State {
		case "ready":
			ready = append(ready, c)
		case "immature", "awaiting-confirmation", "funding-conflict":
			for _, claim := range c.Claims {
				if claim.GymID != "" && (reasons[claim.GymID] == "" || f.State == "funding-conflict") {
					reasons[claim.GymID] = f.State
				}
			}
		default:
			return nil, nil, ErrBackend
		}
	}
	return ready, reasons, ctx.Err()
}
func (m *dailyMutation) checkSource(ctx context.Context, contention *dailyContention, importNew bool) error {
	s := m.store
	snapshot, err := s.db.GetSnapshot()
	if err != nil {
		return ErrCorrupt
	}
	additional, err := s.dailySourceContention(ctx, contention, snapshot, s.daily.head, true)
	snapshot.Release()
	if err != nil {
		return err
	}
	if importNew {
		return m.stageCredits(ctx, additional)
	}
	// Newly staged credits must still match the exact source after signing.
	// Later unrelated confirmations may wait for the next ordinary import.
	want := make(map[string]Credit)
	for seq := s.daily.head.Credits + 1; seq <= m.view.head.Credits; seq++ {
		var c Credit
		if dailyDecode(m.writes[string(dailyNumber("c/", seq))], dailyCreditLimit, &c) != nil {
			return ErrCorrupt
		}
		want[c.RoundHash] = c
	}
	for _, c := range additional {
		if old, ok := want[c.RoundHash]; ok {
			if !reflect.DeepEqual(old, c) {
				return ErrSource
			}
			delete(want, c.RoundHash)
		}
	}
	if len(want) != 0 {
		return ErrSource
	}
	return ctx.Err()
}

func (s *Store) runDailySettlement(ctx context.Context, recoveryOnly bool) (Report, error) {
	return s.runDailySettlementSelected(ctx, recoveryOnly, "")
}

func (s *Store) runDailySettlementSelected(ctx context.Context, recoveryOnly bool, refreshDay string) (Report, error) {
	if s.auditOnly {
		return Report{}, ErrInvalid
	}
	contention := &dailyContention{}
	end, err := s.beginDailyContention(ctx, contention)
	if err != nil {
		return Report{}, err
	}
	defer end()
	v, err := s.auditDailyContention(ctx, contention)
	if err != nil {
		return Report{}, err
	}
	m := newDailyMutation(s, v)
	ms, err := m.sample()
	if err != nil {
		return Report{}, err
	}
	day := time.UnixMilli(ms).UTC().Format("2006-01-02")
	if refreshDay != "" {
		// The caller's durable admission day cannot authorize a new day's
		// payment. These guards precede observations, imports and all writes.
		if day != refreshDay {
			return Report{}, ErrDailyRefreshExpired
		}
		if v.head.LastDay < day || v.head.PendingBatch != 0 {
			return Report{}, ErrInvalid
		}
	}
	// The complete prior signed history remains present. Stream one bounded
	// body at a time; only the original last pending inputs are materialized.
	for seq := uint64(1); seq <= v.head.Batches; seq++ {
		if err = ctx.Err(); err != nil {
			return Report{}, err
		}
		raw, readErr := s.db.Get(dailyNumber("b/", seq), nil)
		var body dailyBatchBody
		if readErr != nil || dailyDecode(raw, dailyBatchLimit, &body) != nil {
			return Report{}, ErrCorrupt
		}
		observation, e := s.backend.ObserveTransaction(ctx, body.TxID)
		if e != nil {
			return m.finish(ctx, day, "deferred", "transaction-unavailable", nil, nil, false)
		}
		pending := seq == v.head.PendingBatch
		if len(observation.Raw) > 0 {
			original, e := hex.DecodeString(body.Raw)
			if e != nil || !bytes.Equal(original, observation.Raw) {
				return m.finish(ctx, day, "deferred", "payout-conflict", nil, nil, false)
			}
			if observation.Confirmations > 0 {
				if pending {
					if e = m.delivery("confirmed"); e != nil {
						return Report{}, e
					}
				}
				continue
			}
			if !pending {
				return m.finish(ctx, day, "deferred", "payout-conflict", nil, nil, false)
			}
			if e = m.delivery("broadcast"); e != nil {
				return Report{}, e
			}
			return m.finish(ctx, day, "pending", "payout-pending", nil, nil, false)
		}
		if observation.Confirmations != 0 {
			return Report{}, ErrBackend
		}
		if !pending {
			return m.finish(ctx, day, "deferred", "payout-conflict", nil, nil, false)
		}
		b := v.pending
		if b == nil {
			return Report{}, ErrCorrupt
		}
		ready, _, e := s.inspectDaily(ctx, b.Inputs)
		if e != nil {
			return m.finish(ctx, day, "pending", "funding-unavailable", nil, nil, false)
		}
		if len(ready) != len(b.Inputs) {
			return m.finish(ctx, day, "pending", "funding-conflict", nil, nil, false)
		}
		if e = m.checkSource(ctx, contention, false); e != nil {
			return Report{}, e
		}
		if _, e = m.finish(ctx, day, "pending", "payout-pending", nil, nil, false); e != nil {
			return Report{}, e
		}
		exact, _ := hex.DecodeString(b.Raw)
		id, e := s.backend.Broadcast(ctx, exact)
		if e != nil || id != b.TxID {
			return m.finish(ctx, day, "pending", "broadcast-unknown", nil, nil, false)
		}
		if e = m.delivery("broadcast"); e != nil {
			return Report{}, e
		}
		return m.finish(ctx, day, "pending", "payout-pending", nil, nil, false)
	}
	if recoveryOnly {
		return m.finish(ctx, day, "complete", "", nil, nil, true)
	}
	// Pending recovery above has no dependency on new intake capacity.
	if err = m.checkSource(ctx, contention, true); err != nil {
		if err == ErrCapacity {
			return m.finish(ctx, day, "deferred", "capacity", nil, nil, false)
		}
		return Report{}, err
	}
	v = m.view
	ready, reasons, err := s.inspectDaily(ctx, v.replay.coins)
	if err != nil {
		return m.finish(ctx, day, "deferred", "funding-unavailable", nil, nil, false)
	}
	if v.head.LastDay >= day {
		return m.finish(ctx, day, "complete", "daily-batch-complete", ready, reasons, false)
	}
	if len(ready) > pooltxshape.MaxNativeInputs {
		return m.finish(ctx, day, "deferred", "transaction-capacity", ready, reasons, false)
	}
	payments, change, fee, reason, err := s.dailyPlanQuote(ctx, ready)
	if err != nil {
		return Report{}, err
	}
	if reason != "" {
		return m.finish(ctx, day, "deferred", reason, ready, reasons, false)
	}
	if v.head.Batches >= uint64(s.state.Config.MaxBatches) {
		return m.finish(ctx, day, "deferred", "capacity", ready, reasons, false)
	}
	unsigned, err := unsignedTransactionProfile(ready, payments, change, true)
	if err != nil {
		return Report{}, err
	}
	signed, err := s.signSettlementRequest(ctx, ready, payments, change, fee, unsigned)
	if err != nil {
		return m.finish(ctx, day, "deferred", "signing-unavailable", ready, reasons, false)
	}
	txid, err := validateSignedProfile(ready, payments, change, fee, signed, true)
	if err != nil {
		return Report{}, err
	}
	if change != nil {
		change.TxID = txid
		change.Index = uint32(len(payments))
	}
	fresh, _, err := s.inspectDaily(ctx, ready)
	if err != nil || len(fresh) != len(ready) {
		return m.finish(ctx, day, "deferred", "funding-changed", nil, nil, false)
	}
	if err = m.checkSource(ctx, contention, false); err != nil {
		return Report{}, err
	}
	ms, err = m.sample()
	if err != nil {
		return Report{}, err
	}
	day = time.UnixMilli(ms).UTC().Format("2006-01-02")
	b := Batch{Day: day, CreatedAtMS: ms, Inputs: ready, Payments: payments, FeeAtoms: fee, Change: change, TxID: txid, Raw: hex.EncodeToString(signed), State: "prepared"}
	if err = m.stageBatch(ctx, b); err != nil {
		return Report{}, err
	}
	// Complete credits/debits/input reservations/change/day/raw are one synced
	// commit, including the latest report, before any network transmission.
	if _, err = m.finish(ctx, day, "pending", "payout-pending", nil, nil, false); err != nil {
		return Report{}, err
	}
	id, err := s.backend.Broadcast(ctx, append([]byte(nil), signed...))
	if err != nil || id != txid {
		return m.finish(ctx, day, "pending", "broadcast-unknown", nil, nil, false)
	}
	if err = m.delivery("broadcast"); err != nil {
		return Report{}, err
	}
	return m.finish(ctx, day, "pending", "payout-pending", nil, nil, false)
}
