// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
)

// This independently authored corpus was frozen before any Go simulation.
// Never replace expected bytes or primitive values with implementation output.
type boutCorpus struct {
	Version  uint32 `json:"version"`
	Contract string `json:"contract_sha256"`
	Cases    []struct {
		Name            string `json:"name"`
		Input           Input  `json:"input"`
		CanonicalInput  string `json:"canonical_input"`
		InputID         string `json:"input_id"`
		CanonicalResult string `json:"canonical_result"`
		ResultDigest    string `json:"result_digest"`
	} `json:"cases"`
	Primitive     map[string]json.RawMessage `json:"primitive"`
	InvalidInputs []struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"invalid_inputs"`
	InputCodec []struct {
		Name     string `json:"name"`
		RawHex   string `json:"raw_hex"`
		Accepted bool   `json:"accepted"`
	} `json:"input_codec_cases"`
	ResultMutations []struct {
		Name  string `json:"name"`
		Path  []any  `json:"path"`
		Value any    `json:"value"`
	} `json:"result_mutations"`
}

func readBoutCorpus(t testing.TB) boutCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/bout_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "87bd70465fa8c4e83dd55783e4ea0ef8b7f6475ffc74dde86475fd07b0d6ee84" {
		t.Fatal("independent corpus changed")
	}
	var c boutCorpus
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Contract != "abb5b54edd921855b81dc5b99ef29b2ac94cadc9296dcd2981af4b57ad25040f" || len(c.Cases) != 55 {
		t.Fatal("corpus schema")
	}
	return c
}
func primitiveCases[T any](t *testing.T, c boutCorpus, name string) []T {
	t.Helper()
	var out []T
	if json.Unmarshal(c.Primitive[name], &out) != nil || len(out) == 0 {
		t.Fatal("primitive schema", name)
	}
	return out
}
func TestReferenceCompleteBouts(t *testing.T) {
	c := readBoutCorpus(t)
	ctx := context.Background()
	for _, tc := range c.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			in, err := ParseInput([]byte(tc.CanonicalInput))
			if err != nil || in != tc.Input {
				t.Fatal("input", err)
			}
			raw, err := MarshalInput(tc.Input)
			if err != nil || string(raw) != tc.CanonicalInput {
				t.Fatal("input bytes", err)
			}
			id, err := InputID(in)
			if err != nil || id != tc.InputID {
				t.Fatal("input digest", err)
			}
			got, err := Simulate(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := canonicalJSON(got, MaxResultBytes)
			if err != nil || string(actual) != tc.CanonicalResult {
				t.Fatalf("complete transcript mismatch (%d vs %d bytes): %v", len(actual), len(tc.CanonicalResult), err)
			}
			expected, err := ParseResult(ctx, in, []byte(tc.CanonicalResult))
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatal("expected result validation", err)
			}
			digest, err := Digest(ctx, in, expected)
			if err != nil || digest != tc.ResultDigest {
				t.Fatal("result digest", err)
			}
		})
	}
}
func TestReferenceInputAndResultRefusals(t *testing.T) {
	c := readBoutCorpus(t)
	for _, tc := range c.InvalidInputs {
		t.Run(tc.Name, func(t *testing.T) {
			var compact bytes.Buffer
			if json.Compact(&compact, tc.Input) != nil {
				t.Fatal("fixture JSON")
			}
			if got, err := ParseInput(compact.Bytes()); err == nil || got != (Input{}) {
				t.Fatal("invalid input accepted")
			}
		})
	}
	for _, tc := range c.InputCodec {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.RawHex)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseInput(raw)
			if (err == nil) != tc.Accepted {
				t.Fatal("codec acceptance", err)
			}
		})
	}
	for _, tc := range c.ResultMutations {
		t.Run(tc.Name, func(t *testing.T) {
			var v any
			if json.Unmarshal([]byte(c.Cases[0].CanonicalResult), &v) != nil {
				t.Fatal("fixture result")
			}
			at := v
			for _, p := range tc.Path[:len(tc.Path)-1] {
				switch k := p.(type) {
				case string:
					at = at.(map[string]any)[k]
				case float64:
					at = at.([]any)[int(k)]
				}
			}
			last := tc.Path[len(tc.Path)-1]
			switch k := last.(type) {
			case string:
				at.(map[string]any)[k] = tc.Value
			case float64:
				at.([]any)[int(k)] = tc.Value
			}
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := ParseResult(context.Background(), c.Cases[0].Input, raw); err == nil || !reflect.DeepEqual(got, Result{}) {
				t.Fatal("mutated raw result accepted")
			}
			if tc.Name != "extra_field" {
				var typed Result
				if json.Unmarshal(raw, &typed) != nil {
					t.Fatal("mutation decoding")
				}
				if got, err := ValidateResult(context.Background(), c.Cases[0].Input, typed); err == nil || !reflect.DeepEqual(got, Result{}) {
					t.Fatal("mutated typed result accepted")
				}
			}
		})
	}
}
func TestReferenceArithmeticAndPhysical(t *testing.T) {
	c := readBoutCorpus(t)
	for _, v := range primitiveCases[struct {
		N        int64 `json:"numerator"`
		D        int64 `json:"denominator"`
		Expected int64 `json:"expected"`
	}](t, c, "rounding") {
		got, err := roundEven(v.N, v.D)
		if err != nil || got != v.Expected {
			t.Fatal("rounding", v, got, err)
		}
	}
	for _, v := range primitiveCases[struct {
		Reach       int   `json:"reach_gap"`
		Height      int   `json:"height_gap"`
		Weight      int   `json:"weight_gap"`
		Hit         int64 `json:"range_hit_q"`
		Mass        int64 `json:"mass_damage_q"`
		Probability int64 `json:"unfatigued_equal_skill_hit_q"`
		Damage      int64 `json:"uppercut_damage_at_power50_factor1_q"`
	}](t, c, "physical") {
		a, b := FighterInput{}, FighterInput{}
		a.Manifest.Physical = fightermanifest.Physical{Height: 70 + v.Height, Reach: 70 + v.Reach, Weight: 180 + v.Weight}
		b.Manifest.Physical = fightermanifest.Physical{Height: 70, Reach: 70, Weight: 180}
		hit, mass := physicalTerms(a, b)
		m := arithmetic{}
		damage := m.mul(m.mul(m.mul(3300000, m.add(650000, m.round(50*Scale, 100))), Scale), mass)
		if hit != v.Hit || mass != v.Mass || 300000+hit != v.Probability || damage != v.Damage || m.err != nil {
			t.Fatal("physical primitive", v)
		}
	}
	for _, v := range primitiveCases[struct {
		A      uint32 `json:"a_total_tenths"`
		B      uint32 `json:"b_total_tenths"`
		Method string `json:"expected_method"`
		Winner int    `json:"expected_winner_index"`
	}](t, c, "decision_comparison") {
		method, winner := scoreDecision(v.A, v.B)
		if method != v.Method || winner != v.Winner {
			t.Fatal("decision comparison", v)
		}
	}
	for _, v := range primitiveCases[struct {
		State    State `json:"state"`
		Expected Score `json:"expected"`
	}](t, c, "scores") {
		got, err := scoreState(v.State)
		if err != nil || got != v.Expected {
			t.Fatal("score primitive", v, got, err)
		}
	}
}
func decimalWord(t *testing.T, s string) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestReferenceStreams(t *testing.T) {
	c := readBoutCorpus(t)
	for _, v := range primitiveCases[struct {
		InputID string `json:"input_id"`
		Label   string `json:"label"`
		Counter uint64 `json:"counter"`
		Word    string `json:"word_decimal"`
	}](t, c, "stream_words") {
		key, err := hex.DecodeString(v.InputID)
		if err != nil {
			t.Fatal(err)
		}
		s := randomStream{label: v.Label, counter: v.Counter, budget: &wordBudget{ctx: context.Background()}}
		copy(s.id[:], key)
		got, err := s.next()
		if err != nil || got != decimalWord(t, v.Word) || s.counter != v.Counter+1 {
			t.Fatal("stream word", v, got, err)
		}
	}
	for _, v := range primitiveCases[struct {
		Total       uint32 `json:"total_before"`
		Combat      uint64 `json:"combat_before"`
		Error       bool   `json:"expected_error"`
		Word        string `json:"word_decimal"`
		After       uint32 `json:"total_after"`
		CombatAfter uint64 `json:"combat_after"`
	}](t, c, "stream_limits") {
		d := newDraws(context.Background(), strings.Repeat("0", 64))
		d.streams[4].budget.count = v.Total
		d.streams[4].counter = v.Combat
		got, err := d.streams[4].next()
		if (err != nil) != v.Error || got != decimalWord(t, v.Word) || d.streams[4].budget.count != v.After || d.streams[4].counter != v.CombatAfter {
			t.Fatal("stream budget", v, got, err)
		}
	}
	for _, v := range primitiveCases[struct {
		Bound    string   `json:"bound_decimal"`
		Words    []string `json:"words_decimal"`
		Error    bool     `json:"expected_error"`
		Expected uint64   `json:"expected"`
		Consumed int      `json:"consumed"`
	}](t, c, "uniform_rejection") {
		calls := 0
		got, err := uniform(decimalWord(t, v.Bound), func() (uint64, error) {
			if calls >= len(v.Words) {
				t.Fatal("excess primitive draw")
			}
			w := decimalWord(t, v.Words[calls])
			calls++
			return w, nil
		})
		if (err != nil) != v.Error || got != v.Expected || calls != v.Consumed {
			t.Fatal("uniform primitive", got, err, calls)
		}
	}
	for _, v := range primitiveCases[struct {
		Probability int64  `json:"probability"`
		Ticket      uint64 `json:"ticket"`
		Bound       uint64 `json:"bound"`
		Expected    bool   `json:"expected"`
		Consumed    int    `json:"consumed"`
	}](t, c, "chance") {
		calls := 0
		d := draws{nextWord: func(stream int) (uint64, error) { calls++; return 10*v.Bound + v.Ticket, nil }}
		if d.chance(v.Probability) != v.Expected || calls != v.Consumed || d.err != nil {
			t.Fatal("chance primitive", v)
		}
	}
}
func TestReferenceInitialGrid(t *testing.T) {
	c := readBoutCorpus(t)
	for _, v := range primitiveCases[struct {
		Aged     fightercondition.AttributesTenths `json:"aged_tenths"`
		XP       uint32                            `json:"pro_xp"`
		Training uint32                            `json:"training_skill"`
		Form     int64                             `json:"form_q"`
		Jitter   int64                             `json:"jitter_q"`
		Expected Initial                           `json:"expected"`
	}](t, c, "initial_grid") {
		b := bout{input: Input{A: FighterInput{ProXP: v.XP, TrainingSkill: v.Training}, B: FighterInput{ProXP: v.XP, TrainingSkill: v.Training}}}
		b.random.nextWord = func(stream int) (uint64, error) {
			if stream < 2 {
				return 10*200001 + uint64(v.Form+100000), nil
			}
			return 10*160001 + uint64(v.Jitter-920000), nil
		}
		p := fightercondition.Snapshot{AgedAttributesTenths: v.Aged}
		b.initialize([2]fightercondition.Snapshot{p, p})
		if b.err() != nil || b.initial[0] != v.Expected || b.initial[1] != v.Expected {
			t.Fatal("initial endpoint", v, b.initial, b.err())
		}
	}
}
func referenceDraws(t *testing.T, values, bounds []uint64) (draws, *int) {
	t.Helper()
	count := 0
	d := draws{nextWord: func(stream int) (uint64, error) {
		if stream != 4 || count >= len(values) || len(values) != len(bounds) {
			t.Fatal("unexpected primitive word", stream, count)
		}
		word := 10*bounds[count] + values[count]
		count++
		return word, nil
	}}
	return d, &count
}
func TestReferencePrivateTransitions(t *testing.T) {
	c := readBoutCorpus(t)
	for _, v := range primitiveCases[struct {
		Name      string                      `json:"name"`
		Initial   [2]Initial                  `json:"initial"`
		States    [2]State                    `json:"states"`
		Manifests [2]fightermanifest.Manifest `json:"manifests"`
		Values    []uint64                    `json:"uniform_values"`
		Bounds    []uint64                    `json:"uniform_bounds"`
		Expected  Tick                        `json:"expected"`
	}](t, c, "ticks") {
		t.Run(v.Name, func(t *testing.T) {
			d, count := referenceDraws(t, v.Values, v.Bounds)
			b := bout{initial: v.Initial, states: v.States, random: d, input: Input{A: FighterInput{Manifest: v.Manifests[0]}, B: FighterInput{Manifest: v.Manifests[1]}}}
			got := b.step(1)
			if b.err() != nil || got != v.Expected || *count != len(v.Values) {
				t.Fatalf("tick differs: got %+v; want %+v; draws=%d err=%v", got, v.Expected, *count, b.err())
			}
		})
	}
	for _, v := range primitiveCases[struct {
		Name     string     `json:"name"`
		Initial  [2]Initial `json:"initial"`
		States   [2]State   `json:"states"`
		Values   []uint64   `json:"uniform_values"`
		Bounds   []uint64   `json:"uniform_bounds"`
		Expected Recovery   `json:"expected"`
		Method   string     `json:"expected_terminal_method"`
		Winner   int        `json:"expected_winner_index"`
	}](t, c, "recovery") {
		t.Run(v.Name, func(t *testing.T) {
			d, count := referenceDraws(t, v.Values, v.Bounds)
			b := bout{initial: v.Initial, states: v.States, random: d}
			got := b.recoverRound()
			method, winner := cornerDecision(got.CornerStopped)
			if b.err() != nil || got != v.Expected || *count != len(v.Values) || method != v.Method || winner != v.Winner {
				t.Fatal("recovery differs", v, got, b.err())
			}
		})
	}
}
