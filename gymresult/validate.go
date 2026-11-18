// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func validateInfo(p Pin, v Info) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.SourceCreatedAtMS) || !timestamp(v.CreatedAtMS) || v.CreatedAtMS < v.SourceCreatedAtMS || v.ResultCount > MaxResults || v.FundingGeneration > 16 || v.FundingRestartCount != v.FundingGeneration || v.FundingHeight < -1 || !optionalID(v.FundingHash) || (v.FundingHeight < 0) != (v.FundingHash == "") || v.FundingHeight < 0 && v.FundingComplete {
		return ErrInvalid
	}
	return nil
}
func structuralStatement(v Statement) error {
	if v.Version != 1 || !hashID(v.ResultKey) || !hashID(v.StoreID) || !hashID(v.SourceStoreID) || !hashID(v.PublisherChainID) || !hashID(v.Genesis) || !hashID(v.RulesID) || !hexSize(v.PublisherPublicKey, 32) || !hexSize(v.CheckpointPublicKey, 32) || !fits(v, MaxStatementSize) {
		return ErrInvalid
	}
	if v.Source.Version != 1 || v.Source.StoreID != v.SourceStoreID || !hashID(v.Source.RecordDigest) || !hashID(v.Source.SourceDigest) || !hashID(v.Source.InputID) || !hashID(v.Source.ResultDigest) || !timestamp(v.Source.StoreCreatedAtMS) || !timestamp(v.Source.CreatedAtMS) {
		return ErrInvalid
	}
	return nil
}
func validateStatement(p Pin, v Statement) error {
	if structuralStatement(v) != nil {
		return ErrInvalid
	}
	if _, e := gymbout.ValidateResultSource(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	if !equal(v, expectedStatement(p, v.Source)) {
		return ErrInvalid
	}
	return nil
}
func validateSignedStatement(p Pin, v SignedStatement) error {
	if v.Version != 1 || validateStatement(p, v.Statement) != nil || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), statementMessage(v.Statement), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}
