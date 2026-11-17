// Copyright (c) 2026 The SLUG developers. ISC license.
package fightermanifest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/taytr-llc/slug/fighter"
)

func corpus(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("testdata/manifests_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil {
		t.Fatal("corpus")
	}
	return result
}
func TestIndependentStreamsAndBoundaries(t *testing.T) {
	doc := corpus(t)
	var streams []struct {
		Seed, Label string
		Counter     string `json:"counter_decimal"`
		Expected    string `json:"uint64_decimal"`
	}
	if json.Unmarshal(doc["stream_vectors"], &streams) != nil || len(streams) != 68 {
		t.Fatal("stream corpus")
	}
	for _, v := range streams {
		seed, err := fighter.ParseSeed(v.Seed)
		if err != nil {
			t.Fatal(err)
		}
		counter, err := strconv.ParseUint(v.Counter, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		want, err := strconv.ParseUint(v.Expected, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		s := stream{seed: seed, label: v.Label, counter: counter}
		got, err := s.next()
		if err != nil || got != want {
			t.Fatal("stream", v.Label, counter, got, want, err)
		}
		if counter == math.MaxUint64 {
			if _, err := s.next(); !errors.Is(err, ErrStream) {
				t.Fatal("counter wrapped")
			}
		}
	}
	var intervals []struct {
		Weight int    `json:"weight_lb"`
		First  uint64 `json:"first_rank"`
		Last   uint64 `json:"last_rank"`
	}
	if json.Unmarshal(doc["weight_rank_intervals"], &intervals) != nil || len(intervals) != 141 {
		t.Fatal("weight corpus")
	}
	for _, v := range intervals {
		for _, rank := range []uint64{v.First, v.Last} {
			got, err := weightForTicket(rank)
			if err != nil || got != v.Weight {
				t.Fatal("weight interval", v, err)
			}
		}
	}
	if _, err := weightForTicket(329511); err == nil {
		t.Fatal("out of range weight ticket")
	}
	var heights []struct {
		Weight int `json:"weight_lb"`
		Jitter int `json:"jitter_steps"`
		Want   int `json:"height_in"`
	}
	if json.Unmarshal(doc["height_vectors"], &heights) != nil || len(heights) != 33 {
		t.Fatal("height corpus")
	}
	for _, v := range heights {
		got, err := heightFor(v.Weight, v.Jitter)
		if err != nil || got != v.Want {
			t.Fatal("height", v, err)
		}
	}
	var classes []struct {
		Weight int    `json:"weight_lb"`
		Want   string `json:"expected"`
	}
	if json.Unmarshal(doc["class_vectors"], &classes) != nil || len(classes) != 10 {
		t.Fatal("class corpus")
	}
	for _, v := range classes {
		got, err := classFor(v.Weight)
		if err != nil || got != v.Want {
			t.Fatal("class", v, err)
		}
	}
	for _, weight := range []int{0, 127, 269, math.MaxInt} {
		if _, err := classFor(weight); err == nil {
			t.Fatal("invalid class weight")
		}
	}
	var styles []struct {
		Attributes fighter.Attributes `json:"attributes"`
		Reach      int                `json:"reach_offset_in"`
		Want       string             `json:"expected"`
	}
	if json.Unmarshal(doc["style_vectors"], &styles) != nil || len(styles) != 21 {
		t.Fatal("style corpus")
	}
	for _, v := range styles {
		if got := styleFor(v.Attributes, Physical{Height: 70, Reach: 70 + v.Reach}); got != v.Want {
			t.Fatal("style", v, got)
		}
	}
}
func TestIndependentSampling(t *testing.T) {
	var vectors []struct {
		Bound    uint64   `json:"bound"`
		Draws    []string `json:"draws_decimal"`
		Repeated string   `json:"repeated_draw"`
		Prefix   string   `json:"prefix_repeated_draw"`
		Count    int      `json:"prefix_count"`
		Last     string   `json:"last_draw"`
		Want     uint64   `json:"expected"`
		Attempts int      `json:"attempts"`
		Error    string   `json:"error"`
		Counter  string   `json:"counter_decimal"`
	}
	if json.Unmarshal(corpus(t)["sampling_vectors"], &vectors) != nil || len(vectors) != 5 {
		t.Fatal("sampling corpus")
	}
	for _, v := range vectors {
		if v.Counter != "" {
			continue
		} // Stream corpus tests the same max counter.
		calls := 0
		next := func() (uint64, error) {
			i := calls
			calls++
			var text string
			switch {
			case v.Repeated != "":
				text = v.Repeated
			case v.Prefix != "":
				if i < v.Count {
					text = v.Prefix
				} else {
					text = v.Last
				}
			case i < len(v.Draws):
				text = v.Draws[i]
			default:
				t.Fatal("unexpected extra sample")
			}
			n, err := strconv.ParseUint(text, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return n, nil
		}
		got, err := uniform(v.Bound, next)
		if calls != v.Attempts {
			t.Fatal("sample attempts", calls, v.Attempts)
		}
		if v.Error != "" {
			if !errors.Is(err, ErrStream) {
				t.Fatal("expected exhaustion", err)
			}
		} else if err != nil || got != v.Want {
			t.Fatal("sample", got, v.Want, err)
		}
	}
	if _, err := uniform(0, func() (uint64, error) { t.Fatal("zero bound drew"); return 0, nil }); err == nil {
		t.Fatal("zero bound")
	}
}
func TestIndependentInvalidManifests(t *testing.T) {
	var vectors []struct {
		Name     string
		Hex      string `json:"input_hex"`
		Accepted bool   `json:"accepted"`
	}
	if json.Unmarshal(corpus(t)["invalid_manifest_inputs"], &vectors) != nil || len(vectors) != 64 {
		t.Fatal("invalid corpus")
	}
	for _, v := range vectors {
		raw, err := hex.DecodeString(v.Hex)
		if err != nil || v.Accepted {
			t.Fatal("invalid fixture")
		}
		if _, err := Parse(raw); err == nil {
			t.Fatal("accepted", v.Name)
		}
	}
	m, err := Generate(Version1, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	m.Name.First = "Contradictory"
	if _, err := Marshal(m); err == nil {
		t.Fatal("accepted unrelated name")
	}
	if _, err := Generate(2, [32]byte{}); err == nil {
		t.Fatal("unknown version")
	}
}
func FuzzCanonicalManifest(f *testing.F) {
	m, err := Generate(Version1, [32]byte{})
	if err != nil {
		f.Fatal(err)
	}
	raw, err := Marshal(m)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte("{\"x\":1,\"x\":2}"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		m, err := Parse(raw)
		if err != nil {
			return
		}
		canonical, err := Marshal(m)
		if err != nil || !bytes.Equal(raw, canonical) {
			t.Fatal("accepted noncanonical manifest")
		}
	})
}
