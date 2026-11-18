// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

type careerPreviewReferenceDTO struct {
	Kind, Name, Canonical, SHA256 string
}
type careerPreviewReferenceRecipe struct {
	Path      []any
	Set       json.RawMessage
	Delete    bool
	Add       string
	Value     json.RawMessage
	Mutations []struct {
		Path []any
		Set  json.RawMessage
	}
	RecomputeScores bool `json:"recompute_scores"`
	RehashEngine    bool `json:"rehash_engine"`
	ReverseKeys     bool `json:"reverse_keys"`
	Pretty          bool
	Replace         []string `json:"replace_once"`
	Raw             *string  `json:"raw_hex"`
	Append          string
	Pad             *int `json:"pad_to"`
}
type careerPreviewReferenceCorpus struct {
	Compositions []struct {
		Name                 string
		Pair                 gymstable.CareerPreviewPair
		CanonicalPair        string `json:"canonical_pair"`
		Conditions           []struct{ Canonical, Preimage, Digest string }
		Recipe               CareerPreviewRecipe
		CanonicalRecipe      string `json:"canonical_recipe"`
		SeedPreimage         string `json:"seed_preimage"`
		Seed                 string
		Input                fighterbout.Input
		CanonicalInput       string `json:"canonical_input"`
		InputID              string `json:"input_id"`
		Result               CareerPreviewResult
		CanonicalResult      string `json:"canonical_result"`
		EngineResultDigest   string `json:"engine_result_digest"`
		CapturedResultDigest string `json:"captured_result_digest"`
	}
	DTOs       []careerPreviewReferenceDTO
	CodecCases []struct {
		Name         string
		Base         int
		Recipe       careerPreviewReferenceRecipe
		Accept       bool
		InputSHA     string `json:"input_sha256"`
		InputBytes   int    `json:"input_bytes"`
		CanonicalSHA string `json:"canonical_sha256"`
		CanonicalRef *int   `json:"canonical_ref"`
	} `json:"codec_cases"`
}

func careerPreviewReference(t *testing.T) careerPreviewReferenceCorpus {
	t.Helper()
	raw, e := os.ReadFile("../../gymstable/testdata/career-preview-reference/corpus.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "70cec03c5ef89765b5b929f41b4fd4314f5dd3a4dcbbae05521fbffc6cbddc8a" {
		t.Fatal("frozen preimplementation corpus changed")
	}
	var c careerPreviewReferenceCorpus
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Compositions) != 13 || len(c.DTOs) != 252 || len(c.CodecCases) != 184 {
		t.Fatal("coverage changed")
	}
	return c
}

func TestCareerPreviewIndependentCompositions(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile frozen authority inputs")
	}
	c := careerPreviewReference(t)
	for _, row := range c.Compositions {
		t.Run(row.Name, func(t *testing.T) {
			canonical := func(v any, want string) {
				t.Helper()
				raw, e := json.Marshal(v)
				if e != nil || string(raw) != want {
					t.Fatal("canonical composition differs", e)
				}
			}
			canonical(row.Pair, row.CanonicalPair)
			for n, f := range row.Pair.Fighters {
				digest, e := CareerPreviewConditionDigest(f.Condition)
				if e != nil || digest != row.Conditions[n].Digest {
					t.Fatal("condition digest", n, e)
				}
				canonical(f.Condition, row.Conditions[n].Canonical)
				preimage, e := hex.DecodeString(row.Conditions[n].Preimage)
				if e != nil || fmt.Sprintf("%x", sha256.Sum256(preimage)) != digest {
					t.Fatal("condition preimage")
				}
			}
			recipe, e := careerPreviewRecipe(row.Pair)
			if e != nil || recipe != row.Recipe {
				t.Fatal("recipe", e)
			}
			canonical(recipe, row.CanonicalRecipe)
			seed, e := CareerPreviewSeed(recipe)
			if e != nil || seed != row.Seed {
				t.Fatal("seed", e)
			}
			preimage, e := hex.DecodeString(row.SeedPreimage)
			if e != nil || fmt.Sprintf("%x", sha256.Sum256(preimage)) != seed {
				t.Fatal("seed preimage")
			}
			input, e := careerPreviewInput(row.Pair, seed)
			if e != nil {
				t.Fatal(e)
			}
			canonical(input, row.CanonicalInput)
			id, e := fighterbout.InputID(input)
			if e != nil || id != row.InputID {
				t.Fatal("input ID", e)
			}
			engine, e := fighterbout.Simulate(t.Context(), input)
			if e != nil {
				t.Fatal(e)
			}
			digest, e := fighterbout.Digest(t.Context(), input, engine)
			if e != nil || digest != row.EngineResultDigest {
				t.Fatal("engine result", e)
			}
			result := CareerPreviewResult{Version: 1, Pair: row.Pair, Recipe: recipe, Seed: seed, EngineResult: engine, EngineResultDigest: digest}
			canonical(result, row.CanonicalResult)
			if fmt.Sprintf("%x", sha256.Sum256([]byte(row.CanonicalResult))) != row.CapturedResultDigest {
				t.Fatal("whole captured result commitment")
			}
		})
	}
}

