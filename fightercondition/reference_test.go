// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercondition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/taytr-llc/slug/fighter"
)

// Expected snapshots and grids are independently generated from the frozen
// contract and Python DNA oracle. Never refresh them from Project's output.
func referenceJSON(t *testing.T, name string, out any) {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/" + name)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"condition_v1.json":  "4f5be20e0e6ceb6b2ef4ee4063fffd2691fecb7a71d984f4624eb97162a9dbfa",
		"aging_grid_v1.json": "d153d08e074fda21c5190de6e39617bcf799178201bdea256355cabf54bb95ff",
		"ties_v1.json":       "f4b72681b304a6dd7dbe68c07bb1568481917cfcdb592561ba2c0400f85d4998",
	}[name]
	h := sha256.Sum256(raw)
	if want == "" || hex.EncodeToString(h[:]) != want {
		t.Fatal("frozen independent corpus changed", name)
	}
	if err = json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceSnapshots(t *testing.T) {
	var corpus struct {
		Version      uint32 `json:"version"`
		ProjectorID  string `json:"projector_id"`
		WeekMS       int64  `json:"week_ms"`
		ContractHash string `json:"approved_contract_sha256"`
		DNA          []struct {
			Name, Seed string
			DNA        fighter.DNA `json:"dna"`
		} `json:"dna_cases"`
		Cases []struct {
			Name     string   `json:"name"`
			DNAIndex int      `json:"dna_index"`
			Version  uint32   `json:"version"`
			Born     int64    `json:"born_at_ms"`
			AsOf     int64    `json:"as_of_ms"`
			XP       int      `json:"pro_xp"`
			Expected Snapshot `json:"expected"`
		} `json:"cases"`
		Invalid []struct {
			Name    string      `json:"name"`
			Version int64       `json:"version"`
			DNA     fighter.DNA `json:"dna"`
			Born    int64       `json:"born_at_ms"`
			AsOf    int64       `json:"as_of_ms"`
			XP      int         `json:"pro_xp"`
			Error   bool        `json:"expected_error"`
			Zero    Snapshot    `json:"expected_zero"`
		} `json:"invalid_cases"`
	}
	referenceJSON(t, "condition_v1.json", &corpus)
	if corpus.Version != 1 || corpus.ProjectorID != ProjectorID || corpus.WeekMS != weekMS || corpus.ContractHash != "8ee75162fcd631cae15b33804c787690e60bf4997fae6b7cd536bb771908e654" {
		t.Fatal("reference contract mismatch")
	}
	if len(corpus.DNA) != 12 || len(corpus.Cases) != 13950 || len(corpus.Invalid) != 48 {
		t.Fatal("incomplete frozen corpus")
	}
	for _, tc := range corpus.DNA {
		seed, err := fighter.ParseSeed(tc.Seed)
		if err != nil {
			t.Fatal(err)
		}
		dna, err := fighter.Generate(1, seed)
		if err != nil || dna != tc.DNA {
			t.Fatal("independent DNA changed", tc.Name, err)
		}
	}
	phases := map[string]bool{}
	belowWhole, aboveSkillCap, wideAge := false, false, false
	for _, tc := range corpus.Cases {
		if tc.DNAIndex < 0 || tc.DNAIndex >= len(corpus.DNA) {
			t.Fatal("invalid fixture index")
		}
		dna := corpus.DNA[tc.DNAIndex].DNA
		before := dna
		got, err := Project(tc.Version, dna, tc.Born, tc.AsOf, tc.XP)
		if err != nil || got != tc.Expected {
			t.Fatalf("%s independent snapshot mismatch: got %+v want %+v (%v)", tc.Name, got, tc.Expected, err)
		}
		if dna != before {
			t.Fatal("original DNA changed")
		}
		phases[got.Phase] = true
		for _, v := range []int{got.AgedAttributesTenths.Power, got.AgedAttributesTenths.Speed, got.AgedAttributesTenths.Chin, got.AgedAttributesTenths.Stamina, got.AgedAttributesTenths.Skill} {
			if v > 0 && v < 10 {
				belowWhole = true
			}
		}
		aboveSkillCap = aboveSkillCap || got.LeagueAttributesTenths.Skill > 1000
		wideAge = wideAge || got.AgeWeeks > math.MaxInt32
	}
	if len(phases) != 3 || !phases["prime"] || !phases["decline"] || !phases["retired"] || !belowWhole || !aboveSkillCap || !wideAge {
		t.Fatal("missing material boundary coverage", phases, belowWhole, aboveSkillCap, wideAge)
	}
	for _, tc := range corpus.Invalid {
		t.Run(tc.Name, func(t *testing.T) {
			if !tc.Error || tc.Zero != (Snapshot{}) {
				t.Fatal("invalid rejection fixture")
			}
			// The Python -1 selector is outside Go's uint32 API; its modulo
			// representation remains an unknown selector, never a JSON API.
			got, err := Project(uint32(tc.Version), tc.DNA, tc.Born, tc.AsOf, tc.XP)
			if !errors.Is(err, ErrCondition) || got != tc.Zero {
				t.Fatal("reference invalid input accepted", got, err)
			}
		})
	}
}

