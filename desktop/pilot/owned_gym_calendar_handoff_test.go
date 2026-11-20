//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymevent"
)

// Real retained core work, registry boundary commit and two HTTP observations;
// only the isolated authority/core calendar clocks are moved. No admission is
// issued in this test, and no caller-supplied ownership map becomes authority.
func TestOwnedGymCalendarBoundaryJoinsCurrentReplay(t *testing.T) {
	r, _, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	if e := r.PrepareOwnedSource(ctx, r.owned.config); e != nil {
		t.Fatal(e)
	}
	config, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.marker.AccountAuthorityID, strings.Repeat("1", 64), strings.Repeat("2", 64))
	if e != nil {
		t.Fatal(e)
	}
	// Select a supported future calendar event without altering either RPC
	// observation or their real elapsed wait.
	next := time.Now().UTC().AddDate(0, 0, 1)
	for next.Weekday() != time.Saturday {
		next = next.AddDate(0, 0, 1)
	}
	event, e := gymevent.OwnedEventForDate(config.EventPin, next.Format("2006-01-02"))
	if e != nil {
		t.Fatal(e)
	}
	offset := time.UnixMilli(event.CutoffMS).Add(-2 * time.Second).Sub(time.Now())
	clock := func() time.Time { return time.Now().Add(offset) }
	if e = r.owned.publisher.UseAuthorityClockForTest(ctx, r.owned.config.Ownership, clock); e != nil {
		t.Fatal(e)
	}
	if e = r.owned.store.UseCalendarClockForTest(ctx, r.owned.config.Ownership, r.owned.config.Store, clock); e != nil {
		t.Fatal(e)
	}
	source, e := fighterownership.NewPublicSource(ctx, r.owned.config.Ownership, r.owned.publisher)
	if e != nil {
		t.Fatal(e)
	}
	current, e := r.owned.store.BeginReplay(ctx, source, r.owned.node)
	if e != nil {
		t.Fatal(e)
	}
	r.owned.source, r.owned.replay = source, current
	r.owned.progress = fighterownership.Progress{WorkID: current.WorkID()}
	l := &OwnedLeague{runtime: r, source: r.owned.store, life: r.life, config: config, ready: &ownedLeagueReady{}}
	t.Cleanup(func() {
		if l.cutoffReplay != nil {
			if e := l.cutoffReplay.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	if _, e = r.owned.store.Head(ctx); e != fighterownership.ErrUnavailable {
		t.Fatal("current fixture not incomplete", e)
	}
	_, e = l.advanceCutoff(ctx, event.Saturday)
	if e != nil {
		t.Fatal("authentic boundary blocked by current partial", e)
	}
	if _, e = current.Step(ctx); !errors.Is(e, fighterownership.ErrClosed) {
		t.Fatal("old work not joined", e)
	}
	intent, e := r.owned.publisher.CutoffIntent(ctx, event.EventID)
	if e != nil || intent.CutoffMS != event.CutoffMS || intent.First.ResponseEndMS >= event.CutoffMS || intent.Second.RequestStartMS < event.CutoffMS || intent.First.InstanceID != intent.Second.InstanceID || intent.First.Revision != intent.Second.Revision {
		t.Fatal("retained intent did not prove original bracket", e)
	}
	if _, e = r.owned.store.Head(ctx); e != fighterownership.ErrUnavailable {
		t.Fatal("handoff invented current readiness", e)
	}
	for n := 0; n < 32 && l.cutoffReplay != nil; n++ {
		if _, _, _, e = l.resumeCutoff(ctx); e != nil {
			t.Fatal("historical continuation", e)
		}
	}
	if l.cutoffReplay != nil {
		t.Fatal("bounded historical replay never joined")
	}
	if _, e = r.owned.store.Head(ctx); e != fighterownership.ErrUnavailable {
		t.Fatal("historical completion substituted for current head", e)
	}
	for n := 0; n < 32 && !r.owned.progress.Ready; n++ {
		if _, e = r.RefreshOwnership(ctx, 8); e != nil {
			t.Fatal("current replay after historical", e)
		}
	}
	if !r.owned.progress.Ready {
		t.Fatal("current replay remained incomplete")
	}
	cutoff, e := r.owned.store.Cutoff(ctx, event.EventID)
	if e != nil || cutoff.Boundary.Intent != intent {
		t.Fatal("retained boundary changed", e)
	}
	// The following date has no retained pre-noon intent. A past supplied
	// cutoff may not become a retrospective authority.
	_, e = r.owned.store.CaptureBoundary(ctx, r.owned.source, r.owned.node, strings.Repeat("4", 64), clock().UnixMilli()-1)
	if e != fighterownership.ErrUnavailable {
		t.Fatal("missed bracket accepted", e)
	}
}