// Materialize only the input recipes frozen before the new Go code existed.
// Every expanded input must match its independently stored byte hash before a
// production decoder runs. No implementation output becomes an expectation.
func careerPreviewReferenceBytes(t *testing.T, base string, r careerPreviewReferenceRecipe) []byte {
	t.Helper()
	adapt := cardReferenceRecipe{}
	edit := func(path []any, op string, value json.RawMessage) {
		keys := make([]string, len(path))
		for i, k := range path {
			keys[i] = fmt.Sprint(k)
		}
		adapt.Edits = append(adapt.Edits, struct {
			Op, Path string
			Value    json.RawMessage
		}{op, "/" + strings.Join(keys, "/"), value})
	}
	if r.Set != nil {
		edit(r.Path, "set", r.Set)
	}
	for _, m := range r.Mutations {
		edit(m.Path, "set", m.Set)
	}
	raw := cardReferenceBytes(t, base, adapt)
	if r.RecomputeScores || r.RehashEngine {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		n, e := seedReferenceParse(d)
		if e != nil {
			t.Fatal(e)
		}
		engine, _ := cardReferenceChild(t, n, "engine_result")
		if r.RecomputeScores {
			final, _ := cardReferenceChild(t, engine, "final")
			var states [2]fighterbout.State
			if e = json.Unmarshal(final.encode(false), &states); e != nil {
				t.Fatal(e)
			}
			// Reference score equation, independent of the runtime shape checker.
			round := func(n, d int64) uint32 {
				q, r := n/d, n%d
				if r*2 > d || r*2 == d && q%2 != 0 {
					q++
				}
				return uint32(q)
			}
			var scores [2]fighterbout.Score
			for i, s := range states {
				a := uint32(0)
				if s.Thrown != 0 {
					a = round(250*int64(s.Landed), int64(s.Thrown))
				}
				v := fighterbout.Score{DamageTenths: round(s.DamageDealtQ, 100000), KnockdownTenths: 100 * s.Knockdowns, AccuracyTenths: a, VolumeTenths: 6 * s.Significant, ControlTenths: round(7*s.ControlQ, 2000000)}
				v.TotalTenths = v.DamageTenths + v.KnockdownTenths + v.AccuracyTenths + v.VolumeTenths + v.ControlTenths
				scores[i] = v
			}
			b, _ := json.Marshal(scores)
			d = json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			v, e := seedReferenceParse(d)
			if e != nil {
				t.Fatal(e)
			}
			_, i := cardReferenceChild(t, engine, "scores")
			engine.values[i] = v
		}
		if r.RehashEngine {
			h := sha256.New()
			h.Write([]byte("SLUG fighter bout result\x00v1\x00"))
			h.Write(engine.encode(false))
			_, i := cardReferenceChild(t, n, "engine_result_digest")
			n.values[i] = &seedReferenceNode{scalar: hex.EncodeToString(h.Sum(nil))}
		}
		raw = n.encode(false)
	}
	adapt = cardReferenceRecipe{}
	if r.Delete {
		edit(r.Path, "delete", nil)
	}
	if r.Add != "" {
		edit([]any{r.Add}, "set", r.Value)
	}
	raw = cardReferenceBytes(t, string(raw), adapt)
	if r.ReverseKeys {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		n, e := seedReferenceParse(d)
		if e != nil {
			t.Fatal(e)
		}
		raw = n.encode(true)
	}
	if r.Pretty {
		var b bytes.Buffer
		if e := json.Indent(&b, raw, "", " "); e != nil {
			t.Fatal(e)
		}
		raw = b.Bytes()
	}
	if len(r.Replace) != 0 {
		if len(r.Replace) != 2 || !bytes.Contains(raw, []byte(r.Replace[0])) {
			t.Fatal("replace input")
		}
		raw = bytes.Replace(raw, []byte(r.Replace[0]), []byte(r.Replace[1]), 1)
	}
	if r.Raw != nil {
		var e error
		raw, e = hex.DecodeString(*r.Raw)
		if e != nil {
			t.Fatal(e)
		}
	}
	raw = append(raw, []byte(r.Append)...)
	if r.Pad != nil {
		if len(raw) > *r.Pad {
			t.Fatal("pad input")
		}
		raw = append(raw, bytes.Repeat([]byte{' '}, *r.Pad-len(raw))...)
	}
	return raw
}

