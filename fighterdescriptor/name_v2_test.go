// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterdescriptor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestIndependentNameV2Descriptor(t *testing.T) {
	raw, err := os.ReadFile("testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent corpus changed")
	}
	type fixture struct {
		Name       string
		Descriptor Descriptor
		Canonical  string
		ID         string
		Supported  bool `json:"supported_candidate"`
	}
	var ref struct {
		Descriptor fixture
		Negative   []fixture `json:"descriptor_negative_cases"`
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	want := ref.Descriptor
	d := CandidateNameV2Descriptor()
	if d != want.Descriptor || !want.Supported || len(ref.Negative) != 7 {
		t.Fatal("independent tuple mismatch")
	}
	encoded, err := Marshal(d)
	if err != nil || !bytes.Equal(encoded, []byte(want.Canonical)) {
		t.Fatal("canonical tuple", err)
	}
	id, err := ID(d)
	if err != nil || id != want.ID {
		t.Fatal("independent descriptor id", id, err)
	}
	if got, err := ResolveCandidate(id); err != nil || got != d {
		t.Fatal("v2 resolution", err)
	}
	if _, err = ResolveProduction(id); err == nil {
		t.Fatal("production activated")
	}
	if CandidateDescriptor().ManifestVersion != 1 {
		t.Fatal("default changed")
	}
	for _, n := range ref.Negative {
		id, err := ID(n.Descriptor)
		if err != nil || id != n.ID {
			t.Fatal("negative tuple derivation", n.Name, err)
		}
		if _, err = ResolveCandidate(id); err == nil || n.Supported {
			t.Fatal("counterfactual tuple resolved", n.Name)
		}
	}
}
