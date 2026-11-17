// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func previewReference(t *testing.T) []struct {
	Name      string
	Response  PreviewResponse
	Canonical string          `json:"canonical_response"`
	Snapshot  PreviewResponse `json:"snapshot_response"`
} {
	t.Helper()
	raw, e := os.ReadFile("../../fighterbout/testdata/reference/preview_transport_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "78d49e9dce74d21d83344a2f13c858ced65c74b24e858844f55779cdf298de32" {
		t.Fatal("independent preview corpus changed")
	}
	var c struct {
		Cases []struct {
			Name      string
			Response  PreviewResponse
			Canonical string          `json:"canonical_response"`
			Snapshot  PreviewResponse `json:"snapshot_response"`
		}
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Cases) != 4 {
		t.Fatal("coverage")
	}
	return c.Cases
}
func TestPreviewIndependentTransportAndDigest(t *testing.T) {
	for _, c := range previewReference(t) {
		t.Run(c.Name, func(t *testing.T) {
			if wire.TestPoW {
				// These frozen authority/descriptor pins belong to normal PoW.
				// Cheap builds must refuse them, not rewrite or adopt the pins.
				if _, e := MarshalPreviewResponse(c.Response); e == nil {
					t.Fatal("cheap profile accepted normal response")
				}
				if _, e := ParsePreviewResponse([]byte(c.Canonical)); e == nil {
					t.Fatal("cheap profile parsed normal response")
				}
				if _, e := MarshalPreviewResponse(c.Snapshot); e == nil {
					t.Fatal("cheap profile accepted normal snapshot")
				}
				return
			}
			raw, e := MarshalPreviewResponse(c.Response)
			if e != nil {
				t.Fatal("independent response refused", e)
			}
			if string(raw) != c.Canonical {
				t.Fatal("canonical expected bytes differ")
			}
			if _, e = ParsePreviewResponse(raw); e != nil {
				t.Fatal(e)
			}
			indented := new(bytes.Buffer)
			if e = json.Indent(indented, raw, "", "  "); e != nil {
				t.Fatal(e)
			}
			if _, e = ParsePreviewResponse(indented.Bytes()); e != nil {
				t.Fatal("transport whitespace rejected", e)
			}
			raw, e = MarshalPreviewResponse(c.Snapshot)
			if e != nil {
				t.Fatal("snapshot", e)
			}
			if _, e = ParsePreviewResponse(raw); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestPreviewSelfRehashedInconsistencyAndOutcomeGrammar(t *testing.T) {
	// This unpinned transport envelope remains valid in either build profile.
	if _, e := MarshalPreviewResponse(PreviewResponse{Version: 1, ID: 1, Error: "outcome_unknown"}); e != nil {
		t.Fatal("transport envelope unknown refused", e)
	}
	if wire.TestPoW {
		t.Skip("semantic mutation controls require the normal-profile authority fixtures")
	}
	for _, c := range previewReference(t) {
		r := c.Response
		owned := clonePreview(*r.Job)
		r.Job = &owned
		engine := &r.Job.Result.EngineResult
		engine.Scores[0].DamageTenths++
		engine.Scores[0].TotalTenths++
		raw, _ := json.Marshal(engine)
		r.Job.Result.EngineResultDigest = previewHash("SLUG fighter bout result\x00v1\x00", raw, false)
		raw, _ = json.Marshal(r)
		if _, e := ParsePreviewResponse(raw); e == nil {
			t.Fatal("self-rehashed score unrelated to final state accepted")
		}
		r = c.Response
		owned = clonePreview(*r.Job)
		r.Job = &owned
		engine = &r.Job.Result.EngineResult
		if engine.Method == "decision" {
			engine.Method = "decision_draw"
			engine.Winner = ""
		} else if engine.Method == "decision_draw" {
			engine.Method = "decision"
			engine.Winner = r.Job.FighterAID
		} else {
			if engine.Winner == r.Job.FighterAID {
				engine.Winner = r.Job.FighterBID
			} else {
				engine.Winner = r.Job.FighterAID
			}
		}
		raw, _ = json.Marshal(engine)
		r.Job.Result.EngineResultDigest = previewHash("SLUG fighter bout result\x00v1\x00", raw, false)
		raw, _ = json.Marshal(r)
		if _, e := ParsePreviewResponse(raw); e == nil {
			t.Fatal("self-rehashed winner accepted", c.Name)
		}
		r = c.Response
		owned = clonePreview(*r.Job)
		r.Job = &owned
		r.Job.State = "failed"
		r.Job.Error = "outcome_unknown"
		r.Job.Result = nil
		raw, _ = json.Marshal(r)
		if _, e := ParsePreviewResponse(raw); e == nil {
			t.Fatal("terminal transport unknown accepted")
		}
		r.Job.Error = "preview_observation_unknown"
		if _, e := MarshalPreviewResponse(r); e != nil {
			t.Fatal("observation unknown refused", e)
		}
		snap := c.Snapshot
		s := *snap.Snapshot
		snap.Snapshot = &s
		s.State = "failed"
		s.Error = "outcome_unknown"
		raw, _ = json.Marshal(snap)
		if _, e := ParsePreviewResponse(raw); e == nil {
			t.Fatal("terminal snapshot transport unknown accepted")
		}
	}
}
func TestPreviewArrayPreallocationClosedCounts(t *testing.T) {
	c := previewReference(t)[0]
	// Isolate structural preallocation from profile/semantic validation. The
	// otherwise-complete shape is accepted in both profiles before mutation.
	var control PreviewResponse
	if _, e := previewDecode([]byte(c.Canonical), MaxResponseSize-1, 12, &control); e != nil {
		t.Fatal("valid structural control", e)
	}
	for _, path := range [][]string{{"pair", "fighters"}, {"pair", "status", "slots"}, {"recipe", "fighters"}, {"recipe", "training"}, {"engine_result", "initial"}, {"engine_result", "final"}, {"engine_result", "scores"}} {
		var r map[string]any
		d := json.NewDecoder(bytes.NewBufferString(c.Canonical))
		d.UseNumber()
		if e := d.Decode(&r); e != nil {
			t.Fatal(e)
		}
		m := r["job"].(map[string]any)["result"].(map[string]any)
		for _, part := range path[:len(path)-1] {
			m = m[part].(map[string]any)
		}
		name := path[len(path)-1]
		a := m[name].([]any)
		m[name] = append(a, a[0])
		raw, _ := json.Marshal(r)
		var decoded PreviewResponse
		if _, e := previewDecode(raw, MaxResponseSize-1, 12, &decoded); e == nil {
			t.Fatal("fixed array silently discarded extra", path)
		}
	}
}
