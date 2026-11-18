//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// A modeled ten-entrant/five-bout source exercises the actual enrolled account
// and original WithSession lease, including both parent-equivalent durable
// rechecks. Only authority is real: histories/finality are explicit disk models,
// not admissions or a mined chain. The scoped clock advances with elapsed time.
// This normal-profile latency gate remains separate from race correctness.
func boutRealAccountFiveFixture(t *testing.T) (*StableController, func() time.Time, func(int64)) {
	t.Helper()
	ctx := t.Context()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UnixMilli()
	var advanceAt time.Time
	clock := func() time.Time {
		if advanceAt.IsZero() {
			return time.UnixMilli(now)
		}
		return time.UnixMilli(now).Add(time.Since(advanceAt))
	}
	s, ep, refresh := cardRealAccountEntryFixture(t, root, clock)
	events, e := gymevent.CreateForTestClock(ctx, filepath.Join(root, "events"), ep, clock)
	if e != nil {
		t.Fatal(e)
	}
	attachEventFixture(t, s, events)
	sp, e := gymseed.CandidatePin(ep)
	if e != nil {
		t.Fatal(e)
	}
	seedPath, cardPath := filepath.Join(root, "seeds"), filepath.Join(root, "cards")
	seeds, e := gymseed.CreateForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal(e)
	}
	si, e := seeds.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymcard.CandidatePin(sp)
	if e != nil {
		t.Fatal(e)
	}
	cards, e := gymcard.CreateForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal(e)
	}
	ci, e := cards.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = cards.Close(); e != nil {
		t.Fatal(e)
	}
	if e = seeds.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../gymbout/testdata/reference/inputs/card46/modeled-five-class-seed.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "2468afc372224e99f20cf89a53605a1cbe1b5bea9787b8cb45817515f0e170ea" {
		t.Fatal("frozen modeled source changed")
	}
	var modeled gymseed.Record
	if json.Unmarshal(raw, &modeled) != nil {
		t.Fatal("modeled source")
	}
	template := modeled.SourceReceipt
	for i := range template.Input.Participants {
		part := &template.Input.Participants[i]
		p := ep.Participants[i]
		sid, err := gymentry.ID(p)
		if err != nil {
			t.Fatal(err)
		}
		part.Owner.AccountAuthorityID = p.StablePin.AccountAuthorityID
		part.Owner.AccountID = p.StablePin.AccountID
		part.EntryStoreID = sid
		part.Entry.StoreID = sid
		part.Entry.LatestReceipt.StoreID = sid
		part.Roster.StoreID = p.StableStoreID
		template.Captures[i].StableFence.StoreID = p.StableStoreID
		template.Captures[i].EntryFence.StoreID = sid
	}
	second := &template.Input.Participants[1]
	rename := map[string]string{}
	for i := range second.Members {
		oldID := second.Members[i].Assignment.ProspectID
		id := cardCapacityHash("capacity-only distinct prospect", cardCapacityRawID(second.Owner.GymID), cardCapacityRawID(oldID))
		rename[oldID] = id
		second.Members[i].Assignment.ProspectID = id
	}
	for i := range second.Entry.LatestReceipt.Members {
		a := &second.Entry.LatestReceipt.Members[i].Assignment
		a.ProspectID = rename[a.ProspectID]
	}
	for i, id := range second.Roster.Slots {
		if id != "" {
			second.Roster.Slots[i] = rename[id]
		}
	}
	sort.Slice(second.Members, func(i, j int) bool {
		return second.Members[i].Assignment.ProspectID < second.Members[j].Assignment.ProspectID
	})
	sort.Slice(second.Entry.LatestReceipt.Members, func(i, j int) bool {
		return second.Entry.LatestReceipt.Members[i].Assignment.ProspectID < second.Entry.LatestReceipt.Members[j].Assignment.ProspectID
	})
	seedBatch, cardBatch := new(leveldb.Batch), new(leveldb.Batch)

	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 1; i++ {
		r := cardCapacitySeedRecord(t, p, template, date.AddDate(0, 0, 7*i).Format("2006-01-02"))
		source := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
		plan, e := gymcard.DerivePlan(p, source)
		if e != nil {
			t.Fatal(e)
		}
		sd, e := gymseed.CardSourceDigest(sp, source)
		if e != nil {
			t.Fatal(e)
		}
		record := gymcard.Record{Version: 1, StoreID: ci.StoreID, CardID: plan.CardID, Source: source, SourceDigest: sd, Plan: plan, CreatedAtMS: max(ci.CreatedAtMS, r.Finality.ObservedAtMS)}
		b, e := gymseed.MarshalRecord(sp, r)
		if e != nil {
			t.Fatal(e)
		}
		seedBatch.Put(append([]byte{'r'}, cardCapacityRawID(r.EventKey)...), b)
		b, e = gymcard.MarshalRecord(p, record)
		if e != nil {
			t.Fatal(e)
		}
		cardBatch.Put(append([]byte{'r'}, cardCapacityRawID(record.CardID)...), b)
		if plan.BoutCount != 5 {
			t.Fatalf("five-class modeled plan: %d", plan.BoutCount)
		}
		now = record.CreatedAtMS
	}
	for _, entry := range []struct {
		path  string
		batch *leveldb.Batch
	}{{seedPath, seedBatch}, {cardPath, cardBatch}} {
		db, e := leveldb.OpenFile(filepath.Join(entry.path, "db"), nil)
		if e != nil {
			t.Fatal(e)
		}
		meta, e := db.Get([]byte("meta"), nil)
		if e != nil {
			t.Fatal(e)
		}
		var old map[string]json.RawMessage
		if json.Unmarshal(meta, &old) != nil {
			t.Fatal("meta")
		}
		for k, v := range map[string]any{"clock_ms": now, "event_count": 1} {
			n, _ := json.Marshal(v)
			needle := append([]byte(`"`+k+`":`), old[k]...)
			if bytes.Count(meta, needle) != 1 {
				t.Fatal("bounded meta patch")
			}
			meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), n...), 1)
		}
		entry.batch.Put([]byte("meta"), meta)
		if e = db.Write(entry.batch, &opt.WriteOptions{Sync: true}); e != nil {
			t.Fatal(e)
		}
		if e = db.Close(); e != nil {
			t.Fatal(e)
		}
	}
	advanceAt = time.Now()
	refresh() // A fresh original session at the retained fixture time, before timing begins.
	seeds, e = gymseed.OpenForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal("seed full audit", e)
	}
	cards, e = gymcard.OpenForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal("card full audit", e)
	}
	if e = s.entryStore.Close(); e != nil {
		t.Fatal(e)
	}
	s.entryStore, e = gymentry.OpenForTestClock(ctx, filepath.Join(s.config.DataDir, "entries"), s.entryPin, s.stableStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	s.seedEnabled, s.seedAttached, s.cardEnabled, s.cardAttached = true, true, true, true
	s.seedStore, s.cardStore = seeds, cards
	s.seedPin, s.cardPin = sp, p
	s.seedID, s.cardID = si.StoreID, ci.StoreID
	s.seedCreated, s.cardCreated, s.cardSourceCreated = si.CreatedAtMS, ci.CreatedAtMS, ci.SourceCreatedAtMS
	s.seedBytes, s.cardBytes = MaxSeedMetadataSize, MaxCardMetadataSize
	t.Cleanup(func() { _ = cards.Close(); _ = seeds.Close() })
	bp, e := gymbout.CandidatePin(p)
	if e != nil {
		t.Fatal(e)
	}
	bouts, e := gymbout.CreateForTestClock(ctx, filepath.Join(root, "bouts"), bp, cards, clock)
	if e != nil {
		t.Fatal(e)
	}
	bi, e := bouts.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	s.boutEnabled, s.boutAttached = true, true
	s.boutStore, s.boutPin, s.boutID = bouts, bp, bi.StoreID
	s.boutCreated, s.boutSourceCreated = bi.CreatedAtMS, bi.SourceCreatedAtMS
	s.boutBytes = MaxBoutMetadataSize
	t.Cleanup(func() { _ = bouts.Close() })
	return s, clock, func(next int64) { now = max(now, next); advanceAt = time.Now(); refresh() }
}

