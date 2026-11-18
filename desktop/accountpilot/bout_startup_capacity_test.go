//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func boutCapacityRaw(s string) []byte { b, _ := hex.DecodeString(s); return b }
func boutCapacityHash(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func boutCapacityLP(b []byte) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(b))), b...)
}

// Storage stress data only: frozen modeled inputs are rebound to actual parent
// accounts, then dates/births move together. This assembles test data; generated
// engine results are not reference vectors or claimed admitted/finalized bouts.
func boutParentCapacityRecord(t *testing.T, p gymbout.Pin, template gymbout.Record, date string, minimum int64) gymbout.Record {
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
	s.CardID, e = gymcard.CardID(p.SourcePin, ev)
	if e != nil {
		t.Fatal(e)
	}
	s.SnapshotDigest = boutCapacityHash("modeled parent capacity snapshot", []byte(date))
	s.SeedEventKey, e = gymseed.EventKey(p.SourcePin.SourcePin, ev)
	if e != nil {
		t.Fatal(e)
	}
	s.CardRecordDigest = boutCapacityHash("modeled parent capacity card record", []byte(date))
	s.CardSourceDigest = boutCapacityHash("modeled parent capacity card source", []byte(date))
	s.CardCreatedAtMS = max(minimum, ev.CutoffMS+2000)
	s.FinalizedAtMS = ev.CutoffMS + 1999
	for i := range s.Corners {
		corner := &s.Corners[i]
		matched := false
		for _, owner := range p.SourcePin.SourcePin.SourcePin.Participants {
			if owner.StablePin.Identity.GymID == corner.Owner.GymID {
				corner.Owner.AccountID = owner.StablePin.AccountID
				corner.Owner.AccountAuthorityID = owner.StablePin.AccountAuthorityID
				matched = true
			}
		}
		if !matched {
			t.Fatal("original fixture owner not found")
		}
		m := &corner.Member
		m.Assignment.BornAtMS += delta
		m.Assignment.AcceptedAtMS += delta
		m.SlotAssignedAtMS += delta
		m.ConditionAtCutoff.BornAtMS += delta
		m.ConditionAtCutoff.AsOfMS += delta
	}
	input := boutCapacityHash("SLUG candidate card input\x00v1\x00", boutCapacityRaw(s.CardID), boutCapacityRaw(s.SnapshotDigest), boutCapacityRaw(s.SeedEventKey), binary.LittleEndian.AppendUint32(nil, s.TargetHeight), boutCapacityRaw(s.TargetHash), boutCapacityRaw(p.SourcePin.RulesID))
	seed := boutCapacityHash("SLUG candidate card seed\x00v1\x00", boutCapacityRaw(input), boutCapacityRaw(s.TargetHash))
	ci := uint32(0)
	for i, c := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if c == s.Bout.Class {
			ci = uint32(i)
		}
	}
	s.Bout.BoutID = boutCapacityHash("SLUG candidate card bout\x00v1\x00", boutCapacityRaw(input), binary.LittleEndian.AppendUint32(nil, ci), boutCapacityRaw(s.Bout.CornerA), boutCapacityRaw(s.Bout.CornerB))
	s.Bout.Seed = boutCapacityHash("SLUG candidate card bout seed\x00v1\x00", boutCapacityRaw(seed), boutCapacityRaw(s.Bout.BoutID))
	r.StoreID, e = gymbout.ID(p)
	if e != nil {
		t.Fatal(e)
	}
	r.CardID = s.CardID
	r.BoutID = s.Bout.BoutID
	r.CreatedAtMS = max(minimum, s.CardCreatedAtMS+1)
	r.SourceDigest, e = gymcard.BoutSourceDigest(p.SourcePin, *s)
	if e != nil {
		t.Fatal("source fixture", e)
	}
	r.Input, e = gymbout.DeriveInput(p, *s)
	if e != nil {
		t.Fatal("input fixture", e)
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
	r.ResultDigest = boutCapacityHash("SLUG fighter bout result\x00v1\x00", raw)
	return r
}

