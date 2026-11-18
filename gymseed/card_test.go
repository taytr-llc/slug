// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCardSourceProjectionClosedAndOwned(t *testing.T) {
	requireNormal(t)
	p, r := structuralRecord(t)
	r = syntheticFinality(t, p, r, r.PreparedAtMS+10)
	v, ok := cardProjection(r)
	if !ok {
		t.Fatal("projection")
	}
	raw, e := MarshalCardSource(p, v)
	if e != nil {
		t.Fatal(e)
	}
	copy, e := ParseCardSource(p, raw)
	if e != nil {
		t.Fatal(e)
	}
	copy.SourceReceipt.Captures[0].CapturedAtMS++
	if equal(copy, v) {
		t.Fatal("borrowed source alias")
	}
	mutations := []func(CardSource) CardSource{
		func(x CardSource) CardSource { x.Target.ObservedAtMS = x.SourceReceipt.CompletedAtMS - 1; return x },
		func(x CardSource) CardSource { x.Finality.ObservedAtMS = x.Target.ObservedAtMS - 1; return x },
		func(x CardSource) CardSource { x.Finality.Checkpoint.Height++; return x },
		func(x CardSource) CardSource { x.EventKey = hash("wrong event"); return x },
		func(x CardSource) CardSource { x.Anchor.Payload = x.Anchor.Payload[:144] + "00"; return x },
	}
	for i, f := range mutations {
		x, _ := ParseCardSource(p, raw)
		if _, e := ValidateCardSource(p, f(x)); e == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
	for _, bad := range [][]byte{append([]byte(`{"version":1,`), raw[1:]...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"merkle_branch":[]`), []byte(`"merkle_branch":null`), 1)} {
		if _, e := ParseCardSource(p, bad); e == nil {
			t.Fatal("invalid source shape accepted")
		}
	}
	// Only inherited Status scan sentinels are signed in the new family.
	state := CardState{1, absentStatus(p, r.SourceReceipt.Input.Event, r.PreparedAtMS), ""}
	b, e := MarshalCardState(p, state)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseCardState(p, b); e != nil {
		t.Fatal(e)
	}
	if _, e = ParseCardState(p, bytes.Replace(b, []byte(`"anchor_height":0`), []byte(`"anchor_height":-1`), 1)); e == nil {
		t.Fatal("unrelated signed path")
	}
}

// The older seed corpus legitimately permitted the same synthetic full IDs in
// both gyms. Set46's source projection must reject that ambiguity before Build.
func TestCardSourceRejectsCrossGymDuplicateProspects(t *testing.T) {
	requireNormal(t)
	c := readSeedReference(t)
	for _, d := range c.DTOs {
		if d.Kind != "Record" || d.Name != "five_class_synthetic" {
			continue
		}
		var r Record
		if e := json.Unmarshal(d.Value, &r); e != nil {
			t.Fatal(e)
		}
		if _, e := ValidateRecord(c.Pin.Value, r); e != nil {
			t.Fatal("old structurally valid source", e)
		}
		r = syntheticFinality(t, c.Pin.Value, r, r.PreparedAtMS+10)
		v, ok := cardProjection(r)
		if !ok {
			t.Fatal("projection")
		}
		ids := map[string]bool{}
		duplicate := false
		for _, p := range v.SourceReceipt.Input.Participants {
			for _, m := range p.Members {
				duplicate = duplicate || ids[m.Assignment.ProspectID]
				ids[m.Assignment.ProspectID] = true
			}
		}
		if !duplicate {
			t.Fatal("fixture not duplicate")
		}
		if _, e := ValidateCardSource(c.Pin.Value, v); e == nil {
			t.Fatal("cross-gym duplicate source accepted")
		}
		return
	}
	t.Fatal("old synthetic fixture missing")
}
