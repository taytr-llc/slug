// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

// Build the actual concrete consumer chain and activate its result reader.
// No wallet recipient verification is fabricated to enter OpenOwnedLeague;
// this fixture is only the calendar/career composition over verified genesis.
func ownedCalendarCareerFixture(t *testing.T) (*OwnedGymChild, *gymaccount.Account) {
	t.Helper()
	r, _, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	if e := r.PrepareOwnedSource(ctx, r.owned.config); e != nil {
		t.Fatal(e)
	}
	config, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.marker.AccountAuthorityID, strings.Repeat("1", 64), strings.Repeat("2", 64))
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(r.life)
	l := &OwnedLeague{config: config, runtime: r, source: r.owned.store, root: filepath.Join(r.config.DataDir, "career-calendar"), life: life, cancel: cancel}
	t.Cleanup(func() {
		if e := l.Close(); e != nil {
			t.Error(e)
		}
	})
	for i, p := range config.Participants {
		l.stable[i], e = gymstable.CreateOwned(filepath.Join(r.config.DataDir, "stable-"+p.Fixture), p.StablePin, l.source)
		if e != nil {
			t.Fatal(e)
		}
		l.entry[i], e = gymentry.CreateOwned(ctx, filepath.Join(r.config.DataDir, "entry-"+p.Fixture), p.EntryPin, l.stable[i])
		if e != nil {
			t.Fatal(e)
		}
	}
	l.events, e = gymevent.CreateOwned(ctx, filepath.Join(r.config.DataDir, "calendar-events"), config.EventPin, l.source)
	if e != nil {
		t.Fatal(e)
	}
	l.seeds, e = gymseed.CreateOwned(ctx, filepath.Join(r.config.DataDir, "calendar-seeds"), config.SeedPin, l.events)
	if e != nil {
		t.Fatal(e)
	}
	l.cards, e = gymcard.CreateOwned(ctx, filepath.Join(r.config.DataDir, "calendar-cards"), config.CardPin, l.seeds)
	if e != nil {
		t.Fatal(e)
	}
	l.bouts, e = gymbout.CreateOwned(ctx, filepath.Join(r.config.DataDir, "calendar-bouts"), config.BoutPin, l.cards)
	if e != nil {
		t.Fatal(e)
	}
	resultPath := filepath.Join(r.config.DataDir, "calendar-results")
	l.results, e = gymresult.CreateOwned(ctx, resultPath, config.ResultPin, l.bouts)
	if e != nil {
		t.Fatal(e)
	}
	if e = l.results.Close(); e != nil {
		t.Fatal(e)
	}
	l.results = nil
	l.results, e = gymresult.ActivateOwnedCareer(ctx, resultPath, config.ResultPin, l.bouts)
	if e != nil {
		t.Fatal(e)
	}
	l.activation, e = l.results.CareerInfo(ctx)
	if e != nil {
		t.Fatal(e)
	}
	l.career, e = l.results.CareerReader()
	if e != nil {
		t.Fatal(e)
	}
	for _, stable := range l.stable {
		if e = stable.BindCareer(ctx, l.career); e != nil {
			t.Fatal(e)
		}
	}
	if e = l.cards.BindCareer(ctx, l.career); e != nil {
		t.Fatal(e)
	}
	l.ready = &ownedLeagueReady{}
	r.ownedGym.league = l
	p := config.Participants[0]
	identity := p.StablePin.Stable.Identity
	a := &gymaccount.Account{ID: p.StablePin.Stable.AccountID, Generation: 1, Enrollment: &identity}
	c := &OwnedGymChild{runtime: r, league: l, stable: l.stable[0], entries: l.entry[0], events: l.events, life: life,
		view: OwnedGymView{Version: 1, ViewID: strings.Repeat("3", 64), AccountAuthorityID: p.StablePin.Stable.AccountAuthorityID,
			AccountID: a.ID, Generation: 1, Fixture: p.Fixture, RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID},
		scope: OwnedGymScope{EventPin: config.EventPin, EntryPin: p.EntryPin}}
	c.account.Lease = func(work context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrForbidden
		}
		return fn(*a, func() error { return work.Err() })
	}
	return c, a
}

func ownedCareerNextEvent(t *testing.T, c *OwnedGymChild) gymevent.Event {
	t.Helper()
	day := time.Now().UTC().AddDate(0, 0, 1)
	for day.Weekday() != time.Saturday {
		day = day.AddDate(0, 0, 1)
	}
	event, e := gymevent.OwnedEventForDate(c.scope.EventPin, day.Format("2006-01-02"))
	if e != nil {
		t.Fatal(e)
	}
	return event
}

func TestOwnedGymCalendarCareerFenceBeforeCapture(t *testing.T) {
	c, _ := ownedCalendarCareerFixture(t)
	ctx := context.Background()
	event := ownedCareerNextEvent(t, c)
	before, e := c.events.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	head, e := c.league.source.Head(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// No cutoff exists. Getting CareerNotDue, rather than core NotFound,
	// proves the real Result guard runs before event capture/source traversal.
	if _, e = c.captureOwnedEvent(ctx, event.Saturday); !errors.Is(e, gymresult.ErrCareerNotDue) {
		t.Fatal("future capture did not run career fence first", e)
	}
	results := c.league.results
	c.league.results = nil
	_, missing := c.captureOwnedEvent(ctx, event.Saturday)
	c.league.results = results
	if !errors.Is(missing, ErrPoolRuntime) {
		t.Fatal("missing career authority became zero", missing)
	}
	after, e := c.events.Info(ctx)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("refused fence reserved an event", e)
	}
	got, e := c.league.source.Head(ctx)
	if e != nil || got != head {
		t.Fatal("refused fence changed source readiness", e)
	}
	info, e := results.Info(ctx)
	if e != nil || info.ResultCount != 0 {
		t.Fatal("calendar guard minted a result", e)
	}
}
