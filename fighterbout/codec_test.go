// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestInputStrictnessAndCompleteCornerSort(t *testing.T) {
	in := testInput(t)
	in.A.ProXP, in.B.TrainingSkill = 15, 5
	got, err := NewInput(in.AsOfMS, in.Seed, in.B, in.A)
	if err != nil || got != in {
		t.Fatal("NewInput did not sort complete fighters")
	}
	raw, err := MarshalInput(in)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseInput(raw)
	if err != nil || parsed != in {
		t.Fatal("canonical input round trip")
	}
	mutations := map[string]func(*Input){
		"version":          func(v *Input) { v.Version++ },
		"engine":           func(v *Input) { v.EngineID += "x" },
		"mode":             func(v *Input) { v.Mode = "title" },
		"seed kind":        func(v *Input) { v.SeedKind = "future" },
		"seed case":        func(v *Input) { v.Seed = strings.Repeat("A", 64) },
		"time zero":        func(v *Input) { v.AsOfMS = 0 },
		"date overflow":    func(v *Input) { v.AsOfMS = maxDateMS + 1 },
		"birth zero":       func(v *Input) { v.A.BornAtMS = 0 },
		"birth after asof": func(v *Input) { v.B.BornAtMS = v.AsOfMS + 1 },
		"xp":               func(v *Input) { v.A.ProXP = 16 },
		"training":         func(v *Input) { v.B.TrainingSkill = 6 },
		"equal ids":        func(v *Input) { v.B.ProspectID = v.A.ProspectID },
		"corner order":     func(v *Input) { v.A, v.B = v.B, v.A },
		"full manifest":    func(v *Input) { v.A.Manifest.Portrait.HairStyle++ },
		"class":            func(v *Input) { v.A.Manifest.Physical.Class = "other" },
		"huge string":      func(v *Input) { v.A.Manifest.Name.First = strings.Repeat("x", 1<<20) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			v := in
			mutate(&v)
			if got, err := ValidateInput(v); got != (Input{}) || !errors.Is(err, ErrInput) {
				t.Fatalf("invalid input accepted: %v", err)
			}
			if raw, err := MarshalInput(v); raw != nil || !errors.Is(err, ErrInput) {
				t.Fatal("invalid typed input marshaled")
			}
			if id, err := InputID(v); id != "" || !errors.Is(err, ErrInput) {
				t.Fatal("invalid typed input hashed")
			}
		})
	}
	for _, bad := range [][]byte{
		append([]byte(" "), raw...), append(append([]byte(nil), raw...), '\n'),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1.0`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":-0`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"Version":1`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"\u0076ersion":1`), 1),
		append(append([]byte(nil), raw...), []byte(`{}`)...),
		bytes.Repeat([]byte{'x'}, MaxInputBytes+1),
		[]byte{'{', '"', 0xff, '"', ':', '1', '}'},
	} {
		if got, err := ParseInput(bad); got != (Input{}) || !errors.Is(err, ErrInput) {
			t.Fatalf("noncanonical input accepted: %v", err)
		}
	}
	// The exact Date boundary remains valid, and does not change condition v1.
	in.AsOfMS = maxDateMS
	if _, err := ValidateInput(in); err != nil {
		t.Fatal("Date boundary rejected", err)
	}
}

