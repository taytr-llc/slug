// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"github.com/taytr-llc/slug/gymseed"
	"sort"
)

func ownedGymAccounts(p OwnedPin) map[string]string {
	m := map[string]string{}
	for _, x := range p.SourcePin.SourcePin.Participants {
		m[x.StablePin.Stable.Identity.GymID] = x.StablePin.Stable.AccountID
	}
	return m
}
func DeriveOwnedPlan(p OwnedPin, source gymseed.OwnedCardSource) (Plan, error) {
	if validateOwnedPin(p) != nil {
		return Plan{}, ErrInvalid
	}
	source, e := gymseed.ValidateOwnedCardSource(p.SourcePin, source)
	if e != nil {
		return Plan{}, ErrInvalid
	}
	return deriveOwnedPlanValidated(p, source)
}

// Called only after complete CardSource semantic validation. It derives every
// card field but does not repeat the source validator for the same owned value.
func deriveOwnedPlanValidated(p OwnedPin, source gymseed.OwnedCardSource) (Plan, error) {
	card := ownedCardID(p, source.SourceReceipt.Input.Event)
	input := ownedInputID(card, source.SourceReceipt.SnapshotDigest, source.EventKey, source.Target.Height, source.Target.Hash, p.RulesID)
	v := Plan{Version: 1, CardID: card, InputID: input, Seed: hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(source.Target.Hash)), RecordBasis: RecordBasis, Entrants: []Entrant{}, Bouts: []Bout{}, Gyms: []GymCounts{}}
	seen := map[string]bool{}
	for _, part := range source.SourceReceipt.Input.Participants {
		for _, m := range part.Members {
			if seen[m.Birth.ProspectID] {
				return Plan{}, ErrInvalid
			}
			seen[m.Birth.ProspectID] = true
			// Source validation binds the historical corner independently of this
			// deterministic appearance projection.
			entrant, e := entrantFromBirth(m.Birth, part.Owner.GymID, part.Owner.AccountID, m.Class, uint32(m.ConditionAtCutoff.RatingHundredths), m.Eligibility)
			if e != nil {
				return Plan{}, e
			}
			v.Entrants = append(v.Entrants, entrant)
		}
	}
	sort.Slice(v.Entrants, func(i, j int) bool {
		a, b := v.Entrants[i], v.Entrants[j]
		if a.Class != b.Class {
			return classIndex(a.Class) < classIndex(b.Class)
		}
		if a.GymID != b.GymID {
			return a.GymID < b.GymID
		}
		return a.ProspectID < b.ProspectID
	})
	gyms := make([]string, 0, 2)
	for gym := range ownedGymAccounts(p) {
		gyms = append(gyms, gym)
	}
	v = matchGyms(gyms, v)
	if validateOwnedPlan(p, v) != nil {
		return Plan{}, ErrInvalid
	}
	return v, nil
}