func TestBoutMetadataFiveRealAccountAdvancingClock(t *testing.T) {
	s, clock, advance := boutRealAccountFiveFixture(t)
	start := time.Now()
	status, e := s.BoutCardStatus(t.Context(), s.view.ViewID, "2026-11-21")
	elapsed := time.Since(start)
	if e != nil {
		t.Fatalf("real-account advancing-clock ten-member CardStatus %s: %v", elapsed, e)
	}
	if len(status.Bouts) != 5 {
		t.Fatalf("rows: %d", len(status.Bouts))
	}
	for _, r := range status.Bouts {
		if r.Exists {
			t.Fatal("unrun row exists")
		}
	}
	t.Logf("real-account advancing-clock ten-member/five-unrun CardStatus %s", elapsed)

	records := []gymbout.Record{}
	for _, row := range status.Bouts {
		run, e := s.boutStore.Run(t.Context(), s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21", row.Bout.BoutID)
		if e != nil || run.Replayed {
			t.Fatalf("explicit modeled run: %v replay=%v", e, run.Replayed)
		}
		records = append(records, run.Record)
	}
	info, e := s.boutStore.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.boutStore.Close(); e != nil {
		t.Fatal(e)
	}
	batch := new(leveldb.Batch)
	last := clock().UnixMilli()
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 251; i++ {
		r := boutExtraCapacityRecord(t, info.Pin, records[i%5], date.AddDate(0, 0, 7*(1+i/5)).Format("2006-01-02"), info.CreatedAtMS)
		raw, e := gymbout.MarshalRecord(t.Context(), info.Pin, r)
		if e != nil {
			t.Fatal("full modeled extra row", e)
		}
		batch.Put(append([]byte{'r'}, cardCapacityRawID(r.BoutID)...), raw)
		last = max(last, r.CreatedAtMS)
	}
	path := filepath.Join(filepath.Dir(s.config.DataDir), "bouts")
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	meta, e := db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var previous map[string]json.RawMessage
	if json.Unmarshal(meta, &previous) != nil {
		t.Fatal("bout meta")
	}
	for k, v := range map[string]any{"clock_ms": last, "bout_count": 256} {
		raw, _ := json.Marshal(v)
		needle := append([]byte(`"`+k+`":`), previous[k]...)
		if bytes.Count(meta, needle) != 1 {
			t.Fatal("exact bout meta")
		}
		meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), raw...), 1)
	}
	batch.Put([]byte("meta"), meta)
	if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	advance(last)
	s.boutStore, e = gymbout.OpenForTestClock(t.Context(), path, info.Pin, s.cardStore, clock)
	if e != nil {
		t.Fatal("full256 bout audit", e)
	}
	t.Cleanup(func() { _ = s.boutStore.Close() })
	got, e := s.boutStore.Info(t.Context())
	if e != nil || got.BoutCount != 256 {
		t.Fatal("capacity", e)
	}
	start = time.Now()
	saved, e := s.BoutCardStatus(t.Context(), s.view.ViewID, "2026-11-21")
	elapsed = time.Since(start)
	if e != nil || len(saved.Bouts) != 5 {
		t.Fatalf("five saved CardStatus %s: %v", elapsed, e)
	}
	for _, row := range saved.Bouts {
		if !row.Exists || !row.Usable {
			t.Fatal("saved row unavailable")
		}
	}
	t.Logf("real-account advancing-clock full256 ten-member/five-saved CardStatus %s", elapsed)
	for _, row := range saved.Bouts {
		start = time.Now()
		read, e := s.BoutRecord(t.Context(), s.view.ViewID, "2026-11-21", row.Bout.BoutID)
		elapsed = time.Since(start)
		if e != nil || read.Record.BoutID != row.Bout.BoutID {
			t.Fatalf("selected saved Read %s: %v", elapsed, e)
		}
		t.Logf("real-account advancing-clock full256 saved Read %s", elapsed)
	}
}