func TestResultStrictnessAndNoPartialFailure(t *testing.T) {
	ctx := context.Background()
	in := testInput(t)
	r, err := Simulate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalResult(ctx, in, r)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseResult(ctx, in, raw)
	if err != nil || !reflect.DeepEqual(parsed, r) {
		t.Fatal("canonical result round trip", err)
	}
	id, err := Digest(ctx, in, r)
	if err != nil || !canonicalHash(id) {
		t.Fatal("result digest", err)
	}
	for name, mutate := range map[string]func(*Result){
		"winner":           func(v *Result) { v.Winner = strings.Repeat("1", 64) },
		"count":            func(v *Result) { v.DrawCounts.Combat++ },
		"score":            func(v *Result) { v.Scores[0].TotalTenths++ },
		"final":            func(v *Result) { v.Final[0].DamageDealtQ++ },
		"input id":         func(v *Result) { v.InputID = strings.Repeat("1", 64) },
		"nil rounds":       func(v *Result) { v.Rounds = nil },
		"too many rounds":  func(v *Result) { v.Rounds = make([]Round, 4) },
		"oversized method": func(v *Result) { v.Method = strings.Repeat("x", 1<<20) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := r
			mutate(&bad)
			got, err := ValidateResult(ctx, in, bad)
			if !reflect.DeepEqual(got, Result{}) || !errors.Is(err, ErrResult) {
				t.Fatal("mutated result accepted", err)
			}
			if encoded, err := MarshalResult(ctx, in, bad); encoded != nil || !errors.Is(err, ErrResult) {
				t.Fatal("mutated result marshaled", err)
			}
			if digest, err := Digest(ctx, in, bad); digest != "" || !errors.Is(err, ErrResult) {
				t.Fatal("mutated result hashed", err)
			}
		})
	}
	for _, bad := range [][]byte{
		append([]byte("\n"), raw...),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"extra":0`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(raw, []byte(`"end_tick":`), []byte(`"unknown":`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":-1`), 1),
		bytes.Repeat([]byte{'x'}, MaxResultBytes+1),
	} {
		if got, err := ParseResult(ctx, in, bad); !reflect.DeepEqual(got, Result{}) || !errors.Is(err, ErrResult) {
			t.Fatal("invalid result bytes accepted", err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := ParseResult(cancelled, in, raw); !reflect.DeepEqual(got, Result{}) || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled result parsing")
	}
}

func TestRawArrayDepthAndNumberPreflight(t *testing.T) {
	for _, bad := range []string{
		`{"rounds":[{},{},{},{}]}`, `{"initial":[{},{},{}]}`,
		`{"initial":[{}]}`, `{"initial":null}`, `{"rounds":[]}`,
		`{"ticks":[` + strings.TrimSuffix(strings.Repeat(`{},`, 61), ",") + `]}`,
		strings.Repeat(`{"a":`, MaxJSONDepth) + `{}` + strings.Repeat("}", MaxJSONDepth),
		`{"form_q":-0}`, `{"form_q":1.0}`, `{"form_q":1e0}`,
		`{"stamina_q":-1}`, `{"form_q":9223372036854775808}`,
	} {
		if err := inspectJSON([]byte(bad), MaxResultBytes); err == nil {
			t.Fatalf("raw preflight accepted %s", bad[:min(len(bad), 80)])
		}
	}
	for _, good := range []string{`{"form_q":-100000}`, `{"shoulder_jitter":-6}`,
		`{"recovery":null}`, `{"corner_stopped":[false,true]}`} {
		if err := inspectJSON([]byte(good), MaxResultBytes); err != nil {
			t.Fatalf("raw scalar shape refused %s: %v", good, err)
		}
	}
}

func TestConservativeMaximumSerialization(t *testing.T) {
	// This is only a maximum-shape calculation, not a valid fight or golden.
	s := State{StaminaQ: 214 * Scale, DamageTakenQ: 2700 * Scale, Thrown: 180,
		Landed: 180, Significant: 180, DamageDealtQ: 2700 * Scale, Knockdowns: 180,
		ControlQ: 630 * Scale, RoundKnockdowns: 3}
	r := Result{Version: 1, EngineID: EngineID, InputID: strings.Repeat("f", 64),
		Winner: strings.Repeat("f", 64), Method: "double_corner_stoppage", EndRound: 3, EndTick: 60,
		Final: [2]State{s, s}, DrawCounts: DrawCounts{65536, 65536, 65536, 65536, 65536}}
	for i := range r.Initial {
		r.Initial[i] = Initial{FormQ: -100000, JitterQ: 1080000,
			Effective:   AttributesQ{110 * Scale, 110 * Scale, 110 * Scale, 110 * Scale, 130 * Scale},
			MaxStaminaQ: 214 * Scale, KOThresholdQ: 130 * Scale}
		r.Scores[i] = Score{27000, 18000, 250, 1080, 2205, 48535}
	}
	for number := uint32(1); number <= Rounds; number++ {
		round := Round{Number: number, End: [2]State{s, s},
			Recovery: &Recovery{States: [2]State{s, s}, CornerStopped: [2]bool{false, false}}}
		for tick := uint32(1); tick <= TicksPerRound; tick++ {
			round.Ticks = append(round.Ticks, Tick{Number: tick, Actor: 1, Strike: "uppercut",
				DamageQ: 9 * Scale, Stoppage: "tko_three_knockdowns", States: [2]State{s, s}})
		}
		r.Rounds = append(r.Rounds, round)
	}
	raw, err := canonicalJSON(r, MaxResultBytes)
	if err != nil || len(raw) >= 146432 || inspectJSON(raw, MaxResultBytes) != nil {
		t.Fatalf("conservative serialization bound: %d, %v", len(raw), err)
	}
	// Worst scalar spellings are still bounded before typed invalid values are
	// compared with a real regeneration; uint32 decoder overflow is refused.
	if _, err := ParseInput([]byte(`{"version":` + "4294967296" + `}`)); err == nil {
		t.Fatal("uint32 overflow accepted")
	}
}

func FuzzParseInput(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte(readBoutCorpus(f).Cases[0].CanonicalInput))
	f.Fuzz(func(t *testing.T, raw []byte) {
		in, err := ParseInput(raw)
		if err != nil {
			if in != (Input{}) {
				t.Fatal("partial input")
			}
			return
		}
		canonical, err := MarshalInput(in)
		if err != nil || !bytes.Equal(canonical, raw) {
			t.Fatal("noncanonical accepted input")
		}
	})
}

func FuzzParseResult(f *testing.F) {
	c := readBoutCorpus(f).Cases[0]
	f.Add([]byte(`{}`))
	f.Add([]byte(c.CanonicalResult))
	f.Fuzz(func(t *testing.T, raw []byte) {
		result, err := ParseResult(context.Background(), c.Input, raw)
		if err != nil {
			if !reflect.DeepEqual(result, Result{}) {
				t.Fatal("partial result")
			}
			return
		}
		canonical, err := canonicalJSON(result, MaxResultBytes)
		if err != nil || !bytes.Equal(canonical, raw) {
			t.Fatal("noncanonical accepted result")
		}
	})
}
