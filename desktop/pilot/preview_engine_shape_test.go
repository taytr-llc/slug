// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fighterbout"
	"os"
	"testing"
)

func TestPreviewEngineShapeIndependentCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../fighterbout/testdata/reference/bout_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "87bd70465fa8c4e83dd55783e4ea0ef8b7f6475ffc74dde86475fd07b0d6ee84" {
		t.Fatal("independent corpus changed")
	}
	var f struct {
		Cases []struct {
			Name            string
			Input           fighterbout.Input
			CanonicalResult string `json:"canonical_result"`
			Digest          string `json:"result_digest"`
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	if len(f.Cases) != 55 {
		t.Fatal("coverage changed", len(f.Cases))
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var r fighterbout.Result
			if e = json.Unmarshal([]byte(c.CanonicalResult), &r); e != nil {
				t.Fatal(e)
			}
			if !previewEngineShape(r, c.Input.A.ProspectID, c.Input.B.ProspectID) {
				t.Fatal("independent valid engine shape refused")
			}
			if previewHash("SLUG fighter bout result\x00v1\x00", []byte(c.CanonicalResult), false) != c.Digest {
				t.Fatal("independent digest mismatch")
			}
		})
	}
}
