// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func hexSize(s string, n int) bool {
	if len(s) != n*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func hashID(s string) bool     { return hexSize(s, 32) }
func optionalID(s string) bool { return s == "" || hashID(s) }
func timestamp(n int64) bool   { return n > 0 && n <= MaxTimestampMS }
func fits(v any, n int) bool   { b, e := json.Marshal(v); return e == nil && len(b) <= n }
func validatePin(p Pin) error {
	want, e := CandidatePin(p.SourcePin)
	if e != nil || !equal(p, want) {
		return ErrInvalid
	}
	return nil
}
func validateInfo(p Pin, v Info) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.CreatedAtMS) || v.EventCount > MaxEvents || v.FundingGeneration > MaxScanRestarts || v.FundingRestartCount != v.FundingGeneration || v.FundingHeight < -1 || !optionalID(v.FundingHash) {
		return ErrInvalid
	}
	if (v.FundingHeight < 0) != (v.FundingHash == "") || v.FundingHeight < 0 && v.FundingComplete {
		return ErrInvalid
	}
	return nil
}
func structuralDeclaration(v Declaration) error {
	if v.Version != 1 || !hashID(v.EventKey) || !hashID(v.StoreID) || !hashID(v.SourceStoreID) || !hashID(v.SnapshotDigest) || !hashID(v.ReceiptDigest) || !hashID(v.PublisherChainID) || !hashID(v.Genesis) || v.FloorHeight != 0 || v.FloorHash != v.Genesis || !hashID(v.RulesID) || !hexSize(v.PublisherPublicKey, 32) || !hexSize(v.CheckpointPublicKey, 32) || v.Event.Version != 1 || !hashID(v.Event.EventID) || !hashID(v.Event.SourceEventID) || v.Event.StoreID != v.SourceStoreID || !hashID(v.Event.NamespaceID) || !hashID(v.Event.CalendarID) || !hashID(v.Event.RulesID) || len(v.Event.Saturday) != 10 || !timestamp(v.Event.CutoffMS) || !timestamp(v.Event.IncidentDeadlineMS) || v.Event.IncidentDeadlineMS <= v.Event.CutoffMS || !fits(v, MaxDeclarationSize) {
		return ErrInvalid
	}
	return nil
}
func validateDeclaration(p Pin, v Declaration) error {
	if structuralDeclaration(v) != nil {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateEvent(p.SourcePin, v.Event); e != nil {
		return ErrInvalid
	}
	if v.SourceStoreID != p.SourceStoreID || v.EventKey != eventKey(p, v.Event) || v.Network != p.Network || v.Genesis != p.Genesis || v.PublisherPublicKey != p.PublisherPublicKey {
		return ErrInvalid
	}
	return nil
}
func validateSignedDeclaration(p Pin, v SignedDeclaration) error {
	if v.Version != 1 || validateDeclaration(p, v.Declaration) != nil || !hexSize(v.Signature, 64) || !ed25519.Verify(rawID(p.PublisherPublicKey), declarationMessage(v.Declaration), rawID(v.Signature)) {
		return ErrInvalid
	}
	return nil
}
func validateOutpoint(_ Pin, v Outpoint) error {
	if !hashID(v.TxID) {
		return ErrInvalid
	}
	return nil
}
func validateFundingCoin(p Pin, v FundingCoin) error {
	if validateOutpoint(p, v.Outpoint) != nil || v.ValueSatoshis <= 0 || v.ValueSatoshis > btcutil.MaxSatoshi || v.Script != p.FundingScript || v.BlockHeight > math.MaxInt32 || !hashID(v.BlockHash) {
		return ErrInvalid
	}
	return nil
}
func validatePublication(p Pin, v Publication) error {
	if v.Version != 1 || !hexSize(v.RawTx, len(v.RawTx)/2) || len(v.RawTx) == 0 || len(v.RawTx) > 2*MaxRawTxSize || !hashID(v.TxID) || validateFundingCoin(p, v.Coin) != nil || !timestamp(v.BuiltAtMS) {
		return ErrInvalid
	}
	if v.DispatchCount == 0 {
		if v.State != "retained" || v.FirstDispatchAtMS != 0 || v.LastDispatchAtMS != 0 {
			return ErrInvalid
		}
	} else {
		if (v.State != "unknown" && v.State != "submitted") || !timestamp(v.FirstDispatchAtMS) || !timestamp(v.LastDispatchAtMS) || v.FirstDispatchAtMS < v.BuiltAtMS || v.LastDispatchAtMS < v.FirstDispatchAtMS {
			return ErrInvalid
		}
	}
	raw := rawID(v.RawTx)
	reader := bytes.NewReader(raw)
	var tx wire.MsgTx
	if tx.Deserialize(reader) != nil || reader.Len() != 0 || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 || tx.TxHash().String() != v.TxID {
		return ErrInvalid
	}
	var encoded bytes.Buffer
	if tx.Serialize(&encoded) != nil || !bytes.Equal(encoded.Bytes(), raw) {
		return ErrInvalid
	}
	in := tx.TxIn[0]
	if in.PreviousOutPoint.Hash.String() != v.Coin.Outpoint.TxID || in.PreviousOutPoint.Index != v.Coin.Outpoint.Index || in.Sequence != math.MaxUint32 || len(in.SignatureScript) != 0 || len(in.Witness) != 2 || !bytes.Equal(in.Witness[1], rawID(p.FundingPublicKey)) || len(in.Witness[0]) < 2 || in.Witness[0][len(in.Witness[0])-1] != byte(txscript.SigHashAll) {
		return ErrInvalid
	}
	if v.Coin.ValueSatoshis < 1294 || tx.TxOut[0].Value != 0 || len(tx.TxOut[0].PkScript) != 75 || tx.TxOut[0].PkScript[0] != txscript.OP_RETURN || tx.TxOut[0].PkScript[1] != 73 || !bytes.Equal(tx.TxOut[0].PkScript[2:10], []byte("SLUGSEED")) || tx.TxOut[0].PkScript[10] != 1 || tx.TxOut[1].Value != v.Coin.ValueSatoshis-1000 || !bytes.Equal(tx.TxOut[1].PkScript, rawID(p.FundingScript)) {
		return ErrInvalid
	}
	fetch := txscript.NewCannedPrevOutputFetcher(rawID(p.FundingScript), v.Coin.ValueSatoshis)
	hashes := txscript.NewTxSigHashes(&tx, fetch)
	vm, e := txscript.NewEngine(rawID(p.FundingScript), &tx, 0, txscript.StandardVerifyFlags, nil, hashes, v.Coin.ValueSatoshis, fetch)
	if e != nil || vm.Execute() != nil {
		return ErrInvalid
	}
	return nil
}
func validateCursor(_ Pin, v Cursor) error {
	if v.Generation > MaxScanRestarts || v.Height < -1 || v.CompleteToHeight < -1 || v.CompleteToHeight > v.Height || !optionalID(v.Hash) || !optionalID(v.CompleteToHash) || !hashID(v.ScanDigest) || (v.Height < 0) != (v.Hash == "") || (v.CompleteToHeight < 0) != (v.CompleteToHash == "") {
		return ErrInvalid
	}
	if v.Height == v.CompleteToHeight && v.Hash != v.CompleteToHash {
		return ErrInvalid
	}
	return nil
}
func validateTip(_ Pin, v Tip) error {
	if v.Height > math.MaxInt32 || !hashID(v.Hash) {
		return ErrInvalid
	}
	return nil
}
func validateScanRestart(p Pin, v ScanRestart) error {
	if v.Generation < 1 || v.Generation > MaxScanRestarts || validateCursor(p, v.Previous) != nil || v.Previous.Generation+1 != v.Generation || v.Previous.Height < 0 || !timestamp(v.ObservedAtMS) || validateTip(p, v.ObservedTip) != nil || !optionalID(v.ReplacementHash) {
		return ErrInvalid
	}
	if v.ObservedTip.Height < uint32(v.Previous.Height) {
		if v.ReplacementHash != "" {
			return ErrInvalid
		}
	} else if !hashID(v.ReplacementHash) || v.ReplacementHash == v.Previous.Hash {
		return ErrInvalid
	}
	return nil
}
func header(raw string) (wire.BlockHeader, error) {
	var h wire.BlockHeader
	if !hexSize(raw, 80) {
		return h, ErrInvalid
	}
	if h.Deserialize(bytes.NewReader(rawID(raw))) != nil {
		return h, ErrInvalid
	}
	return h, nil
}
func validateAnchor(p Pin, v Anchor) error {
	if v.Height > math.MaxInt32 || !hashID(v.Hash) || !hashID(v.TxID) || !hashID(v.RawTxDigest) || v.RawTxSize == 0 || v.RawTxSize > wire.MaxBlockPayload || v.TxCount == 0 || v.TxCount > 1000000 || v.TxIndex >= v.TxCount || v.OutputIndex >= wire.MaxBlockPayload/9 || !hexSize(v.Payload, 73) || v.MerkleBranch == nil || len(v.MerkleBranch) > 20 {
		return ErrInvalid
	}
	payload := rawID(v.Payload)
	if !bytes.Equal(payload[:8], []byte("SLUGSEED")) || payload[8] != 1 {
		return ErrInvalid
	}
	h, e := header(v.Header)
	if e != nil {
		return e
	}
	leaf, e := chainhash.NewHashFromStr(v.TxID)
	if e != nil {
		return ErrInvalid
	}
	index, count, used := v.TxIndex, v.TxCount, 0
	for count > 1 {
		if used >= len(v.MerkleBranch) || !hashID(v.MerkleBranch[used]) {
			return ErrInvalid
		}
		sib, e := chainhash.NewHashFromStr(v.MerkleBranch[used])
		if e != nil {
			return ErrInvalid
		}
		if index%2 == 0 && index+1 == count && *sib != *leaf {
			return ErrInvalid
		}
		var pair []byte
		if index%2 == 0 {
			pair = append(append([]byte{}, leaf[:]...), sib[:]...)
		} else {
			pair = append(append([]byte{}, sib[:]...), leaf[:]...)
		}
		sum := chainhash.DoubleHashH(pair)
		leaf = &sum
		index /= 2
		count = (count + 1) / 2
		used++
	}
	if used != len(v.MerkleBranch) || *leaf != h.MerkleRoot {
		return ErrInvalid
	}
	return nil
}
func validateTarget(p Pin, v Target) error {
	if v.Height == 0 || v.Height > math.MaxInt32 || !hashID(v.Hash) || !timestamp(v.ObservedAtMS) || validateTip(p, v.ObservedTip) != nil || v.ObservedTip.Height < v.Height {
		return ErrInvalid
	}
	_, e := header(v.Header)
	return e
}
func validCheckpoint(p Pin, c chaincfg.SignedCheckpoint) bool {
	if c.Network != wire.BitcoinNet(p.Network) || c.GenesisHash.String() != p.Genesis || c.Height <= 0 || len(c.Signature) != 64 {
		return false
	}
	params := chaincfg.MainNetParams
	return c.Verify(&params, rawID(p.CheckpointPublicKey)) == nil
}
func validateFinality(p Pin, v Finality) error {
	if !validCheckpoint(p, v.Checkpoint) || !timestamp(v.ObservedAtMS) || validateTip(p, v.ObservedTip) != nil || v.ObservedTip.Height < uint32(v.Checkpoint.Height) || v.TargetDescendants != v.ObservedTip.Height-uint32(v.Checkpoint.Height) || v.TargetDescendants < 60 {
		return ErrInvalid
	}
	return nil
}
func validateConflict(p Pin, v Conflict) error {
	if !timestamp(v.ObservedAtMS) {
		return ErrInvalid
	}
	switch v.Kind {
	case "declaration":
		if v.Declaration == nil || v.Checkpoint != nil || v.ObservedTip != nil || v.ExpectedHeight != 0 || v.ExpectedHash != "" || v.ObservedHash != "" || validateSignedDeclaration(p, *v.Declaration) != nil {
			return ErrInvalid
		}
	case "checkpoint":
		if v.Declaration != nil || v.Checkpoint == nil || v.ObservedTip != nil || v.ExpectedHeight != 0 || v.ExpectedHash != "" || v.ObservedHash != "" || !validCheckpoint(p, *v.Checkpoint) {
			return ErrInvalid
		}
	case "chain":
		if v.Declaration != nil || v.Checkpoint != nil || v.ObservedTip == nil || validateTip(p, *v.ObservedTip) != nil || v.ExpectedHeight > math.MaxInt32 || !hashID(v.ExpectedHash) || !optionalID(v.ObservedHash) {
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
func validateObservation(p Pin, v Observation) error {
	if v.ObservedAtMS != 0 && !timestamp(v.ObservedAtMS) || v.ChainObservedAtMS != 0 && !timestamp(v.ChainObservedAtMS) || v.ChainObservedAtMS > v.ObservedAtMS {
		return ErrInvalid
	}
	switch v.NodeState {
	case "unobserved":
		if v.ObservedAtMS != 0 || v.ChainObservedAtMS != 0 {
			return ErrInvalid
		}
	case "online":
		if v.ChainObservedAtMS != v.ObservedAtMS || v.ChainObservedAtMS == 0 {
			return ErrInvalid
		}
	case "offline", "unavailable":
		if v.ObservedAtMS == 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if v.ChainObservedAtMS == 0 {
		if v.Tip != nil || v.TargetDescendants != 0 || v.CurrentFinality {
			return ErrInvalid
		}
	} else if v.Tip == nil || validateTip(p, *v.Tip) != nil {
		return ErrInvalid
	}
	return nil
}
func validateRecord(p Pin, v Record) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != eventKey(p, v.SourceReceipt.Input.Event) || !timestamp(v.PreparedAtMS) || v.ScanRestarts == nil || len(v.ScanRestarts) > 16 || v.Conflicts == nil || len(v.Conflicts) > 8 || v.ConflictOverflow && len(v.Conflicts) != 8 {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateSnapshotReceipt(p.SourcePin, v.SourceReceipt); e != nil {
		return ErrInvalid
	}
	want, e := expectedDeclaration(p, v.SourceReceipt)
	if e != nil || !equal(want, v.Declaration.Declaration) || validateSignedDeclaration(p, v.Declaration) != nil || v.SourceReceipt.CompletedAtMS > v.PreparedAtMS || validateCursor(p, v.Cursor) != nil || v.Cursor.Generation != uint32(len(v.ScanRestarts)) || validateObservation(p, v.Observation) != nil {
		return ErrInvalid
	}
	last := v.PreparedAtMS
	for i, x := range v.ScanRestarts {
		if validateScanRestart(p, x) != nil || x.Generation != uint32(i+1) || x.ObservedAtMS < last {
			return ErrInvalid
		}
		last = x.ObservedAtMS
	}
	if v.Cursor.Height < 0 && !equal(v.Cursor, initialCursor(p, v.EventKey, v.Cursor.Generation)) {
		return ErrInvalid
	}
	payload, _ := carrier(v.Declaration.Declaration)
	if v.Publication != nil {
		if validatePublication(p, *v.Publication) != nil || v.Publication.BuiltAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		var tx wire.MsgTx
		tx.Deserialize(bytes.NewReader(rawID(v.Publication.RawTx)))
		if !bytes.Equal(tx.TxOut[0].PkScript[2:], payload[:]) {
			return ErrInvalid
		}
	}
	if v.Anchor != nil {
		if validateAnchor(p, *v.Anchor) != nil || v.Anchor.Payload != hex.EncodeToString(payload[:]) || v.Cursor.Height != int32(v.Anchor.Height) || v.Cursor.Hash != v.Anchor.Hash || v.Cursor.CompleteToHeight != v.Cursor.Height || v.Cursor.CompleteToHash != v.Cursor.Hash {
			return ErrInvalid
		}
	}
	if v.Target != nil {
		if v.Anchor == nil || validateTarget(p, *v.Target) != nil || v.Anchor.Height == math.MaxInt32 || v.Target.Height != v.Anchor.Height+1 || v.Target.ObservedAtMS < v.PreparedAtMS {
			return ErrInvalid
		}
		h, _ := header(v.Target.Header)
		if h.PrevBlock.String() != v.Anchor.Hash {
			return ErrInvalid
		}
	}
	if v.Checkpoint != nil {
		if v.Target == nil || !validCheckpoint(p, *v.Checkpoint) || v.Checkpoint.Height != int32(v.Target.Height) || v.Checkpoint.Hash.String() != v.Target.Hash {
			return ErrInvalid
		}
	}
	if v.Finality != nil {
		if v.Checkpoint == nil || validateFinality(p, *v.Finality) != nil || !equal(v.Finality.Checkpoint, *v.Checkpoint) || v.Finality.ObservedAtMS < v.Target.ObservedAtMS {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	last = v.PreparedAtMS
	for _, x := range v.Conflicts {
		if validateConflict(p, x) != nil || x.ObservedAtMS < last {
			return ErrInvalid
		}
		last = x.ObservedAtMS
		key := conflictKey(x)
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
	if v.IncidentAtMS != 0 && (!timestamp(v.IncidentAtMS) || v.IncidentAtMS < v.SourceReceipt.Input.Event.IncidentDeadlineMS || v.IncidentAtMS < v.PreparedAtMS) {
		return ErrInvalid
	}
	return nil
}
func conflictKey(c Conflict) string { c.ObservedAtMS = 0; b, _ := json.Marshal(c); return string(b) }
func validateStatus(p Pin, v Status) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || v.EventKey != eventKey(p, v.Event) || !timestamp(v.ObservedAtMS) || v.ChainObservedAtMS < 0 || v.ChainObservedAtMS > v.ObservedAtMS || v.ScanHeight < -1 || v.ScanTipHeight < -1 || v.ScanGeneration > 16 || v.FundingGeneration > 16 || !optionalID(v.AnchorHash) || !optionalID(v.TargetHash) || v.ConflictCount > 8 || v.ConflictOverflow && v.ConflictCount != 8 {
		return ErrInvalid
	}
	if _, e := gymevent.ValidateEvent(p.SourcePin, v.Event); e != nil {
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
		want := absentStatus(p, v.Event, v.ObservedAtMS)
		if !equal(v, want) {
			return ErrInvalid
		}
		return nil
	}
	if !timestamp(v.PreparedAtMS) || v.PreparedAtMS > v.ObservedAtMS || v.ChainObservedAtMS > 0 && v.ChainObservedAtMS < v.PreparedAtMS {
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
		if !timestamp(v.FinalizedAtMS) || !v.CheckpointPresent || v.TargetHash == "" || v.FinalizedAtMS > v.ObservedAtMS || v.FinalizedAtMS < v.PreparedAtMS {
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
	if v.IncidentAtMS != 0 && (!timestamp(v.IncidentAtMS) || v.IncidentAtMS < v.Event.IncidentDeadlineMS || v.IncidentAtMS > v.ObservedAtMS) {
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
func validateHistoryPage(p Pin, v HistoryPage) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !optionalID(v.AfterEventKey) || !optionalID(v.NextEventKey) || !timestamp(v.ObservedAtMS) || v.Events == nil || len(v.Events) > 8 {
		return ErrInvalid
	}
	last := int64(0)
	key := ""
	for _, x := range v.Events {
		if validateStatus(p, x) != nil || !x.Exists || x.ObservedAtMS != v.ObservedAtMS || x.Event.CutoffMS < last || x.Event.CutoffMS == last && x.EventKey <= key {
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
