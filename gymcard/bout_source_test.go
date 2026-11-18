// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoutProjectionCanonicalSourceOwnership(t *testing.T) {
	requireNormal(t)
	c := readCardReference(t)
	for _, d := range c.DTOs {
		if d.Kind != "Record" || d.Context != "core" {
			continue
		}
		r, e := ParseRecord(c.Pin.Value, []byte(d.Canonical))
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Plan.Bouts) == 0 {
			continue
		}
		src, e := projectBooked(c.Pin.Value, r)
		if e != nil || len(src) != len(r.Plan.Bouts) {
			t.Fatal(e)
		}
		original, _ := json.Marshal(r)
		for _, s := range src {
			raw, e := MarshalBoutSource(c.Pin.Value, s)
			if e != nil {
				t.Fatal("projection", e)
			}
			owned, e := ParseBoutSource(c.Pin.Value, raw)
			if e != nil || !equal(owned, s) {
				t.Fatal("owned source codec", e)
			}
			owned.Corners[0].Member.Assignment.WorkHash = strings.Repeat("0", 64)
			if equal(owned, s) {
				t.Fatal("owned codec aliased")
			}
			digest, e := BoutSourceDigest(c.Pin.Value, s)
			if e != nil || digest != boutSourceDigest(s) {
				t.Fatal("source domain", e)
			}
		}
		after, _ := json.Marshal(r)
		if string(after) != string(original) {
			t.Fatal("projection mutated full retained record")
		}
		return
	}
	t.Fatal("no booked source fixture")
}
