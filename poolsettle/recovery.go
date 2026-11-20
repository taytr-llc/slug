// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import "context"

// RecoverPending observes every retained batch and may retransmit only an
// already persisted transaction's exact bytes. It never imports new credits,
// plans a payment, quotes a fee, signs, or occupies a new daily slot.
//
// Pending and deferred reports retain RunDaily's recovery meanings. A complete
// report with an empty Reason means only that this recovery scan finished: no
// retained transaction remains unresolved. It does not mean today's payment
// ran or that any unreserved funds are spendable. Available is conservatively
// zero because recovery does not inspect unreserved funding. Confirmed,
// ReservedAtoms and PaidAtoms still derive from the existing ledger and the
// saved transaction observations. The report's Day dates the recovery attempt;
// only a retained Batch.Day occupies a payment slot.
func (s *Store) RecoverPending(ctx context.Context) (Report, error) {
	return s.runSettlement(ctx, true)
}

func (s *Store) finishRecovery(ctx context.Context, st state, day string) (Report, error) {
	balances, err := makeBalances(st, nil, "", nil)
	if err != nil {
		return Report{}, err
	}
	for i := range balances {
		// No unreserved funding was inspected. The generic balance builder's
		// zero-availability fallback must not claim a below-minimum decision.
		// Preserve an independently known missing destination, if any.
		if balances[i].Reason == "below-minimum" {
			balances[i].Reason = ""
		}
	}
	return s.finishBalances(ctx, st, day, "complete", "", balances)
}
