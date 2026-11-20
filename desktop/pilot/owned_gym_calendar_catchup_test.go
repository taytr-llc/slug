// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

// This exercises real entry -> stable -> core completeness with the existing
// bounded genesis node fixture. No mocked readiness or prepared fighter data
// enters the source. The account callback is only this local lifecycle fixture.
func TestOwnedGymCalendarContinuesUnavailableCurrentReplay(t *testing.T) {
	r, _, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	if e := r.PrepareOwnedSource(ctx, r.owned.config); e != nil {
		t.Fatal(e)
	}
	config, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.marker.AccountAuthorityID, strings.Repeat("1", 64), strings.Repeat("2", 64))
	if e != nil {
		t.Fatal(e)
	}
	p := config.Participants[0]
	stable, e := gymstable.CreateOwned(filepath.Join(r.config.DataDir, "calendar-stable"), p.StablePin, r.owned.store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := stable.Close(); e != nil {
			t.Error(e)
		}
	})
	entry, e := gymentry.CreateOwned(ctx, filepath.Join(r.config.DataDir, "calendar-entry"), p.EntryPin, stable)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := entry.Close(); e != nil {
			t.Error(e)
		}
	})
	f := p.StablePin.Stable.Identity
	a := gymaccount.Account{ID: p.StablePin.Stable.AccountID, Generation: 1, Enrollment: &f}
	c := &OwnedGymChild{runtime: r, stable: stable, entries: entry, life: r.life,
		view: OwnedGymView{Version: 1, ViewID: strings.Repeat("3", 64), AccountAuthorityID: p.StablePin.Stable.AccountAuthorityID,
			AccountID: a.ID, Generation: a.Generation, Fixture: p.Fixture, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID},
		scope: OwnedGymScope{EventPin: config.EventPin, EntryPin: p.EntryPin}}
	c.account.Lease = func(work context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrForbidden
		}
		return fn(a, func() error { return work.Err() })
	}
	c.league = &OwnedLeague{runtime: r, source: r.owned.store, life: r.life, config: config, ready: &ownedLeagueReady{}}
	// Calendar metadata only: keep this scheduling regression away from the
	// near-noon branch regardless of the real time at which the suite executes.
	c.league.calendarNow = func() time.Time { return time.Date(2027, 1, 4, 12, 0, 0, 0, time.UTC) }
	if _, e = entry.Upcoming(ctx, c.session()); e != nil {
		t.Fatal("complete original source", e)
	}
	source, e := fighterownership.NewPublicSource(ctx, r.owned.config.Ownership, r.owned.publisher)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := r.owned.store.BeginReplay(ctx, source, r.owned.node)
	if e != nil {
		t.Fatal(e)
	}
	r.owned.source, r.owned.replay = source, replay
	r.owned.progress = fighterownership.Progress{WorkID: replay.WorkID()}
	if _, e = entry.Upcoming(ctx, c.session()); e != fighterownership.ErrUnavailable {
		t.Fatal("fixture did not enter actual incomplete replay", e)
	}
	// A definite original-session refusal must not be treated as catch-up.
	lease := c.account.Lease
	c.account.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	before := r.owned.progress
	if _, done, e := c.begin(ctx, c.view.ViewID); e == nil {
		done()
		t.Fatal("denied original View admitted calendar work")
	}
	if r.owned.progress != before || r.owned.replay != replay {
		t.Fatal("denied View advanced current work")
	}
	c.account.Lease = lease
	steps := 0
	for ; steps < 32 && !r.owned.progress.Ready; steps++ {
		work, done, e := c.begin(ctx, c.view.ViewID)
		if e != nil {
			t.Fatal(e)
		}
		continueReplay := c.refreshOwnedEvents(work)
		done()
		if continueReplay != !r.owned.progress.Ready {
			t.Fatal("real incomplete replay lost continuation or advertised completion", continueReplay, r.owned.progress)
		}
		// A continuation is not an unreleased action lease: the original UI
		// View can take the gate before the next bounded background slice.
		_, releaseUI, e := c.begin(ctx, c.view.ViewID)
		if e != nil {
			t.Fatal("replay continuation retained child action gate", e)
		}
		releaseUI()
	}
	if steps < 1 || !r.owned.progress.Ready || r.owned.replay != nil {
		t.Fatal("calendar starved retained current replay", steps, r.owned.progress.Phase)
	}
	head, e := r.owned.store.Head(ctx)
	if e != nil || head.ChainHeight != 0 || head.ChainHash != r.owned.config.Ownership.Source.Genesis {
		t.Fatal("actual replay did not restore complete genesis head", e)
	}
	if _, e = entry.Upcoming(ctx, c.session()); e != nil {
		t.Fatal("original entry remained unavailable after replay", e)
	}
	// Cancellation never supplies a cached ready result.
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, e = r.owned.store.Head(canceled); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled head read disclosed", e)
	}
}

func TestOwnedGymCalendarOrdinaryWorkReservesBoundary(t *testing.T) {
	event := gymentry.Event{CutoffMS: 50000}
	for _, row := range []struct {
		now    int64
		want   time.Duration
		wallet bool
	}{
		{10000, 20 * time.Second, true},
		{30000, 12 * time.Second, true},
		{36999, 5001 * time.Millisecond, true},
		{37000, 5 * time.Second, false},
		{41999, time.Millisecond, false},
		{42000, 0, false},
		{49999, 0, false},
		{50000, 0, false},
	} {
		got := ownedCalendarWorkBudget(event, row.now, 20*time.Second)
		wallet := ownedCalendarWorkBudget(event, row.now, 6*time.Second) > 5*time.Second
		if got != row.want || wallet != row.wallet {
			t.Fatal("reserved boundary budget", row.now, got, wallet)
		}
		if got > 0 && row.now+got.Milliseconds() > event.CutoffMS-ownedCalendarReserve.Milliseconds() {
			t.Fatal("ordinary work can consume reserved bracket")
		}
	}
}

func TestOwnedGymCalendarUpcomingUsesPinnedCalendar(t *testing.T) {
	p := ownedCalendarPin(t).Participants[0]
	for _, row := range []struct {
		before, after string
		hours         int64
	}{
		{"2027-03-13", "2027-03-20", 167},
		{"2027-11-06", "2027-11-13", 169},
		{"2026-12-26", "2027-01-02", 168},
	} {
		event, e := gymentry.OwnedEventForDate(p, row.before)
		if e != nil {
			t.Fatal(e)
		}
		pre, e := gymentry.OwnedUpcomingEvent(p, event.CutoffMS-1)
		if e != nil || pre != event {
			t.Fatal("pre-noon calendar", e)
		}
		next, e := gymentry.OwnedUpcomingEvent(p, event.CutoffMS)
		if e != nil || next.Saturday != row.after || next.CutoffMS-event.CutoffMS != row.hours*int64(time.Hour/time.Millisecond) {
			t.Fatal("exact noon/DST calendar", e)
		}
	}
	if _, e := gymentry.OwnedUpcomingEvent(gymentry.OwnedPin{}, time.Now().UnixMilli()); e == nil {
		t.Fatal("unbound entry pin admitted")
	}
	if _, e := gymentry.OwnedUpcomingEvent(p, -1); e == nil {
		t.Fatal("invalid calendar time admitted")
	}
}
