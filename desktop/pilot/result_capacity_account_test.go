//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"path/filepath"
	"testing"
	"time"
)

// Normal-profile latency only. The account/session and both parent-equivalent
// durable rechecks are real and the scoped authority clock advances with elapsed
// wall time. The ten-entrant card/history is explicitly modeled test data, not a
// mined admission/finality claim or a full native parent epoch measurement.
func TestResultMetadataRealAccountAdvancingClock(t *testing.T) {
	s, clock, _ := boutRealAccountFiveFixture(t)
	rows, e := s.boutStore.CardStatus(t.Context(), s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21")
	if e != nil || len(rows.Bouts) != 5 {
		t.Fatal("source", e)
	}
	bid := rows.Bouts[0].Bout.BoutID
	saved, e := s.boutStore.Run(t.Context(), s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21", bid)
	if e != nil || saved.Replayed {
		t.Fatal("explicit modeled Run", e)
	}
	p, e := gymresult.CandidatePin(s.boutPin)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(filepath.Dir(s.config.DataDir), "results")
	store, e := gymresult.CreateForTestClock(t.Context(), path, p, s.boutStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	info, e := store.Info(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	s.resultEnabled, s.resultAttached = true, true
	s.resultStore, s.resultPin, s.resultID = store, p, info.StoreID
	s.resultCreated, s.resultSourceCreated = info.CreatedAtMS, info.SourceCreatedAtMS
	s.resultBytes = MaxResultMetadataSize
	start := time.Now()
	status, e := s.ResultStatus(t.Context(), s.view.ViewID, "2026-11-21", bid)
	elapsed := time.Since(start)
	if e != nil || status.Exists || !status.Source.Saved.Exists || elapsed >= resultMetadataTimeout {
		t.Fatalf("empty actual-account Result Status %s: %v", elapsed, e)
	}
	t.Logf("empty Result Status real account + parent rechecks advancing clock: %s", elapsed)
	start = time.Now()
	missing, e := s.ResultRecord(t.Context(), s.view.ViewID, "2026-11-21", bid)
	elapsed = time.Since(start)
	if !errors.Is(e, gymresult.ErrNotFound) || missing.Version != 0 || elapsed >= resultMetadataTimeout {
		t.Fatalf("empty Read refusal %s: %v", elapsed, e)
	}
	t.Logf("empty Result Read refusal real account + parent rechecks: %s", elapsed)
	prepared, e := store.Prepare(t.Context(), s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), "2026-11-21", bid)
	if e != nil || prepared.Record.Publication != nil {
		t.Fatal("explicit signed preparation only", e)
	}
	for _, command := range []string{"status", "record"} {
		start = time.Now()
		switch command {
		case "status":
			status, e = s.ResultStatus(t.Context(), s.view.ViewID, "2026-11-21", bid)
			if e == nil && (!status.Exists || status.Phase != "prepared") {
				t.Fatal("prepared status")
			}
		case "record":
			read, err := s.ResultRecord(t.Context(), s.view.ViewID, "2026-11-21", bid)
			e = err
			if e == nil && read.Record.ResultKey != prepared.Record.ResultKey {
				t.Fatal("selected result changed")
			}
		}
		elapsed = time.Since(start)
		if e != nil || elapsed >= resultMetadataTimeout {
			t.Fatalf("prepared %s %s: %v", command, elapsed, e)
		}
		t.Logf("prepared Result %s real account + parent rechecks: %s", command, elapsed)
	}
}

func TestResultMetadataAt256RealAccountAdvancingClock(t *testing.T) {
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
	last := clock().UnixMilli()
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 251; i++ {
		r := boutExtraCapacityRecord(t, bi.Pin, records[i%5], date.AddDate(0, 0, 7*(1+i/5)).Format("2006-01-02"), bi.CreatedAtMS)
		raw, e := gymbout.MarshalRecord(t.Context(), bi.Pin, r)
		if e != nil {
			t.Fatal(e)
		}
		boutBatch.Put(append([]byte{'r'}, cardCapacityRawID(r.BoutID)...), raw)
		rr := resultCapacityPrepared(t, p, r, bi.CreatedAtMS, ri.CreatedAtMS)
		raw, e = gymresult.MarshalRecord(p, rr)
		if e != nil {
			t.Fatal(e)
		}
		resultBatch.Put(append([]byte{'r'}, cardCapacityRawID(rr.ResultKey)...), raw)
		last = max(last, r.CreatedAtMS, rr.PreparedAtMS)
	}
	resultCapacityWrite(t, boutPath, boutBatch, map[string]any{"clock_ms": last, "bout_count": 256})
	resultCapacityWrite(t, resultPath, resultBatch, map[string]any{"clock_ms": last, "result_count": 256})
	advance(last)
	s.boutStore, e = gymbout.OpenForTestClock(t.Context(), boutPath, bi.Pin, s.cardStore, clock)
	if e != nil {
		t.Fatal("full256 source audit", e)
	}
	defer s.boutStore.Close()
	s.resultStore, e = gymresult.OpenForTestClock(t.Context(), resultPath, p, s.boutStore, clock)
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
	for _, r := range records {
		expectedDigest := resultCapacityPrepared(t, p, r, bi.CreatedAtMS, ri.CreatedAtMS).Statement.Statement.Source.RecordDigest
		for _, cmd := range []string{"status", "record"} {
			start := time.Now()
			if cmd == "status" {
				v, err := s.ResultStatus(t.Context(), s.view.ViewID, r.Source.Event.Saturday, r.BoutID)
				e = err
				if e == nil && (!v.Exists || v.Phase != "prepared") {
					t.Fatal("selected status")
				}
			} else {
				v, err := s.ResultRecord(t.Context(), s.view.ViewID, r.Source.Event.Saturday, r.BoutID)
				e = err
				if e == nil && v.Record.Statement.Statement.Source.RecordDigest != expectedDigest {
					t.Fatal("selected record")
				}
			}
			elapsed := time.Since(start)
			if e != nil || elapsed >= resultMetadataTimeout {
				t.Fatalf("full256 result %s %s: %v", cmd, elapsed, e)
			}
			t.Logf("full256 result+256 bout five-saved source %s real-account advancing-clock: %s", cmd, elapsed)
		}
	}
}
