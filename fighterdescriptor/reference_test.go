// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterdescriptor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func descriptorReference(t *testing.T, out any) {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/manifest_job_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "34b9484db90fd6d9ca9d704ee1ac76843a717bddaeb38194df1932f77fe754ad" {
		t.Fatal("independent corpus changed")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentDescriptorCorpus(t *testing.T) {
	var corpus struct {
		Contract    string `json:"approved_contract_sha256"`
		Descriptors []struct {
			Name          string
			Descriptor    Descriptor
			Canonical, ID string
			Supported     bool `json:"supported_candidate"`
			Production    bool `json:"production_available"`
		} `json:"descriptors"`
		Inputs []struct {
			Name     string
			Hex      string `json:"input_hex"`
			Accepted bool
			Expected Descriptor
		} `json:"descriptor_inputs"`
		Structural []struct {
			Name          string
			Input         json.RawMessage
			Accepted      bool
			Expected      Descriptor
			Canonical, ID string
			Supported     bool `json:"supported_candidate"`
		} `json:"structural_descriptor_cases"`
		Normalize []struct {
			Name, Kind string
			Input      json.RawMessage
			Accepted   bool
			Expected   Descriptor
		} `json:"normalization_cases"`
		Resolve []struct {
			Name, ID             string
			Production, Accepted bool
			Expected             Descriptor
			Category             string `json:"error_category"`
		} `json:"resolver_inputs"`
	}
	descriptorReference(t, &corpus)
	if corpus.Contract != "6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66" || len(corpus.Descriptors) != 2 || len(corpus.Inputs) != 68 || len(corpus.Structural) != 59 || len(corpus.Normalize) != 3 || len(corpus.Resolve) != 10 {
		t.Fatal("incomplete or wrong frozen corpus")
	}
	for _, tc := range corpus.Descriptors {
		raw, err := Marshal(tc.Descriptor)
		if err != nil || string(raw) != tc.Canonical {
			t.Fatal("independent descriptor bytes", tc.Name, err)
		}
		id, err := ID(tc.Descriptor)
		if err != nil || id != tc.ID {
			t.Fatal("independent descriptor ID", tc.Name, err)
		}
		got, err := ResolveCandidate(tc.ID)
		if tc.Supported {
			if err != nil || got != tc.Descriptor {
				t.Fatal("candidate did not resolve")
			}
		} else if !errors.Is(err, ErrUnsupportedDescriptor) || got != (Descriptor{}) {
			t.Fatal("counterfactual resolved")
		}
		if got, err := ResolveProduction(tc.ID); !errors.Is(err, ErrProductionUnavailable) || got != (Descriptor{}) || tc.Production {
			t.Fatal("production catalog opened")
		}
	}
	for _, tc := range corpus.Inputs {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.Hex)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(raw)
			if tc.Accepted {
				if err != nil || got != tc.Expected {
					t.Fatal("independent transport acceptance", err)
				}
			} else if err == nil || got != (Descriptor{}) {
				t.Fatal("independent descriptor refusal failed")
			}
		})
	}
	for _, tc := range corpus.Structural {
		t.Run("typed_"+tc.Name, func(t *testing.T) {
			var d Descriptor
			if err := json.Unmarshal(tc.Input, &d); err != nil {
				if tc.Accepted {
					t.Fatal(err)
				}
				return
			} // Out-of-range DTO encodings cannot enter the typed API.
			err := Validate(d)
			if !tc.Accepted {
				if err == nil {
					t.Fatal("invalid typed descriptor accepted")
				}
				return
			}
			if err != nil || d != tc.Expected {
				t.Fatal("typed independent value mismatch", err)
			}
			raw, err := Marshal(d)
			if err != nil || string(raw) != tc.Canonical {
				t.Fatal("typed canonical bytes mismatch", err)
			}
			id, err := ID(d)
			if err != nil || id != tc.ID {
				t.Fatal("typed ID mismatch", err)
			}
			if _, err := ResolveCandidate(id); (err == nil) != tc.Supported {
				t.Fatal("structural codec implied support")
			}
		})
	}
	for _, tc := range corpus.Normalize {
		if tc.Kind != "descriptor" {
			continue
		}
		var d Descriptor
		if json.Unmarshal(tc.Input, &d) != nil {
			t.Fatal("normalization input")
		}
		got, err := Normalize(d)
		if !tc.Accepted || err != nil || got != tc.Expected {
			t.Fatal("independent normalization mismatch", tc.Name, err)
		}
	}
	for _, tc := range corpus.Resolve {
		var got Descriptor
		var err error
		if tc.Production {
			got, err = ResolveProduction(tc.ID)
		} else {
			got, err = ResolveCandidate(tc.ID)
		}
		if tc.Accepted {
			if err != nil || got != tc.Expected {
				t.Fatal("independent resolution mismatch", tc.Name, err)
			}
			continue
		}
		want := map[string]error{"hash shape": ErrDescriptor, "unsupported descriptor": ErrUnsupportedDescriptor, "production unavailable": ErrProductionUnavailable}[tc.Category]
		if want == nil || !errors.Is(err, want) || got != (Descriptor{}) {
			t.Fatal("resolver boundary conflated", tc.Name, err)
		}
	}
	for path, want := range map[string]string{"inputs/fighter-manifest-contract.md": CandidateDescriptor().ManifestContractHash, "inputs/portrait-renderer-contract.md": CandidateDescriptor().RendererContractHash} {
		raw, err := os.ReadFile("testdata/reference/" + path)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != want {
			t.Fatal("archived normative bytes changed", path)
		}
	}
}