func validateInclusion(p Pin, v Inclusion) error {
	if v.Version != 1 || v.Generation > 16 || validateAnchor(p, v.Anchor) != nil || !timestamp(v.ObservedAtMS) || validateTip(p, v.ObservedTip) != nil || v.ObservedTip.Height < v.Anchor.Height || v.ObservedTip.Height == v.Anchor.Height && v.ObservedTip.Hash != v.Anchor.Hash {
		return ErrInvalid
	}
	return nil
}
func replacement(height uint32, h string, t Tip, repl string) bool {
	return optionalID(repl) && repl != h && (repl == "") == (t.Height < height)
}
func validatePriorInclusion(p Pin, v PriorInclusion) error {
	if v.Version != 1 || validateInclusion(p, v.Inclusion) != nil || !timestamp(v.OrphanedAtMS) || v.OrphanedAtMS < v.Inclusion.ObservedAtMS || validateTip(p, v.ObservedTip) != nil || !replacement(v.Inclusion.Anchor.Height, v.Inclusion.Anchor.Hash, v.ObservedTip, v.ReplacementHash) {
		return ErrInvalid
	}
	return nil
}
func validateFinality(p Pin, v Finality) error {
	a := v.Inclusion.Anchor
	if !validCheckpoint(p, v.Checkpoint) || validateInclusion(p, v.Inclusion) != nil || !timestamp(v.ObservedAtMS) || v.ObservedAtMS < v.Inclusion.ObservedAtMS || validateTip(p, v.ObservedTip) != nil || v.ObservedTip.Height < a.Height || v.Descendants != v.ObservedTip.Height-a.Height || v.Descendants < 60 || v.Checkpoint.Height != int32(a.Height) || v.Checkpoint.Hash.String() != a.Hash {
		return ErrInvalid
	}
	return nil
}
func validateConflict(p Pin, v Conflict) error {
	if !timestamp(v.ObservedAtMS) || v.ExpectedHeight > math.MaxInt32 || !optionalID(v.ExpectedHash) || !optionalID(v.ObservedHash) {
		return ErrInvalid
	}
	if v.ObservedTip != nil && validateTip(p, *v.ObservedTip) != nil || v.Checkpoint != nil && !validCheckpoint(p, *v.Checkpoint) {
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
func validateObservation(p Pin, v Observation) error {
	if v.ObservedAtMS != 0 && !timestamp(v.ObservedAtMS) || v.ChainObservedAtMS != 0 && !timestamp(v.ChainObservedAtMS) || v.ChainObservedAtMS > v.ObservedAtMS || v.Descendants > math.MaxInt32 {
		return ErrInvalid
	}
	if v.NodeState != "unknown" && v.NodeState != "online" && v.NodeState != "offline" {
		return ErrInvalid
	}
	if v.Tip == nil {
		if v.ChainObservedAtMS != 0 || v.Descendants != 0 || v.CurrentFinality {
			return ErrInvalid
		}
	} else if validateTip(p, *v.Tip) != nil {
		return ErrInvalid
	}
	if v.CurrentFinality && (v.ChainObservedAtMS == 0 || v.Descendants < 60 || v.NodeState != "online") {
		return ErrInvalid
	}
	return nil
}
func conflictKey(c Conflict) string { c.ObservedAtMS = 0; b, _ := json.Marshal(c); return string(b) }
func anchorMatches(a Anchor, pub *Publication, payload [73]byte) bool {
	if pub == nil {
		return false
	}
	raw := rawID(pub.RawTx)
	sum := sha256.Sum256(raw)
	return a.TxID == pub.TxID && a.Payload == hex.EncodeToString(payload[:]) && a.RawTxDigest == hex.EncodeToString(sum[:]) && a.RawTxSize == uint32(len(raw))
}
func validateRecord(p Pin, v Record) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !hashID(v.ResultKey) || validateSignedStatement(p, v.Statement) != nil || v.ResultKey != v.Statement.Statement.ResultKey || !timestamp(v.PreparedAtMS) {
		return ErrInvalid
	}
	src := v.Statement.Statement.Source
	if v.PreparedAtMS < src.CreatedAtMS || v.PreparedAtMS < src.StoreCreatedAtMS {
		return ErrInvalid
	}
	times := []int64{v.PreparedAtMS}
	payload, e := Carrier(v.Statement.Statement)
	if e != nil {
		return e
	}
	if v.Publication != nil {
		pub := v.Publication
		if validatePublication(p, *pub) != nil || pub.BuiltAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		var tx wire.MsgTx
		if tx.Deserialize(bytes.NewReader(rawID(pub.RawTx))) != nil || !bytes.Equal(tx.TxOut[0].PkScript[2:], payload[:]) {
			return ErrInvalid
		}
		times = append(times, pub.BuiltAtMS, pub.LastDispatchAtMS)
	}
	if validateCursor(p, v.Cursor) != nil || v.ScanRestarts == nil || len(v.ScanRestarts) > 16 || v.Cursor.Generation != uint32(len(v.ScanRestarts)) {
		return ErrInvalid
	}
	for i, x := range v.ScanRestarts {
		if validateScanRestart(p, x) != nil || x.Generation != uint32(i+1) {
			return ErrInvalid
		}
		times = append(times, x.ObservedAtMS)
	}
	if v.Publication == nil && (!equal(v.Cursor, initialCursor(p, v.ResultKey, 0)) || len(v.ScanRestarts) != 0 || len(v.PriorInclusions) != 0) {
		return ErrInvalid
	}
	if v.Cursor.Height < 0 && !equal(v.Cursor, initialCursor(p, v.ResultKey, v.Cursor.Generation)) {
		return ErrInvalid
	}
	if v.Inclusion != nil {
		if validateInclusion(p, *v.Inclusion) != nil || !anchorMatches(v.Inclusion.Anchor, v.Publication, payload) {
			return ErrInvalid
		}
		times = append(times, v.Inclusion.ObservedAtMS)
	}
	if v.PriorInclusions == nil || len(v.PriorInclusions) > 16 {
		return ErrInvalid
	}
	for i, x := range v.PriorInclusions {
		if validatePriorInclusion(p, x) != nil || x.Inclusion.Generation != uint32(i) || !anchorMatches(x.Inclusion.Anchor, v.Publication, payload) {
			return ErrInvalid
		}
		times = append(times, x.Inclusion.ObservedAtMS, x.OrphanedAtMS)
	}
	if v.Inclusion != nil && v.Inclusion.Generation != uint32(len(v.PriorInclusions)) {
		return ErrInvalid
	}
	if v.Checkpoint != nil {
		if v.Inclusion == nil || !validCheckpoint(p, *v.Checkpoint) || v.Checkpoint.Height != int32(v.Inclusion.Anchor.Height) || v.Checkpoint.Hash.String() != v.Inclusion.Anchor.Hash {
			return ErrInvalid
		}
	}
	if v.Finality != nil {
		if v.Checkpoint == nil || v.Inclusion == nil || validateFinality(p, *v.Finality) != nil || !equal(v.Finality.Checkpoint, *v.Checkpoint) || !equal(v.Finality.Inclusion, *v.Inclusion) {
			return ErrInvalid
		}
		times = append(times, v.Finality.ObservedAtMS)
	}
	if v.Equivocation != nil {
		if validateEvidenceStatement(p, *v.Equivocation) != nil || v.Equivocation.Statement.ResultKey != v.ResultKey || v.Equivocation.Statement.Source.Source.CardID != src.Source.CardID || v.Equivocation.Statement.Source.Source.Bout.BoutID != src.Source.Bout.BoutID || equal(v.Equivocation.Statement, v.Statement.Statement) {
			return ErrInvalid
		}
	}
	if v.Conflicts == nil || len(v.Conflicts) > 8 || v.ConflictOverflow && len(v.Conflicts) != 8 && v.Equivocation == nil {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, x := range v.Conflicts {
		if validateConflict(p, x) != nil || seen[conflictKey(x)] {
			return ErrInvalid
		}
		seen[conflictKey(x)] = true
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
			a, _ := StatementID(v.Statement.Statement)
			b, _ := StatementID(v.Equivocation.Statement)
			if x.ExpectedHash != a || x.ObservedHash != b {
				return ErrInvalid
			}
		}
	}
	if validateObservation(p, v.Observation) != nil {
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
func validateProofSummary(p Pin, v ProofSummary) error {
	if v.Version != 1 || v.ResultKey != resultKey(p, v.CardID, v.BoutID) || !hashID(v.CardID) || !hashID(v.BoutID) || !hashID(v.RecordDigest) || !hashID(v.SourceRecordDigest) || !hashID(v.InputID) || !hashID(v.ResultDigest) || !hashID(v.TxID) || !hashID(v.SHA256) || v.Bytes == 0 || v.Bytes > MaxProofSize || v.HeaderCount == 0 || v.HeaderCount > 512 || v.BlockCount == 0 || v.BlockCount > 3 || v.FinalizedAtMS != 0 && !timestamp(v.FinalizedAtMS) || validateTip(p, v.ObservedTip) != nil {
		return ErrInvalid
	}
	return nil
}
