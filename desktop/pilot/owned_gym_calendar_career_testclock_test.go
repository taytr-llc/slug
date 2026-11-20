//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func TestOwnedGymCalendarCareerCaptureOriginalView(t *testing.T) {
	c, account := ownedCalendarCareerFixture(t)
	ctx := context.Background()
	r, l := c.runtime, c.league
	event := ownedCareerNextEvent(t, c)
	ids := []string{strings.Repeat("0", 64)} // inherited empty-selection sentinel
	beforeHead, e := l.source.Head(ctx)
	if e != nil {
		t.Fatal(e)
	}
	activation, e := l.results.CareerInfo(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.career.Snapshot(ctx, ids, event.CutoffMS, careerstate.Cutoff); !errors.Is(e, careerstate.ErrUnfenced) {
		t.Fatal("future career was already sealed", e)
	}
	// The existing idle-only transition must include Result, but setting a
	// clock source alone must not persist a career fence, award, or new Head.
	offset := time.UnixMilli(event.CutoffMS).Add(-2 * time.Second).Sub(time.Now())
	if e = r.UseOwnedCalendarOffsetForTest(ctx, l.config, offset); e != nil {
		t.Fatal(e)
	}
	if _, e = l.career.Snapshot(ctx, ids, event.CutoffMS, careerstate.Cutoff); !errors.Is(e, careerstate.ErrUnfenced) {
		t.Fatal("clock setter fabricated durable fence", e)
	}
	if head, e := l.source.Head(ctx); e != nil || head != beforeHead {
		t.Fatal("clock setter fabricated Head", e)
	}
	// Use the existing authentic two-observation boundary and normal genesis
	// verification. This contains no fighter, award, wallet, or transfer fixture.
	if _, e = l.advanceCutoff(ctx, event.Saturday); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 32 && l.cutoffReplay != nil; n++ {
		if _, _, _, e = l.resumeCutoff(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if l.cutoffReplay != nil {
		t.Fatal("historical replay did not join")
	}
	for n := 0; n < 32; n++ {
		p, e := r.RefreshOwnership(ctx, 8)
		if e != nil {
			t.Fatal(e)
		}
		if p.Ready {
			break
		}
		if n == 31 {
			t.Fatal("current replay did not complete")
		}
	}
	cutoff, e := l.source.Cutoff(ctx, event.EventID)
	if e != nil {
		t.Fatal(e)
	}
	head, e := l.source.Head(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// Original View refusal precedes any Result fence write, even after noon.
	account.Generation++
	_, e = c.captureOwnedEvent(ctx, event.Saturday)
	account.Generation--
	if !errors.Is(e, gymresult.ErrStaleView) {
		t.Fatal("stale View captured", e)
	}
	if _, e = l.career.Snapshot(ctx, ids, event.CutoffMS, careerstate.Cutoff); !errors.Is(e, careerstate.ErrUnfenced) {
		t.Fatal("stale View sealed career", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = c.captureOwnedEvent(canceled, event.Saturday); e == nil {
		t.Fatal("cancelled View captured")
	}
	// Exact direct consumer call reproduces the original missing-coordinator
	// defect: event reservation is possible but no participant is committed.
	if _, e = c.events.Capture(ctx, c.entries, c.session(), event.Saturday); !errors.Is(e, careerstate.ErrUnfenced) {
		t.Fatal("unfenced consumer did not refuse", e)
	}
	status, e := c.events.Observe(ctx, c.entries, c.session(), event.Saturday)
	if e != nil || !status.Exists || len(status.CapturedParticipants) != 0 {
		t.Fatal("unfenced capture committed participant", e)
	}
	// A held original View is enough for observation. No publishing/signing
	// capability is requested by the fixture's strict View-only lease.
	account.HoldUntilMS = event.CutoffMS + int64(time.Hour/time.Millisecond)
	result, e := c.captureOwnedEvent(ctx, event.Saturday)
	if e != nil || len(result.Status.CapturedParticipants) != 1 || result.Replayed {
		t.Fatal("fenced capture", e)
	}
	sealed, e := l.career.Snapshot(ctx, ids, event.CutoffMS, careerstate.Cutoff)
	if e != nil || sealed.Activation() != activation {
		t.Fatal("original career not sealed", e)
	}
	totals, ok := sealed.Totals(ids[0])
	if !ok || totals.Wins != 0 || totals.Losses != 0 || totals.Draws != 0 {
		t.Fatal("calendar fence fabricated awards")
	}
	again, e := c.captureOwnedEvent(ctx, event.Saturday)
	if e != nil || !again.Replayed || !reflect.DeepEqual(again.Capture, result.Capture) {
		t.Fatal("exact participant changed on replay", e)
	}
	if got, e := l.source.Head(ctx); e != nil || got != head {
		t.Fatal("career fence changed current Head", e)
	}
	if got, e := l.source.Cutoff(ctx, event.EventID); e != nil || got != cutoff {
		t.Fatal("career fence changed ownership cutoff", e)
	}
	info, e := l.results.Info(ctx)
	if e != nil || info.ResultCount != 0 {
		t.Fatal("career fence minted result", e)
	}
}