func TestReferenceExhaustiveAging(t *testing.T) {
	var grid struct {
		Losses []int `json:"loss_percentages"`
		Count  int   `json:"cell_count"`
		Rows   []struct {
			Original int     `json:"original"`
			Decline  int     `json:"decline_weeks"`
			Values   [][]int `json:"aged_tenths_by_loss"`
		} `json:"rows"`
	}
	referenceJSON(t, "aging_grid_v1.json", &grid)
	if len(grid.Losses) != 4 || grid.Losses[0] != 10 || grid.Losses[1] != 20 || grid.Losses[2] != 30 || grid.Losses[3] != 40 || len(grid.Rows) != 2700 || grid.Count != 291600 {
		t.Fatal("incomplete independent aging grid")
	}
	count := 0
	for i, row := range grid.Rows {
		if row.Original != 1+i/27 || row.Decline != 13+i%27 || len(row.Values) != 4 {
			t.Fatal("grid order/coverage changed")
		}
		for k, values := range row.Values {
			loss := grid.Losses[k]
			if len(values) != row.Decline+1 {
				t.Fatal("incomplete step coverage")
			}
			previous := math.MaxInt
			for step, want := range values {
				got := ageAttribute(row.Original, loss, step, row.Decline)
				if got != want {
					t.Fatalf("A=%d L=%d d=%d D=%d: %d != independent %d", row.Original, loss, step, row.Decline, got, want)
				}
				if got > previous {
					t.Fatal("decline increased an attribute")
				}
				previous = got
				count++
			}
			if values[0] != row.Original*10 || values[len(values)-1] != row.Original*(100-loss)/10 {
				t.Fatal("exact endpoints changed")
			}
		}
	}
	if count != grid.Count {
		t.Fatal("incomplete cell execution")
	}
}

func TestReferenceEveryRoundEvenTie(t *testing.T) {
	var corpus struct {
		Count int `json:"tie_count"`
		Ties  []struct {
			Original    int   `json:"original"`
			Decline     int   `json:"decline_weeks"`
			Step        int   `json:"elapsed_decline_weeks"`
			Loss        int   `json:"loss_percent"`
			Numerator   int64 `json:"numerator"`
			Denominator int64 `json:"denominator"`
			Floor       int   `json:"floor"`
			Aged        int   `json:"aged_tenths"`
		} `json:"ties"`
	}
	referenceJSON(t, "ties_v1.json", &corpus)
	if corpus.Count != 7836 || len(corpus.Ties) != corpus.Count {
		t.Fatal("incomplete tie corpus")
	}
	for _, tc := range corpus.Ties {
		if 2*(tc.Numerator%tc.Denominator) != tc.Denominator {
			t.Fatal("fixture is not a tie")
		}
		got := ageAttribute(tc.Original, tc.Loss, tc.Step, tc.Decline)
		if got != tc.Aged || got%2 != 0 || (got != tc.Floor && got != tc.Floor+1) {
			t.Fatal("independent tie mismatch", tc, got)
		}
	}
}
