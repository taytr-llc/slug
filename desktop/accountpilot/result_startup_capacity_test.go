//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Both256-row histories are explicitly modeled; original account enrollment,
// all source constructors and the full parent startup audits remain concrete.
func TestResultParentStartupAt256SavedAndResults(t *testing.T) {
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
	c, e := NewResultsForTestClock(t.Context(), cfg, clock)
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
		c, e = NewResultsForTestClock(t.Context(), cfg, clock)
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
	resultInfo, e := c.resultOwner.store.Info(t.Context())
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
	resultBatch := new(leveldb.Batch)
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
		resultRow := resultCapacityPrepared(t, resultInfo.Pin, r, info.CreatedAtMS, resultInfo.CreatedAtMS)
		resultRaw, e := gymresult.MarshalRecord(resultInfo.Pin, resultRow)
		if e != nil {
			t.Fatal(e)
		}
		resultBatch.Put(append([]byte{'r'}, boutCapacityRaw(resultRow.ResultKey)...), resultRaw)
		now = max(now, r.CreatedAtMS, resultRow.PreparedAtMS)
	}
	resultCapacityWrite(t, filepath.Join(cfg.DataDir, "bouts"), batch, map[string]any{"clock_ms": now, "bout_count": 256})
	resultCapacityWrite(t, filepath.Join(cfg.DataDir, "results"), resultBatch, map[string]any{"clock_ms": now, "result_count": 256})

	// Constructor derives its unchanged 25-second deadline from this original
	// caller. A successful return, plus immediate signed-out snapshot, must fit
	// the existing first-request30-second envelope without a detached audit.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	c, e = NewResultsForTestClock(ctx, cfg, clock)
	elapsed := time.Since(start)
	if e != nil {
		t.Fatalf("complete parent256 startup %s: %v", elapsed, e)
	}
	defer c.Close()
	got, e := c.boutOwner.store.Info(ctx)
	if e != nil || got.BoutCount != 256 || got.CreatedAtMS != info.CreatedAtMS {
		t.Fatal("capacity identity", e)
	}
	ri, e := c.resultOwner.store.Info(ctx)
	if e != nil || ri.ResultCount != 256 || ri.CreatedAtMS != resultInfo.CreatedAtMS {
		t.Fatal("result capacity identity", e)
	}
	snapshot := c.Snapshot()
	if snapshot.View != nil || snapshot.Account.State != "signed_out" {
		t.Fatal("startup invented login")
	}
	if elapsed >= 25*time.Second || time.Since(start) >= 30*time.Second {
		t.Fatal("startup admission/first-response bound", elapsed, time.Since(start))
	}
	t.Logf("complete real two-fixture parent startup256 modeled transcripts+256 prepared results=%s first snapshot=%s", elapsed, time.Since(start))
}
