//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// Capacity inputs are explicitly modeled histories with ten members per event.
// They derive from the old independent source fixture and never claim admissions
// for its synthetic all-five-class fighters or actual mined finality.
func capacitySeedRecord(t *testing.T, p Pin, template gymevent.SnapshotReceipt, date string) gymseed.Record {
	t.Helper()
	raw, _ := json.Marshal(template)
	var rec gymevent.SnapshotReceipt
	json.Unmarshal(raw, &rec)
	ep := p.SourcePin.SourcePin
	ev, e := gymevent.EventForDate(ep, date)
	if e != nil {
		t.Fatal(e)
	}
	rec.Input.Event = ev
	rec.CompletedAtMS = ev.CutoffMS + 100
	for i := range rec.Input.Participants {
		part := &rec.Input.Participants[i]
		entryEv, e := gymentry.EventForDate(ep.Participants[i], date)
		if e != nil {
			t.Fatal(e)
		}
		entryID, e := gymentry.EntryID(part.Entry.StoreID, entryEv.EventID, part.Owner.GymID)
		if e != nil {
			t.Fatal(e)
		}
		part.Entry.Event = entryEv
		part.Entry.EntryID = entryID
		part.Entry.LatestReceipt.Event = entryEv
		part.Entry.LatestReceipt.EntryID = entryID
		part.Roster.CutoffMS = ev.CutoffMS
		for j := range part.Members {
			m := &part.Members[j]
			manifest, e := fightermanifest.GenerateText(1, m.Assignment.WorkHash)
			if e != nil {
				t.Fatal(e)
			}
			m.ConditionAtCutoff, e = fightercondition.Project(1, manifest.DNA, m.Assignment.BornAtMS, ev.CutoffMS, 0)
			if e != nil {
				t.Fatal(e)
			}
			m.Eligibility = "eligible"
			if m.Slot == 0 {
				m.Eligibility = "not_rostered"
			}
			if m.ConditionAtCutoff.Phase == "retired" {
				m.Eligibility = "retired"
			}
		}
		rec.Captures[i].StableFence.CutoffMS = ev.CutoffMS
		rec.Captures[i].StableFence.ObservedAtMS = rec.CompletedAtMS
		rec.Captures[i].EntryFence.CutoffMS = ev.CutoffMS
		rec.Captures[i].EntryFence.ObservedAtMS = rec.CompletedAtMS
		rec.Captures[i].CapturedAtMS = rec.CompletedAtMS
	}
	rec.SnapshotDigest, e = gymevent.SnapshotDigest(ep, rec.Input)
	if e != nil {
		t.Fatal(e)
	}
	rd, e := gymevent.ReceiptDigest(ep, rec)
	if e != nil {
		t.Fatal(e)
	}
	key, e := gymseed.EventKey(p.SourcePin, ev)
	if e != nil {
		t.Fatal(e)
	}
	sp := p.SourcePin
	decl := gymseed.Declaration{Version: 1, EventKey: key, StoreID: p.SourceStoreID, SourceStoreID: sp.SourceStoreID, Event: ev, SnapshotDigest: rec.SnapshotDigest, ReceiptDigest: rd, PublisherChainID: sp.PublisherChainID, Network: sp.Network, Genesis: sp.Genesis, FloorHeight: 0, FloorHash: sp.Genesis, RulesID: sp.RulesID, PublisherPublicKey: sp.PublisherPublicKey, CheckpointPublicKey: sp.CheckpointPublicKey}
	b, _ := json.Marshal(decl)
	keySeed := sha256.Sum256([]byte("SLUG candidate seed declaration fixture\x00v1\x00"))
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(keySeed[:]), append([]byte("SLUG candidate seed declaration\x00v1\x00"), lp(b)...))
	r := gymseed.Record{Version: 1, StoreID: p.SourceStoreID, EventKey: key, SourceReceipt: rec, Declaration: gymseed.SignedDeclaration{Version: 1, Declaration: decl, Signature: hex.EncodeToString(sig)}, PreparedAtMS: rec.CompletedAtMS, Cursor: gymseed.Cursor{Height: -1, CompleteToHeight: -1, ScanDigest: hash("modeled scan")}, ScanRestarts: []gymseed.ScanRestart{}, Conflicts: []gymseed.Conflict{}, Observation: gymseed.Observation{NodeState: "unobserved"}}
	return syntheticSeedFinality(t, sp, r, rec.CompletedAtMS+1)
}
func TestCardHistoryCapacityUsesOnlySelectedRows(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	ctx := context.Background()
	raw, e := os.ReadFile("../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var old struct {
		Records []struct{ Value gymevent.SnapshotReceipt }
	}
	json.Unmarshal(raw, &old)
	template := old.Records[10].Value
	// The old all-five-class model deliberately repeats ProspectIDs between gyms.
	// New card sources require global uniqueness. Build a separate synthetic
	// capacity input with consistently renamed second-gym IDs; never alter old
	// fixture bytes or treat these modeled assignments as proof-backed admissions.
	second := &template.Input.Participants[1]
	rename := map[string]string{}
	for i := range second.Members {
		oldID := second.Members[i].Assignment.ProspectID
		newID := hash("capacity-only distinct prospect", rawID(second.Owner.GymID), rawID(oldID))
		rename[oldID] = newID
		second.Members[i].Assignment.ProspectID = newID
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
	if len(template.Input.Participants[0].Members) != 5 {
		t.Fatal("capacity requires ten-member template")
	}
	f.seed.Close()
	db, e := leveldb.OpenFile(filepath.Join(f.seedPath, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	seedBatch := new(leveldb.Batch)
	cardBatch := new(leveldb.Batch)
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	ids := []string{}
	for i := 0; i < 256; i++ {
		r := capacitySeedRecord(t, f.p, template, date.AddDate(0, 0, 7*i).Format("2006-01-02"))
		source := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
		plan, e := DerivePlan(f.p, source)
		if e != nil {
			t.Fatal(e)
		}
		sd, e := gymseed.CardSourceDigest(f.p.SourcePin, source)
		if e != nil {
			t.Fatal(e)
		}
		record := Record{1, f.s.meta.StoreID, plan.CardID, source, sd, plan, max(f.s.meta.CreatedAtMS, r.Finality.ObservedAtMS)}
		ids = append(ids, plan.CardID)
		b, e := gymseed.MarshalRecord(f.p.SourcePin, r)
		if e != nil {
			t.Fatal(e)
		}
		seedBatch.Put(append([]byte{'r'}, rawID(r.EventKey)...), b)
		if e = put(cardBatch, key('r', plan.CardID), record, MaxRecordSize); e != nil {
			t.Fatal(e)
		}
		x.now = record.CreatedAtMS
	}
	meta, _ := db.Get([]byte("meta"), nil)
	var m map[string]json.RawMessage
	json.Unmarshal(meta, &m)
	for k, v := range map[string]any{"clock_ms": x.now, "event_count": 256} {
		newRaw, _ := json.Marshal(v)
		meta = bytes.Replace(meta, append([]byte(`"`+k+`":`), m[k]...), append([]byte(`"`+k+`":`), newRaw...), 1)
	}
	seedBatch.Put([]byte("meta"), meta)
	if e = db.Write(seedBatch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	db.Close()
	f.seed, e = gymseed.OpenForTestClock(ctx, f.seedPath, f.p.SourcePin, x.s, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal("modeled source audit", e)
	}
	next := f.s.meta
	next.EventCount = 256
	next.ClockMS = x.now
	if e = f.s.commitLocked(cardBatch, next); e != nil {
		t.Fatal(e)
	}
	f.s.Close()
	f.s, e = open(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, false)
	if e != nil {
		t.Fatal("full card audit", e)
	}
	if len(f.s.index) != 256 {
		t.Fatal("audit index")
	}
	for _, start := range []int{0, 248, 256} {
		count := 0
		f.s.checkpoint = func(stage string) {
			if stage == "record-validated" {
				count++
			}
		}
		after := ""
		if start > 0 {
			after = ids[start-1]
		}
		deadline, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		begin := time.Now()
		page, e := f.s.History(deadline, f.seed, x.s, x.entry[0], x.session[0], after, 8)
		elapsed := time.Since(begin)
		cancel()
		if e != nil {
			t.Fatalf("capacity page %d took %s: %v", start, elapsed, e)
		}
		want := min(8, 256-start)
		if count != want || len(page.Cards) != want || page.HasMore != (start+want < 256) {
			t.Fatalf("validation count page%d: %d/%d", start, count, want)
		}
		t.Logf("capacity page %d: %d records, %s", start, count, elapsed)
	}
	// A selected missing row cannot be served from the ID/date index.
	if e = f.s.db.Delete(key('r', ids[249]), &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.History(ctx, f.seed, x.s, x.entry[0], x.session[0], ids[247], 8); !errors.Is(e, ErrCorrupt) {
		t.Fatal("selected missing row", e)
	}
	// Cancellation after one completed validation joins that row and stops before
	// touching any later row, even though the audited journal is at capacity.
	work, cancel := context.WithCancel(ctx)
	count := 0
	f.s.checkpoint = func(stage string) {
		if stage == "record-validated" {
			count++
			cancel()
		}
	}
	if _, e = f.s.History(work, f.seed, x.s, x.entry[0], x.session[0], "", 8); !errors.Is(e, ErrCancelled) || count != 1 {
		t.Fatal("history cancellation", e, count)
	}
}