// The normal-profile performance gate opens the entire actual two-fixture v7
// parent, not merely gymbout.Open. Prior source journals are real and empty;
// the 256 saved transcript histories are explicitly modeled. Five-row source
// semantic/read latency has a separate real-account advancing-clock gate.
func TestBoutParentStartupAt256Saved(t *testing.T) {
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node required")
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UnixMilli()
	clock := func() time.Time { return time.UnixMilli(now) }
	cfg := pilot.Config{DataDir: filepath.Join(root, "accounts"), NodeBinary: node}
	c, e := NewBoutsForTestClock(t.Context(), cfg, clock)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if c != nil {
			_ = c.Close()
		}
	})
	for _, fixture := range []string{"zero256", "incrementing256"} {
		if e = seedInstallLogin(t, c.ManifestController, fixture); e != nil {
			t.Fatal(e)
		}
		stableInstallWaitLogin(t, c.ManifestController, true)
		if e = c.Close(); e != nil {
			t.Fatal(e)
		}
		c, e = NewBoutsForTestClock(t.Context(), cfg, clock)
		if e != nil {
			t.Fatal(e)
		}
	}
	if c.boutOwner == nil || c.boutOwner.store == nil {
		t.Fatal("shared journal unavailable")
	}
	info, e := c.boutOwner.store.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../gymbout/testdata/reference/bout_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec" {
		t.Fatal("frozen corpus changed")
	}
	var corpus struct {
		Compositions []struct {
			Name   string
			Record gymbout.Record
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("fixture decode")
	}
	batch := new(leveldb.Batch)
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 256; i++ {
		template := corpus.Compositions[1+i%5]
		if template.Name != "modeled_"+[]string{"light", "welter", "middle", "light_heavy", "heavy"}[i%5]+"_attribution_0" {
			t.Fatal("single coherent attribution profile")
		}
		r := boutParentCapacityRecord(t, info.Pin, template.Record, date.AddDate(0, 0, 7*(i/5)).Format("2006-01-02"), info.CreatedAtMS)
		b, e := gymbout.MarshalRecord(t.Context(), info.Pin, r)
		if e != nil {
			t.Fatal("full fixture validation", i, e)
		}
		batch.Put(append([]byte{'r'}, boutCapacityRaw(r.BoutID)...), b)
		now = max(now, r.CreatedAtMS)
	}
	db, e := leveldb.OpenFile(filepath.Join(cfg.DataDir, "bouts", "db"), nil)
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
	for k, v := range map[string]any{"clock_ms": now, "bout_count": 256} {
		n, _ := json.Marshal(v)
		needle := append([]byte(`"`+k+`":`), old[k]...)
		if bytes.Count(meta, needle) != 1 {
			t.Fatal("exact meta field")
		}
		meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), n...), 1)
	}
	batch.Put([]byte("meta"), meta)
	if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	// Constructor derives its unchanged 25-second deadline from this original
	// caller. A successful return, plus immediate signed-out snapshot, must fit
	// the existing first-request30-second envelope without a detached audit.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	c, e = NewBoutsForTestClock(ctx, cfg, clock)
	elapsed := time.Since(start)
	if e != nil {
		t.Fatalf("complete parent256 startup %s: %v", elapsed, e)
	}
	defer c.Close()
	got, e := c.boutOwner.store.Info(ctx)
	if e != nil || got.BoutCount != 256 || got.CreatedAtMS != info.CreatedAtMS {
		t.Fatal("capacity identity", e)
	}
	snapshot := c.Snapshot()
	if snapshot.View != nil || snapshot.Account.State != "signed_out" {
		t.Fatal("startup invented login")
	}
	if elapsed >= 25*time.Second || time.Since(start) >= 30*time.Second {
		t.Fatal("startup admission/first-response bound", elapsed, time.Since(start))
	}
	t.Logf("complete real two-fixture parent startup256 modeled transcripts=%s first snapshot=%s", elapsed, time.Since(start))
}
