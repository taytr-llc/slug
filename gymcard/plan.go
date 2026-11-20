// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"sort"
)

var classes = []string{"light", "welter", "middle", "light_heavy", "heavy"}

func classIndex(c string) int {
	for i, x := range classes {
		if c == x {
			return i
		}
	}
	return -1
}
func gymAccounts(p Pin) map[string]string {
	m := map[string]string{}
	for _, x := range p.SourcePin.SourcePin.Participants {
		m[x.StablePin.Identity.GymID] = x.StablePin.AccountID
	}
	return m
}
func DerivePlan(p Pin, source gymseed.CardSource) (Plan, error) {
	if validatePin(p) != nil {
		return Plan{}, ErrInvalid
	}
	source, e := gymseed.ValidateCardSource(p.SourcePin, source)
	if e != nil {
		return Plan{}, ErrInvalid
	}
	return derivePlanValidated(p, source)
}

// Called only after complete CardSource semantic validation. It derives every
// card field but does not repeat the source validator for the same owned value.
func derivePlanValidated(p Pin, source gymseed.CardSource) (Plan, error) {
	card := cardID(p, source.SourceReceipt.Input.Event)
	input := hash("SLUG candidate card input\x00v1\x00", rawID(card), rawID(source.SourceReceipt.SnapshotDigest), rawID(source.EventKey), le32(source.Target.Height), rawID(source.Target.Hash), rawID(p.RulesID))
	v := Plan{Version: 1, CardID: card, InputID: input, Seed: hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(source.Target.Hash)), RecordBasis: RecordBasis, Entrants: []Entrant{}, Bouts: []Bout{}, Gyms: []GymCounts{}}
	seen := map[string]bool{}
	for _, part := range source.SourceReceipt.Input.Participants {
		for _, m := range part.Members {
			if seen[m.Assignment.ProspectID] {
				return Plan{}, ErrInvalid
			}
			seen[m.Assignment.ProspectID] = true
			// Source validation binds the historical corner independently of this
			// deterministic appearance projection.
			entrant, e := entrantFromBirth(m.Assignment, part.Owner.GymID, part.Owner.AccountID, m.Class, uint32(m.ConditionAtCutoff.RatingHundredths), m.Eligibility)
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
	v = match(p, v)
	if validatePlan(p, v) != nil {
		return Plan{}, ErrInvalid
	}
	return v, nil
}

func entrantFromBirth(birth gymstable.Assignment, gym, account, class string, rating uint32, eligibility string) (Entrant, error) {
	descriptor, e := fighterdescriptor.ResolveCandidate(birth.DescriptorID)
	if e != nil {
		return Entrant{}, ErrInvalid
	}
	manifest, e := fightermanifest.GenerateText(descriptor.ManifestVersion, birth.WorkHash)
	if e != nil {
		return Entrant{}, ErrInvalid
	}
	digest, e := fightermanifest.Digest(manifest)
	if e != nil || digest != birth.ManifestDigest {
		return Entrant{}, ErrInvalid
	}
	return Entrant{ProspectID: birth.ProspectID, GymID: gym, AccountID: account, DisplayName: manifest.Name.Display, Class: class, RatingHundredths: rating, CutoffEligibility: eligibility, State: "unmatched"}, nil
}
func addCount(c *Counts, x Entrant) {
	c.TotalCount++
	if x.CutoffEligibility == "eligible" {
		c.EligibleCount++
	}
	if x.State == "paired" {
		c.PairedCount++
		return
	}
	c.UnmatchedCount++
	switch x.Reason {
	case "retired_at_cutoff":
		c.Reasons.RetiredAtCutoff++
	case "not_rostered_at_cutoff":
		c.Reasons.NotRosteredAtCutoff++
	case "no_opponent_in_class":
		c.Reasons.NoOpponentInClass++
	case "opponent_ineligible_at_cutoff":
		c.Reasons.OpponentIneligibleAtCutoff++
	case "outside_rating_band":
		c.Reasons.OutsideRatingBand++
	}
}
func match(p Pin, v Plan) Plan {
	gyms := make([]string, 0, len(p.SourcePin.SourcePin.Participants))
	for _, part := range p.SourcePin.SourcePin.Participants {
		gyms = append(gyms, part.StablePin.Identity.GymID)
	}
	return matchGyms(gyms, v)
}

// matchGyms is the single physical C1 matching engine. The caller supplies
// already authenticated corner identities; this function grants no ownership.
func matchGyms(gyms []string, v Plan) Plan {
	v.Entrants = append([]Entrant{}, v.Entrants...)
	v.Bouts = []Bout{}
	v.Gyms = []GymCounts{}
	v.Counts = Counts{}
	v.BoutCount = 0
	for _, gym := range gyms {
		v.Gyms = append(v.Gyms, GymCounts{GymID: gym})
	}
	sort.Slice(v.Gyms, func(i, j int) bool { return v.Gyms[i].GymID < v.Gyms[j].GymID })
	for i := range v.Entrants {
		a := &v.Entrants[i]
		a.State = "unmatched"
		a.Reason = ""
		a.BoutID = ""
		var b *Entrant
		for j := range v.Entrants {
			if a.GymID != v.Entrants[j].GymID && a.Class == v.Entrants[j].Class {
				b = &v.Entrants[j]
			}
		}
		switch {
		case a.CutoffEligibility == "retired":
			a.Reason = "retired_at_cutoff"
		case a.CutoffEligibility == "not_rostered":
			a.Reason = "not_rostered_at_cutoff"
		case b == nil:
			a.Reason = "no_opponent_in_class"
		case b.CutoffEligibility != "eligible":
			a.Reason = "opponent_ineligible_at_cutoff"
		case 2*uint64(min(a.RatingHundredths, b.RatingHundredths)) < uint64(max(a.RatingHundredths, b.RatingHundredths)):
			a.Reason = "outside_rating_band"
		default:
			a.State = "paired"
			ca, cb := a.ProspectID, b.ProspectID
			if ca > cb {
				ca, cb = cb, ca
			}
			id := hash("SLUG candidate card bout\x00v1\x00", rawID(v.InputID), le32(uint32(classIndex(a.Class))), rawID(ca), rawID(cb))
			a.BoutID = id
			if a.GymID < b.GymID {
				v.Bouts = append(v.Bouts, Bout{id, a.Class, ca, cb, hash("SLUG candidate card bout seed\x00v1\x00", rawID(v.Seed), rawID(id))})
			}
		}
		addCount(&v.Counts, *a)
		for j := range v.Gyms {
			if v.Gyms[j].GymID == a.GymID {
				addCount(&v.Gyms[j].Counts, *a)
			}
		}
	}
	v.BoutCount = uint32(len(v.Bouts))
	return v
}
