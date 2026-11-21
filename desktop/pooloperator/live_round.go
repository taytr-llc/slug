// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/poolround"
)

// EnableLiveRoundMaintenance is an explicit local selection on the original
// runtime before its first Run. It adds no wire/config mode or financial
// authority. Full Sync and financial audits remain unchanged.
func (r *Runtime) EnableLiveRoundMaintenance(ctx context.Context) error {
	if r == nil || ctx == nil {
		return ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	work, done, e := r.beginAction(bounded)
	if e != nil {
		return e
	}
	defer done()
	r.mu.Lock()
	if r.started || r.closing || r.closed {
		r.mu.Unlock()
		return ErrUnavailable
	}
	if r.rounds == nil || r.round.store != r.rounds {
		r.mu.Unlock()
		return ErrIntegrity
	}
	r.selectingLiveRounds = true
	store := r.rounds
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.selectingLiveRounds = false
		r.mu.Unlock()
	}()
	live, e := store.BeginLive(work)
	if e != nil {
		return e
	}
	if live == nil {
		return ErrIntegrity
	}
	if e = r.actionDisclosure(work); e != nil {
		return e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e = work.Err(); e != nil {
		return e
	}
	if r.started || r.closing || r.closed || r.rounds != store || r.round.store != store {
		return ErrUnavailable
	}
	r.round.live = live
	return nil
}

// refreshLive is used only by the original serial round loop. Reanchoring is
// explicit, at most once per refresh, and shares the caller's existing budget.
// Exact stale handles may reanchor; corruption/unknown/closed errors never do.
func (m *roundMaintenance) refreshLive(ctx context.Context) error {
	return m.refreshLiveSelected(ctx, false)
}

func (m *roundMaintenance) refreshLiveSelected(ctx context.Context, daily bool) error {
	if daily {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	reanchored := false
	reanchor := func(e error) error {
		if e != poolround.ErrLiveStale || reanchored {
			return e
		}
		reanchored = true
		live, err := dailyRoundCall(ctx, daily, func(work context.Context) (*poolround.Live, error) { return m.store.BeginLive(work) })
		if err != nil {
			return err
		}
		if live == nil {
			return ErrIntegrity
		}
		m.live = live
		return nil
	}
	list := func(after uint64) ([]poolround.Record, error) {
		rows, e := dailyRoundCall(ctx, daily, func(work context.Context) ([]poolround.Record, error) { return m.live.List(work, after) })
		if e == poolround.ErrLiveStale {
			if e = reanchor(e); e == nil {
				rows, e = dailyRoundCall(ctx, daily, func(work context.Context) ([]poolround.Record, error) { return m.live.List(work, after) })
			}
		}
		return rows, e
	}
	rows, e := list(m.cursor)
	if e != nil {
		return e
	}
	if len(rows) == 0 && m.cursor != 0 {
		m.cursor = 0
		rows, e = list(0)
		if e != nil {
			return e
		}
	}
	for _, row := range rows {
		_, e = dailyRoundCall(ctx, daily, func(work context.Context) (poolround.Record, error) { return m.live.Refresh(work, row.Round.BlockHash) })
		if e == poolround.ErrLiveStale {
			if e = reanchor(e); e == nil {
				_, e = dailyRoundCall(ctx, daily, func(work context.Context) (poolround.Record, error) { return m.live.Refresh(work, row.Round.BlockHash) })
			}
		}
		if e != nil {
			return e
		}
		m.cursor = row.Round.Sequence
	}
	return ctx.Err()
}
