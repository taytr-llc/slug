// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"github.com/taytr-llc/slug/gymbout"
	"math"
)

func ownedPhase(r OwnedRecord, ss gymbout.OwnedStatus) string {
	if len(r.Conflicts) > 0 || r.ConflictOverflow || r.Equivocation != nil || !ss.Saved.Usable {
		return "conflict"
	}
	if r.Finality != nil {
		return "finalized"
	}
	if r.Inclusion != nil || r.Publication != nil && r.Publication.DispatchCount > 0 {
		return "provisional"
	}
	return "prepared"
}
func ownedStatusFor(p OwnedPin, r *OwnedRecord, ss gymbout.OwnedStatus, funding string) OwnedStatus {
	id, _ := OwnedID(p)
	v := OwnedStatus{Version: 1, StoreID: id, ResultKey: ownedResultKey(p, ss.CardID, ss.Saved.Bout.BoutID), CardID: ss.CardID, BoutID: ss.Saved.Bout.BoutID, Event: ss.Source.Event, Phase: "absent", PublicationState: "none", ScanHeight: -1, ScanTipHeight: -1, ObservedAtMS: ss.ObservedAtMS, NodeState: "unknown", FundingState: funding, Source: ss}
	if r == nil {
		return v
	}
	v.Exists = true
	v.Phase = ownedPhase(*r, ss)
	v.RecordDigest = ownedRecordDigest(*r)
	src := r.Statement.Statement.Source
	v.SourceRecordDigest = src.RecordDigest
	v.InputID = src.InputID
	v.ResultDigest = src.ResultDigest
	v.PreparedAtMS = r.PreparedAtMS
	v.ScanHeight = r.Cursor.Height
	v.ScanGeneration = r.Cursor.Generation
	v.PriorInclusionCount = uint32(len(r.PriorInclusions))
	v.Descendants = r.Observation.Descendants
	v.CheckpointPresent = r.Checkpoint != nil
	v.ConflictCount = uint32(len(r.Conflicts))
	v.ConflictOverflow = r.ConflictOverflow
	v.ChainObservedAtMS = r.Observation.ChainObservedAtMS
	v.NodeState = r.Observation.NodeState
	v.CurrentFinality = r.Observation.CurrentFinality
	if r.Publication != nil {
		v.PublicationState = r.Publication.State
		v.TxID = r.Publication.TxID
	}
	if r.Observation.Tip != nil {
		v.ScanTipHeight = int32(r.Observation.Tip.Height)
	}
	if r.Inclusion != nil {
		v.InclusionHeight = r.Inclusion.Anchor.Height
		v.InclusionHash = r.Inclusion.Anchor.Hash
		v.InclusionGeneration = r.Inclusion.Generation
	}
	if r.Finality != nil {
		v.FinalizedAtMS = r.Finality.ObservedAtMS
	}
	return v
}
func ownedValidateStatus(p OwnedPin, v OwnedStatus) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !ownedTimestamp(v.ObservedAtMS) || v.ResultKey != ownedResultKey(p, v.CardID, v.BoutID) || v.CardID != v.Source.CardID || v.BoutID != v.Source.Saved.Bout.BoutID || !equal(v.Event, v.Source.Source.Event) || v.ObservedAtMS != v.Source.ObservedAtMS {
		return ErrInvalid
	}
	if _, e := gymbout.ValidateOwnedStatus(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	if !oneOf(v.Phase, "absent", "prepared", "provisional", "finalized", "conflict") || !oneOf(v.PublicationState, "none", "built", "dispatch_unknown", "submitted") || !oneOf(v.NodeState, "unknown", "online", "offline") || !oneOf(v.FundingState, "unknown", "scanning", "available", "waiting_for_funds") {
		return ErrInvalid
	}
	for _, x := range []string{v.RecordDigest, v.SourceRecordDigest, v.InputID, v.ResultDigest, v.TxID, v.InclusionHash} {
		if !optionalID(x) {
			return ErrInvalid
		}
	}
	for _, t := range []int64{v.PreparedAtMS, v.FinalizedAtMS, v.ChainObservedAtMS} {
		if t != 0 && (!ownedTimestamp(t) || t > v.ObservedAtMS) {
			return ErrInvalid
		}
	}
	if v.ScanHeight < -1 || v.ScanTipHeight < -1 || v.ScanGeneration > 16 || v.InclusionGeneration > 16 || v.PriorInclusionCount > 16 || v.InclusionHeight > math.MaxInt32 || v.Descendants > math.MaxInt32 || v.ConflictCount > 8 || (v.PublicationState == "none") != (v.TxID == "") || (v.InclusionHeight == 0) != (v.InclusionHash == "") {
		return ErrInvalid
	}
	if !v.Exists {
		want := ownedStatusFor(p, nil, v.Source, v.FundingState)
		want.NodeState = v.NodeState
		if !equal(v, want) {
			return ErrInvalid
		}
	} else {
		if !hashID(v.RecordDigest) || !hashID(v.SourceRecordDigest) || !hashID(v.InputID) || !hashID(v.ResultDigest) || !ownedTimestamp(v.PreparedAtMS) || v.Phase == "absent" || !v.Source.Saved.Exists || v.SourceRecordDigest != v.Source.Saved.RecordDigest || v.InputID != v.Source.Saved.InputID || v.ResultDigest != v.Source.Saved.ResultDigest || v.PreparedAtMS < v.Source.Saved.CreatedAtMS {
			return ErrInvalid
		}
	}
	if v.FinalizedAtMS != 0 && (!v.CheckpointPresent || v.InclusionHeight == 0 || !oneOf(v.Phase, "finalized", "conflict")) || v.Phase == "finalized" && v.FinalizedAtMS == 0 || v.Phase == "prepared" && (!oneOf(v.PublicationState, "none", "built") || v.InclusionHeight != 0 || v.FinalizedAtMS != 0) || v.Phase == "provisional" && (!oneOf(v.PublicationState, "dispatch_unknown", "submitted") && v.InclusionHeight == 0) {
		return ErrInvalid
	}
	if v.CurrentFinality && (v.FinalizedAtMS == 0 || v.Descendants < 60 || v.ChainObservedAtMS == 0 || v.NodeState != "online" || int64(v.ScanTipHeight)-int64(v.InclusionHeight) != int64(v.Descendants)) {
		return ErrInvalid
	}
	if (v.ConflictCount > 0 || v.ConflictOverflow) && v.CurrentFinality {
		return ErrInvalid
	}
	if (v.ConflictCount > 0 || v.ConflictOverflow || v.Exists && !v.Source.Saved.Usable) && v.Phase != "conflict" {
		return ErrInvalid
	}
	return nil
}
func ownedSourceMatches(src gymbout.OwnedResultSource, st gymbout.OwnedStatus) bool {
	b := st.Saved
	s := st.Source
	c := src.Source
	return b.Exists && b.RecordDigest == src.RecordDigest && b.SourceDigest == src.SourceDigest && b.CreatedAtMS == src.CreatedAtMS && b.InputID == src.InputID && b.ResultDigest == src.ResultDigest && equal(b.Bout, c.Bout) && b.Winner == src.Summary.Winner && b.Method == src.Summary.Method && b.EndRound == src.Summary.EndRound && b.EndTick == src.Summary.EndTick && s.CardID == c.CardID && s.CardRecordDigest == c.CardRecordDigest && s.CardSourceDigest == c.CardSourceDigest && s.CardCreatedAtMS == c.CardCreatedAtMS && equal(s.Event, c.Event) && s.FinalizedAtMS == c.FinalizedAtMS
}
func ownedValidateResult(p OwnedPin, version uint32, r OwnedRecord, st OwnedStatus) error {
	if version != 1 || ownedValidateRecord(p, r) != nil || ownedValidateStatus(p, st) != nil || !equal(st, ownedStatusFor(p, &r, st.Source, st.FundingState)) || r.PreparedAtMS > st.ObservedAtMS || r.Observation.ObservedAtMS > st.ObservedAtMS || !ownedSourceMatches(r.Statement.Statement.Source, st.Source) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateActionResult(p OwnedPin, v OwnedActionResult) error {
	return ownedValidateResult(p, v.Version, v.Record, v.Status)
}
func ownedValidateReadResult(p OwnedPin, v OwnedReadResult) error {
	return ownedValidateResult(p, v.Version, v.Record, v.Status)
}
