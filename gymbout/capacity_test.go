//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"testing"
	"time"
)

// Modeled capacity records are newly assembled storage fixtures, not golden
// outputs or claimed admissions. They preserve the frozen composition equations
// and deliberately shift modeled births with the modeled event date.
func capacityRecord(t testing.TB, p Pin, template Record, date string, minimum int64) Record {
	t.Helper()
	raw, _ := json.Marshal(template)
	var r Record
	json.Unmarshal(raw, &r)
	s := &r.Source
	ev, e := gymevent.EventForDate(p.SourcePin.SourcePin.SourcePin, date)
	if e != nil {
		t.Fatal(e)
	}
	delta := ev.CutoffMS - s.Event.CutoffMS
	s.Event = ev
	s.StoreID = p.SourceStoreID
	s.CardID, _ = gymCardID(p, ev)
	s.SnapshotDigest = hash("modeled bout capacity snapshot", []byte(date))
	s.SeedEventKey, _ = gymseed.EventKey(p.SourcePin.SourcePin, ev)
	s.CardRecordDigest = hash("modeled bout capacity card record", []byte(date))
	s.CardSourceDigest = hash("modeled bout capacity card source", []byte(date))
	s.CardCreatedAtMS = max(minimum, ev.CutoffMS+2000)
	s.FinalizedAtMS = ev.CutoffMS + 1999
	for i := range s.Corners {
		m := &s.Corners[i].Member
		m.Assignment.BornAtMS += delta
		m.Assignment.AcceptedAtMS += delta
		m.SlotAssignedAtMS += delta
		m.ConditionAtCutoff.BornAtMS += delta
		m.ConditionAtCutoff.AsOfMS += delta
	}
	input := hash("SLUG candidate card input\x00v1\x00", rawID(s.CardID), rawID(s.SnapshotDigest), rawID(s.SeedEventKey), le32(s.TargetHeight), rawID(s.TargetHash), rawID(p.SourcePin.RulesID))
	seed := hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(s.TargetHash))
	ci := uint32(0)
	for i, c := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if c == s.Bout.Class {
			ci = uint32(i)
		}
	}
	s.Bout.BoutID = hash("SLUG candidate card bout\x00v1\x00", rawID(input), le32(ci), rawID(s.Bout.CornerA), rawID(s.Bout.CornerB))
	s.Bout.Seed = hash("SLUG candidate card bout seed\x00v1\x00", rawID(seed), rawID(s.Bout.BoutID))
	r.Version = 1
	r.StoreID, _ = ID(p)
	r.CardID = s.CardID
	r.BoutID = s.Bout.BoutID
	r.SourceDigest = sourceDigest(*s)
	r.CreatedAtMS = max(minimum, s.CardCreatedAtMS+1)
	r.Input, e = DeriveInput(p, *s)
	if e != nil {
		t.Fatal("modeled capacity input", e)
	}
	r.InputID = inputDigest(r.Input)
	r.Result, e = fighterbout.Simulate(context.Background(), r.Input)
	if e != nil {
		t.Fatal(e)
	}
	r.ResultDigest = resultDigest(r.Result)
	return r
}
func gymCardID(p Pin, e gymevent.Event) (string, error) {
	return hash("SLUG candidate card event\x00v1\x00", rawID(p.SourceStoreID), rawID(e.EventID)), nil
}

func TestBoutCapacityFullAuditAndReplay(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	for _, b := range f.cardRecord.Plan.Bouts[:4] {
		if _, e := f.s.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), b.BoutID); e != nil {
			t.Fatal(e)
		}
	}
	c := readReference(t)
	date, _ := time.Parse("2006-01-02", f.date())
	batch := new(leveldb.Batch)
	last := f.now.Load()
	for i := 0; i < 252; i++ {
		r := capacityRecord(t, f.p, c.Compositions[1+i%5].Record, date.AddDate(0, 0, 7*(1+i/5)).Format("2006-01-02"), f.s.meta.CreatedAtMS)
		b, _ := json.Marshal(r)
		batch.Put(key('r', r.BoutID), b)
		last = max(last, r.CreatedAtMS)
	}
	next := f.s.meta
	next.BoutCount = 256
	next.ClockMS = last
	put(batch, []byte("meta"), next, MaxMetadataSize)
	if e := f.s.db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	f.s.Close()
	f.now.Store(last)
	path := filepath.Join(f.dir, "bout")
	start := time.Now()
	var e error
	f.s, e = open(ctx, path, f.p, f.card, f.clock, false)
	if e != nil {
		t.Fatal("full capacity Open", e)
	}
	t.Logf("full256-row audit elapsed=%s privateIndexRows=%d", time.Since(start), len(f.s.index))
	if len(f.s.index) != 256 || !indexBound(f.s.index) {
		t.Fatal("bounded complete index")
	}
	rawIndex, _ := json.Marshal(f.s.index)
	t.Logf("private index=%d bytes cap=%d", len(rawIndex), MaxIndexSize)
	engine := 0
	rows := 0
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			engine++
		}
		if stage == "retained-row-checked" {
			rows++
		}
	}
	got, e := f.run(ctx)
	if e != nil || !got.Replayed || engine != 0 {
		t.Fatal("capacity replay", e, engine)
	}
	if _, e = f.s.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.cardRecord.Plan.Bouts[4].BoutID); !errors.Is(e, ErrCapacity) || engine != 0 {
		t.Fatal("capacity new row", e, engine)
	}
	if rows > 10 {
		t.Fatal("metadata scanned payloads outside selected card", rows)
	}
	f.s.Close()
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := newStore(db, f.clock)
	s.sourceCreatedAtMS = f.s.sourceCreatedAtMS
	cancelCtx, cancel := context.WithCancel(ctx)
	count := 0
	s.checkpoint = func(stage string) {
		if stage == "audit-row" {
			count++
			if count == 3 {
				cancel()
			}
		}
	}
	m, e := readMarker(path, f.p)
	if e != nil {
		t.Fatal(e)
	}
	e = s.audit(cancelCtx, m)
	if !errors.Is(e, ErrCancelled) || count != 3 || len(s.index) != 0 {
		t.Fatal("cancelled partial audit published", e, count, len(s.index))
	}
	s.Close()
	f.s = nil
	t.Log(fmt.Sprintf("selected row checks=%d, no replay/new-at-capacity engine calls", rows))
}
