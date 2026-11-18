//go:build slugtestclock && slugtestenginecount

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"os"
	"path/filepath"
	"testing"
)

// Runs only with the separately preserved overlay on the unchanged engine's
// actual Simulate entry. It is not runtime/performance or expected-vector data.
func TestResultActualEngineEntryZero(t *testing.T) {
	counter := os.Getenv("SLUG_BOUT_ENGINE_COUNTER")
	if counter == "" {
		t.Fatal("explicit overlay counter required")
	}
	f := newResultFixture(t) // Existing Bout Open audits its old saved row once.
	if e := os.WriteFile(counter, nil, 0600); e != nil {
		t.Fatal(e)
	}
	c, done := fixtureCall(t, f)
	first, e := f.s.prepare(c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.prepare(c); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.read(c, gymaccount.View, true); e != nil {
		t.Fatal(e)
	}
	m := fundedPublisher(t, f, first.Record)
	if _, e = f.s.publish(c, m); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.read(c, gymaccount.View, true); e != nil {
		t.Fatal(e)
	}
	done()
	f.s.Close()
	f.s, e = open(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock, false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(counter)
	if e != nil || len(b) != 0 {
		t.Fatal("Result operations/audit invoked Simulate", len(b), e)
	}
	t.Log("actual Simulate entries: Result Prepare/replay/Read/Publish/full Open = 0; old Bout startup excluded")
}
