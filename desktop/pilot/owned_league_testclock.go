//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"reflect"
	"time"
)

// UseOwnedCalendarOffsetForTest is an idle-only fixture phase transition. It
// is absent from ordinary binaries and has no CLI/protocol/config counterpart.
// A failed partial installation requires closing the fixture; never continue
// with mixed clocks or attempt to rewind them. The harness must also prohibit
// further mining/job/admission work and compare its final authority counters.
func (r *PoolRuntime) UseOwnedCalendarOffsetForTest(ctx context.Context, expected OwnedLeagueConfig, offset time.Duration) error {
	if r == nil || offset <= 0 || offset > 8*24*time.Hour || ValidateOwnedLeagueConfig(expected) != nil {
		return ErrPoolRuntime
	}
	work, done, e := r.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	if !r.mu.TryLock() {
		return ErrPoolRuntimeBusy
	}
	if r.closed || len(r.borrows) != 0 || r.ownedGym == nil || r.ownedGym.league == nil || r.owned == nil {
		r.mu.Unlock()
		return ErrPoolRuntimeBusy
	}
	l, o := r.ownedGym.league, r.owned
	valid := l.ready != nil && l.life.Err() == nil && l.runtime == r && l.source == o.store && reflect.DeepEqual(l.config, expected) && o.config == expected.Source && o.replay == nil && o.progress.Ready && l.cutoffReplay == nil
	// Child borrowing also takes r.mu, without r.gate. Retain this bounded
	// metadata lock through installation so idle cannot change underneath us.
	// Every leaf setter uses only its own nonblocking gate/mutex; none reenters r.
	defer r.mu.Unlock()
	if !valid {
		return ErrPoolRuntime
	}
	// The fixed offset preserves real elapsed time. CaptureBoundary's timer and
	// both real node requests remain untouched; no synthetic observation enters.
	clock := func() time.Time { return time.Now().Add(offset) }
	if e = o.publisher.UseAuthorityClockForTest(work, expected.Source.Ownership, clock); e != nil {
		return e
	}
	if e = o.store.UseCalendarClockForTest(work, expected.Source.Ownership, expected.Source.Store, clock); e != nil {
		return e
	}
	for i, participant := range expected.Participants {
		if l.stable[i] == nil || l.entry[i] == nil {
			return ErrPoolRuntime
		}
		if e = l.stable[i].UseOwnedClockForTest(work, participant.StablePin, clock); e != nil {
			return e
		}
		if e = l.entry[i].UseOwnedClockForTest(work, participant.EntryPin, clock); e != nil {
			return e
		}
	}
	if l.events == nil || l.results == nil {
		return ErrPoolRuntime
	}
	if e = l.events.UseOwnedClockForTest(work, expected.EventPin, clock); e != nil {
		return e
	}
	if e = l.results.UseOwnedClockForTest(work, expected.ResultPin, clock); e != nil {
		return e
	}
	l.calendarNow = clock
	return nil
}
