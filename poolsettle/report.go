// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"math/big"
	"time"
)

func knownReason(s string) bool {
	switch s {
	case "", "payout-pending", "broadcast-unknown", "transaction-unavailable",
		"payout-conflict", "funding-unavailable", "funding-conflict", "funding-changed",
		"immature", "awaiting-confirmation", "below-minimum", "destination-unavailable",
		"daily-batch-complete", "fee-unavailable", "fee-budget", "change-dust",
		"signing-unavailable", "capacity", "transaction-capacity":
		return true
	}
	return false
}

// Available and reasons are historical observations. Ledger amounts always
// rederive from authenticated credits and validated exact transaction intents.
func validateReport(st state) error {
	r := st.Report
	if st.LastClockMS == 0 {
		if len(st.Credits) != 0 || len(st.Batches) != 0 || !same(r, Report{}) {
			return ErrCorrupt
		}
		return nil
	}
	if r.ObservedAtMS <= 0 || r.ObservedAtMS > st.LastClockMS || !knownReason(r.Reason) {
		return ErrCorrupt
	}
	day, e := time.Parse("2006-01-02", r.Day)
	if e != nil || day.Format("2006-01-02") != r.Day || r.Day > time.UnixMilli(r.ObservedAtMS).UTC().Format("2006-01-02") {
		return ErrCorrupt
	}
	switch r.State {
	case "pending", "complete", "deferred":
	default:
		return ErrCorrupt
	}
	wantTx := ""
	if len(st.Batches) > 0 {
		last := st.Batches[len(st.Batches)-1]
		wantTx = last.TxID
		if r.Day < last.Day {
			return ErrCorrupt
		}
	}
	if r.TxID != wantTx {
		return ErrCorrupt
	}
	expected, e := makeBalances(st, nil, "", nil)
	if e != nil || len(expected) != len(r.Balances) || len(expected) > 256 {
		return ErrCorrupt
	}
	for i, b := range r.Balances {
		want := expected[i]
		if b.GymID != want.GymID || b.Confirmed != want.Confirmed || b.PaidAtoms != want.PaidAtoms || b.ReservedAtoms != want.ReservedAtoms || !knownReason(b.Reason) {
			return ErrCorrupt
		}
		available, e := rat(b.Available)
		if e != nil {
			return ErrCorrupt
		}
		confirmed, e := rat(b.Confirmed)
		if e != nil {
			return ErrCorrupt
		}
		if available.Cmp(new(big.Rat).Sub(confirmed, new(big.Rat).SetInt64(b.ReservedAtoms))) > 0 || confirmed.Sign() == 0 && b.Reason != "" {
			return ErrCorrupt
		}
	}
	return nil
}
