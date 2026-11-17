// Copyright (c) 2026 The SLUG developers. ISC license.
package fightermanifest

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestIndependentManifests(t *testing.T) {
	raw, err := os.ReadFile("testdata/manifests_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Preview bool `json:"preview_only"`
		Cases   []struct {
			Seed      string   `json:"seed"`
			Manifest  Manifest `json:"manifest"`
			Canonical string   `json:"canonical"`
			Digest    string   `json:"digest"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &document) != nil || !document.Preview || len(document.Cases) != 50 {
		t.Fatal("independent corpus malformed")
	}
	for _, c := range document.Cases {
		m, err := GenerateText(Version1, c.Seed)
		if err != nil || m != c.Manifest {
			t.Fatal("independent generated fields", c.Seed, err)
		}
		raw, err := Marshal(m)
		if err != nil || !bytes.Equal(raw, []byte(c.Canonical)) {
			t.Fatal("independent canonical bytes", err)
		}
		parsed, err := Parse(raw)
		if err != nil || parsed != m {
			t.Fatal("canonical parse", err)
		}
		digest, err := Digest(m)
		if err != nil || digest != c.Digest {
			t.Fatal("independent digest", err)
		}
	}
}
