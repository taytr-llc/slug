// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"crypto/ed25519"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"math"
)

// Disputed signed claims are evidence, never a source for an action. This
// purpose-private validator checks their closed structural relationships but
// deliberately does not regenerate the disputed manifest or condition.
func validateEvidenceSource(p Pin, s gymbout.ResultSource) error {
	c, cp := s.Source, p.SourcePin.SourcePin
	cardStore, _ := gymcard.ID(cp)
	cardID, e := gymcard.CardID(cp, c.Event)
	if e != nil {
		return ErrInvalid
	}
	eventKey, e := gymseed.EventKey(cp.SourcePin, c.Event)
	if e != nil {
		return ErrInvalid
	}
	if s.Version != 1 || s.StoreID != p.SourceStoreID || !timestamp(s.StoreCreatedAtMS) || !timestamp(s.CreatedAtMS) || s.CreatedAtMS < max(s.StoreCreatedAtMS, c.CardCreatedAtMS, c.FinalizedAtMS) || c.Version != 1 || c.StoreID != cardStore || c.CardID != cardID || c.SeedEventKey != eventKey || !timestamp(c.CardCreatedAtMS) || !timestamp(c.FinalizedAtMS) || c.FinalizedAtMS < c.Event.CutoffMS || c.CardCreatedAtMS < c.FinalizedAtMS || c.TargetHeight < 1 || c.TargetHeight > math.MaxInt32 || len(c.Corners) != 2 {
		return ErrInvalid
	}
	for _, id := range []string{s.RecordDigest, s.SourceDigest, s.InputID, s.ResultDigest, c.CardRecordDigest, c.CardSourceDigest, c.SnapshotDigest, c.TargetHash} {
		if !hashID(id) {
			return ErrInvalid
		}
	}
	if _, e = gymcard.ValidateBout(cp, c.Bout); e != nil {
		return ErrInvalid
	}
	for i, corner := range c.Corners {
		if !evidenceCorner(cp, corner, c.Event) || corner.Member.Class != c.Bout.Class || corner.Member.Assignment.ProspectID != []string{c.Bout.CornerA, c.Bout.CornerB}[i] {
			return ErrInvalid
		}
	}
	if c.Corners[0].Owner.GymID == c.Corners[1].Owner.GymID {
		return ErrInvalid
	}
	a, b := c.Corners[0].Member.ConditionAtCutoff.RatingHundredths, c.Corners[1].Member.ConditionAtCutoff.RatingHundredths
	if 2*min(a, b) < max(a, b) {
		return ErrInvalid
	}
	class := -1
	for i, v := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if v == c.Bout.Class {
			class = i
		}
	}
	if class < 0 {
		return ErrInvalid
	}
	input := hash("SLUG candidate card input\x00v1\x00", rawID(c.CardID), rawID(c.SnapshotDigest), rawID(c.SeedEventKey), le32(c.TargetHeight), rawID(c.TargetHash), rawID(cp.RulesID))
	seed := hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(c.TargetHash))
	id := hash("SLUG candidate card bout\x00v1\x00", rawID(input), le32(uint32(class)), rawID(c.Bout.CornerA), rawID(c.Bout.CornerB))
	if c.Bout.BoutID != id || c.Bout.Seed != hash("SLUG candidate card bout seed\x00v1\x00", rawID(seed), rawID(id)) {
		return ErrInvalid
	}
	raw, _ := json.Marshal(c)
	if len(raw) > gymcard.MaxBoutSourceSize || s.SourceDigest != hash("SLUG candidate booked bout source\x00v1\x00", lp(raw)) {
		return ErrInvalid
	}
	_, e = gymbout.ValidateSavedBout(p.SourcePin, gymbout.SavedBout{Bout: c.Bout, SourceDigest: s.SourceDigest, Exists: true, RecordDigest: s.RecordDigest, CreatedAtMS: s.CreatedAtMS, InputID: s.InputID, ResultDigest: s.ResultDigest, Winner: s.Summary.Winner, Method: s.Summary.Method, EndRound: s.Summary.EndRound, EndTick: s.Summary.EndTick})
	if e != nil {
		return ErrInvalid
	}
	return nil
}
func evidenceCorner(p gymcard.Pin, v gymcard.BoutCorner, ev gymevent.Event) bool {
	m, a, o := v.Member, v.Member.Assignment, v.Owner
	if o.OwnershipBasis != "candidate_assignment" || a.Version != 1 || a.Sequence < 1 || a.Sequence > 1000 || m.Slot < 1 || m.Slot > 10 || !timestamp(a.BornAtMS) || !timestamp(a.AcceptedAtMS) || a.BornAtMS > a.AcceptedAtMS || a.AcceptedAtMS > ev.CutoffMS || !timestamp(m.SlotAssignedAtMS) || m.SlotAssignedAtMS < a.AcceptedAtMS || m.SlotAssignedAtMS >= ev.CutoffMS || m.Eligibility != "eligible" {
		return false
	}
	for _, id := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !hashID(id) {
			return false
		}
	}
	found := false
	for _, ep := range p.SourcePin.SourcePin.Participants {
		sp := ep.StablePin
		if sp.AccountID == o.AccountID && sp.AccountAuthorityID == o.AccountAuthorityID && sp.Identity.GymID == o.GymID && a.GymID == o.GymID && a.DescriptorID == sp.Source.DescriptorID {
			found = true
		}
	}
	if !found {
		return false
	}
	c := m.ConditionAtCutoff
	const week int64 = 604800000
	if c.Version != 1 || c.DNAVersion != 1 || c.ProjectorID != fightercondition.ProjectorID || c.Seed != a.WorkHash || c.BornAtMS != a.BornAtMS || c.AsOfMS != ev.CutoffMS || c.ProXP != 0 || c.AgeWeeks < 0 || c.AgeWeeks > 90 || c.AgeWeeks != (ev.CutoffMS-a.BornAtMS)/week || c.ElapsedDeclineWeeks < 0 || c.ElapsedDeclineWeeks > 38 || c.PrimeRemainingWeeks < 0 || c.PrimeRemainingWeeks > 52 || c.PrimeRemainingMS < 0 || c.PrimeRemainingMS > 52*week || c.CareerRemainingMS < 1 || c.CareerRemainingMS > 91*week || c.AgedAttributesTenths != c.LeagueAttributesTenths {
		return false
	}
	x := c.LeagueAttributesTenths
	for _, v := range []int{x.Power, x.Speed, x.Chin, x.Stamina, x.Skill} {
		if v < 1 || v > 1000 {
			return false
		}
	}
	if c.RatingHundredths != 10*x.Power+25*x.Speed+2*x.Chin+4*x.Stamina+8*x.Skill {
		return false
	}
	if c.Phase == "prime" {
		return c.ElapsedDeclineWeeks == 0 && c.PrimeRemainingWeeks > 0 && c.PrimeRemainingMS > 0
	}
	return c.Phase == "decline" && c.PrimeRemainingWeeks == 0 && c.PrimeRemainingMS == 0
}
func validateEvidenceStatement(p Pin, v SignedStatement) error {
	if v.Version != 1 || structuralStatement(v.Statement) != nil || validateEvidenceSource(p, v.Statement.Source) != nil || !equal(v.Statement, expectedStatement(p, v.Statement.Source)) || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), statementMessage(v.Statement), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}
