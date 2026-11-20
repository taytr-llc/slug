// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"crypto/ed25519"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"math"
)

// Disputed signed claims are evidence, never a source for an action. This
// purpose-private validator checks their closed structural relationships but
// deliberately does not regenerate the disputed manifest or condition.
func ownedValidateEvidenceSource(p OwnedPin, s gymbout.OwnedResultSource) error {
	c, cp := s.Source, p.SourcePin.SourcePin
	cardStore, _ := gymcard.OwnedID(cp)
	cardID, e := gymcard.OwnedCardID(cp, c.Event)
	if e != nil {
		return ErrInvalid
	}
	eventKey, e := gymseed.OwnedEventKey(cp.SourcePin, c.Event)
	if e != nil {
		return ErrInvalid
	}
	if s.Version != 1 || s.StoreID != p.SourceStoreID || !ownedTimestamp(s.StoreCreatedAtMS) || !ownedTimestamp(s.CreatedAtMS) || s.CreatedAtMS < max(s.StoreCreatedAtMS, c.CardCreatedAtMS, c.FinalizedAtMS) || c.Version != 1 || c.StoreID != cardStore || c.CardID != cardID || c.SeedEventKey != eventKey || !ownedTimestamp(c.CardCreatedAtMS) || !ownedTimestamp(c.FinalizedAtMS) || c.FinalizedAtMS < c.Event.CutoffMS || c.CardCreatedAtMS < c.FinalizedAtMS || c.TargetHeight < 1 || c.TargetHeight > math.MaxInt32 || len(c.Corners) != 2 {
		return ErrInvalid
	}
	for _, id := range []string{s.RecordDigest, s.SourceDigest, s.InputID, s.ResultDigest, c.CardRecordDigest, c.CardSourceDigest, c.SnapshotDigest, c.TargetHash} {
		if !hashID(id) {
			return ErrInvalid
		}
	}
	if _, e = gymcard.ValidateOwnedBout(cp, c.Bout); e != nil {
		return ErrInvalid
	}
	for i, corner := range c.Corners {
		if !ownedEvidenceCorner(cp, corner, c.Event) || corner.Member.Class != c.Bout.Class || corner.Member.Birth.ProspectID != []string{c.Bout.CornerA, c.Bout.CornerB}[i] {
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
	input, _ := ownedcodec.Hash("card input", struct {
		Version        uint32 `json:"version"`
		CardID         string `json:"card_id"`
		SnapshotDigest string `json:"snapshot_digest"`
		SeedEventKey   string `json:"seed_event_key"`
		TargetHeight   uint32 `json:"target_height"`
		TargetHash     string `json:"target_hash"`
		RulesID        string `json:"rules_id"`
	}{1, c.CardID, c.SnapshotDigest, c.SeedEventKey, c.TargetHeight, c.TargetHash, cp.RulesID})
	seed := hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(c.TargetHash))
	id := hash("SLUG candidate card bout\x00v1\x00", rawID(input), le32(uint32(class)), rawID(c.Bout.CornerA), rawID(c.Bout.CornerB))
	if c.Bout.BoutID != id || c.Bout.Seed != hash("SLUG candidate card bout seed\x00v1\x00", rawID(seed), rawID(id)) {
		return ErrInvalid
	}
	raw, _ := json.Marshal(c)
	if len(raw) > gymcard.OwnedMaxBoutSourceSize || s.SourceDigest != hash("SLUG owned card bout source\x00v1\x00", lp(raw)) {
		return ErrInvalid
	}
	_, e = gymbout.ValidateOwnedSavedBout(p.SourcePin, gymbout.OwnedSavedBout{Bout: c.Bout, SourceDigest: s.SourceDigest, Exists: true, RecordDigest: s.RecordDigest, CreatedAtMS: s.CreatedAtMS, InputID: s.InputID, ResultDigest: s.ResultDigest, Winner: s.Summary.Winner, Method: s.Summary.Method, EndRound: s.Summary.EndRound, EndTick: s.Summary.EndTick})
	if e != nil {
		return ErrInvalid
	}
	return nil
}
func ownedEvidenceCorner(p gymcard.OwnedPin, v gymcard.OwnedBoutCorner, ev gymevent.Event) bool {
	m, a, o := v.Member, v.Member.Birth, v.Owner
	if o.OwnershipBasis != "finalized_public_chain" || a.Version != 1 || a.Sequence < 1 || a.Sequence > 1000 || m.Slot < 1 || m.Slot > 10 || !ownedTimestamp(a.BornAtMS) || !ownedTimestamp(a.AcceptedAtMS) || a.BornAtMS > a.AcceptedAtMS || a.AcceptedAtMS > ev.CutoffMS || !ownedTimestamp(m.SlotAssignedAtMS) || m.SlotAssignedAtMS < a.AcceptedAtMS || m.SlotAssignedAtMS >= ev.CutoffMS || m.Eligibility != "eligible" {
		return false
	}
	for _, id := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !hashID(id) {
			return false
		}
	}
	found := false
	for _, ep := range p.SourcePin.SourcePin.Participants {
		sp := ep.StablePin.Stable
		if sp.AccountID == o.AccountID && sp.AccountAuthorityID == o.AccountAuthorityID && sp.Identity.GymID == o.GymID && a.DescriptorID == sp.Source.DescriptorID {
			if fighterownership.ValidateEffectiveState(p.SourcePin.SourcePin.Ownership, m.Ownership) != nil || m.Ownership.ProspectID != a.ProspectID || m.Ownership.OwnerPublicKey != sp.Identity.GymPublicKey {
				return false
			}
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
func ownedValidateEvidenceStatement(p OwnedPin, v OwnedSignedStatement) error {
	if v.Version != 1 || ownedStructuralStatement(v.Statement) != nil || ownedValidateEvidenceSource(p, v.Statement.Source) != nil || !equal(v.Statement, ownedExpectedStatement(p, v.Statement.Source)) || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), ownedStatementMessage(v.Statement), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}
