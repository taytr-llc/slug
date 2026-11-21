// Copyright (c) 2026 The SLUG developers. ISC license.
package poolcapacity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestRecoveryIndependentTwoAxisReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/two-axis-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "699cf788958f4f11e64582a2d31b37102db0afc76fdcf19403337e226405a03c" {
		t.Fatal("independent two-axis corpus changed")
	}
	var corpus struct {
		Cases []struct {
			Name                string
			Inputs              Inputs
			Submissions, Blocks uint64
			Expected            *Reservation
			Error               string
		}
		Legacy []struct {
			Submissions, Blocks, Gyms uint64
			Expected                  Reservation
		} `json:"legacy_equivalence"`
		Invalid  []map[string]uint64 `json:"invalid_inputs"`
		Ordinary []struct {
			Inputs   Inputs
			Expected uint64
		} `json:"true_ordinary_ceilings"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil || len(corpus.Cases) != 22 || len(corpus.Legacy) != 7 || len(corpus.Invalid) != 12 {
		t.Fatal("independent case inventory", err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, e := Recovery(c.Inputs, c.Submissions, c.Blocks)
			if c.Error != "" {
				if e == nil || got != (Reservation{}) {
					t.Fatal("bounded refusal granted a reservation", got, e)
				}
				return
			}
			if c.Expected == nil || e != nil || got != *c.Expected {
				t.Fatalf("two-axis mismatch: got%+v expected%+v error%v", got, c.Expected, e)
			}
		})
	}
	for _, c := range corpus.Legacy {
		in := Inputs{SourceMax: 4096, Rounds: 256, RoundBytes: 256 << 20, CreditRounds: 256, Batches: 366, Gyms: c.Gyms}
		got, e := Recovery(in, c.Submissions, c.Blocks)
		if e != nil || got != c.Expected {
			t.Fatal("old all-block bound changed", c.Submissions, got, c.Expected, e)
		}
	}
	for _, c := range corpus.Ordinary {
		got, e := Envelope(c.Inputs)
		if e != nil || got.Submissions != c.Expected {
			t.Fatal("scenario artificially lowered ordinary ceiling", got, c.Expected, e)
		}
	}
	for _, change := range corpus.Invalid {
		values := map[string]uint64{"source_max": 4096, "rounds": 256, "round_bytes": 256 << 20, "credit_rounds": 256, "batches": 366, "gyms": 64}
		for key, value := range change {
			values[key] = value
		}
		data, _ := json.Marshal(values)
		var in Inputs
		if err = json.Unmarshal(data, &in); err != nil {
			t.Fatal(err)
		}
		if got, e := Recovery(in, 1, 1); e == nil || got != (Reservation{}) {
			t.Fatal("invalid source limit accepted", change, got, e)
		}
	}
}
