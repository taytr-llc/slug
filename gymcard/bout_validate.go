// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"math"
)

func validateBoutCorner(p Pin, v BoutCorner) error {
	m, a, o := v.Member, v.Member.Assignment, v.Owner
	if o.OwnershipBasis != "candidate_assignment" || a.Version != 1 || a.Sequence < 1 || a.Sequence > 1000 || m.Slot < 1 || m.Slot > 10 || !timestamp(a.BornAtMS) || !timestamp(a.AcceptedAtMS) || a.BornAtMS > a.AcceptedAtMS || !timestamp(m.SlotAssignedAtMS) || m.SlotAssignedAtMS < a.AcceptedAtMS || m.SlotAssignedAtMS >= m.ConditionAtCutoff.AsOfMS || m.Eligibility != "eligible" {
		return ErrInvalid
	}
	for _, id := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !hashID(id) {
			return ErrInvalid
		}
	}
	found := false
	for _, ep := range p.SourcePin.SourcePin.Participants {
		sp := ep.StablePin
		if sp.AccountID == o.AccountID && sp.AccountAuthorityID == o.AccountAuthorityID && sp.Identity.GymID == o.GymID && a.GymID == o.GymID && a.DescriptorID == sp.Source.DescriptorID {
			found = true
		}
	}
	if !found || !timestamp(m.ConditionAtCutoff.AsOfMS) {
		return ErrInvalid
	}
	man, e := fightermanifest.GenerateText(1, a.WorkHash)
	if e != nil {
		return ErrInvalid
	}
	digest, e := fightermanifest.Digest(man)
	if e != nil || digest != a.ManifestDigest || man.Physical.Class != m.Class {
		return ErrInvalid
	}
	c, e := fightercondition.Project(1, man.DNA, a.BornAtMS, m.ConditionAtCutoff.AsOfMS, 0)
	if e != nil || c.Phase == "retired" || !equal(c, m.ConditionAtCutoff) {
		return ErrInvalid
	}
	return nil
}
func validateBoutSource(p Pin, v BoutSource) error {
	id, _ := ID(p)
	ek, e := gymseed.EventKey(p.SourcePin, v.Event)
	if e != nil || v.Version != 1 || v.StoreID != id || v.CardID != cardID(p, v.Event) || v.SeedEventKey != ek || v.TargetHeight < 1 || v.TargetHeight > math.MaxInt32 || !timestamp(v.FinalizedAtMS) || !timestamp(v.CardCreatedAtMS) || v.Event.CutoffMS > v.FinalizedAtMS || v.FinalizedAtMS > v.CardCreatedAtMS || len(v.Corners) != 2 || validateBout(p, v.Bout) != nil {
		return ErrInvalid
	}
	for _, id := range []string{v.CardRecordDigest, v.CardSourceDigest, v.SnapshotDigest, v.TargetHash} {
		if !hashID(id) {
			return ErrInvalid
		}
	}
	for i, c := range v.Corners {
		if validateBoutCorner(p, c) != nil || c.Member.ConditionAtCutoff.AsOfMS != v.Event.CutoffMS || c.Member.Class != v.Bout.Class || c.Member.Assignment.ProspectID != []string{v.Bout.CornerA, v.Bout.CornerB}[i] {
			return ErrInvalid
		}
	}
	if v.Corners[0].Owner.GymID == v.Corners[1].Owner.GymID {
		return ErrInvalid
	}
	a, b := v.Corners[0].Member.ConditionAtCutoff.RatingHundredths, v.Corners[1].Member.ConditionAtCutoff.RatingHundredths
	if 2*min(a, b) < max(a, b) {
		return ErrInvalid
	}
	input := hash("SLUG candidate card input\x00v1\x00", rawID(v.CardID), rawID(v.SnapshotDigest), rawID(v.SeedEventKey), le32(v.TargetHeight), rawID(v.TargetHash), rawID(p.RulesID))
	seed := hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(v.TargetHash))
	bid := hash("SLUG candidate card bout\x00v1\x00", rawID(input), le32(uint32(classIndex(v.Bout.Class))), rawID(v.Bout.CornerA), rawID(v.Bout.CornerB))
	if v.Bout.BoutID != bid || v.Bout.Seed != hash("SLUG candidate card bout seed\x00v1\x00", rawID(seed), rawID(bid)) {
		return ErrInvalid
	}
	return nil
}
func validateBoutState(p Pin, v BoutState) error {
	if _, e := gymevent.ValidateEvent(p.SourcePin.SourcePin, v.Event); e != nil {
		return ErrInvalid
	}
	if v.Version != 1 || v.CardID != cardID(p, v.Event) || !timestamp(v.ObservedAtMS) || v.ConflictCount > 8 || v.ConflictOverflow && v.ConflictCount != 8 || v.TargetDescendants > math.MaxInt32 {
		return ErrInvalid
	}
	for _, t := range []int64{v.CardCreatedAtMS, v.FinalizedAtMS, v.IncidentAtMS, v.ChainObservedAtMS} {
		if t < 0 || t > v.ObservedAtMS {
			return ErrInvalid
		}
	}
	switch v.NodeState {
	case "unobserved", "online", "offline", "unavailable":
	default:
		return ErrInvalid
	}
	due := v.ObservedAtMS >= v.Event.IncidentDeadlineMS && (v.FinalizedAtMS == 0 || v.ConflictCount > 0 || v.ConflictOverflow)
	if due != v.EscalationDue || v.IncidentAtMS != 0 && v.IncidentAtMS < v.Event.IncidentDeadlineMS || v.ChainObservedAtMS == 0 && (v.TargetDescendants != 0 || v.CurrentFinality) || v.CurrentFinality && (v.FinalizedAtMS == 0 || v.ChainObservedAtMS == 0 || v.TargetDescendants < 60 || v.ConflictCount != 0 || v.ConflictOverflow) {
		return ErrInvalid
	}
	if v.CardExists {
		if !hashID(v.CardRecordDigest) || !hashID(v.CardSourceDigest) || v.FinalizedAtMS == 0 || v.FinalizedAtMS > v.CardCreatedAtMS {
			return ErrInvalid
		}
	} else if v.CardRecordDigest != "" || v.CardSourceDigest != "" || v.CardCreatedAtMS != 0 || v.Usable {
		return ErrInvalid
	}
	if v.Usable && (!v.CardExists || v.FinalizedAtMS == 0 || v.ConflictCount != 0 || v.ConflictOverflow) {
		return ErrInvalid
	}
	return nil
}
func validateBoutRef(p Pin, v BoutRef) error {
	if validateBout(p, v.Bout) != nil || !hashID(v.SourceDigest) {
		return ErrInvalid
	}
	return nil
}
func validateBoutView(p Pin, v BoutView) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.CreatedAtMS) || v.ObservedAtMS < v.CreatedAtMS || v.ObservedAtMS != v.State.ObservedAtMS || validateBoutState(p, v.State) != nil || v.Bouts == nil || len(v.Bouts) > 5 || !v.State.CardExists && len(v.Bouts) != 0 || gymAccounts(p)[v.ParticipantGymID] != v.ParticipantAccountID || v.ParticipantAccountID == "" {
		return ErrInvalid
	}
	seen := map[string]bool{}
	boutIDs := map[string]bool{}
	last := -1
	for _, r := range v.Bouts {
		if validateBoutRef(p, r) != nil || classIndex(r.Bout.Class) <= last {
			return ErrInvalid
		}
		last = classIndex(r.Bout.Class)
		if boutIDs[r.Bout.BoutID] {
			return ErrInvalid
		}
		boutIDs[r.Bout.BoutID] = true
		for _, id := range []string{r.Bout.CornerA, r.Bout.CornerB} {
			if seen[id] {
				return ErrInvalid
			}
			seen[id] = true
		}
	}
	return nil
}
func boutSourceDigest(v BoutSource) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate booked bout source\x00v1\x00", lp(b))
}
func BoutSourceDigest(p Pin, v BoutSource) (string, error) {
	v, e := ValidateBoutSource(p, v)
	if e != nil {
		return "", e
	}
	return boutSourceDigest(v), nil
}
