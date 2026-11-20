// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

func hashID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func optionalID(s string) bool { return s == "" || hashID(s) }
func timestamp(n int64) bool   { return n > 0 && n <= MaxTimestampMS }
func validatePin(p Pin) error {
	want, e := CandidatePin(p.SourcePin)
	if e != nil || !equal(want, p) {
		return ErrInvalid
	}
	return nil
}
func validateInfo(p Pin, v Info) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.SourceCreatedAtMS) || !timestamp(v.CreatedAtMS) || v.CreatedAtMS < v.SourceCreatedAtMS || v.EventCount > MaxEvents {
		return ErrInvalid
	}
	return nil
}
func validateProfessionalRecord(_ Pin, v ProfessionalRecord) error {
	if v != (ProfessionalRecord{}) {
		return ErrInvalid
	}
	return nil
}
func validateReasonCounts(_ Pin, v ReasonCounts) error {
	if v.RetiredAtCutoff > 10 || v.NotRosteredAtCutoff > 10 || v.NoOpponentInClass > 10 || v.OpponentIneligibleAtCutoff > 10 || v.OutsideRatingBand > 10 || v.RetiredAtCutoff+v.NotRosteredAtCutoff+v.NoOpponentInClass+v.OpponentIneligibleAtCutoff+v.OutsideRatingBand > 10 {
		return ErrInvalid
	}
	return nil
}
func validateCounts(p Pin, v Counts) error {
	if validateReasonCounts(p, v.Reasons) != nil || v.TotalCount > 10 || v.EligibleCount > v.TotalCount || v.PairedCount > v.EligibleCount || v.UnmatchedCount > 10 || v.PairedCount+v.UnmatchedCount != v.TotalCount || v.UnmatchedCount != v.Reasons.RetiredAtCutoff+v.Reasons.NotRosteredAtCutoff+v.Reasons.NoOpponentInClass+v.Reasons.OpponentIneligibleAtCutoff+v.Reasons.OutsideRatingBand {
		return ErrInvalid
	}
	return nil
}
func validateGymCounts(p Pin, v GymCounts) error {
	if gymAccounts(p)[v.GymID] == "" || validateCounts(p, v.Counts) != nil || v.Counts.TotalCount > 5 {
		return ErrInvalid
	}
	return nil
}
func validateEntrant(p Pin, v Entrant) error {
	return validateEntrantGyms(gymAccounts(p), v)
}
func validateEntrantGyms(gyms map[string]string, v Entrant) error {
	if !hashID(v.ProspectID) || gyms[v.GymID] != v.AccountID || v.AccountID == "" || len(v.DisplayName) < 1 || len(v.DisplayName) > 128 || !ascii(v.DisplayName) || classIndex(v.Class) < 0 || v.RatingHundredths > 100000 || v.ProfessionalRecord != (ProfessionalRecord{}) {
		return ErrInvalid
	}
	switch v.CutoffEligibility {
	case "eligible":
		if v.RatingHundredths == 0 {
			return ErrInvalid
		}
	case "retired", "not_rostered":
	default:
		return ErrInvalid
	}
	if v.State == "paired" {
		if v.CutoffEligibility != "eligible" || v.Reason != "" || !hashID(v.BoutID) {
			return ErrInvalid
		}
	} else if v.State == "unmatched" {
		if v.BoutID != "" {
			return ErrInvalid
		}
		switch v.Reason {
		case "retired_at_cutoff", "not_rostered_at_cutoff", "no_opponent_in_class", "opponent_ineligible_at_cutoff", "outside_rating_band":
		default:
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	return nil
}
func validateBout(_ Pin, v Bout) error {
	if !hashID(v.BoutID) || !hashID(v.Seed) || !hashID(v.CornerA) || !hashID(v.CornerB) || v.CornerA >= v.CornerB || classIndex(v.Class) < 0 {
		return ErrInvalid
	}
	return nil
}
func validatePlan(p Pin, v Plan) error {
	return validatePlanGyms(gymAccounts(p), v)
}
func validatePlanGyms(gyms map[string]string, v Plan) error {
	if v.Version != 1 || !hashID(v.CardID) || !hashID(v.InputID) || !hashID(v.Seed) || v.RecordBasis != RecordBasis || v.Gyms == nil || len(v.Gyms) != 2 || v.Entrants == nil || len(v.Entrants) > 10 || v.Bouts == nil || len(v.Bouts) > 5 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	pairs := map[string]bool{}
	last := ""
	for _, x := range v.Entrants {
		order := string(rune('0'+classIndex(x.Class))) + x.GymID + x.ProspectID
		if validateEntrantGyms(gyms, x) != nil || seen[x.ProspectID] || pairs[x.GymID+"/"+x.Class] || order <= last {
			return ErrInvalid
		}
		seen[x.ProspectID] = true
		pairs[x.GymID+"/"+x.Class] = true
		last = order
	}
	keys := make([]string, 0, len(gyms))
	for gym := range gyms {
		keys = append(keys, gym)
	}
	if !equal(v, matchGyms(keys, v)) {
		return ErrInvalid
	}
	return nil
}
func validateRecord(p Pin, v Record) error {
	source, e := gymseed.ValidateCardSource(p.SourcePin, v.Source)
	if e != nil {
		return ErrInvalid
	}
	idRaw, _ := json.Marshal(p)
	id := hash("SLUG candidate card store\x00v1\x00", lp(idRaw))
	if v.Version != 1 || v.StoreID != id || v.CardID != cardID(p, source.SourceReceipt.Input.Event) || !timestamp(v.CreatedAtMS) || v.CreatedAtMS < source.Finality.ObservedAtMS {
		return ErrInvalid
	}
	raw, _ := json.Marshal(source)
	digest := hash("SLUG candidate card source\x00v1\x00", lp(raw))
	if digest != v.SourceDigest {
		return ErrInvalid
	}
	plan, e := derivePlanValidated(p, source)
	if e != nil || !equal(plan, v.Plan) {
		return ErrInvalid
	}
	return nil
}
func usable(v gymseed.CardState) bool {
	return v.SourceDigest != "" && v.Status.FinalizedAtMS > 0 && v.Status.ConflictCount == 0 && !v.Status.ConflictOverflow
}
func validateStatus(p Pin, v Status) error {
	id, _ := ID(p)
	card, e := CardID(p, v.Event)
	if e != nil || v.Version != 1 || v.StoreID != id || v.CardID != card || !timestamp(v.ObservedAtMS) || validateCounts(p, v.Counts) != nil || v.BoutCount > 5 || v.Counts.PairedCount != v.BoutCount*2 {
		return ErrInvalid
	}
	if _, e := gymseed.ValidateCardState(p.SourcePin, v.Source); e != nil || !equal(v.Event, v.Source.Status.Event) || v.Source.Status.ObservedAtMS > v.ObservedAtMS {
		return ErrInvalid
	}
	if !v.Exists {
		if v.RecordDigest != "" || v.CreatedAtMS != 0 || v.Counts != (Counts{}) || v.BoutCount != 0 || v.Usable {
			return ErrInvalid
		}
	} else if !hashID(v.RecordDigest) || !timestamp(v.CreatedAtMS) || v.CreatedAtMS > v.ObservedAtMS || v.Source.SourceDigest == "" || v.Source.Status.FinalizedAtMS > v.CreatedAtMS || v.Usable && !usable(v.Source) {
		return ErrInvalid
	}
	return nil
}
func validateResult(p Pin, r Record, s Status) error {
	if validateRecord(p, r) != nil || validateStatus(p, s) != nil || !s.Exists || r.StoreID != s.StoreID || r.CardID != s.CardID || r.CreatedAtMS != s.CreatedAtMS || !equal(r.Plan.Counts, s.Counts) || r.Plan.BoutCount != s.BoutCount || r.SourceDigest != s.Source.SourceDigest || recordDigest(r) != s.RecordDigest || s.Usable != usable(s.Source) {
		return ErrInvalid
	}
	return nil
}
func validateBuildResult(p Pin, v BuildResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return validateResult(p, v.Record, v.Status)
}
func validateReadResult(p Pin, v ReadResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return validateResult(p, v.Record, v.Status)
}
func validateHistoryPage(p Pin, v HistoryPage) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !optionalID(v.AfterCardID) || !optionalID(v.NextCardID) || !timestamp(v.ObservedAtMS) || v.Cards == nil || len(v.Cards) > 8 {
		return ErrInvalid
	}
	var last gymevent.Event
	lastID := ""
	for _, x := range v.Cards {
		if validateStatus(p, x) != nil || !x.Exists || x.ObservedAtMS != v.ObservedAtMS || x.CardID == v.AfterCardID || x.Event.CutoffMS < last.CutoffMS || (x.Event.CutoffMS == last.CutoffMS && x.CardID <= lastID) {
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
