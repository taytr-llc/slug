// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func ownedValidateInfo(p OwnedPin, v OwnedInfo) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !ownedTimestamp(v.SourceCreatedAtMS) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS < v.SourceCreatedAtMS || v.ResultCount > MaxResults || v.FundingGeneration > 16 || v.FundingRestartCount != v.FundingGeneration || v.FundingHeight < -1 || !optionalID(v.FundingHash) || (v.FundingHeight < 0) != (v.FundingHash == "") || v.FundingHeight < 0 && v.FundingComplete {
		return ErrInvalid
	}
	return nil
}
func ownedStructuralStatement(v OwnedStatement) error {
	if v.Version != 1 || !hashID(v.ResultKey) || !hashID(v.StoreID) || !hashID(v.SourceStoreID) || !hashID(v.PublisherChainID) || !hashID(v.Genesis) || !hashID(v.RulesID) || !hexSize(v.PublisherPublicKey, 32) || !hexSize(v.CheckpointPublicKey, 32) || !fits(v, OwnedMaxStatementSize) {
		return ErrInvalid
	}
	if v.Source.Version != 1 || v.Source.StoreID != v.SourceStoreID || !hashID(v.Source.RecordDigest) || !hashID(v.Source.SourceDigest) || !hashID(v.Source.InputID) || !hashID(v.Source.ResultDigest) || !ownedTimestamp(v.Source.StoreCreatedAtMS) || !ownedTimestamp(v.Source.CreatedAtMS) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateStatement(p OwnedPin, v OwnedStatement) error {
	if ownedStructuralStatement(v) != nil {
		return ErrInvalid
	}
	if _, e := gymbout.ValidateOwnedResultSource(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	if !equal(v, ownedExpectedStatement(p, v.Source)) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateSignedStatement(p OwnedPin, v OwnedSignedStatement) error {
	if v.Version != 1 || ownedValidateStatement(p, v.Statement) != nil || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), ownedStatementMessage(v.Statement), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}
func ownedValidateConflict(p OwnedPin, v OwnedConflict) error {
	if !ownedTimestamp(v.ObservedAtMS) || v.ExpectedHeight > math.MaxInt32 || !optionalID(v.ExpectedHash) || !optionalID(v.ObservedHash) {
		return ErrInvalid
	}
	if v.ObservedTip != nil && ownedValidateTip(p, *v.ObservedTip) != nil || v.Checkpoint != nil && !ownedValidCheckpoint(p, *v.Checkpoint) {
		return ErrInvalid
	}
	switch v.Kind {
	case "checkpoint_equivocation":
		if v.Checkpoint == nil || uint32(v.Checkpoint.Height) != v.ExpectedHeight || v.Checkpoint.Hash.String() != v.ObservedHash || v.ObservedHash == v.ExpectedHash {
			return ErrInvalid
		}
	case "source_chain_conflict", "checkpoint_chain_conflict":
		if v.ObservedTip == nil || !hashID(v.ExpectedHash) || v.Checkpoint != nil || !replacement(v.ExpectedHeight, v.ExpectedHash, *v.ObservedTip, v.ObservedHash) {
			return ErrInvalid
		}
	case "statement_equivocation":
		if v.Checkpoint != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func ownedConflictKey(c OwnedConflict) string {
	c.ObservedAtMS = 0
	b, _ := json.Marshal(c)
	return string(b)
}
func ownedValidateRecord(p OwnedPin, v OwnedRecord) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !hashID(v.ResultKey) || ownedValidateSignedStatement(p, v.Statement) != nil || v.ResultKey != v.Statement.Statement.ResultKey || !ownedTimestamp(v.PreparedAtMS) {
		return ErrInvalid
	}
	src := v.Statement.Statement.Source
	if v.PreparedAtMS < src.CreatedAtMS || v.PreparedAtMS < src.StoreCreatedAtMS {
		return ErrInvalid
	}
	times := []int64{v.PreparedAtMS}
	payload, e := OwnedCarrier(v.Statement.Statement)
	if e != nil {
		return e
	}
	if v.Publication != nil {
		pub := v.Publication
		if ownedValidatePublication(p, *pub) != nil || pub.BuiltAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		var tx wire.MsgTx
		if tx.Deserialize(bytes.NewReader(rawID(pub.RawTx))) != nil || !bytes.Equal(tx.TxOut[0].PkScript[2:], payload[:]) {
			return ErrInvalid
		}
		times = append(times, pub.BuiltAtMS, pub.LastDispatchAtMS)
	}
	if ownedValidateCursor(p, v.Cursor) != nil || v.ScanRestarts == nil || len(v.ScanRestarts) > 16 || v.Cursor.Generation != uint32(len(v.ScanRestarts)) {
		return ErrInvalid
	}
	for i, x := range v.ScanRestarts {
		if ownedValidateScanRestart(p, x) != nil || x.Generation != uint32(i+1) {
			return ErrInvalid
		}
		times = append(times, x.ObservedAtMS)
	}
	if v.Publication == nil && (!equal(v.Cursor, ownedInitialCursor(p, v.ResultKey, 0)) || len(v.ScanRestarts) != 0 || len(v.PriorInclusions) != 0) {
		return ErrInvalid
	}
	if v.Cursor.Height < 0 && !equal(v.Cursor, ownedInitialCursor(p, v.ResultKey, v.Cursor.Generation)) {
		return ErrInvalid
	}
	if v.Inclusion != nil {
		if ownedValidateInclusion(p, *v.Inclusion) != nil || !anchorMatches(v.Inclusion.Anchor, v.Publication, payload) {
			return ErrInvalid
		}
		times = append(times, v.Inclusion.ObservedAtMS)
	}
	if v.PriorInclusions == nil || len(v.PriorInclusions) > 16 {
		return ErrInvalid
	}
	for i, x := range v.PriorInclusions {
		if ownedValidatePriorInclusion(p, x) != nil || x.Inclusion.Generation != uint32(i) || !anchorMatches(x.Inclusion.Anchor, v.Publication, payload) {
			return ErrInvalid
		}
		times = append(times, x.Inclusion.ObservedAtMS, x.OrphanedAtMS)
	}
	if v.Inclusion != nil && v.Inclusion.Generation != uint32(len(v.PriorInclusions)) {
		return ErrInvalid
	}
	if v.Checkpoint != nil {
		if v.Inclusion == nil || !ownedValidCheckpoint(p, *v.Checkpoint) || v.Checkpoint.Height != int32(v.Inclusion.Anchor.Height) || v.Checkpoint.Hash.String() != v.Inclusion.Anchor.Hash {
			return ErrInvalid
		}
	}
	if v.Finality != nil {
		if v.Checkpoint == nil || v.Inclusion == nil || ownedValidateFinality(p, *v.Finality) != nil || !equal(v.Finality.Checkpoint, *v.Checkpoint) || !equal(v.Finality.Inclusion, *v.Inclusion) {
			return ErrInvalid
		}
		times = append(times, v.Finality.ObservedAtMS)
	}
	if v.Equivocation != nil {
		if ownedValidateEvidenceStatement(p, *v.Equivocation) != nil || v.Equivocation.Statement.ResultKey != v.ResultKey || v.Equivocation.Statement.Source.Source.CardID != src.Source.CardID || v.Equivocation.Statement.Source.Source.Bout.BoutID != src.Source.Bout.BoutID || equal(v.Equivocation.Statement, v.Statement.Statement) {
			return ErrInvalid
		}
	}
	if v.Conflicts == nil || len(v.Conflicts) > 8 || v.ConflictOverflow && len(v.Conflicts) != 8 && v.Equivocation == nil {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, x := range v.Conflicts {
		if ownedValidateConflict(p, x) != nil || seen[ownedConflictKey(x)] {
			return ErrInvalid
		}
		seen[ownedConflictKey(x)] = true
		times = append(times, x.ObservedAtMS)
		switch x.Kind {
		case "source_chain_conflict":
			if x.ExpectedHeight != src.Source.TargetHeight || x.ExpectedHash != src.Source.TargetHash {
				return ErrInvalid
			}
		case "checkpoint_chain_conflict", "checkpoint_equivocation":
			if v.Inclusion == nil || x.ExpectedHeight != v.Inclusion.Anchor.Height || x.ExpectedHash != v.Inclusion.Anchor.Hash || x.Kind == "checkpoint_chain_conflict" && v.Checkpoint == nil {
				return ErrInvalid
			}
		case "statement_equivocation":
			if v.Equivocation == nil {
				return ErrInvalid
			}
			a, _ := OwnedStatementID(v.Statement.Statement)
			b, _ := OwnedStatementID(v.Equivocation.Statement)
			if x.ExpectedHash != a || x.ObservedHash != b {
				return ErrInvalid
			}
		}
	}
	if ownedValidateObservation(p, v.Observation) != nil {
		return ErrInvalid
	}
	obs := v.Observation
	for _, t := range times {
		if t != 0 && (t < v.PreparedAtMS || obs.ObservedAtMS != 0 && t > obs.ObservedAtMS) {
			return ErrInvalid
		}
	}
	if obs.ObservedAtMS == 0 && (v.Publication != nil || len(v.ScanRestarts) != 0 || v.Inclusion != nil || len(v.Conflicts) != 0 || v.Equivocation != nil) {
		return ErrInvalid
	}
	if obs.CurrentFinality && (v.Finality == nil || v.Inclusion == nil || len(v.Conflicts) != 0 || v.ConflictOverflow || v.Equivocation != nil || obs.Tip == nil || obs.Tip.Height < v.Inclusion.Anchor.Height || obs.Tip.Height-v.Inclusion.Anchor.Height != obs.Descendants) {
		return ErrInvalid
	}
	return nil
}
