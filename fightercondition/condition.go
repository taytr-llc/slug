// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fightercondition projects candidate current condition from canonical
// DNA and separately supplied birth, observation time and accounted experience.
// It authenticates none of those external facts and grants no eligibility,
// ownership or admission. Historical DNA and birth are never modified.
package fightercondition

import (
	"errors"

	"github.com/taytr-llc/slug/fighter"
)

const (
	Version1    uint32 = 1
	ProjectorID        = "slug-fighter-condition-candidate-v1"
	weekMS      int64  = 604800000
)

// ErrCondition indicates an unsupported version, noncanonical DNA, invalid
// timestamp ordering or experience outside the already-accounted [0,15] range.
var ErrCondition = errors.New("invalid fighter condition")

// AttributesTenths contains exact tenths of attribute units, not mined values.
// An aged attribute may be below ten tenths; XP-adjusted Skill may exceed 1000.
type AttributesTenths struct {
	Power   int `json:"power"`
	Speed   int `json:"speed"`
	Chin    int `json:"chin"`
	Stamina int `json:"stamina"`
	Skill   int `json:"skill"`
}

// Snapshot is a pure projection. Phase is an age phase, not a determination of
// professional eligibility; the caller must authenticate birth, time and XP.
// The DTO does not claim a canonical JSON or digest identity.
type Snapshot struct {
	Version                uint32           `json:"version"`
	ProjectorID            string           `json:"projector_id"`
	DNAVersion             uint32           `json:"dna_version"`
	Seed                   string           `json:"seed"`
	BornAtMS               int64            `json:"born_at_ms"`
	AsOfMS                 int64            `json:"as_of_ms"`
	AgeWeeks               int64            `json:"age_weeks"`
	Phase                  string           `json:"phase"`
	ElapsedDeclineWeeks    int              `json:"elapsed_decline_weeks"`
	PrimeRemainingWeeks    int              `json:"prime_remaining_weeks"`
	PrimeRemainingMS       int64            `json:"prime_remaining_ms"`
	CareerRemainingMS      int64            `json:"career_remaining_ms"`
	ProXP                  int              `json:"pro_xp"`
	AgedAttributesTenths   AttributesTenths `json:"aged_attributes_tenths"`
	LeagueAttributesTenths AttributesTenths `json:"league_attributes_tenths"`
	RatingHundredths       int              `json:"rating_hundredths"`
}

// Project applies completed elapsed weeks of linear decline, then adds the
// accounted professional experience to Skill and computes the D65 rating.
// It does not use the system clock, training, fight form or mutable state.
// Failure always returns the zero Snapshot.
func Project(version uint32, dna fighter.DNA, bornAtMS, asOfMS int64, proXP int) (Snapshot, error) {
	if version != Version1 || dna.DNAVersion != fighter.Version1 || bornAtMS <= 0 || asOfMS < bornAtMS || proXP < 0 || proXP > 15 {
		return Snapshot{}, ErrCondition
	}
	seed, err := fighter.ParseSeed(dna.Seed)
	if err != nil {
		return Snapshot{}, ErrCondition
	}
	want, err := fighter.Generate(fighter.Version1, seed)
	if err != nil || dna != want {
		return Snapshot{}, ErrCondition
	}

	// Both times are now positive and ordered: 0 <= elapsed <= MaxInt64-1.
	// Durations are small, fixed DNA bounds and are never added to birth.
	elapsed := asOfMS - bornAtMS
	age := elapsed / weekMS
	prime := int64(dna.PrimeWeeks)
	decline := int64(dna.DeclineWeeks)
	phase := "prime"
	d := int64(0)
	remainingWeeks := 0
	if age < prime {
		remainingWeeks = int(prime - age) // Bounded 1..52 before conversion.
	} else if age < prime+decline {
		phase = "decline"
		d = age - prime
	} else {
		phase = "retired"
		d = decline
	}
	primeRemaining := remaining(prime*weekMS, elapsed)
	careerRemaining := remaining((prime+decline)*weekMS, elapsed)
	steps := int(d) // Clamped to canonical decline duration, at most 39.
	a := dna.Attributes
	aged := AttributesTenths{
		Power:   ageAttribute(a.Power, 20, steps, dna.DeclineWeeks),
		Speed:   ageAttribute(a.Speed, 40, steps, dna.DeclineWeeks),
		Chin:    ageAttribute(a.Chin, 30, steps, dna.DeclineWeeks),
		Stamina: ageAttribute(a.Stamina, 30, steps, dna.DeclineWeeks),
		Skill:   ageAttribute(a.Skill, 10, steps, dna.DeclineWeeks),
	}
	league := aged
	league.Skill += 10 * proXP
	return Snapshot{
		Version: Version1, ProjectorID: ProjectorID, DNAVersion: dna.DNAVersion, Seed: dna.Seed,
		BornAtMS: bornAtMS, AsOfMS: asOfMS, AgeWeeks: age, Phase: phase,
		ElapsedDeclineWeeks: steps, PrimeRemainingWeeks: remainingWeeks,
		PrimeRemainingMS: primeRemaining, CareerRemainingMS: careerRemaining, ProXP: proXP,
		AgedAttributesTenths: aged, LeagueAttributesTenths: league,
		RatingHundredths: rating(league),
	}, nil
}

func remaining(duration, elapsed int64) int64 {
	if elapsed >= duration {
		return 0
	}
	return duration - elapsed
}

// ageAttribute receives only regenerated A in [1,100], an internal loss in
// {10,20,30,40}, D in [13,39] and d in [0,D]. Positive numerator <=390000;
// signed int64 intermediates are exact, and the result fits a 32-bit int.
func ageAttribute(original, loss, step, duration int) int {
	n := int64(original) * (100*int64(duration) - int64(loss)*int64(step))
	d := 10 * int64(duration)
	q, r := n/d, n%d
	if 2*r > d || (2*r == d && q%2 != 0) {
		q++
	}
	return int(q)
}

func rating(a AttributesTenths) int {
	// At most 50,200 even ignoring the canonical talent budget. No form or
	// training term, second rounding, final attribute floor or Skill cap.
	return 10*a.Power + 25*a.Speed + 2*a.Chin + 4*a.Stamina + 8*a.Skill
}
