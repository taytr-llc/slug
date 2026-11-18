//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"strings"
	"testing"
)

func careerTransportFixture(t *testing.T) CareerResponse {
	v, s, items := stableTransportReference(t)
	i := items[0]
	s.LastAsOfMS = i.Condition.AsOfMS
	s.Checkpoint = i.Assignment.Sequence
	s.ObservedSourceCount = s.Checkpoint
	s.CaughtUp = true
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("b", 64), ResultCreatedAtMS: i.Assignment.BornAtMS, SourceCreatedAtMS: i.Assignment.BornAtMS, ActivatedAtMS: i.Assignment.BornAtMS}
	a.ActivationID = careerstate.ActivationID(a)
	c, e := fightercondition.Project(1, i.Manifest.DNA, i.Assignment.BornAtMS, i.Condition.AsOfMS, 1)
	if e != nil {
		t.Fatal(e)
	}
	p := gymstable.CareerPage{Version: 1, Activation: a, CareerDigest: strings.Repeat("c", 64), AsOfMS: i.Condition.AsOfMS, Source: gymstable.Page{Version: 1, Status: s, AsOfMS: i.Condition.AsOfMS, After: i.Assignment.Sequence - 1, NextAfter: i.Assignment.Sequence, HasMore: false, Items: []gymstable.Item{i}}, Items: []gymstable.CareerItem{{ProspectID: i.Assignment.ProspectID, Totals: fightercareer.Totals{Wins: 1, XP: 1, BasePoints: 10}, Condition: c}}}
	return CareerResponse{ID: 1, JobID: strings.Repeat("f", 64), OK: true, View: &v, StoreID: s.StoreID, Page: &p}
}
func TestCareerReadStrictEnvelopeAndProjection(t *testing.T) {
	good := careerTransportFixture(t)
	raw, e := MarshalCareerResponse(good)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseCareerResponse(raw); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*CareerResponse){
		"xp_without_record":     func(r *CareerResponse) { r.Page.Items[0].Totals.Wins = 0 },
		"wrong_condition":       func(r *CareerResponse) { r.Page.Items[0].Condition.RatingHundredths++ },
		"wrong_fighter":         func(r *CareerResponse) { r.Page.Items[0].ProspectID = strings.Repeat("d", 64) },
		"wrong_time":            func(r *CareerResponse) { r.Page.AsOfMS++ },
		"wrong_activation":      func(r *CareerResponse) { r.Page.Activation.ActivatedAtMS++ },
		"missing_overlay":       func(r *CareerResponse) { r.Page.Items = []gymstable.CareerItem{} },
		"mixed_payload":         func(r *CareerResponse) { r.Roster = &gymstable.CareerRosterPage{} },
		"legacy_source_changed": func(r *CareerResponse) { r.Page.Source.Items[0].Condition.ProXP = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			var bad CareerResponse
			if e = json.Unmarshal(raw, &bad); e != nil {
				t.Fatal(e)
			}
			change(&bad)
			b, _ := json.Marshal(bad)
			if _, e = ParseCareerResponse(b); e == nil {
				t.Fatal("accepted mutated career evidence")
			}
		})
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"id":1`), 1), bytes.Replace(raw, []byte(`"wins":1`), []byte(`"wins":true`), 1), bytes.Replace(raw, []byte(`"wins":1`), []byte(`"wins":1.0`), 1), bytes.Replace(raw, []byte(`"wins":1`), []byte(`"wins":-0`), 1), append(raw, []byte(` {}`)...)} {
		if _, e = ParseCareerResponse(bad); e == nil {
			t.Fatal("accepted malformed JSON")
		}
	}
}
func TestCareerRequestNoMutationOrUnusedFields(t *testing.T) {
	r := CareerRequest{ID: 1, JobID: strings.Repeat("f", 64), Command: "career_inventory", ViewID: strings.Repeat("a", 64), Limit: 8}
	raw, e := MarshalCareerRequest(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseCareerRequest(raw); e != nil {
		t.Fatal(e)
	}
	for _, command := range []string{"career_apply", "career_run", "career_finalize", "stable_inventory"} {
		x := r
		x.Command = command
		if _, e = MarshalCareerRequest(x); e == nil {
			t.Fatal(command)
		}
	}
	r.FirstSlot = 1
	if _, e = MarshalCareerRequest(r); e == nil {
		t.Fatal("unselected field")
	}
}
