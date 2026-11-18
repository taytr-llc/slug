// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymseed"
	"strings"
	"testing"
)

func TestCardModeledSourceDerivationAndCodec(t *testing.T) {
	requireNormal(t)
	p, source := modelSource(t)
	plan, e := DerivePlan(p, source)
	if e != nil {
		t.Fatal(e)
	}
	if int(plan.Counts.TotalCount) != len(plan.Entrants) || plan.Counts.PairedCount != 2*plan.BoutCount {
		t.Fatal("conservation")
	}
	digest, e := gymseed.CardSourceDigest(p.SourcePin, source)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := ID(p)
	r := Record{1, id, plan.CardID, source, digest, plan, source.Finality.ObservedAtMS}
	b, e := MarshalRecord(p, r)
	if e != nil {
		t.Fatal(e)
	}
	got, e := ParseRecord(p, b)
	if e != nil || !equal(got, r) {
		t.Fatal(e)
	}
	var any map[string]any
	json.Unmarshal(b, &any)
	permuted, _ := json.MarshalIndent(any, "", "  ")
	if _, e = ParseRecord(p, permuted); e != nil {
		t.Fatal("ordered keys/whitespace", e)
	}
	for _, bad := range [][]byte{append([]byte(`{"version":1,`), b[1:]...), bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(b, []byte(`"wins":0`), []byte(`"wins":false`), 1)} {
		if _, e := ParseRecord(p, bad); e == nil {
			t.Fatal("closed JSON accepted")
		}
	}
	if len(plan.Entrants) > 0 {
		bad, _ := ParseRecord(p, b)
		bad.Plan.Entrants[0].DisplayName = "forged"
		if _, e = ValidateRecord(p, bad); e == nil {
			t.Fatal("source name not bound")
		}
	}
	// Local Plan validation cannot assert absent source preimages; complete Record can.
	alternate := plan
	alternate.InputID = hash("arbitrary input")
	alternate = match(p, alternate)
	if _, e = ValidatePlan(p, alternate); e != nil {
		t.Fatal("local-only plan", e)
	}
	r.Plan = alternate
	if _, e = ValidateRecord(p, r); e == nil {
		t.Fatal("complete source recipe omitted")
	}
	if _, e = ValidatePin(Pin{}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestCardBandAndReasonPriorityLocalScalars(t *testing.T) {
	requireNormal(t)
	p, _ := modelSource(t)
	parts := p.SourcePin.SourcePin.Participants
	base := Plan{Version: 1, CardID: hash("card"), InputID: hash("input"), Seed: hash("seed"), RecordBasis: RecordBasis, Gyms: []GymCounts{}, Entrants: []Entrant{}, Bouts: []Bout{}}
	for i, part := range parts {
		base.Entrants = append(base.Entrants, Entrant{ProspectID: hash("fighter", []byte{byte(i)}), GymID: part.StablePin.Identity.GymID, AccountID: part.StablePin.AccountID, DisplayName: "Scalar control", Class: "heavy", RatingHundredths: 100, CutoffEligibility: "eligible"})
	}
	// These scalar controls intentionally do not claim valid canonical DNA ratings.
	for _, pair := range [][3]uint32{{100, 100, 1}, {100, 200, 1}, {200, 100, 1}, {100, 201, 0}, {201, 100, 0}, {1, 2, 1}, {1, 3, 0}} {
		v := base
		v.Entrants = append([]Entrant{}, base.Entrants...)
		v.Entrants[0].RatingHundredths = pair[0]
		v.Entrants[1].RatingHundredths = pair[1]
		v = match(p, v)
		if v.BoutCount != pair[2] {
			t.Fatal("band", pair, v.BoutCount)
		}
		if _, e := ValidatePlan(p, v); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct{ a, b, reasonA, reasonB string }{{"retired", "not_rostered", "retired_at_cutoff", "not_rostered_at_cutoff"}, {"eligible", "retired", "opponent_ineligible_at_cutoff", "retired_at_cutoff"}, {"not_rostered", "eligible", "not_rostered_at_cutoff", "opponent_ineligible_at_cutoff"}} {
		v := base
		v.Entrants = append([]Entrant{}, base.Entrants...)
		v.Entrants[0].CutoffEligibility = tc.a
		v.Entrants[1].CutoffEligibility = tc.b
		v = match(p, v)
		if v.Entrants[0].Reason != tc.reasonA || v.Entrants[1].Reason != tc.reasonB || v.Counts.UnmatchedCount != 2 {
			t.Fatal(tc, v)
		}
	}
	lone := base
	lone.Entrants = base.Entrants[:1]
	lone = match(p, lone)
	if lone.Entrants[0].Reason != "no_opponent_in_class" {
		t.Fatal(lone)
	}
	bad := match(p, base)
	bad.Entrants[1].ProspectID = bad.Entrants[0].ProspectID
	if _, e := ValidatePlan(p, bad); e == nil {
		t.Fatal("duplicate global prospect")
	}
}

// Standalone Entrant is structurally bounded ASCII, while full Records bind
// the actual canonical generated name. Controls use six-byte JSON escapes.
func TestCardEntrantEscapedASCIIEnvelope(t *testing.T) {
	requireNormal(t)
	p, source := modelSource(t)
	plan, err := DerivePlan(p, source)
	if err != nil || len(plan.Entrants) == 0 {
		t.Fatal(err)
	}
	for _, name := range []string{strings.Repeat("\x00", 128), strings.Repeat("\x7f", 128)} {
		entrant := plan.Entrants[0]
		entrant.DisplayName = name
		raw, err := MarshalEntrant(p, entrant)
		if err != nil {
			t.Fatal("bounded ASCII", err)
		}
		parsed, err := ParseEntrant(p, raw)
		if err != nil || !equal(parsed, entrant) {
			t.Fatal("ASCII round trip", err)
		}
		if name[0] == 0 && len(raw) <= 1024 {
			t.Fatal("test did not exceed old cap")
		}
	}
}
