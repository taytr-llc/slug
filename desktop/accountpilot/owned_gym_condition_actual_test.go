//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

// SOURCE-ONLY proposal. This is an assertion for the two fresh, XP-zero
// fixtures, not a general condition projector. It reads the inherited frozen
// independent aging table; it never calls fightercondition.Project or uses a
// product-produced condition as an expected value.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

// ownedFundedAssertPrimeCondition compares every condition field for the exact
// accepted public birth/DNA and target cutoff. gridPath must name the separately
// source-bound fightercondition/testdata/reference/aging_grid_v1.json. The
// caller retains item from the original public admission, including after a
// transfer; neither birth nor DNA is replaced with the cutoff result.
func ownedFundedAssertPrimeCondition(t *testing.T, gridPath string, item gymstable.OwnedItem, member gymevent.OwnedMember, cutoffMS int64) fightercondition.Snapshot {
	t.Helper()
	const weekMS int64 = 604800000 // Independent contract/reference WEEK_MS.
	const eightDaysMS int64 = 8 * 24 * 60 * 60 * 1000
	const tableSHA = "d153d08e074fda21c5190de6e39617bcf799178201bdea256355cabf54bb95ff"
	if member.Birth != item.Birth || member.Class != item.Manifest.Physical.Class || item.Birth.BornAtMS <= 0 || cutoffMS < item.Birth.BornAtMS {
		t.Fatal("cutoff changed the original accepted birth, class, or time ordering")
	}
	elapsed := cutoffMS - item.Birth.BornAtMS
	dna := item.Manifest.DNA
	if elapsed >= eightDaysMS || dna.DNAVersion != 1 || len(dna.Seed) != 64 || dna.PrimeWeeks < 26 || dna.PrimeWeeks > 52 || dna.DeclineWeeks < 13 || dna.DeclineWeeks > 39 {
		t.Fatal("condition assertion is restricted to fresh canonical prime fixtures")
	}
	seed, err := hex.DecodeString(dna.Seed)
	if err != nil || len(seed) != 32 || hex.EncodeToString(seed) != dna.Seed {
		t.Fatal("condition fixture seed grammar")
	}
	// Size and exact hash are checked before decoding this public fixture. No
	// generator/import executes here, and no actual result updates this file.
	f, err := os.Open(gridPath)
	if err != nil {
		t.Fatal("open frozen independent condition grid", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) > 2<<20 {
		t.Fatal("bounded independent condition grid read", readErr, closeErr)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != tableSHA {
		t.Fatal("independent condition grid changed")
	}
	var grid struct {
		Version         uint32 `json:"version"`
		LossPercentages []int  `json:"loss_percentages"`
		CellCount       int    `json:"cell_count"`
		Rows            []struct {
			Original     int     `json:"original"`
			DeclineWeeks int     `json:"decline_weeks"`
			ByLoss       [][]int `json:"aged_tenths_by_loss"`
		} `json:"rows"`
	}
	if json.Unmarshal(raw, &grid) != nil || grid.Version != 1 || grid.CellCount != 291600 || len(grid.Rows) != 2700 || len(grid.LossPercentages) != 4 {
		t.Fatal("independent condition grid framing")
	}
	for i, loss := range [...]int{10, 20, 30, 40} {
		if grid.LossPercentages[i] != loss {
			t.Fatal("independent condition loss ordering")
		}
	}
	// The independent table enumerates A=1..100, D=13..39, then loss
	// 10/20/30/40 and decline step0..D. Both fresh fixtures are before the
	// minimum 26-week prime boundary, so use exactly step zero for every attr.
	lookup := func(original, lossSlot int) int {
		if original < 1 || original > 100 {
			t.Fatal("public DNA attribute outside inherited condition domain")
		}
		row := grid.Rows[(original-1)*27+dna.DeclineWeeks-13]
		if row.Original != original || row.DeclineWeeks != dna.DeclineWeeks || len(row.ByLoss) != 4 || len(row.ByLoss[lossSlot]) != dna.DeclineWeeks+1 {
			t.Fatal("independent grid row identity")
		}
		return row.ByLoss[lossSlot][0]
	}
	a := dna.Attributes
	attrs := fightercondition.AttributesTenths{
		Power: lookup(a.Power, 1), Speed: lookup(a.Speed, 3),
		Chin: lookup(a.Chin, 2), Stamina: lookup(a.Stamina, 2), Skill: lookup(a.Skill, 0),
	}
	age := elapsed / weekMS
	// Exact timing/rating arithmetic is from the independently frozen Python
	// oracle: RATING power10/speed25/chin2/stamina4/skill8. XP is exactly zero;
	// no float rounding, condition.Project call, or later career state enters.
	want := fightercondition.Snapshot{
		Version: 1, ProjectorID: "slug-fighter-condition-candidate-v1", DNAVersion: 1, Seed: dna.Seed,
		BornAtMS: item.Birth.BornAtMS, AsOfMS: cutoffMS, AgeWeeks: age, Phase: "prime",
		ElapsedDeclineWeeks: 0, PrimeRemainingWeeks: dna.PrimeWeeks - int(age),
		PrimeRemainingMS:  int64(dna.PrimeWeeks)*weekMS - elapsed,
		CareerRemainingMS: int64(dna.PrimeWeeks+dna.DeclineWeeks)*weekMS - elapsed,
		ProXP:             0, AgedAttributesTenths: attrs, LeagueAttributesTenths: attrs,
		RatingHundredths: 10*attrs.Power + 25*attrs.Speed + 2*attrs.Chin + 4*attrs.Stamina + 8*attrs.Skill,
	}
	if member.ConditionAtCutoff != want {
		t.Fatalf("exact independent fresh condition mismatch: got=%+v want=%+v", member.ConditionAtCutoff, want)
	}
	return want
}
