//go:build !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/internal/careerconsumer"
)

// This opt-in normal-clock gate uses two real enrolled public fixture accounts,
// owned disposable journals and the frozen normal node. It never starts mining.
// Empty pages are an installation/auth gate, not the separate 256-award capacity gate.
func TestCareerParentReadyRecoveryAndRealAccountReads(t *testing.T) {
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node required")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute node required")
	}
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	cfg := pilot.Config{DataDir: filepath.Join(base, "accounts"), NodeBinary: node}
	open := func() *StableController {
		t.Helper()
		start := time.Now()
		c, e := NewCareers(t.Context(), cfg)
		if e != nil {
			t.Fatal(e)
		}
		elapsed := time.Since(start)
		t.Logf("actual parent startup %s", elapsed)
		if elapsed >= 25*time.Second {
			_ = c.Close()
			t.Fatal("startup exceeded 25s")
		}
		return c
	}
	c := open()
	t.Cleanup(func() {
		if c != nil {
			_ = c.Close()
		}
	})
	if e = seedInstallLogin(t, c.ManifestController, "zero256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	m, e := readCareerMarker(filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json"))
	if e != nil || m.CareerPhase != "waiting_source" || m.CareerActivation != nil {
		t.Fatal("first gym invented activation", m, e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = open()
	if e = seedInstallLogin(t, c.ManifestController, "incrementing256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	marker := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e = readCareerMarker(marker)
	if e != nil || m.CareerPhase != "ready" || m.CareerActivation == nil {
		t.Fatal("activation not ready", m, e)
	}
	activation := *m.CareerActivation
	checkReads := func() {
		t.Helper()
		snap := c.Snapshot()
		if snap.View == nil || snap.State != "stopped" || snap.AcceptedCount != 0 {
			t.Fatal("unexpected mining/admission or view", snap.State, snap.AcceptedCount)
		}
		for _, roster := range []bool{false, true} {
			jobID := careerParentPageJob(t, c, snap.View.ViewID, roster)
			start := time.Now()
			if roster {
				p, e := c.CareerRoster(t.Context(), snap.View.ViewID, jobID, 1, 8)
				if e != nil || p.Activation != activation || len(p.Items) != 0 || len(p.Source.Items) != 0 {
					t.Fatal("roster", p, e)
				}
			} else {
				p, e := c.CareerInventory(t.Context(), snap.View.ViewID, jobID, 0, 8)
				if e != nil || p.Activation != activation || len(p.Items) != 0 || len(p.Source.Items) != 0 {
					t.Fatal("inventory", p, e)
				}
			}
			elapsed := time.Since(start)
			t.Logf("actual account career roster=%v elapsed=%s", roster, elapsed)
			if elapsed >= 500*time.Millisecond {
				t.Fatal("career metadata exceeded 500ms")
			}
		}
	}
	checkReads()
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	before, e := os.ReadFile(marker)
	if e != nil {
		t.Fatal(e)
	}
	if old, e := NewResults(t.Context(), cfg); e == nil || old != nil {
		if old != nil {
			_ = old.Close()
		}
		t.Fatal("legacy route accepted v9", e)
	}
	after, e := os.ReadFile(marker)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("legacy refusal altered root", e)
	}
	// Delete only a test-owned guard. Ready must refuse, never silently rebuild it.
	guardPath := filepath.Join(cfg.DataDir, "gyms", m.StableInstalls[0].AccountID, "stable", "db")
	db, e := leveldb.OpenFile(guardPath, nil)
	if e != nil {
		t.Fatal(e)
	}
	guard, e := db.Get([]byte(careerconsumer.GuardKey), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Delete([]byte(careerconsumer.GuardKey), &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	if bad, e := NewCareers(context.Background(), cfg); e == nil || bad != nil {
		if bad != nil {
			_ = bad.Close()
		}
		t.Fatal("ready accepted missing guard", e)
	}
	db, e = leveldb.OpenFile(guardPath, nil)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Get([]byte(careerconsumer.GuardKey), nil)
	if e != leveldb.ErrNotFound {
		t.Fatal("ready refusal repaired guard", e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	// This explicitly modeled prepared crash state carries the exact activation
	// already durable before the first consumer bind. Recovery may restore it.
	m.CareerPhase = "prepared"
	if e = writeCareerMarker(marker, m); e != nil {
		t.Fatal(e)
	}
	c = open()
	m, e = readCareerMarker(marker)
	if e != nil || m.CareerPhase != "ready" || m.CareerActivation == nil || *m.CareerActivation != activation {
		t.Fatal("prepared recovery changed activation", e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	db, e = leveldb.OpenFile(guardPath, &opt.Options{ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	got, e := db.Get([]byte(careerconsumer.GuardKey), nil)
	if e != nil || !bytes.Equal(got, guard) {
		t.Fatal("recovered guard differs", e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	c = open()
	if e = seedInstallLogin(t, c.ManifestController, "zero256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	checkReads()
}
