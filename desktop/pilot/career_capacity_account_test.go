//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"path/filepath"
	"testing"
	"time"
)

// Real original-account authority plus both parent-equivalent checks. All256
// finalized histories are modeled stress data, not real chain/funding claims.
func TestCareerMetadataAt256RealAccountAdvancingClock(t *testing.T) {
	s, clock, advance := boutRealAccountFiveFixture(t)
	rows, e := s.boutStore.CardStatus(t.Context(), s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21")
	if e != nil || len(rows.Bouts) != 5 {
		t.Fatal("five-source setup", e)
	}
	records := []gymbout.Record{}
	for _, row := range rows.Bouts {
		v, e := s.boutStore.Run(t.Context(), s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21", row.Bout.BoutID)
		if e != nil || v.Replayed {
			t.Fatal("modeled saved setup", e)
		}
		records = append(records, v.Record)
	}
	// Retain the actual admitted inventory IDs unchanged. Only synthetic extra
	// compact histories reuse these same-class IDs to exercise nonzero display;
	// this is not a claim that those modeled historical admissions occurred.
	inventory, e := s.stableStore.Inventory(t.Context(), s.session(), 0, 8)
	if e != nil || len(inventory.Items) != 3 {
		t.Fatal("actual retained inventory", e)
	}
	actualIDs := map[string]string{}
	for _, x := range inventory.Items {
		actualIDs[x.Manifest.Physical.Class] = x.Assignment.ProspectID
	}
	p, e := gymresult.CandidatePin(s.boutPin)
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Dir(s.config.DataDir)
	resultPath, boutPath := filepath.Join(root, "results"), filepath.Join(root, "bouts")
	store, e := gymresult.CreateForTestClock(t.Context(), resultPath, p, s.boutStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e = gymresult.ActivateCareerForTestClock(t.Context(), resultPath, p, s.boutStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	activation, e := store.CareerInfo(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	ri, e := store.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	bi, e := s.boutStore.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range records {
		if _, e = store.Prepare(t.Context(), s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), row.Source.Event.Saturday, row.BoutID); e != nil {
			t.Fatal("modeled signed setup", e)
		}
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	if e = s.boutStore.Close(); e != nil {
		t.Fatal(e)
	}
	boutBatch, resultBatch := new(leveldb.Batch), new(leveldb.Batch)
	counts := map[string]uint32{}
	putFinal := func(r gymresult.Record, n int) {
		r = careerFinalizeModeled(t, p, r, n, activation.ActivatedAtMS)
		careerPutFinalized(t, resultBatch, p, r)
		for _, corner := range r.Statement.Statement.Source.Source.Corners {
			counts[corner.Member.Assignment.ProspectID]++
		}
	}
	last := clock().UnixMilli()
	for n, r := range records {
		rr := careerFinalizeModeled(t, p, resultCapacityPrepared(t, p, r, bi.CreatedAtMS, ri.CreatedAtMS), n, activation.ActivatedAtMS)
		putFinal(rr, n)
		last = max(last, rr.Finality.ObservedAtMS+1)
	}
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 251; i++ {
		var model gymbout.Record
		owned, _ := json.Marshal(records[i%5])
		if e = json.Unmarshal(owned, &model); e != nil {
			t.Fatal(e)
		}
		if id := actualIDs[model.Source.Bout.Class]; id != "" {
			for n := range model.Source.Corners {
				corner := &model.Source.Corners[n]
				if corner.Owner.GymID == inventory.Status.Pin.Identity.GymID {
					old := corner.Member.Assignment.ProspectID
					corner.Member.Assignment.ProspectID = id
					if model.Source.Bout.CornerA == old {
						model.Source.Bout.CornerA = id
					} else if model.Source.Bout.CornerB == old {
						model.Source.Bout.CornerB = id
					} else {
						t.Fatal("modeled corner identity")
					}
				}
			}
		}
		if model.Source.Bout.CornerA > model.Source.Bout.CornerB {
			model.Source.Bout.CornerA, model.Source.Bout.CornerB = model.Source.Bout.CornerB, model.Source.Bout.CornerA
			model.Source.Corners[0], model.Source.Corners[1] = model.Source.Corners[1], model.Source.Corners[0]
		}
		r := boutExtraCapacityRecord(t, bi.Pin, model, date.AddDate(0, 0, 7*(1+i/5)).Format("2006-01-02"), bi.CreatedAtMS)
		raw, e := gymbout.MarshalRecord(t.Context(), bi.Pin, r)
		if e != nil {
			t.Fatal(e)
		}
		boutBatch.Put(append([]byte{'r'}, cardCapacityRawID(r.BoutID)...), raw)
		rr := resultCapacityPrepared(t, p, r, bi.CreatedAtMS, ri.CreatedAtMS)
		rr = careerFinalizeModeled(t, p, rr, i+5, activation.ActivatedAtMS)
		putFinal(rr, i+5)
		last = max(last, r.CreatedAtMS, rr.Finality.ObservedAtMS+1)
	}
	resultCapacityWrite(t, boutPath, boutBatch, map[string]any{"clock_ms": last, "bout_count": 256})
	resultCapacityWrite(t, resultPath, resultBatch, map[string]any{"clock_ms": last, "result_count": 256})
	advance(last)
	s.boutStore, e = gymbout.OpenForTestClock(t.Context(), boutPath, bi.Pin, s.cardStore, clock)
	if e != nil {
		t.Fatal("full256 source audit", e)
	}
	defer s.boutStore.Close()
	s.resultStore, e = gymresult.OpenCareerForTestClock(t.Context(), resultPath, p, s.boutStore, clock)
	if e != nil {
		t.Fatal("full256 result audit", e)
	}
	defer s.resultStore.Close()
	s.resultEnabled, s.resultAttached = true, true
	s.resultPin, s.resultID = p, ri.StoreID
	s.resultCreated, s.resultSourceCreated = ri.CreatedAtMS, ri.SourceCreatedAtMS
	s.resultBytes = MaxResultMetadataSize
	got, e := s.resultStore.Info(t.Context())
	if e != nil || got.ResultCount != 256 {
		t.Fatal("result capacity", e)
	}
	reader, e := s.resultStore.CareerReader()
	if e != nil {
		t.Fatal(e)
	}
	open := func(path string, pin gymstable.Pin, source *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
		return gymstable.OpenCareerForTestClock(path, pin, source, a, clock)
	}
	if e = s.BindCareer(t.Context(), reader, open); e != nil {
		t.Fatal(e)
	}
	for _, roster := range []bool{false, true} {
		jobID := careerPageJob(t, s, roster, 0, 1, 8)
		start := time.Now()
		var items []gymstable.CareerItem
		if roster {
			v, err := s.CareerRoster(t.Context(), s.view.ViewID, jobID, 1, 8)
			e = err
			items = v.Items
			if e == nil && v.Activation != activation {
				t.Fatal("roster activation")
			}
		} else {
			v, err := s.CareerInventory(t.Context(), s.view.ViewID, jobID, 0, 8)
			e = err
			items = v.Items
			if e == nil && v.Activation != activation {
				t.Fatal("inventory activation")
			}
		}
		elapsed := time.Since(start)
		if e != nil || elapsed >= careerMetadataTimeout {
			t.Fatalf("full256 career page roster=%v %s: %v", roster, elapsed, e)
		}
		if !roster && len(items) != 3 {
			t.Fatal("missing original admitted items", len(items))
		}
		for _, x := range items {
			want := counts[x.ProspectID]
			if want < 50 || x.Totals.Wins+x.Totals.Losses+x.Totals.Draws != want || x.Totals.XP != 15 || x.Condition.ProXP != 15 {
				t.Fatal("missing/capped award totals", x.ProspectID, want, x.Totals)
			}
		}
		t.Logf("full256 career +256 Bout real-account parent-rechecks advancing-clock roster=%v items=%d elapsed=%s", roster, len(items), elapsed)
	}
}
