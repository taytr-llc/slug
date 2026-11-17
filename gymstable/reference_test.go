// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// The Python reference consumes public receipts as inputs and independently
// verifies Argon/signature/job/DNA/binding before deriving these expectations.
func TestIndependentStableNamespacesAndCanonicalReceipts(t *testing.T) {
	if wire.TestPoW {
		t.Skip("published normal-profile reference identities")
	}
	raw, err := os.ReadFile("testdata/reference/stable_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057" {
		t.Fatal("frozen stable reference changed")
	}
	var f struct {
		Approved string `json:"approved_contract_sha256"`
		Count    int    `json:"normal_argon_verifications"`
		Pins     []struct {
			Name      string
			Pin       gymstable.Pin
			Canonical string
			Preimage  string `json:"id_preimage"`
			ID        string `json:"expected_id"`
		}
		Cases []struct {
			Record     registry.ManifestRecord
			Canonical  string `json:"record_canonical"`
			Assignment gymstable.Assignment
			Conditions []struct {
				AsOf     int64 `json:"as_of_ms"`
				Expected fightercondition.Snapshot
			}
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Approved != "f977e8a7cef5ab470e5019f4f1b9d88cdb4fd03d5ee1feb6acc32321389d84d2" || f.Count != 6 || len(f.Pins) != 2 || len(f.Cases) != 6 {
		t.Fatal("incomplete approved reference")
	}
	for _, p := range f.Pins {
		t.Run(p.Name, func(t *testing.T) {
			raw, err := gymstable.MarshalPin(p.Pin)
			if err != nil || string(raw) != p.Canonical {
				t.Fatal("canonical pin differs", err)
			}
			parsed, err := gymstable.ParsePin(raw)
			if err != nil || parsed != p.Pin {
				t.Fatal("pin roundtrip", err)
			}
			id, err := gymstable.ID(p.Pin)
			if err != nil || id != p.ID {
				t.Fatal("independent framed namespace differs", id, err)
			}
			preimage, err := hex.DecodeString(p.Preimage)
			if err != nil || !bytes.HasPrefix(preimage, []byte("SLUG candidate stable\x00v1\x00")) {
				t.Fatal("wrong independent domain")
			}
			sum := sha256.Sum256(preimage)
			if hex.EncodeToString(sum[:]) != id {
				t.Fatal("independent preimage")
			}
		})
	}
	for _, tc := range f.Cases {
		raw, err := json.Marshal(tc.Record)
		if err != nil || string(raw) != tc.Canonical {
			t.Fatal("source canonical order changed", err)
		}
		for _, c := range tc.Conditions {
			got, err := fightercondition.Project(1, tc.Record.Verified.DNA, tc.Assignment.BornAtMS, c.AsOf, 0)
			if err != nil || got != c.Expected {
				t.Fatal("candidate boundary differs from independent integer projection", err)
			}
		}
	}
}

func TestFrozenZeroXPConditionCoverage(t *testing.T) {
	raw, err := os.ReadFile("../fightercondition/testdata/reference/condition_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "4f5be20e0e6ceb6b2ef4ee4063fffd2691fecb7a71d984f4624eb97162a9dbfa" {
		t.Fatal("set35 reference changed")
	}
	var f struct {
		DNA   []struct{ DNA fighter.DNA } `json:"dna_cases"`
		Cases []struct {
			DNAIndex int `json:"dna_index"`
			Version  uint32
			Born     int64 `json:"born_at_ms"`
			AsOf     int64 `json:"as_of_ms"`
			XP       int   `json:"pro_xp"`
			Expected fightercondition.Snapshot
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	count := 0
	phases := map[string]bool{}
	fractional, belowOne := false, false
	for _, c := range f.Cases {
		if c.XP != 0 {
			continue
		}
		count++
		got, err := fightercondition.Project(c.Version, f.DNA[c.DNAIndex].DNA, c.Born, c.AsOf, 0)
		if err != nil || got != c.Expected {
			t.Fatal("frozen set35 zero-XP expectation changed", err)
		}
		phases[got.Phase] = true
		for _, n := range []int{got.AgedAttributesTenths.Power, got.AgedAttributesTenths.Speed, got.AgedAttributesTenths.Chin, got.AgedAttributesTenths.Stamina, got.AgedAttributesTenths.Skill} {
			fractional = fractional || n%10 != 0
			belowOne = belowOne || n > 0 && n < 10
		}
	}
	if count != 2790 || len(phases) != 3 || !fractional || !belowOne {
		t.Fatal("missing unchanged condition boundaries", count, phases, fractional, belowOne)
	}
}
