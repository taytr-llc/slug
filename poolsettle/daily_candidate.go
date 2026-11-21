// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"github.com/taytr-llc/slug/pooltxshape"
)

// dailyPlanQuote uses the whole already-observed ready set. Capacity is decided
// before the fee RPC, whose selected daily profile conservatively sizes change.
// It neither signs nor changes the ledger/day; RunDaily commits any deferral.
func (s *Store) dailyPlanQuote(ctx context.Context, ready []Coin) ([]Payment, *Coin, int64, string, error) {
	payments, _, reason, err := planDaily(ready, s.state.Config, 0)
	if err == ErrCapacity {
		return nil, nil, 0, "transaction-capacity", nil
	}
	if err != nil {
		return nil, nil, 0, "", err
	}
	if reason != "" || len(payments) == 0 {
		return nil, nil, 0, reason, nil
	}
	if _, err = pooltxshape.NativeP2WPKH(len(ready), len(payments), true); err != nil {
		return nil, nil, 0, "transaction-capacity", nil
	}
	fee, err := s.backend.EstimateFee(ctx, dailyOwnedCoins(ready), append([]Payment(nil), payments...), s.state.Config.ChangeScript)
	if err != nil {
		return nil, nil, 0, "fee-unavailable", nil
	}
	payments, change, reason, err := planDaily(ready, s.state.Config, fee)
	if err == ErrCapacity {
		return nil, nil, 0, "transaction-capacity", nil
	}
	if err != nil {
		return nil, nil, 0, "", err
	}
	if reason != "" {
		return nil, nil, 0, reason, nil
	}
	if _, err = pooltxshape.NativeP2WPKH(len(ready), len(payments), change != nil); err != nil {
		return nil, nil, 0, "transaction-capacity", nil
	}
	return payments, change, fee, "", ctx.Err()
}
