//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCareerWaitingRootAndPredecessorRefusal(t *testing.T) {
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	c, e := NewCareers(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e := readCareerMarker(path)
	if e != nil || m.CareerPhase != "waiting_source" || m.CareerActivation != nil {
		t.Fatal(m, e)
	}
	if _, e = c.CareerInventory(t.Context(), "", "", 0, 8); e == nil {
		t.Fatal("signed out page")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, open := range []func() error{func() error {
		x, e := NewResults(t.Context(), cfg)
		if x != nil {
			_ = x.Close()
		}
		return e
	}, func() error {
		x, e := NewBouts(t.Context(), cfg)
		if x != nil {
			_ = x.Close()
		}
		return e
	}} {
		if e = open(); e == nil {
			t.Fatal("legacy route accepted v9")
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Fatal("refusal changed root")
		}
	}
	c, e = NewCareers(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	b, e := c.readResultRoot(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.writeResultRoot(path, b); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("inherited writer dropped career suffix")
	}
}

func careerParentPageJob(t *testing.T, c *StableController, view string, roster bool) string {
	t.Helper()
	var j pilot.StableJob
	var e error
	if roster {
		j, e = c.BeginStableRoster(t.Context(), view, 1, 8)
	} else {
		j, e = c.BeginStableInventory(t.Context(), view, 0, 8)
	}
	if e != nil {
		t.Fatal("career page admission", e)
	}
	id := j.JobID
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		j, e = c.StableJob(t.Context(), view, id)
		if e != nil {
			t.Fatal("career page poll", e)
		}
		if j.State == "done" {
			return id
		}
		if j.State != "queued" && j.State != "running" {
			t.Fatal("career page terminal", j.State, j.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("career page deadline")
	return ""
}
