// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func OwnedCareerHistoryID(c OwnedCareerHistoryCapture) string {
	body := struct {
		Version        uint32                          `json:"version"`
		Status         OwnedStatus                     `json:"status"`
		Activation     careerstate.Activation          `json:"activation"`
		CareerDigest   string                          `json:"career_digest"`
		AsOfMS         int64                           `json:"as_of_ms"`
		Birth          Assignment                      `json:"birth"`
		Ownership      fighterownership.EffectiveState `json:"ownership"`
		PrefixCount    uint32                          `json:"prefix_count"`
		EventCount     uint32                          `json:"event_count"`
		Totals         fightercareer.Totals            `json:"totals"`
		CurrentGymWins uint32                          `json:"current_gym_wins"`
	}{c.Version, c.Status, c.Activation, c.CareerDigest, c.AsOfMS, c.Birth, c.Ownership, c.PrefixCount, c.EventCount, c.Totals, c.CurrentGymWins}
	id, _ := ownedcodec.Hash("fighter history", body)
	return id
}
func ownedBirthShape(b Assignment, p OwnedPin) bool {
	return b.Version == 1 && b.Sequence > 0 && b.Sequence <= 1000 && hashID(b.ProspectID) && hashID(b.GymID) && hashID(b.WorkHash) && ownedTime(b.BornAtMS) && b.AcceptedAtMS >= b.BornAtMS && ownedTime(b.AcceptedAtMS) && b.DescriptorID == p.Stable.Source.DescriptorID && hashID(b.ManifestDigest) && hashID(b.PortraitDigest) && hashID(b.SourceRecordDigest)
}
func ValidateOwnedCareerHistoryCapture(c OwnedCareerHistoryCapture) (OwnedCareerHistoryCapture, error) {
	if _, e := ValidateOwnedStatus(c.Status); e != nil {
		return OwnedCareerHistoryCapture{}, e
	}
	if c.Version != 1 || careerstate.ValidateActivation(c.Activation) != nil || !hashID(c.CareerDigest) || !ownedTime(c.AsOfMS) || c.AsOfMS < c.Activation.ActivatedAtMS || !ownedBirthShape(c.Birth, c.Status.Pin) || c.AsOfMS < c.Birth.AcceptedAtMS || fighterownership.ValidateEffectiveState(c.Status.Pin.Ownership, c.Ownership) != nil || c.Ownership.ProspectID != c.Birth.ProspectID || c.Ownership.OwnerPublicKey != c.Status.Pin.Stable.Identity.GymPublicKey || c.PrefixCount > 256 || c.EventCount > c.PrefixCount || !historyTotalsValid(c.Totals, c.EventCount) || c.CurrentGymWins > c.Totals.Wins || c.HistoryID != OwnedCareerHistoryID(c) || !ownedcodec.Fits(c, OwnedMaxHistoryCaptureSize) {
		return OwnedCareerHistoryCapture{}, ErrInvalid
	}
	c.Ownership.Evidence = append(json.RawMessage(nil), c.Ownership.Evidence...)
	return c, nil
}
func ValidateOwnedCareerHistoryPage(p OwnedCareerHistoryPage) (OwnedCareerHistoryPage, error) {
	c, e := ValidateOwnedCareerHistoryCapture(p.Capture)
	if e != nil || p.Version != 1 || p.After > c.EventCount || p.Items == nil || len(p.Items) > 8 {
		return OwnedCareerHistoryPage{}, ErrInvalid
	}
	n := uint32(len(p.Items))
	if (n == 0) != (p.After == c.EventCount) || n > c.EventCount-p.After || p.Next != p.After+n || p.HasMore != (p.Next < c.EventCount) {
		return OwnedCareerHistoryPage{}, ErrInvalid
	}
	awards, results := map[string]bool{}, map[string]bool{}
	for i, r := range p.Items {
		if r.Ordinal != c.EventCount-p.After-uint32(i) || !historyRowPhysicalValid(r, c.Birth.ProspectID, c.Activation, c.AsOfMS, c.EventCount, c.Totals) || awards[r.Event.AwardID] || results[r.Event.ResultKey] {
			return OwnedCareerHistoryPage{}, ErrInvalid
		}
		awards[r.Event.AwardID] = true
		results[r.Event.ResultKey] = true
		if i > 0 {
			newer := p.Items[i-1]
			if r.Event.EffectiveAtMS > newer.Event.EffectiveAtMS || r.Event.EffectiveAtMS == newer.Event.EffectiveAtMS && r.Event.AwardID >= newer.Event.AwardID {
				return OwnedCareerHistoryPage{}, ErrInvalid
			}
			t, e := fightercareer.Apply(r.TotalsAfter, newer.Earned)
			if e != nil || t != newer.TotalsAfter {
				return OwnedCareerHistoryPage{}, ErrInvalid
			}
		}
	}
	if !ownedcodec.Fits(p, OwnedMaxHistoryPageSize) {
		return OwnedCareerHistoryPage{}, ErrInvalid
	}
	p.Capture = c
	p.Items = append([]CareerHistoryRow{}, p.Items...)
	return p, nil
}
func MarshalOwnedCareerHistoryCapture(c OwnedCareerHistoryCapture) ([]byte, error) {
	c, e := ValidateOwnedCareerHistoryCapture(c)
	if e != nil {
		return nil, e
	}
	return json.Marshal(c)
}
func ParseOwnedCareerHistoryCapture(raw []byte) (OwnedCareerHistoryCapture, error) {
	var c OwnedCareerHistoryCapture
	if ownedcodec.Decode(raw, OwnedMaxHistoryCaptureSize, &c) != nil {
		return c, ErrInvalid
	}
	return ValidateOwnedCareerHistoryCapture(c)
}
func MarshalOwnedCareerHistoryPage(p OwnedCareerHistoryPage) ([]byte, error) {
	p, e := ValidateOwnedCareerHistoryPage(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func ParseOwnedCareerHistoryPage(raw []byte) (OwnedCareerHistoryPage, error) {
	var p OwnedCareerHistoryPage
	if ownedcodec.Decode(raw, OwnedMaxHistoryPageSize, &p) != nil {
		return p, ErrInvalid
	}
	return ValidateOwnedCareerHistoryPage(p)
}