// Extra stored rows are explicitly modeled histories; only the original five
// selected records correspond to the concrete source card in this fixture.
func boutExtraCapacityRecord(t *testing.T, p gymbout.Pin, template gymbout.Record, date string, minimum int64) gymbout.Record {
	t.Helper()
	raw, _ := json.Marshal(template)
	var r gymbout.Record
	json.Unmarshal(raw, &r)
	s := &r.Source
	ev, e := gymevent.EventForDate(p.SourcePin.SourcePin.SourcePin, date)
	if e != nil {
		t.Fatal(e)
	}
	delta := ev.CutoffMS - s.Event.CutoffMS
	s.Event = ev
	s.StoreID = p.SourceStoreID
	s.CardID, _ = gymcard.CardID(p.SourcePin, ev)
	s.SnapshotDigest = cardCapacityHash("modeled bout capacity snapshot", []byte(date))
	s.SeedEventKey, _ = gymseed.EventKey(p.SourcePin.SourcePin, ev)
	s.CardRecordDigest = cardCapacityHash("modeled bout capacity card record", []byte(date))
	s.CardSourceDigest = cardCapacityHash("modeled bout capacity card source", []byte(date))
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
	input := cardCapacityHash("SLUG candidate card input\x00v1\x00", cardCapacityRawID(s.CardID), cardCapacityRawID(s.SnapshotDigest), cardCapacityRawID(s.SeedEventKey), binary.LittleEndian.AppendUint32(nil, s.TargetHeight), cardCapacityRawID(s.TargetHash), cardCapacityRawID(p.SourcePin.RulesID))
	seed := cardCapacityHash("SLUG candidate card seed\x00v1\x00", cardCapacityRawID(input), cardCapacityRawID(s.TargetHash))
	ci := uint32(0)
	for i, c := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if c == s.Bout.Class {
			ci = uint32(i)
		}
	}
	s.Bout.BoutID = cardCapacityHash("SLUG candidate card bout\x00v1\x00", cardCapacityRawID(input), binary.LittleEndian.AppendUint32(nil, ci), cardCapacityRawID(s.Bout.CornerA), cardCapacityRawID(s.Bout.CornerB))
	s.Bout.Seed = cardCapacityHash("SLUG candidate card bout seed\x00v1\x00", cardCapacityRawID(seed), cardCapacityRawID(s.Bout.BoutID))
	r.Version = 1
	r.StoreID, _ = gymbout.ID(p)
	r.CardID = s.CardID
	r.BoutID = s.Bout.BoutID
	r.SourceDigest, e = gymcard.BoutSourceDigest(p.SourcePin, *s)
	if e != nil {
		t.Fatal(e)
	}
	r.CreatedAtMS = max(minimum, s.CardCreatedAtMS+1)
	r.Input, e = gymbout.DeriveInput(p, *s)
	if e != nil {
		t.Fatal("modeled capacity input", e)
	}
	r.InputID, e = fighterbout.InputID(r.Input)
	if e != nil {
		t.Fatal(e)
	}
	r.Result, e = fighterbout.Simulate(t.Context(), r.Input)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(r.Result)
	r.ResultDigest = cardCapacityHash("SLUG fighter bout result\x00v1\x00", raw)
	return r
}