// Pair/Result/Job/Snapshot have no public import API. Their finite value controls
// run through the actual response decoder at their actual nested field path,
// then the corresponding runtime semantic check. The standalone raw-cap cases
// are reference-model bounds; only Request/Response are live framed parsers.
func careerPreviewReferenceParse(t *testing.T, kind string, raw []byte, context CareerPreviewJob) ([]byte, error) {
	t.Helper()
	if kind == "CareerPreviewRequest" {
		v, e := ParseCareerPreviewRequest(raw)
		if e != nil {
			return nil, e
		}
		return MarshalCareerPreviewRequest(v)
	}
	if kind == "CareerPreviewResponse" {
		v, e := ParseCareerPreviewResponse(raw)
		if e != nil {
			return nil, e
		}
		return MarshalCareerPreviewResponse(v)
	}
	var wrapped []byte
	switch kind {
	case "CareerPreviewJob":
		wrapped = append(append([]byte(`{"id":1,"version":1,"ok":true,"job":`), raw...), '}')
	case "CareerPreviewSnapshot":
		wrapped = append(append([]byte(`{"id":1,"version":1,"ok":true,"snapshot":`), raw...), '}')
	case "CareerPreviewResult", "CareerPreviewPair":
		b, _ := json.Marshal(CareerPreviewResponse{ID: 1, Version: 1, OK: true, Job: &context})
		old, _ := json.Marshal(context.Result)
		if kind == "CareerPreviewPair" {
			old, _ = json.Marshal(context.Result.Pair)
		}
		wrapped = bytes.Replace(b, old, raw, 1)
	default:
		return nil, fmt.Errorf("unknown helper kind %s", kind)
	}
	var response CareerPreviewResponse
	if _, e := careerPreviewDecode(wrapped, MaxResponseSize-1, 12, &response); e != nil {
		return nil, e
	}
	var v any
	switch kind {
	case "CareerPreviewSnapshot":
		if response.Snapshot == nil || !careerPreviewSnapshotValid(*response.Snapshot) {
			return nil, gymstable.ErrInvalid
		}
		v = response.Snapshot
	case "CareerPreviewPair":
		if response.Job == nil || response.Job.Result == nil || !careerPreviewPairValid(response.Job.Result.Pair, context.View, context.StoreID) {
			return nil, gymstable.ErrInvalid
		}
		v = response.Job.Result.Pair
	default:
		if response.Job == nil || !careerPreviewJobValid(*response.Job) {
			return nil, gymstable.ErrInvalid
		}
		v = response.Job
		if kind == "CareerPreviewResult" {
			v = response.Job.Result
		}
	}
	return json.Marshal(v)
}

func TestCareerPreviewIndependentValuesAndWire(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile frozen authority inputs")
	}
	c := careerPreviewReference(t)
	jobs := map[string]CareerPreviewJob{}
	for _, d := range c.DTOs {
		if d.Kind == "CareerPreviewJob" {
			var j CareerPreviewJob
			if json.Unmarshal([]byte(d.Canonical), &j) == nil && j.State == "done" {
				jobs[d.Name] = j
			}
		}
	}
	contextFor := func(name string) CareerPreviewJob {
		if j, ok := jobs[strings.Split(name, "/")[0]]; ok {
			return j
		}
		return jobs["zero"]
	}
	for _, d := range c.DTOs {
		if d.Kind == "CareerPreviewPortraitResponse" {
			continue
		} // native-owned tests
		t.Run("dto/"+d.Kind+"/"+d.Name, func(t *testing.T) {
			if fmt.Sprintf("%x", sha256.Sum256([]byte(d.Canonical))) != d.SHA256 {
				t.Fatal("stored DTO hash")
			}
			out, e := careerPreviewReferenceParse(t, d.Kind, []byte(d.Canonical), contextFor(d.Name))
			if e != nil || string(out) != d.Canonical {
				t.Fatal("canonical DTO", e)
			}
		})
	}
	for _, row := range c.CodecCases {
		d := c.DTOs[row.Base]
		if d.Kind == "CareerPreviewPortraitResponse" {
			continue
		}
		t.Run("codec/"+row.Name, func(t *testing.T) {
			raw := careerPreviewReferenceBytes(t, d.Canonical, row.Recipe)
			if len(raw) != row.InputBytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != row.InputSHA {
				t.Fatalf("recipe bytes differ: %d/%d, %x/%s", len(raw), row.InputBytes, sha256.Sum256(raw), row.InputSHA)
			}
			if strings.HasSuffix(row.Name, "/oversize") && d.Kind != "CareerPreviewRequest" && d.Kind != "CareerPreviewResponse" {
				return
			} // no standalone runtime import
			out, e := careerPreviewReferenceParse(t, d.Kind, raw, contextFor(d.Name))
			if (e == nil) != row.Accept {
				t.Fatalf("accept=%v, want=%v: %v", e == nil, row.Accept, e)
			}
			if e == nil && (row.CanonicalRef == nil || string(out) != c.DTOs[*row.CanonicalRef].Canonical || fmt.Sprintf("%x", sha256.Sum256(out)) != row.CanonicalSHA) {
				t.Fatal("accepted canonical bytes changed")
			}
		})
	}
}
