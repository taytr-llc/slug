//go:build slugtestclock && slugtestenginecount

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// This tag is used only with the separately retained external engine overlay.
// The overlay appends one byte at actual Simulate entry; ordinary builds and
// all performance/behavioral evidence use the unchanged engine source.
func TestBoutActualEngineEntryCounts(t *testing.T) {
	counter := os.Getenv("SLUG_BOUT_ENGINE_COUNTER")
	if counter == "" {
		t.Fatal("explicit external instrumentation required")
	}
	reset := func() {
		if e := os.WriteFile(counter, nil, 0600); e != nil {
			t.Fatal(e)
		}
	}
	count := func(want int, label string) {
		t.Helper()
		raw, e := os.ReadFile(counter)
		if e != nil || len(raw) != want {
			t.Fatal(label, "actual Simulate entries", len(raw), "expected", want, e)
		}
	}
	f := newBoutFixture(t)
	ctx := context.Background()
	reset()
	if _, e := f.run(ctx); e != nil {
		t.Fatal(e)
	}
	count(1, "first Run")
	reset()
	if _, e := f.status(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.CardStatus(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date()); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.Read(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.id()); e != nil {
		t.Fatal(e)
	}
	r, e := f.run(ctx)
	if e != nil || !r.Replayed {
		t.Fatal(e)
	}
	count(0, "Status/CardStatus/Read/replay")
	for _, b := range f.cardRecord.Plan.Bouts[1:] {
		if _, e = f.s.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), b.BoutID); e != nil {
			t.Fatal(e)
		}
	}
	f.s.Close()
	reset()
	f.s, e = open(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, false)
	if e != nil {
		t.Fatal(e)
	}
	count(5, "Open exactly one engine replay per row")
	reset()
	if _, e = f.s.CardStatus(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date()); e != nil {
		t.Fatal(e)
	}
	count(0, "five-saved CardStatus")
	t.Log("actual engine entries: first Run=1, Status/CardStatus/Read/replay=0, Open five rows=5")
}
