// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func seedErrorCode(s string) bool {
	switch s {
	case "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown", "seed_unavailable", "seed_setup_incomplete", "seed_corrupt", "seed_capacity", "seed_clock", "seed_calendar", "seed_created_too_late", "seed_source_incomplete", "seed_not_found", "seed_not_prepared", "seed_node_offline", "seed_waiting_for_funds", "seed_scan_incomplete", "seed_conflict", "seed_target_unavailable", "seed_not_mature", "seed_cancelled":
		return true
	}
	return false
}
func seedRequestValid(r SeedRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	b := r
	b.ID = 0
	b.Command = ""
	switch r.Command {
	case "seed_snapshot":
	case "seed_status", "seed_record", "seed_prepare", "seed_publish", "seed_reconcile", "seed_attest":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) {
			return false
		}
		b.ViewID = ""
		b.Saturday = ""
	case "seed_history":
		if !canonicalManifestID(r.ViewID) || r.AfterEventKey != "" && !canonicalManifestID(r.AfterEventKey) || r.Limit < 1 || r.Limit > 8 {
			return false
		}
		b.ViewID = ""
		b.AfterEventKey = ""
		b.Limit = 0
	case "seed_job", "seed_cancel":
		if !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) {
			return false
		}
		b.ViewID = ""
		b.JobID = ""
	default:
		return false
	}
	return b == (SeedRequest{})
}
func seedSnapshotValid(s SeedSnapshot) bool {
	if s.Version != 1 || s.NodeState != "ready" && s.NodeState != "offline" {
		return false
	}
	empty := s.Pin == nil && s.StoreID == "" && s.PublisherAccountID == "" && s.CreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == "" && s.NodeState == "offline"
	if s.State == "unavailable" {
		return empty && s.View == nil && s.Error == "seed_unavailable"
	}
	if s.View == nil || !stableViewValid(*s.View) {
		return false
	}
	if s.State == "setup_incomplete" {
		return empty && s.Error == "seed_setup_incomplete"
	}
	if s.Pin == nil || !stableTime(s.CreatedAtMS) {
		return false
	}
	p, e := gymseed.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	id, e := gymseed.ID(p)
	if e != nil || id != s.StoreID || s.PublisherAccountID != p.PublisherAccountID {
		return false
	}
	if _, ok := eventViewParticipant(*s.View, p.SourcePin); !ok {
		return false
	}
	switch s.State {
	case "ready", "busy":
		if s.Error != "" {
			return false
		}
	case "failed":
		if s.Error != "seed_unavailable" && s.Error != "seed_corrupt" && s.Error != "seed_clock" {
			return false
		}
	default:
		return false
	}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" && !canonicalManifestID(id) {
			return false
		}
	}
	return s.ActiveJobID == "" || s.ActiveJobID != s.QueuedJobID
}
func seedHex(s string, n int) bool {
	if len(s) != 2*n {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(b) == s
}
func seedHash(domain string, v any) string {
	b, _ := json.Marshal(v)
	raw := []byte(domain)
	raw = binary.LittleEndian.AppendUint32(raw, uint32(len(b)))
	raw = append(raw, b...)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func seedFixturePublicKey(checkpoint bool) string {
	domain := "SLUG candidate seed declaration fixture\x00v1\x00"
	if checkpoint {
		domain = "SLUG candidate seed checkpoint fixture\x00v1\x00"
	}
	seed := sha256.Sum256([]byte(domain))
	key := ed25519.NewKeyFromSeed(seed[:])
	return hex.EncodeToString(key.Public().(ed25519.PublicKey))
}
func seedEventKey(d gymseed.Declaration) string {
	b := []byte("SLUG candidate seed event\x00v1\x00")
	b = binary.LittleEndian.AppendUint32(b, d.Network)
	for _, s := range []string{d.Genesis, d.SourceStoreID, d.Event.EventID, d.PublisherPublicKey} {
		v, _ := hex.DecodeString(s)
		b = append(b, v...)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// These are bounded transport checks. They do not call core receipt validation,
// regenerate conditions or portraits, execute PoW, or contact a node.
func seedSignedShape(s gymseed.SignedDeclaration) bool {
	d := s.Declaration
	if s.Version != 1 || d.Version != 1 || !eventShape(d.Event) || d.SourceStoreID != d.Event.StoreID || d.Network != uint32(chaincfg.MainNetParams.Net) || d.Genesis != chaincfg.MainNetParams.GenesisHash.String() || d.FloorHeight != 0 || d.FloorHash != d.Genesis || !seedHex(s.Signature, 64) {
		return false
	}
	for _, id := range []string{d.EventKey, d.StoreID, d.SourceStoreID, d.SnapshotDigest, d.ReceiptDigest, d.PublisherChainID, d.RulesID, d.PublisherPublicKey, d.CheckpointPublicKey} {
		if !canonicalManifestID(id) {
			return false
		}
	}
	if d.PublisherPublicKey != seedFixturePublicKey(false) || seedEventKey(d) != d.EventKey {
		return false
	}
	raw, _ := json.Marshal(d)
	msg := []byte("SLUG candidate seed declaration\x00v1\x00")
	msg = binary.LittleEndian.AppendUint32(msg, uint32(len(raw)))
	msg = append(msg, raw...)
	key, _ := hex.DecodeString(d.PublisherPublicKey)
	sig, _ := hex.DecodeString(s.Signature)
	return ed25519.Verify(key, msg, sig)
}
func seedTipShape(t gymseed.Tip) bool {
	return t.Height <= math.MaxInt32 && canonicalManifestID(t.Hash)
}
func seedCursorShape(c gymseed.Cursor) bool {
	return c.Generation <= 16 && c.Height >= -1 && c.CompleteToHeight >= -1 && c.CompleteToHeight <= c.Height && canonicalManifestID(c.ScanDigest) && ((c.Height == -1 && c.Hash == "") || (c.Height >= 0 && canonicalManifestID(c.Hash))) && ((c.CompleteToHeight == -1 && c.CompleteToHash == "") || (c.CompleteToHeight >= 0 && canonicalManifestID(c.CompleteToHash))) && (c.CompleteToHeight != c.Height || c.CompleteToHash == c.Hash)
}
func seedCheckpointShape(c chaincfg.SignedCheckpoint, d gymseed.Declaration) bool {
	if c.Version != 1 || uint32(c.Network) != d.Network || c.GenesisHash.String() != d.Genesis || c.Height < 1 || len(c.Signature) != 64 {
		return false
	}
	key, _ := hex.DecodeString(d.CheckpointPublicKey)
	return c.Verify(&chaincfg.MainNetParams, key) == nil
}

func seedPublicationShape(p gymseed.Publication, d gymseed.Declaration, at int64) bool {
	if p.Version != 1 || !stableTime(p.BuiltAtMS) || p.BuiltAtMS < at || len(p.RawTx) > 2048 || len(p.RawTx) == 0 || !seedHex(p.RawTx, len(p.RawTx)/2) || !canonicalManifestID(p.TxID) || !canonicalManifestID(p.Coin.Outpoint.TxID) || !canonicalManifestID(p.Coin.BlockHash) || p.Coin.BlockHeight > math.MaxInt32 || p.Coin.ValueSatoshis < 1294 || p.Coin.ValueSatoshis > 2100000000000000 || p.Coin.Script != "0014751e76e8199196d454941c45d1b3a323f1433bd6" {
		return false
	}
	if p.DispatchCount == 0 {
		if p.State != "retained" || p.FirstDispatchAtMS != 0 || p.LastDispatchAtMS != 0 {
			return false
		}
	} else if (p.State != "unknown" && p.State != "submitted") || p.FirstDispatchAtMS < p.BuiltAtMS || p.LastDispatchAtMS < p.FirstDispatchAtMS || !stableTime(p.LastDispatchAtMS) {
		return false
	}
	raw, _ := hex.DecodeString(p.RawTx)
	r := bytes.NewReader(raw)
	var tx wire.MsgTx
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.TxHash().String() != p.TxID || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 {
		return false
	}
	in := tx.TxIn[0]
	if in.PreviousOutPoint.Hash.String() != p.Coin.Outpoint.TxID || in.PreviousOutPoint.Index != p.Coin.Outpoint.Index || in.Sequence != math.MaxUint32 || len(in.SignatureScript) != 0 || len(in.Witness) != 2 || len(in.Witness[0]) < 9 || len(in.Witness[0]) > 73 || in.Witness[0][len(in.Witness[0])-1] != 1 || hex.EncodeToString(in.Witness[1]) != "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798" {
		return false
	}
	carrier := []byte("SLUGSEED\x01")
	for _, x := range []string{d.EventKey, seedHash("SLUG candidate seed declaration id\x00v1\x00", d)} {
		b, _ := hex.DecodeString(x)
		carrier = append(carrier, b...)
	}
	script := append([]byte{0x6a, 73}, carrier...)
	fund, _ := hex.DecodeString(p.Coin.Script)
	var canonical bytes.Buffer
	if tx.Serialize(&canonical) != nil || !bytes.Equal(canonical.Bytes(), raw) || tx.TxOut[0].Value != 0 || !bytes.Equal(tx.TxOut[0].PkScript, script) || tx.TxOut[1].Value != p.Coin.ValueSatoshis-1000 || !bytes.Equal(tx.TxOut[1].PkScript, fund) {
		return false
	}
	fetch := txscript.NewCannedPrevOutputFetcher(fund, p.Coin.ValueSatoshis)
	hashes := txscript.NewTxSigHashes(&tx, fetch)
	vm, err := txscript.NewEngine(fund, &tx, 0, txscript.StandardVerifyFlags, nil, hashes, p.Coin.ValueSatoshis, fetch)
	return err == nil && vm.Execute() == nil
}
func seedRecordShape(r gymseed.Record) bool {
	d := r.Declaration.Declaration
	if r.Version != 1 || !canonicalManifestID(r.StoreID) || !canonicalManifestID(r.EventKey) || !eventRecordShape(r.SourceReceipt) || !seedSignedShape(r.Declaration) || d.RulesID != gymseed.RulesID() || d.CheckpointPublicKey != seedFixturePublicKey(true) || r.StoreID != d.StoreID || r.EventKey != d.EventKey || d.Event != r.SourceReceipt.Input.Event || d.SnapshotDigest != r.SourceReceipt.SnapshotDigest || d.ReceiptDigest != seedHash("SLUG candidate cutoff receipt\x00v1\x00", r.SourceReceipt) || !stableTime(r.PreparedAtMS) || r.PreparedAtMS < r.SourceReceipt.CompletedAtMS || !seedCursorShape(r.Cursor) || r.ScanRestarts == nil || len(r.ScanRestarts) > 16 || int(r.Cursor.Generation) != len(r.ScanRestarts) || r.Conflicts == nil || len(r.Conflicts) > 8 || r.ConflictOverflow && len(r.Conflicts) != 8 {
		return false
	}
	// Pin-free parsing still fixes the published candidate signer and rules. The
	// established native Pin supplies the remaining source/chain identity check.
	if r.Publication != nil && !seedPublicationShape(*r.Publication, d, r.PreparedAtMS) {
		return false
	}
	last := r.PreparedAtMS
	for i, x := range r.ScanRestarts {
		if x.Generation != uint32(i+1) || x.Previous.Generation != uint32(i) || !seedCursorShape(x.Previous) || x.Previous.Height < 0 || !stableTime(x.ObservedAtMS) || x.ObservedAtMS < last || !seedTipShape(x.ObservedTip) || (x.ObservedTip.Height < uint32(x.Previous.Height) && x.ReplacementHash != "") || (x.ObservedTip.Height >= uint32(x.Previous.Height) && (!canonicalManifestID(x.ReplacementHash) || x.ReplacementHash == x.Previous.Hash)) {
			return false
		}
		last = x.ObservedAtMS
	}
	if a := r.Anchor; a != nil {
		if a.Height > math.MaxInt32 || !canonicalManifestID(a.Hash) || !seedHex(a.Header, 80) || !canonicalManifestID(a.TxID) || !canonicalManifestID(a.RawTxDigest) || a.RawTxSize < 1 || a.RawTxSize > 4000000 || a.TxCount < 1 || a.TxCount > 1000000 || a.TxIndex >= a.TxCount || a.OutputIndex >= 4000000/9 || a.MerkleBranch == nil || len(a.MerkleBranch) > 20 || !seedHex(a.Payload, 73) || r.Cursor.Height != int32(a.Height) || r.Cursor.Hash != a.Hash || r.Cursor.CompleteToHeight != int32(a.Height) || r.Cursor.CompleteToHash != a.Hash {
			return false
		}
		for _, h := range a.MerkleBranch {
			if !canonicalManifestID(h) {
				return false
			}
		}
		want := "534c55475345454401" + d.EventKey + seedHash("SLUG candidate seed declaration id\x00v1\x00", d)
		if a.Payload != want || !seedAnchorMerkle(*a) {
			return false
		}
	}
	if t := r.Target; t != nil {
		if r.Anchor == nil || t.Height != r.Anchor.Height+1 || t.Height > math.MaxInt32 || !canonicalManifestID(t.Hash) || !seedHex(t.Header, 80) || !stableTime(t.ObservedAtMS) || t.ObservedAtMS < r.PreparedAtMS || !seedTipShape(t.ObservedTip) || t.ObservedTip.Height < t.Height {
			return false
		}
		b, _ := hex.DecodeString(t.Header)
		parent, _ := chainhash.NewHashFromStr(r.Anchor.Hash)
		if !bytes.Equal(b[4:36], parent[:]) {
			return false
		}
	}
	if c := r.Checkpoint; c != nil {
		if r.Target == nil || !seedCheckpointShape(*c, d) || c.Height != int32(r.Target.Height) || c.Hash.String() != r.Target.Hash {
			return false
		}
	}
	if f := r.Finality; f != nil {
		if r.Checkpoint == nil || r.Target == nil || !reflect.DeepEqual(f.Checkpoint, *r.Checkpoint) || !stableTime(f.ObservedAtMS) || f.ObservedAtMS < r.Target.ObservedAtMS || !seedTipShape(f.ObservedTip) || f.ObservedTip.Height < r.Target.Height || f.TargetDescendants != f.ObservedTip.Height-r.Target.Height || f.TargetDescendants < 60 {
			return false
		}
	}
	seen := map[string]bool{}
	last = r.PreparedAtMS
	for _, c := range r.Conflicts {
		if !stableTime(c.ObservedAtMS) || c.ObservedAtMS < last {
			return false
		}
		last = c.ObservedAtMS
		switch c.Kind {
		case "declaration":
			if c.Declaration == nil || c.Checkpoint != nil || c.ObservedTip != nil || c.ExpectedHeight != 0 || c.ExpectedHash != "" || c.ObservedHash != "" || !seedSignedShape(*c.Declaration) || c.Declaration.Declaration.EventKey != r.EventKey || c.Declaration.Declaration == d {
				return false
			}
		case "checkpoint":
			if c.Declaration != nil || c.Checkpoint == nil || c.ObservedTip != nil || c.ExpectedHeight != 0 || c.ExpectedHash != "" || c.ObservedHash != "" || r.Target == nil || !seedCheckpointShape(*c.Checkpoint, d) || c.Checkpoint.Height != int32(r.Target.Height) || c.Checkpoint.Hash.String() == r.Target.Hash {
				return false
			}
		case "chain":
			if c.Declaration != nil || c.Checkpoint != nil || c.ObservedTip == nil || !seedTipShape(*c.ObservedTip) || !canonicalManifestID(c.ExpectedHash) || c.ExpectedHeight > math.MaxInt32 {
				return false
			}
			if !(r.Anchor != nil && c.ExpectedHeight == r.Anchor.Height && c.ExpectedHash == r.Anchor.Hash) && !(r.Target != nil && c.ExpectedHeight == r.Target.Height && c.ExpectedHash == r.Target.Hash) {
				return false
			}
			if c.ObservedTip.Height < c.ExpectedHeight {
				if c.ObservedHash != "" {
					return false
				}
			} else if !canonicalManifestID(c.ObservedHash) || c.ObservedHash == c.ExpectedHash {
				return false
			}
		default:
			return false
		}
		c.ObservedAtMS = 0
		b, _ := json.Marshal(c)
		if seen[string(b)] {
			return false
		}
		seen[string(b)] = true
	}
	o := r.Observation
	if o.ObservedAtMS != 0 && (!stableTime(o.ObservedAtMS) || o.ObservedAtMS < r.PreparedAtMS) || o.ChainObservedAtMS < 0 || o.ChainObservedAtMS > o.ObservedAtMS || (o.Tip == nil) != (o.ChainObservedAtMS == 0) {
		return false
	}
	switch o.NodeState {
	case "unobserved":
		if o.ObservedAtMS != 0 || o.ChainObservedAtMS != 0 || o.Tip != nil || o.TargetDescendants != 0 || o.CurrentFinality {
			return false
		}
	case "online":
		if !stableTime(o.ObservedAtMS) || o.ChainObservedAtMS != o.ObservedAtMS || o.Tip == nil {
			return false
		}
	case "offline", "unavailable":
		if !stableTime(o.ObservedAtMS) {
			return false
		}
	default:
		return false
	}
	if o.Tip != nil && !seedTipShape(*o.Tip) {
		return false
	}
	var descendants uint32
	if r.Target != nil && o.Tip != nil && o.Tip.Height >= r.Target.Height {
		descendants = o.Tip.Height - r.Target.Height
	}
	if o.TargetDescendants != descendants {
		return false
	}
	if o.CurrentFinality && (r.Finality == nil || r.Checkpoint == nil || len(r.Conflicts) != 0 || r.ConflictOverflow || o.TargetDescendants < 60) {
		return false
	}
	return r.IncidentAtMS == 0 || stableTime(r.IncidentAtMS) && r.IncidentAtMS >= d.Event.IncidentDeadlineMS && r.IncidentAtMS >= r.PreparedAtMS
}
func seedStatusShape(s gymseed.Status) bool {
	if s.EventKey != seedEventKey(gymseed.Declaration{Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), SourceStoreID: s.Event.StoreID, Event: s.Event, PublisherPublicKey: seedFixturePublicKey(false)}) || s.Version != 1 || !canonicalManifestID(s.StoreID) || !canonicalManifestID(s.EventKey) || !eventShape(s.Event) || !stableTime(s.ObservedAtMS) || s.ScanHeight < -1 || s.ScanTipHeight < -1 || s.ScanGeneration > 16 || s.FundingGeneration > 16 || s.ConflictCount > 8 || s.ConflictOverflow && s.ConflictCount != 8 || s.ChainObservedAtMS < 0 || s.ChainObservedAtMS > s.ObservedAtMS {
		return false
	}
	switch s.FundingState {
	case "unobserved", "scanning", "ready", "waiting_for_funds", "unavailable":
	default:
		return false
	}
	switch s.NodeState {
	case "unobserved", "online", "offline", "unavailable":
	default:
		return false
	}
	switch s.PublicationState {
	case "none", "retained", "unknown", "submitted":
	default:
		return false
	}
	for _, h := range []string{s.AnchorHash, s.TargetHash} {
		if h != "" && !canonicalManifestID(h) {
			return false
		}
	}
	if s.EscalationDue != (s.ObservedAtMS >= s.Event.IncidentDeadlineMS && (s.FinalizedAtMS == 0 || s.ConflictCount > 0 || s.ConflictOverflow)) {
		return false
	}
	if !s.Exists {
		return s.Phase == "absent" && s.PublicationState == "none" && s.ScanHeight == -1 && s.ScanTipHeight == -1 && s.AnchorHeight == 0 && s.AnchorHash == "" && s.TargetHeight == 0 && s.TargetHash == "" && s.TargetDescendants == 0 && !s.CheckpointPresent && s.FinalizedAtMS == 0 && s.ConflictCount == 0 && !s.ConflictOverflow && s.IncidentAtMS == 0 && s.PreparedAtMS == 0 && s.ChainObservedAtMS == 0 && s.NodeState == "unobserved" && s.FundingState == "unobserved" && !s.CurrentFinality && s.ScanGeneration == 0 && s.FundingGeneration == 0
	}
	if !stableTime(s.PreparedAtMS) || s.PreparedAtMS < s.Event.CutoffMS || s.PreparedAtMS > s.ObservedAtMS || s.IncidentAtMS != 0 && (!stableTime(s.IncidentAtMS) || s.IncidentAtMS < s.Event.IncidentDeadlineMS || s.IncidentAtMS > s.ObservedAtMS) {
		return false
	}
	if s.AnchorHash == "" && s.AnchorHeight != 0 || s.TargetHash == "" && s.TargetHeight != 0 || s.TargetHash != "" && (s.AnchorHash == "" || s.TargetHeight != s.AnchorHeight+1) || s.CheckpointPresent && s.TargetHash == "" || s.CurrentFinality && (s.TargetDescendants < 60 || !s.CheckpointPresent || s.ConflictCount != 0 || s.ConflictOverflow || s.FinalizedAtMS == 0) {
		return false
	}
	phase := "prepared"
	if s.PublicationState != "none" {
		phase = "waiting_anchor"
	}
	if s.AnchorHash != "" {
		phase = "waiting_target"
	}
	if s.TargetHash != "" {
		phase = "provisional"
	}
	if s.FinalizedAtMS != 0 {
		if !stableTime(s.FinalizedAtMS) || s.FinalizedAtMS < s.PreparedAtMS || s.FinalizedAtMS > s.ObservedAtMS || !s.CheckpointPresent {
			return false
		}
		phase = "finalized"
	}
	if s.ConflictCount > 0 || s.ConflictOverflow {
		phase = "conflict"
	}
	return s.Phase == phase
}
func seedHistoryShape(h gymseed.HistoryPage) bool {
	if h.Version != 1 || !canonicalManifestID(h.StoreID) || !stableTime(h.ObservedAtMS) || h.AfterEventKey != "" && !canonicalManifestID(h.AfterEventKey) || h.Events == nil || len(h.Events) > 8 {
		return false
	}
	for i, s := range h.Events {
		if !seedStatusShape(s) || !s.Exists || s.StoreID != h.StoreID || s.ObservedAtMS != h.ObservedAtMS || s.EventKey == h.AfterEventKey || i > 0 && (h.Events[i-1].Event.CutoffMS > s.Event.CutoffMS || h.Events[i-1].Event.CutoffMS == s.Event.CutoffMS && h.Events[i-1].EventKey >= s.EventKey) {
			return false
		}
	}
	if h.HasMore {
		return len(h.Events) > 0 && h.NextEventKey == h.Events[len(h.Events)-1].EventKey
	}
	return h.NextEventKey == ""
}
func seedJobValid(j *SeedJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || !stableViewValid(j.View) || !canonicalManifestID(j.StoreID) || !canonicalManifestID(j.EventKey) || !entryDateValid(j.Saturday) {
		return false
	}
	switch j.Kind {
	case "prepare", "publish", "reconcile", "attest":
	default:
		return false
	}
	switch j.State {
	case "queued", "running":
		return j.Result == nil && j.Error == ""
	case "failed":
		return j.Result == nil && seedErrorCode(j.Error)
	case "cancelled":
		return j.Result == nil && j.Error == "seed_cancelled"
	case "completed":
		return j.Error == "" && j.Result != nil && seedRecordShape(*j.Result) && j.Result.StoreID == j.StoreID && j.Result.EventKey == j.EventKey && j.Result.SourceReceipt.Input.Event.Saturday == j.Saturday
	}
	return false
}
func seedResponseValid(r *SeedResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	n := 0
	for _, b := range []bool{r.Snapshot != nil, r.Status != nil, r.Record != nil, r.History != nil, r.Job != nil} {
		if b {
			n++
		}
	}
	if !r.OK {
		return n == 0 && r.View == nil && r.StoreID == "" && seedErrorCode(r.Error)
	}
	if n != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return seedSnapshotValid(*r.Snapshot) && reflect.DeepEqual(r.View, r.Snapshot.View) && r.StoreID == r.Snapshot.StoreID
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	switch {
	case r.Status != nil:
		return seedStatusShape(*r.Status) && r.Status.StoreID == r.StoreID
	case r.Record != nil:
		return seedRecordShape(*r.Record) && r.Record.StoreID == r.StoreID
	case r.History != nil:
		return seedHistoryShape(*r.History) && r.History.StoreID == r.StoreID
	case r.Job != nil:
		return seedJobValid(r.Job) && r.Job.StoreID == r.StoreID && r.Job.View == *r.View
	}
	return false
}

// Check the compact txid proof with SHA256d only; the authoritative action has
// already verified the full block and normal-PoW header before retaining it.
func seedAnchorMerkle(a gymseed.Anchor) bool {
	raw, err := hex.DecodeString(a.Header)
	if err != nil || len(raw) != 80 {
		return false
	}
	leaf, err := chainhash.NewHashFromStr(a.TxID)
	if err != nil {
		return false
	}
	count, index, used := a.TxCount, a.TxIndex, 0
	for count > 1 {
		if used >= len(a.MerkleBranch) {
			return false
		}
		sibling, err := chainhash.NewHashFromStr(a.MerkleBranch[used])
		if err != nil || index%2 == 0 && index+1 == count && *sibling != *leaf {
			return false
		}
		var pair [64]byte
		if index%2 == 0 {
			copy(pair[:32], leaf[:])
			copy(pair[32:], sibling[:])
		} else {
			copy(pair[:32], sibling[:])
			copy(pair[32:], leaf[:])
		}
		h := chainhash.DoubleHashH(pair[:])
		leaf = &h
		count = (count + 1) / 2
		index /= 2
		used++
	}
	return used == len(a.MerkleBranch) && bytes.Equal(leaf[:], raw[36:68])
}
