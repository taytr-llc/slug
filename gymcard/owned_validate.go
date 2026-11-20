// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func validateOwnedPin(p OwnedPin) error {
	want, e := CandidateOwnedPin(p.SourcePin)
	if e != nil || !equal(want, p) {
		return ErrInvalid
	}
	return nil
}
func validateOwnedInfo(p OwnedPin, v OwnedInfo) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !ownedTimestamp(v.SourceCreatedAtMS) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS < v.SourceCreatedAtMS || v.EventCount > MaxEvents {
		return ErrInvalid
	}
	return nil
}
func validateOwnedGymCounts(p OwnedPin, v GymCounts) error {
	if ownedGymAccounts(p)[v.GymID] == "" || validateCounts(Pin{}, v.Counts) != nil || v.Counts.TotalCount > 5 {
		return ErrInvalid
	}
	return nil
}
func validateOwnedEntrant(p OwnedPin, v Entrant) error {
	return validateEntrantGyms(ownedGymAccounts(p), v)
}
func validateOwnedPlan(p OwnedPin, v Plan) error {
	return validatePlanGyms(ownedGymAccounts(p), v)
}
func validateOwnedRecord(p OwnedPin, v OwnedRecord) error {
	source, e := gymseed.ValidateOwnedCardSource(p.SourcePin, v.Source)
	if e != nil {
		return ErrInvalid
	}
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.CardID != ownedCardID(p, source.SourceReceipt.Input.Event) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS < source.Finality.ObservedAtMS {
		return ErrInvalid
	}
	digest, _ := ownedcodec.Hash("seed card source", source)
	if digest != v.SourceDigest {
		return ErrInvalid
	}
	plan, e := deriveOwnedPlanValidated(p, source)
	if e != nil || !equal(plan, v.Plan) {
		return ErrInvalid
	}
	return nil
}
func ownedUsable(v gymseed.OwnedCardState) bool {
	return v.SourceDigest != "" && v.Status.FinalizedAtMS > 0 && v.Status.ConflictCount == 0 && !v.Status.ConflictOverflow
}
func validateOwnedStatus(p OwnedPin, v OwnedStatus) error {
	id, _ := OwnedID(p)
	card, e := OwnedCardID(p, v.Event)
	if e != nil || v.Version != 1 || v.StoreID != id || v.CardID != card || !ownedTimestamp(v.ObservedAtMS) || validateCounts(Pin{}, v.Counts) != nil || v.BoutCount > 5 || v.Counts.PairedCount != v.BoutCount*2 {
		return ErrInvalid
	}
	if _, e := gymseed.ValidateOwnedCardState(p.SourcePin, v.Source); e != nil || !equal(v.Event, v.Source.Status.Event) || v.Source.Status.ObservedAtMS > v.ObservedAtMS {
		return ErrInvalid
	}
	if !v.Exists {
		if v.RecordDigest != "" || v.CreatedAtMS != 0 || v.Counts != (Counts{}) || v.BoutCount != 0 || v.Usable {
			return ErrInvalid
		}
	} else if !hashID(v.RecordDigest) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS > v.ObservedAtMS || v.Source.SourceDigest == "" || v.Source.Status.FinalizedAtMS > v.CreatedAtMS || v.Usable && !ownedUsable(v.Source) {
		return ErrInvalid
	}
	return nil
}
func validateOwnedResult(p OwnedPin, r OwnedRecord, s OwnedStatus) error {
	if validateOwnedRecord(p, r) != nil || validateOwnedStatus(p, s) != nil || !s.Exists || r.StoreID != s.StoreID || r.CardID != s.CardID || r.CreatedAtMS != s.CreatedAtMS || !equal(r.Plan.Counts, s.Counts) || r.Plan.BoutCount != s.BoutCount || r.SourceDigest != s.Source.SourceDigest || ownedRecordDigest(r) != s.RecordDigest || s.Usable != ownedUsable(s.Source) {
		return ErrInvalid
	}
	return nil
}
func validateOwnedBuildResult(p OwnedPin, v OwnedBuildResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return validateOwnedResult(p, v.Record, v.Status)
}
func validateOwnedReadResult(p OwnedPin, v OwnedReadResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return validateOwnedResult(p, v.Record, v.Status)
}
func validateOwnedHistoryPage(p OwnedPin, v OwnedHistoryPage) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !optionalID(v.AfterCardID) || !optionalID(v.NextCardID) || !ownedTimestamp(v.ObservedAtMS) || v.Cards == nil || len(v.Cards) > 8 {
		return ErrInvalid
	}
	var last gymevent.Event
	lastID := ""
	for _, x := range v.Cards {
		if validateOwnedStatus(p, x) != nil || !x.Exists || x.ObservedAtMS != v.ObservedAtMS || x.CardID == v.AfterCardID || x.Event.CutoffMS < last.CutoffMS || (x.Event.CutoffMS == last.CutoffMS && x.CardID <= lastID) {
			return ErrInvalid
		}
		last = x.Event
		lastID = x.CardID
	}
	if v.HasMore {
		if len(v.Cards) == 0 || v.NextCardID != lastID {
			return ErrInvalid
		}
	} else if v.NextCardID != "" {
		return ErrInvalid
	}
	return nil
}
