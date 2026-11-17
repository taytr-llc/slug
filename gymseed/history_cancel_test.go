//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/gymevent"
)

func TestSeedHistoryCancellationBetweenValidatedRecords(t *testing.T) {
	x, s, _ := seedFixture(t)
	ctx := context.Background()
	if _, err := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21"); err != nil {
		t.Fatal(err)
	}
	const next = "2026-11-28"
	event, err := gymevent.EventForDate(x.p, next)
	if err != nil {
		t.Fatal(err)
	}
	x.now = event.CutoffMS + 1
	for i := range x.entry {
		if _, err := x.s.Capture(ctx, x.entry[i], x.session[i], next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Prepare(ctx, x.s, x.entry[0], x.session[0], next); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	validated := 0
	s.checkpoint = func(stage string) {
		if stage == "history-record-validated" {
			validated++
			cancel()
		}
	}
	defer func() { s.checkpoint = nil }()
	if _, err := s.History(work, x.s, x.entry[0], x.session[0], "", 8); !errors.Is(err, ErrCancelled) {
		t.Fatal("cancelled history", err)
	}
	if validated != 1 {
		t.Fatalf("validated %d retained rows after cancellation; want one completed row", validated)
	}
}
