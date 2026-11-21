//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle

import (
	"context"
	"time"
)

// DailyActualClock changes only the idle original journal's bookkeeping clock.
// It exists only in the opt-in test binary. Work/node/expiry clocks are untouched.
func DailyActualClock(ctx context.Context, s *Store, at time.Time) error {
	if s == nil || s.daily == nil || at.UnixMilli() <= 0 {
		return ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if at.UnixMilli() < s.daily.head.LastClockMS {
		return ErrClock
	}
	s.now = func() time.Time { return at }
	return ctx.Err()
}

type DailyActualState struct {
	Credits  uint64
	Batches  []Batch
	Pending  uint64
	Frontier []Coin
	Report   Report
}

// DailyActualSnapshot is bounded evidence access, never a capability or source.
// The original owner first audits its whole namespace and source. A second
// bounded replay reconstructs at most two selected original bodies, with owned
// slices. No raw DB or arbitrary callback escapes to the external fixture.
func DailyActualSnapshot(ctx context.Context, s *Store) (DailyActualState, error) {
	if s == nil || s.daily == nil {
		return DailyActualState{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return DailyActualState{}, err
	}
	defer end()
	v, err := s.auditDaily(ctx)
	if err != nil {
		return DailyActualState{}, err
	}
	if v.head.Credits > 721 || v.head.Batches > 2 || v.head.Events > 723 {
		return DailyActualState{}, ErrCapacity
	}
	r, err := newDailySettlementReplay(s.state.Config, v.head.LastClockMS, replayCounts{Credits: v.head.Credits, Batches: v.head.Batches})
	if err != nil {
		return DailyActualState{}, err
	}
	read := func(key []byte, limit int, out any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := s.db.Get(key, nil)
		if err != nil {
			return ErrCorrupt
		}
		return dailyDecode(raw, limit, out)
	}
	out := DailyActualState{Credits: v.head.Credits, Pending: v.head.PendingBatch, Report: clone(v.report), Batches: []Batch{}}
	for i := uint64(1); i <= v.head.Events; i++ {
		var ref dailyReference
		if err = read(dailyNumber("e/", i), dailyReferenceLimit, &ref); err != nil {
			return DailyActualState{}, err
		}
		switch ref.Kind {
		case "credit":
			var c Credit
			if err = read(dailyNumber("c/", ref.Sequence), dailyCreditLimit, &c); err != nil {
				return DailyActualState{}, err
			}
			if err = r.addCredit(c, replayCreditCheck{Sequence: ref.Sequence}); err != nil {
				return DailyActualState{}, err
			}
		case "batch":
			var body dailyBatchBody
			var state dailyDelivery
			if err = read(dailyNumber("b/", ref.Sequence), dailyBatchLimit, &body); err != nil {
				return DailyActualState{}, err
			}
			if err = read(dailyNumber("bs/", ref.Sequence), dailyReferenceLimit, &state); err != nil {
				return DailyActualState{}, err
			}
			b, e := dailyReconstruct(ctx, r, body, state)
			if e != nil {
				return DailyActualState{}, e
			}
			if err = r.applyBatch(b, replayBatchCheck{Sequence: ref.Sequence}); err != nil {
				return DailyActualState{}, err
			}
			out.Batches = append(out.Batches, clone(b))
		default:
			return DailyActualState{}, ErrCorrupt
		}
	}
	if err = r.complete(); err != nil {
		return DailyActualState{}, err
	}
	out.Frontier = dailyOwnedCoins(r.coins)
	if err = ctx.Err(); err != nil {
		return DailyActualState{}, err
	}
	return out, nil
}
