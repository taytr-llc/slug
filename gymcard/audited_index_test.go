//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymevent"
)

func TestCardAuditedIndexRejectsSelectedRowDrift(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	built, e := f.build(t)
	if e != nil {
		t.Fatal(e)
	}
	k := key('r', built.Record.CardID)
	raw, e := f.s.db.Get(k, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(f.s.index) != 1 || f.s.index[0].digest != sha256.Sum256(raw) || cap(f.s.index) > MaxEvents {
		t.Fatal("successful insert did not publish exact retained byte identity")
	}
	// A semantically legal replacement must still fail the immutable byte
	// binding established by this Open/insert, before any selected disclosure.
	x.now += 2
	if _, e = f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); e != nil {
		t.Fatal(e)
	}
	changed := built.Record
	changed.CreatedAtMS++
	replacement, e := MarshalRecord(f.p, changed)
	if e != nil || bytes.Equal(raw, replacement) {
		t.Fatal("bad legal mutation control", e)
	}
	refuse := func() {
		t.Helper()
		if _, e := f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrCorrupt) {
			t.Fatal("status accepted row drift", e)
		}
		if _, e := f.s.Read(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrCorrupt) {
			t.Fatal("read accepted row drift", e)
		}
		if _, e := f.s.History(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "", 8); !errors.Is(e, ErrCorrupt) {
			t.Fatal("history accepted row drift", e)
		}
	}
	if e = f.s.db.Put(k, replacement, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	refuse()
	if e = f.s.db.Delete(k, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	refuse()
	if e = f.s.db.Put(k, raw, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	date := "2026-11-28"
	ev, e := gymevent.EventForDate(f.p.SourcePin.SourcePin, date)
	if e != nil {
		t.Fatal(e)
	}
	extra := key('r', cardID(f.p, ev))
	if e = f.s.db.Put(extra, raw, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], date); !errors.Is(e, ErrCorrupt) {
		t.Fatal("unindexed physical row accepted", e)
	}
	if e = f.s.db.Delete(extra, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	v, e := f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], date)
	if e != nil || v.Exists {
		t.Fatal("both-absent control", e)
	}
	// A decoded caller copy cannot alter the private index or stored payload.
	read, e := f.s.Read(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	read.Record.Plan.Entrants[0].DisplayName = "caller mutation"
	read, e = f.s.Read(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !equal(read.Record, built.Record) {
		t.Fatal("borrowed payload escaped", e)
	}
	// Reopen must still perform full canonical/semantic auditing, not trust a
	// previously cached digest or merely hash any bytes found on disk.
	changed = built.Record
	changed.Plan.Entrants[0].DisplayName = "not the derived name"
	bad, e := json.Marshal(changed)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.db.Put(k, bad, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Close(); e != nil {
		t.Fatal(e)
	}
	f.s = nil
	opened, e := open(t.Context(), f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, false)
	if opened != nil {
		opened.Close()
	}
	if !errors.Is(e, ErrCorrupt) {
		t.Fatal("reopen skipped full semantic audit", e)
	}
}

func TestCardAuditedIndexPublishesAfterSyncOnly(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	write := f.s.writeBatch
	f.s.checkpoint = func(stage string) {
		if stage == "before-card-sync" {
			if len(f.s.index) != 0 {
				t.Fatal("index published before Sync")
			}
			f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := write(b, o); e != nil {
					return e
				}
				return errors.New("lost retained batch acknowledgement")
			}
		}
	}
	if _, e := f.build(t); !errors.Is(e, ErrOutcomeUnknown) || len(f.s.index) != 0 {
		t.Fatal("uncertain batch published usable index", e, len(f.s.index))
	}
	if _, e := f.s.Info(t.Context()); !errors.Is(e, ErrPoisoned) {
		t.Fatal("uncertain store remained usable", e)
	}
	f.s.Close()
	var e error
	f.s, e = open(t.Context(), f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, false)
	if e != nil {
		t.Fatal(e)
	}
	replayed, e := f.build(t)
	if e != nil || !replayed.Replayed || len(f.s.index) != 1 {
		t.Fatal("reopen failed to audit retained outcome", e)
	}
	raw, e := f.s.db.Get(key('r', replayed.Record.CardID), nil)
	if e != nil || f.s.index[0].digest != sha256.Sum256(raw) {
		t.Fatal("reopen index differs from stored bytes", e)
	}
}

func TestCardAuditedIndexBound(t *testing.T) {
	// At most256 fixed-width IDs, cutoff timestamps and 32-byte digests. The
	// backing slice is allocated at the exact count; no row payload is retained.
	n := uint64(MaxEvents)*(uint64(reflect.TypeOf(indexRow{}).Size())+64) + uint64(reflect.TypeOf([]indexRow{}).Size())
	if n > 64<<10 {
		t.Fatal("private index budget", n)
	}
	t.Logf("maximum retained index fields/backing bytes: %d (allocator metadata excluded)", n)
}
