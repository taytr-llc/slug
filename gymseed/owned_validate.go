// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func validateOwnedInfo(p OwnedPin, v OwnedInfo) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !ownedTimestamp(v.CreatedAtMS) || v.EventCount > MaxEvents || v.FundingGeneration > MaxScanRestarts || v.FundingRestartCount != v.FundingGeneration || v.FundingHeight < -1 || !optionalID(v.FundingHash) {
		return ErrInvalid
	}
	if (v.FundingHeight < 0) != (v.FundingHash == "") || v.FundingHeight < 0 && v.FundingComplete {
		return ErrInvalid
	}
	return nil
}

func validateOwnedDeclaration(p OwnedPin, v OwnedDeclaration) error {
	if structuralOwnedDeclaration(v) != nil {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedEvent(p.SourcePin, v.Event); e != nil {
		return ErrInvalid
	}
	id, _ := OwnedID(p)
	if v.StoreID != id || v.SourceStoreID != p.SourceStoreID || v.EventKey != ownedEventKey(p, v.Event) || v.Network != p.Network || v.Genesis != p.Genesis || v.PublisherPublicKey != p.PublisherPublicKey || v.PublisherChainID != p.PublisherChainID || v.RulesID != p.RulesID || v.CheckpointPublicKey != p.CheckpointPublicKey {
		return ErrInvalid
	}
	return nil
}

func validateOwnedSignedDeclaration(p OwnedPin, v OwnedSignedDeclaration) error {
	if v.Version != 1 || validateOwnedDeclaration(p, v.Declaration) != nil || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), ownedDeclarationMessage(v.Declaration), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}

