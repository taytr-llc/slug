// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymcard"
)

func ownedValidatePin(p OwnedPin) error {
	v, e := CandidateOwnedPin(p.SourcePin)
	if e != nil || !equal(v, p) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateInfo(p OwnedPin, v OwnedInfo) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !ownedTimestamp(v.SourceCreatedAtMS) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS < v.SourceCreatedAtMS || v.BoutCount > MaxSavedBouts {
		return ErrInvalid
	}
	return nil
}
func DeriveOwnedInput(p OwnedPin, s gymcard.OwnedBoutSource) (fighterbout.Input, error) {
	if ownedValidatePin(p) != nil {
		return fighterbout.Input{}, ErrInvalid
	}
	s, e := gymcard.ValidateOwnedBoutSource(p.SourcePin, s)
	if e != nil {
		return fighterbout.Input{}, ErrInvalid
	}
	var f [2]fighterbout.FighterInput
	for i, c := range s.Corners {
		a := c.Member.Birth
		// ValidateBoutSource has already checked the exact participant descriptor.
		descriptor, e := fighterdescriptor.ResolveCandidate(a.DescriptorID)
		if e != nil {
			return fighterbout.Input{}, ErrInvalid
		}
		man, e := fightermanifest.GenerateText(descriptor.ManifestVersion, a.WorkHash)
		if e != nil {
			return fighterbout.Input{}, ErrInvalid
		}
		f[i] = fighterbout.FighterInput{ProspectID: a.ProspectID, Manifest: man, BornAtMS: a.BornAtMS, ProXP: 0, TrainingSkill: 0}
	}
	in, e := fighterbout.NewInput(s.Event.CutoffMS, s.Bout.Seed, f[0], f[1])
	if e != nil || in.A.ProspectID != s.Bout.CornerA || in.B.ProspectID != s.Bout.CornerB {
		return fighterbout.Input{}, ErrInvalid
	}
	return in, nil
}
func ownedValidateRecord(ctx context.Context, p OwnedPin, r OwnedRecord) error {
	if ctx.Err() != nil {
		return ErrCancelled
	}
	id, _ := OwnedID(p)
	in, e := DeriveOwnedInput(p, r.Source)
	if e != nil || r.Version != 1 || r.StoreID != id || r.CardID != r.Source.CardID || r.BoutID != r.Source.Bout.BoutID || r.SourceDigest != ownedSourceDigest(r.Source) || !equal(in, r.Input) || r.InputID != inputDigest(in) || !ownedTimestamp(r.CreatedAtMS) || r.CreatedAtMS < r.Source.CardCreatedAtMS {
		return ErrInvalid
	}
	if _, e = fighterbout.ValidateResult(ctx, in, r.Result); e != nil {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		return ErrInvalid
	}
	if r.ResultDigest != resultDigest(r.Result) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateSavedBout(p OwnedPin, v OwnedSavedBout) error {
	if _, e := gymcard.ValidateOwnedBout(p.SourcePin, v.Bout); e != nil || !hashID(v.SourceDigest) {
		return ErrInvalid
	}
	if !v.Exists {
		if v.RecordDigest != "" || v.InputID != "" || v.ResultDigest != "" || v.Winner != "" || v.Method != "" || v.CreatedAtMS != 0 || v.EndRound != 0 || v.EndTick != 0 || v.Usable {
			return ErrInvalid
		}
		return nil
	}
	if !hashID(v.RecordDigest) || !hashID(v.InputID) || !hashID(v.ResultDigest) || !ownedTimestamp(v.CreatedAtMS) || v.EndRound < 1 || v.EndRound > 3 || v.EndTick < 1 || v.EndTick > 60 {
		return ErrInvalid
	}
	switch v.Method {
	case "decision_draw", "double_corner_stoppage":
		if v.Winner != "" {
			return ErrInvalid
		}
	case "decision", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner":
		if v.Winner != v.Bout.CornerA && v.Winner != v.Bout.CornerB {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func ownedValidateStatus(p OwnedPin, v OwnedStatus) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.CardID != v.Source.CardID || v.ObservedAtMS != v.Source.ObservedAtMS || !v.Source.CardExists || ownedValidateSavedBout(p, v.Saved) != nil {
		return ErrInvalid
	}
	if _, e := gymcard.ValidateOwnedBoutState(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	if v.Saved.Exists && (v.Saved.CreatedAtMS < v.Source.CardCreatedAtMS || v.Saved.CreatedAtMS > v.ObservedAtMS) || v.Saved.Usable && !v.Source.Usable {
		return ErrInvalid
	}
	return nil
}
func ownedValidateCardStatus(p OwnedPin, v OwnedCardStatus) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.CardID != v.Source.CardID || v.ObservedAtMS != v.Source.ObservedAtMS || v.Bouts == nil || len(v.Bouts) > 5 || !v.Source.CardExists && len(v.Bouts) != 0 {
		return ErrInvalid
	}
	if _, e := gymcard.ValidateOwnedBoutState(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	bids, ids := map[string]bool{}, map[string]bool{}
	last := -1
	for _, b := range v.Bouts {
		if ownedValidateSavedBout(p, b) != nil || classIndex(b.Bout.Class) <= last || bids[b.Bout.BoutID] {
			return ErrInvalid
		}
		last = classIndex(b.Bout.Class)
		bids[b.Bout.BoutID] = true
		for _, id := range []string{b.Bout.CornerA, b.Bout.CornerB} {
			if ids[id] {
				return ErrInvalid
			}
			ids[id] = true
		}
		if b.Exists && (b.CreatedAtMS < v.Source.CardCreatedAtMS || b.CreatedAtMS > v.ObservedAtMS) || b.Usable && !v.Source.Usable {
			return ErrInvalid
		}
	}
	return nil
}
func ownedSavedFor(ref gymcard.BoutRef, r *OwnedRecord, healthy bool) OwnedSavedBout {
	v := OwnedSavedBout{Bout: ref.Bout, SourceDigest: ref.SourceDigest}
	if r != nil {
		v.Exists = true
		v.RecordDigest = ownedRecordDigest(*r)
		v.CreatedAtMS = r.CreatedAtMS
		v.InputID = r.InputID
		v.ResultDigest = r.ResultDigest
		v.Winner = r.Result.Winner
		v.Method = r.Result.Method
		v.EndRound = r.Result.EndRound
		v.EndTick = r.Result.EndTick
		v.Usable = healthy
	}
	return v
}
func ownedValidateResult(ctx context.Context, p OwnedPin, r OwnedRecord, v OwnedStatus) error {
	if e := ownedValidateRecord(ctx, p, r); e != nil {
		return e
	}
	if ownedValidateStatus(p, v) != nil || r.CardID != v.CardID || !equal(v.Saved, ownedSavedFor(gymcard.BoutRef{Bout: r.Source.Bout, SourceDigest: r.SourceDigest}, &r, v.Source.Usable)) || !ownedSameState(r.Source, v.Source) {
		return ErrInvalid
	}
	return nil
}
func ownedSameState(s gymcard.OwnedBoutSource, v gymcard.OwnedBoutState) bool {
	return v.CardExists && s.CardID == v.CardID && s.CardRecordDigest == v.CardRecordDigest && s.CardSourceDigest == v.CardSourceDigest && s.CardCreatedAtMS == v.CardCreatedAtMS && s.FinalizedAtMS == v.FinalizedAtMS && equal(s.Event, v.Event)
}
func ownedValidateRunResult(ctx context.Context, p OwnedPin, v OwnedRunResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return ownedValidateResult(ctx, p, v.Record, v.Status)
}
func ownedValidateReadResult(ctx context.Context, p OwnedPin, v OwnedReadResult) error {
	if v.Version != 1 {
		return ErrInvalid
	}
	return ownedValidateResult(ctx, p, v.Record, v.Status)
}
