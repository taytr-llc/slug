// Copyright (c) 2026 The SLUG developers. ISC license.
package careerconsumer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestGuardIndependentCommitmentReference(t *testing.T) {
	raw, e := os.ReadFile("testdata/reference/corpus.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "058196afe3b6db1f53bea96aef72fc10f335ca0846d00f7db88f4b1d6165f06a" {
		t.Fatal("frozen independent corpus changed")
	}
	var corpus struct {
		Receipts []struct {
			ID        string `json:"id"`
			Row       fence  `json:"row"`
			Key       string `json:"key"`
			Canonical string `json:"canonical_json"`
		}
		Cases []struct {
			ID      string `json:"id"`
			Indices []int  `json:"input_receipt_indices"`
			Count   uint32 `json:"fence_count"`
			Digest  string `json:"fence_digest"`
			Binding string `json:"binding_canonical_json"`
		}
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Receipts) != 2 || len(corpus.Cases) != 4 {
		t.Fatal("corpus shape")
	}
	for _, r := range corpus.Receipts {
		if fenceKey(r.Row.CutoffMS, r.Row.ProspectIDs) != r.Key {
			t.Fatal("reference key", r.ID)
		}
		b, _ := json.Marshal(r.Row)
		if string(b) != r.Canonical {
			t.Fatal("reference row", r.ID)
		}
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			rows := map[string]fence{}
			for _, i := range c.Indices {
				r := corpus.Receipts[i]
				rows[r.Key] = r.Row
			}
			if uint32(len(rows)) != c.Count || fenceCommitment(rows) != c.Digest {
				t.Fatal("reference complete-set commitment")
			}
			var b binding
			if json.Unmarshal([]byte(c.Binding), &b) != nil {
				t.Fatal("binding input")
			}
			actual, _ := json.Marshal(binding{1, b.Activation, uint32(len(rows)), fenceCommitment(rows)})
			if string(actual) != c.Binding {
				t.Fatal("reference binding bytes")
			}
			g, _ := New(&b.Activation, false)
			if _, e := g.Audit([]byte(GuardKey), actual); e != nil {
				t.Fatal(e)
			}
			for _, i := range c.Indices {
				r := corpus.Receipts[i]
				if _, e := g.Audit([]byte(r.Key), []byte(r.Canonical)); e != nil {
					t.Fatal(e)
				}
			}
			if e := g.Audited(1797105600000); e != nil {
				t.Fatal("reference persisted set", e)
			}
		})
	}
}