func validateOwnedConflict(p OwnedPin, v OwnedConflict) error {
	if !ownedTimestamp(v.ObservedAtMS) {
		return ErrInvalid
	}
	switch v.Kind {
	case "declaration":
		if v.Declaration == nil || v.Checkpoint != nil || v.ObservedTip != nil || v.ExpectedHeight != 0 || v.ExpectedHash != "" || v.ObservedHash != "" || validateOwnedSignedDeclaration(p, *v.Declaration) != nil {
			return ErrInvalid
		}
	case "checkpoint":
		if v.Declaration != nil || v.Checkpoint == nil || v.ObservedTip != nil || v.ExpectedHeight != 0 || v.ExpectedHash != "" || v.ObservedHash != "" || !validCheckpoint(physicalOwnedPin(p), *v.Checkpoint) {
			return ErrInvalid
		}
	case "chain":
		if v.Declaration != nil || v.Checkpoint != nil || v.ObservedTip == nil || validateTip(physicalOwnedPin(p), *v.ObservedTip) != nil || v.ExpectedHeight > math.MaxInt32 || !hashID(v.ExpectedHash) || !optionalID(v.ObservedHash) {
			return ErrInvalid
		}
		if v.ObservedTip.Height < v.ExpectedHeight {
			if v.ObservedHash != "" {
				return ErrInvalid
			}
		} else if v.ObservedHash == "" || v.ObservedHash == v.ExpectedHash {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validateOwnedRecord(p OwnedPin, v OwnedRecord) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != ownedEventKey(p, v.SourceReceipt.Input.Event) || !ownedTimestamp(v.PreparedAtMS) || v.ScanRestarts == nil || len(v.ScanRestarts) > 16 || v.Conflicts == nil || len(v.Conflicts) > 8 || v.ConflictOverflow && len(v.Conflicts) != 8 {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedSnapshotReceipt(p.SourcePin, v.SourceReceipt); e != nil {
		return ErrInvalid
	}
	want, e := expectedOwnedDeclaration(p, v.SourceReceipt)
	if e != nil || !equal(want, v.Declaration.Declaration) || validateOwnedSignedDeclaration(p, v.Declaration) != nil || v.SourceReceipt.CompletedAtMS > v.PreparedAtMS || validateCursor(physicalOwnedPin(p), v.Cursor) != nil || v.Cursor.Generation != uint32(len(v.ScanRestarts)) || validateObservation(physicalOwnedPin(p), v.Observation) != nil {
		return ErrInvalid
	}
	last := v.PreparedAtMS
	for i, x := range v.ScanRestarts {
		if validateScanRestart(physicalOwnedPin(p), x) != nil || x.Generation != uint32(i+1) || x.ObservedAtMS < last {
			return ErrInvalid
		}
		last = x.ObservedAtMS
	}
	if v.Cursor.Height < 0 && !equal(v.Cursor, initialOwnedCursor(p, v.EventKey, v.Cursor.Generation)) {
		return ErrInvalid
	}
	payload, _ := ownedCarrier(v.Declaration.Declaration)
	if v.Publication != nil {
		if validatePublication(physicalOwnedPin(p), *v.Publication) != nil || v.Publication.BuiltAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		var tx wire.MsgTx
		tx.Deserialize(bytes.NewReader(rawID(v.Publication.RawTx)))
		if !bytes.Equal(tx.TxOut[0].PkScript[2:], payload[:]) {
			return ErrInvalid
		}
	}
	if v.Anchor != nil {
		if validateAnchor(physicalOwnedPin(p), *v.Anchor) != nil || v.Anchor.Payload != hex.EncodeToString(payload[:]) || v.Cursor.Height != int32(v.Anchor.Height) || v.Cursor.Hash != v.Anchor.Hash || v.Cursor.CompleteToHeight != v.Cursor.Height || v.Cursor.CompleteToHash != v.Cursor.Hash {
			return ErrInvalid
		}
	}
	if v.Target != nil {
		if v.Anchor == nil || validateTarget(physicalOwnedPin(p), *v.Target) != nil || v.Anchor.Height == math.MaxInt32 || v.Target.Height != v.Anchor.Height+1 || v.Target.ObservedAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		h, _ := header(v.Target.Header)
		if h.PrevBlock.String() != v.Anchor.Hash {
			return ErrInvalid
		}
	}
	if v.Checkpoint != nil {
		if v.Target == nil || !validCheckpoint(physicalOwnedPin(p), *v.Checkpoint) || v.Checkpoint.Height != int32(v.Target.Height) || v.Checkpoint.Hash.String() != v.Target.Hash {
			return ErrInvalid
		}
	}
	if v.Finality != nil {
		if v.Checkpoint == nil || validateFinality(physicalOwnedPin(p), *v.Finality) != nil || !equal(v.Finality.Checkpoint, *v.Checkpoint) || v.Finality.ObservedAtMS < v.Target.ObservedAtMS {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	last = v.PreparedAtMS
	for _, x := range v.Conflicts {
		if validateOwnedConflict(p, x) != nil || x.ObservedAtMS < last {
			return ErrInvalid
		}
		last = x.ObservedAtMS
		key := ownedConflictKey(x)
		if seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		switch x.Kind {
		case "declaration":
			if x.Declaration.Declaration.EventKey != v.EventKey || equal(x.Declaration.Declaration, v.Declaration.Declaration) {
				return ErrInvalid
			}
		case "checkpoint":
			if v.Target == nil || x.Checkpoint.Height != int32(v.Target.Height) || x.Checkpoint.Hash.String() == v.Target.Hash {
				return ErrInvalid
			}
		case "chain":
			if !((v.Anchor != nil && x.ExpectedHeight == v.Anchor.Height && x.ExpectedHash == v.Anchor.Hash) || (v.Target != nil && x.ExpectedHeight == v.Target.Height && x.ExpectedHash == v.Target.Hash)) {
				return ErrInvalid
			}
		}
	}
	if v.Observation.ObservedAtMS != 0 && v.Observation.ObservedAtMS < v.PreparedAtMS {
		return ErrInvalid
	}
	if v.Target == nil {
		if v.Observation.TargetDescendants != 0 || v.Observation.CurrentFinality {
			return ErrInvalid
		}
	} else if v.Observation.Tip != nil {
		n := uint32(0)
		if v.Observation.Tip.Height >= v.Target.Height {
			n = v.Observation.Tip.Height - v.Target.Height
		}
		if n != v.Observation.TargetDescendants {
			return ErrInvalid
		}
	}
	if v.Observation.CurrentFinality && (v.Checkpoint == nil || v.Finality == nil || len(v.Conflicts) > 0 || v.ConflictOverflow || v.Observation.TargetDescendants < 60) {
		return ErrInvalid
	}
	if v.IncidentAtMS != 0 && (!ownedTimestamp(v.IncidentAtMS) || v.IncidentAtMS < v.SourceReceipt.Input.Event.IncidentDeadlineMS || v.IncidentAtMS < v.PreparedAtMS) {
		return ErrInvalid
	}
	return nil
}

func ownedConflictKey(c OwnedConflict) string {
	c.ObservedAtMS = 0
	b, _ := json.Marshal(c)
	return string(b)
}

func validateOwnedStatus(p OwnedPin, v OwnedStatus) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != ownedEventKey(p, v.Event) || !ownedTimestamp(v.ObservedAtMS) || v.ChainObservedAtMS < 0 || v.ChainObservedAtMS > v.ObservedAtMS || v.ScanHeight < -1 || v.ScanTipHeight < -1 || v.ScanGeneration > 16 || v.FundingGeneration > 16 || !optionalID(v.AnchorHash) || !optionalID(v.TargetHash) || v.ConflictCount > 8 || v.ConflictOverflow && v.ConflictCount != 8 {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedEvent(p.SourcePin, v.Event); e != nil {
		return ErrInvalid
	}
	switch v.NodeState {
	case "unobserved", "online", "offline", "unavailable":
	default:
		return ErrInvalid
	}
	switch v.FundingState {
	case "unobserved", "scanning", "ready", "waiting_for_funds", "unavailable":
	default:
		return ErrInvalid
	}
	switch v.PublicationState {
	case "none", "retained", "unknown", "submitted":
	default:
		return ErrInvalid
	}
	if !v.Exists {
		want := absentOwnedStatus(p, v.Event, v.ObservedAtMS)
		if !equal(v, want) {
			return ErrInvalid
		}
		return nil
	}
	if !ownedTimestamp(v.PreparedAtMS) || v.PreparedAtMS > v.ObservedAtMS || v.ChainObservedAtMS > 0 && v.ChainObservedAtMS < v.PreparedAtMS {
		return ErrInvalid
	}
	if v.AnchorHeight > math.MaxInt32 || v.TargetHeight > math.MaxInt32 || v.TargetHash == "" && (v.TargetHeight != 0 || v.TargetDescendants != 0 || v.CheckpointPresent) || v.ChainObservedAtMS == 0 && (v.ScanTipHeight != -1 || v.CurrentFinality) {
		return ErrInvalid
	}
	if v.ChainObservedAtMS > 0 && v.ScanTipHeight < 0 {
		return ErrInvalid
	}
	if v.CheckpointPresent && v.TargetHash == "" {
		return ErrInvalid
	}

	if (v.AnchorHash == "") != (v.AnchorHeight == 0) && v.AnchorHeight != 0 {
		return ErrInvalid
	}
	if v.TargetHash != "" && (v.AnchorHash == "" || v.TargetHeight != v.AnchorHeight+1) {
		return ErrInvalid
	}
	phase := "prepared"
	if v.PublicationState != "none" {
		phase = "waiting_anchor"
	}
	if v.AnchorHash != "" {
		phase = "waiting_target"
	}
	if v.TargetHash != "" {
		phase = "provisional"
	}
	if v.FinalizedAtMS != 0 {
		if !ownedTimestamp(v.FinalizedAtMS) || !v.CheckpointPresent || v.TargetHash == "" || v.FinalizedAtMS > v.ObservedAtMS || v.FinalizedAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		phase = "finalized"
	}
	if v.ConflictCount > 0 || v.ConflictOverflow {
		phase = "conflict"
	}
	if v.Phase != phase {
		return ErrInvalid
	}
	if v.IncidentAtMS != 0 && (!ownedTimestamp(v.IncidentAtMS) || v.IncidentAtMS < v.Event.IncidentDeadlineMS || v.IncidentAtMS > v.ObservedAtMS) {
		return ErrInvalid
	}
	due := v.ObservedAtMS >= v.Event.IncidentDeadlineMS && (v.FinalizedAtMS == 0 || phase == "conflict")
	if v.EscalationDue != due {
		return ErrInvalid
	}
	if v.CurrentFinality && (v.FinalizedAtMS == 0 || v.ChainObservedAtMS == 0 || v.TargetDescendants < 60 || phase == "conflict") {
		return ErrInvalid
	}
	return nil
}

func validateOwnedHistoryPage(p OwnedPin, v OwnedHistoryPage) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !optionalID(v.AfterEventKey) || !optionalID(v.NextEventKey) || !ownedTimestamp(v.ObservedAtMS) || v.Events == nil || len(v.Events) > 8 {
		return ErrInvalid
	}
	last := int64(0)
	key := ""
	for _, x := range v.Events {
		if validateOwnedStatus(p, x) != nil || !x.Exists || x.ObservedAtMS != v.ObservedAtMS || x.Event.CutoffMS < last || x.Event.CutoffMS == last && x.EventKey <= key {
			return ErrInvalid
		}
		last = x.Event.CutoffMS
		key = x.EventKey
	}
	if v.HasMore {
		if len(v.Events) == 0 || v.NextEventKey != key {
			return ErrInvalid
		}
	} else if v.NextEventKey != "" {
		return ErrInvalid
	}
	return nil
}
