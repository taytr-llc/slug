// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Closed transport validation: no source regeneration, engine, PoW, Merkle
// verification, signing, store or RPC operation is reachable from this file.
const resultPublisherPublicKey = "984ebcc3fd73fcf64dd8ebb6a03b01a6ed2403548a479ce3089dca1392841bcf"
const resultCheckpointPublicKey = "aee7dd24b6a03c5a02f452f5f588c8ca7e1dba000d9e21a736993e5842b30ca4"
const resultFundingScript = "001406afd46bcdfd22ef94ac122aa11f241244a37ecc"
const resultGenesis = "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"

func resultErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown", "result_unavailable", "result_corrupt", "result_capacity", "result_clock", "result_calendar", "result_not_found", "result_not_prepared", "result_source_incomplete", "result_source_changed", "result_source_conflict", "result_node_offline", "result_waiting_for_funds", "result_scan_incomplete", "result_not_mature", "result_conflict", "result_cancelled", "result_proof_unavailable", "result_setup_incomplete", "invalid_frame")
}
func resultPathValid(s string) bool {
	return len(s) > 0 && len(s) <= 1024 && utf8.ValidString(s) && filepath.IsAbs(s) && !strings.ContainsAny(s, "\x00\r\n\t")
}
func resultRequestValid(r ResultRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	b := r
	b.ID = 0
	b.Command = ""
	switch r.Command {
	case "result_snapshot":
	case "result_status", "result_record", "result_publish", "result_reconcile", "result_attest", "result_export":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) || !canonicalManifestID(r.BoutID) {
			return false
		}
		b.ViewID = ""
		b.Saturday = ""
		b.BoutID = ""
		if r.Command == "result_export" {
			if !resultPathValid(r.Path) {
				return false
			}
			b.Path = ""
		}
	case "result_job", "result_cancel":
		if !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) {
			return false
		}
		b.ViewID = ""
		b.JobID = ""
	default:
		return false
	}
	return b == (ResultRequest{})
}
func resultSnapshotValid(s ResultSnapshot) bool {
	if s.Version != 1 || !(s.NodeState == "offline" && s.NodeOwner == "none" || s.NodeState == "starting" && s.NodeOwner == "publication" || s.NodeState == "online" && stableOne(s.NodeOwner, "publication", "mining", "shared_pool")) {
		return false
	}
	blank := s.Pin == nil && s.StoreID == "" && s.PublisherAccountID == "" && s.SourceCreatedAtMS == 0 && s.CreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == "" && s.NodeState == "offline"
	if s.State == "unavailable" {
		return blank && s.View == nil && s.Error == "result_unavailable"
	}
	if s.View == nil || !stableViewValid(*s.View) {
		return false
	}
	if s.State == "setup_incomplete" {
		return blank && s.Error == "result_setup_incomplete"
	}
	if s.Pin == nil || !stableTime(s.CreatedAtMS) || !stableTime(s.SourceCreatedAtMS) || s.SourceCreatedAtMS > s.CreatedAtMS {
		return false
	}
	p, e := gymresult.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	id, e := gymresult.ID(p)
	if e != nil || id != s.StoreID || s.PublisherAccountID != p.PublisherAccountID {
		return false
	}
	if s.NodeOwner == "shared_pool" {
		for _, entry := range p.SourcePin.SourcePin.SourcePin.SourcePin.Participants {
			if entry.StablePin.Version != gymstable.SharedPinVersion2 || entry.StablePin.Source != s.View.AuthorityPin || entry.StablePin.AccountAuthorityID != s.View.AccountAuthorityID {
				return false
			}
		}
	}
	if _, ok := eventViewParticipant(*s.View, p.SourcePin.SourcePin.SourcePin.SourcePin); !ok {
		return false
	}
	switch s.State {
	case "ready", "busy":
		if s.Error != "" {
			return false
		}
	case "failed":
		if !stableOne(s.Error, "result_unavailable", "result_corrupt", "result_clock", "result_source_changed") || s.ActiveJobID != "" || s.QueuedJobID != "" {
			return false
		}
	default:
		return false
	}
	seen := map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			if !canonicalManifestID(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	return !(s.ActiveJobID != "" && s.QueuedJobID != "") && (s.ActiveJobID == "" && s.QueuedJobID == "" || s.State == "busy")
}
func resultKeyFor(p gymresult.Pin, card, bid string) string {
	k, _ := gymresult.ResultKey(p, card, bid)
	return k
}
func resultRaw(s string) []byte      { b, _ := hex.DecodeString(s); return b }
func resultEqual(a, b any) bool      { return reflect.DeepEqual(a, b) }
func resultFits(v any, n int) bool   { b, e := json.Marshal(v); return e == nil && len(b) <= n }
func resultOptionalID(s string) bool { return s == "" || canonicalManifestID(s) }
func resultHashParts(d string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(d))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func resultLE(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func resultStatementKey(v gymresult.Statement) string {
	return resultHashParts("SLUG candidate result key\x00v1\x00", resultLE(v.Network), resultRaw(v.Genesis), resultRaw(v.PublisherChainID), resultRaw(v.SourceStoreID), resultRaw(v.Source.Source.CardID), resultRaw(v.Source.Source.Bout.BoutID), resultRaw(v.PublisherPublicKey))
}
func resultSourceShape(s gymbout.ResultSource) bool {
	if s.Version != 1 || !canonicalManifestID(s.StoreID) || !stableTime(s.StoreCreatedAtMS) || !stableTime(s.CreatedAtMS) || s.CreatedAtMS < max(s.StoreCreatedAtMS, s.Source.CardCreatedAtMS) || !boutSourceShape(s.Source) || s.SourceDigest != boutSourceHash(s.Source) {
		return false
	}
	for _, v := range []string{s.RecordDigest, s.InputID, s.ResultDigest} {
		if !canonicalManifestID(v) {
			return false
		}
	}
	q := s.Summary
	draw := stableOne(q.Method, "decision_draw", "double_corner_stoppage")
	return stableOne(q.Method, "decision", "decision_draw", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner", "double_corner_stoppage") && q.EndRound >= 1 && q.EndRound <= 3 && q.EndTick >= 1 && q.EndTick <= 60 && (q.Winner == "") == draw && (draw || q.Winner == s.Source.Bout.CornerA || q.Winner == s.Source.Bout.CornerB)
}
func resultSignedStatementShape(v gymresult.SignedStatement) bool {
	s := v.Statement
	if v.Version != 1 || s.Version != 1 || !resultSourceShape(s.Source) || !canonicalManifestID(s.StoreID) || s.SourceStoreID != s.Source.StoreID || !canonicalManifestID(s.PublisherChainID) || s.Genesis != resultGenesis || s.Network != uint32(wire.MainNet) || s.RulesID != gymresult.RulesID() || s.PublisherPublicKey != resultPublisherPublicKey || s.CheckpointPublicKey != resultCheckpointPublicKey || s.ResultKey != resultStatementKey(s) || !seedHex(v.Signature, 64) {
		return false
	}
	raw, _ := json.Marshal(s)
	msg := append([]byte("SLUG candidate result statement\x00v1\x00"), binary.LittleEndian.AppendUint32(nil, uint32(len(raw)))...)
	msg = append(msg, raw...)
	return ed25519.Verify(resultRaw(resultPublisherPublicKey), msg, resultRaw(v.Signature))
}
func resultCheckpointShape(c chaincfg.SignedCheckpoint) bool {
	if c.Network != wire.MainNet || c.GenesisHash.String() != resultGenesis || c.Height <= 0 || len(c.Signature) != 64 {
		return false
	}
	p := chaincfg.MainNetParams
	return c.Verify(&p, resultRaw(resultCheckpointPublicKey)) == nil
}
func resultPublicationShape(v gymresult.Publication) bool {
	c := v.Coin
	if v.Version != 1 || !seedHex(v.RawTx, len(v.RawTx)/2) || len(v.RawTx) < 2 || len(v.RawTx) > 2048 || !canonicalManifestID(v.TxID) || !canonicalManifestID(c.Outpoint.TxID) || c.ValueSatoshis < 1294 || c.ValueSatoshis > 2100000000000000 || c.Script != resultFundingScript || c.BlockHeight > math.MaxInt32 || !canonicalManifestID(c.BlockHash) || !stableTime(v.BuiltAtMS) {
		return false
	}
	if v.DispatchCount == 0 {
		return v.State == "built" && v.FirstDispatchAtMS == 0 && v.LastDispatchAtMS == 0
	}
	return stableOne(v.State, "submitted", "dispatch_unknown") && stableTime(v.FirstDispatchAtMS) && stableTime(v.LastDispatchAtMS) && v.FirstDispatchAtMS >= v.BuiltAtMS && v.LastDispatchAtMS >= v.FirstDispatchAtMS
}
func resultAnchorShape(a gymresult.Anchor) bool {
	if a.Height < 1 || a.Height > math.MaxInt32 || !canonicalManifestID(a.Hash) || !canonicalManifestID(a.TxID) || !canonicalManifestID(a.RawTxDigest) || a.RawTxSize == 0 || a.RawTxSize > 1024 || a.TxCount == 0 || a.TxCount > 400000 || a.TxIndex >= a.TxCount || a.OutputIndex != 0 || !seedHex(a.Header, 80) || !seedHex(a.Payload, 73) || a.MerkleBranch == nil || len(a.MerkleBranch) > 20 {
		return false
	}
	p := resultRaw(a.Payload)
	if !bytes.Equal(p[:8], []byte("SLUGRSLT")) || p[8] != 1 || bytes.Equal(p[9:41], make([]byte, 32)) || bytes.Equal(p[41:], make([]byte, 32)) {
		return false
	}
	for _, h := range a.MerkleBranch {
		if !canonicalManifestID(h) {
			return false
		}
	}
	return true
}
func resultTipShape(v gymresult.Tip) bool {
	return v.Height <= math.MaxInt32 && canonicalManifestID(v.Hash)
}
func resultInclusionShape(v gymresult.Inclusion) bool {
	return v.Version == 1 && v.Generation <= 16 && resultAnchorShape(v.Anchor) && stableTime(v.ObservedAtMS) && resultTipShape(v.ObservedTip) && v.ObservedTip.Height >= v.Anchor.Height && (v.ObservedTip.Height != v.Anchor.Height || v.ObservedTip.Hash == v.Anchor.Hash)
}
func resultReplacement(h uint32, hash string, t gymresult.Tip, replacement string) bool {
	return resultOptionalID(replacement) && replacement != hash && (replacement == "") == (t.Height < h)
}
func resultRecordHash(r gymresult.Record) string {
	return seedHash("SLUG candidate result record\x00v1\x00", r)
}
func resultCursorInitial(r gymresult.Record, g uint32) gymresult.Cursor {
	return gymresult.Cursor{Generation: g, Height: -1, CompleteToHeight: -1, ScanDigest: resultHashParts("SLUG candidate result inclusion scan\x00v1\x00", resultRaw(r.StoreID), resultRaw(r.ResultKey), resultLE(g), resultLE(0), resultRaw(resultGenesis))}
}
func resultRecordShape(r gymresult.Record) bool {
	st := r.Statement.Statement
	if r.Version != 1 || !canonicalManifestID(r.StoreID) || r.StoreID != st.StoreID || r.ResultKey != st.ResultKey || !resultSignedStatementShape(r.Statement) || !stableTime(r.PreparedAtMS) || r.PreparedAtMS < st.Source.CreatedAtMS {
		return false
	}
	times := []int64{r.PreparedAtMS}
	payload, e := gymresult.Carrier(st)
	if e != nil {
		return false
	}
	matches := func(a gymresult.Anchor) bool {
		return r.Publication != nil && a.TxID == r.Publication.TxID && a.Payload == hex.EncodeToString(payload[:]) && a.RawTxDigest == boutRawHash(resultRaw(r.Publication.RawTx)) && a.RawTxSize == uint32(len(r.Publication.RawTx)/2)
	}
	if r.Publication != nil {
		if !resultPublicationShape(*r.Publication) || r.Publication.BuiltAtMS < r.PreparedAtMS {
			return false
		}
		times = append(times, r.Publication.BuiltAtMS, r.Publication.LastDispatchAtMS)
	}
	if !seedCursorShape(r.Cursor) || r.Cursor.Generation > 16 || r.ScanRestarts == nil || len(r.ScanRestarts) > 16 || int(r.Cursor.Generation) != len(r.ScanRestarts) {
		return false
	}
	if r.Publication == nil && (!resultEqual(r.Cursor, resultCursorInitial(r, 0)) || len(r.ScanRestarts) != 0 || len(r.PriorInclusions) != 0) {
		return false
	}
	if r.Cursor.Height < 0 && !resultEqual(r.Cursor, resultCursorInitial(r, r.Cursor.Generation)) {
		return false
	}
	for i, x := range r.ScanRestarts {
		if x.Generation != uint32(i+1) || x.Previous.Generation != uint32(i) || !seedCursorShape(x.Previous) || x.Previous.Height < 0 || !stableTime(x.ObservedAtMS) || !resultTipShape(x.ObservedTip) || !resultReplacement(uint32(x.Previous.Height), x.Previous.Hash, x.ObservedTip, x.ReplacementHash) {
			return false
		}
		times = append(times, x.ObservedAtMS)
	}
	if r.Inclusion != nil {
		if !resultInclusionShape(*r.Inclusion) || !matches(r.Inclusion.Anchor) {
			return false
		}
		times = append(times, r.Inclusion.ObservedAtMS)
	}
	if r.PriorInclusions == nil || len(r.PriorInclusions) > 16 {
		return false
	}
	for i, x := range r.PriorInclusions {
		if x.Version != 1 || !resultInclusionShape(x.Inclusion) || x.Inclusion.Generation != uint32(i) || !matches(x.Inclusion.Anchor) || !stableTime(x.OrphanedAtMS) || x.OrphanedAtMS < x.Inclusion.ObservedAtMS || !resultTipShape(x.ObservedTip) || !resultReplacement(x.Inclusion.Anchor.Height, x.Inclusion.Anchor.Hash, x.ObservedTip, x.ReplacementHash) {
			return false
		}
		times = append(times, x.Inclusion.ObservedAtMS, x.OrphanedAtMS)
	}
	if r.Inclusion != nil && r.Inclusion.Generation != uint32(len(r.PriorInclusions)) {
		return false
	}
	if r.Checkpoint != nil {
		if r.Inclusion == nil || !resultCheckpointShape(*r.Checkpoint) || uint32(r.Checkpoint.Height) != r.Inclusion.Anchor.Height || r.Checkpoint.Hash.String() != r.Inclusion.Anchor.Hash {
			return false
		}
	}
	if f := r.Finality; f != nil {
		if r.Checkpoint == nil || r.Inclusion == nil || !resultEqual(f.Checkpoint, *r.Checkpoint) || !resultEqual(f.Inclusion, *r.Inclusion) || !stableTime(f.ObservedAtMS) || f.ObservedAtMS < f.Inclusion.ObservedAtMS || !resultTipShape(f.ObservedTip) || f.ObservedTip.Height < f.Inclusion.Anchor.Height || f.Descendants < 60 || f.Descendants != f.ObservedTip.Height-f.Inclusion.Anchor.Height {
			return false
		}
		times = append(times, f.ObservedAtMS)
	}
	if r.Equivocation != nil && (!resultSignedStatementShape(*r.Equivocation) || r.Equivocation.Statement.ResultKey != r.ResultKey || resultEqual(r.Equivocation.Statement, st)) {
		return false
	}
	if r.Conflicts == nil || len(r.Conflicts) > 8 || r.ConflictOverflow && len(r.Conflicts) != 8 && r.Equivocation == nil {
		return false
	}
	seen := map[string]bool{}
	for _, x := range r.Conflicts {
		if !stableTime(x.ObservedAtMS) || x.ExpectedHeight > math.MaxInt32 || !resultOptionalID(x.ExpectedHash) || !resultOptionalID(x.ObservedHash) || x.ObservedTip != nil && !resultTipShape(*x.ObservedTip) || x.Checkpoint != nil && !resultCheckpointShape(*x.Checkpoint) {
			return false
		}
		key := x
		key.ObservedAtMS = 0
		b, _ := json.Marshal(key)
		if seen[string(b)] {
			return false
		}
		seen[string(b)] = true
		times = append(times, x.ObservedAtMS)
		switch x.Kind {
		case "source_chain_conflict", "checkpoint_chain_conflict":
			if x.ObservedTip == nil || x.Checkpoint != nil || !canonicalManifestID(x.ExpectedHash) || !resultReplacement(x.ExpectedHeight, x.ExpectedHash, *x.ObservedTip, x.ObservedHash) {
				return false
			}
			if x.Kind == "source_chain_conflict" {
				if x.ExpectedHeight != st.Source.Source.TargetHeight || x.ExpectedHash != st.Source.Source.TargetHash {
					return false
				}
			} else if r.Checkpoint == nil || r.Inclusion == nil || x.ExpectedHeight != r.Inclusion.Anchor.Height || x.ExpectedHash != r.Inclusion.Anchor.Hash {
				return false
			}
		case "checkpoint_equivocation":
			if x.Checkpoint == nil || r.Inclusion == nil || x.ExpectedHeight != r.Inclusion.Anchor.Height || x.ExpectedHash != r.Inclusion.Anchor.Hash || uint32(x.Checkpoint.Height) != x.ExpectedHeight || x.Checkpoint.Hash.String() != x.ObservedHash || x.ObservedHash == x.ExpectedHash {
				return false
			}
		case "statement_equivocation":
			if x.Checkpoint != nil || r.Equivocation == nil {
				return false
			}
			a, _ := gymresult.StatementID(st)
			b, _ := gymresult.StatementID(r.Equivocation.Statement)
			if x.ExpectedHash != a || x.ObservedHash != b {
				return false
			}
		default:
			return false
		}
	}
	ob := r.Observation
	if ob.ObservedAtMS != 0 && !stableTime(ob.ObservedAtMS) || ob.ChainObservedAtMS != 0 && !stableTime(ob.ChainObservedAtMS) || ob.ChainObservedAtMS > ob.ObservedAtMS || ob.Descendants > math.MaxInt32 || !stableOne(ob.NodeState, "unknown", "online", "offline") {
		return false
	}
	if ob.Tip == nil {
		if ob.ChainObservedAtMS != 0 || ob.Descendants != 0 || ob.CurrentFinality {
			return false
		}
	} else if !resultTipShape(*ob.Tip) {
		return false
	}
	for _, t := range times {
		if t != 0 && (t < r.PreparedAtMS || ob.ObservedAtMS != 0 && t > ob.ObservedAtMS) {
			return false
		}
	}
	if ob.ObservedAtMS == 0 && (r.Publication != nil || len(r.ScanRestarts) != 0 || r.Inclusion != nil || len(r.Conflicts) != 0 || r.Equivocation != nil) {
		return false
	}
	if ob.CurrentFinality && (ob.ChainObservedAtMS == 0 || ob.NodeState != "online" || ob.Descendants < 60 || r.Finality == nil || len(r.Conflicts) != 0 || r.ConflictOverflow || r.Equivocation != nil || r.Inclusion == nil || ob.Tip == nil || ob.Tip.Height < r.Inclusion.Anchor.Height || ob.Tip.Height-r.Inclusion.Anchor.Height != ob.Descendants) {
		return false
	}
	return resultFits(r, gymresult.MaxRecordSize)
}
func resultStatusShape(s gymresult.Status) bool {
	if s.Version != 1 || !canonicalManifestID(s.StoreID) || !canonicalManifestID(s.ResultKey) || !boutStatusShape(s.Source) || s.CardID != s.Source.CardID || s.BoutID != s.Source.Saved.Bout.BoutID || s.Event != s.Source.Source.Event || s.ObservedAtMS != s.Source.ObservedAtMS || !stableOne(s.Phase, "absent", "prepared", "provisional", "finalized", "conflict") || !stableOne(s.PublicationState, "none", "built", "dispatch_unknown", "submitted") || !stableOne(s.NodeState, "unknown", "online", "offline") || !stableOne(s.FundingState, "unknown", "scanning", "available", "waiting_for_funds") {
		return false
	}
	for _, v := range []string{s.RecordDigest, s.SourceRecordDigest, s.InputID, s.ResultDigest, s.TxID, s.InclusionHash} {
		if !resultOptionalID(v) {
			return false
		}
	}
	for _, t := range []int64{s.PreparedAtMS, s.FinalizedAtMS, s.ChainObservedAtMS} {
		if t < 0 || t > s.ObservedAtMS {
			return false
		}
	}
	if s.ScanHeight < -1 || s.ScanTipHeight < -1 || s.ScanGeneration > 16 || s.InclusionGeneration > 16 || s.PriorInclusionCount > 16 || s.InclusionHeight > math.MaxInt32 || s.Descendants > math.MaxInt32 || s.ConflictCount > 8 || (s.PublicationState == "none") != (s.TxID == "") || (s.InclusionHeight == 0) != (s.InclusionHash == "") {
		return false
	}
	if !s.Exists {
		if s.Phase != "absent" || s.PublicationState != "none" || s.ScanHeight != -1 || s.ScanTipHeight != -1 || s.RecordDigest != "" || s.SourceRecordDigest != "" || s.InputID != "" || s.ResultDigest != "" || s.TxID != "" || s.InclusionHash != "" || s.PreparedAtMS != 0 || s.ScanGeneration != 0 || s.InclusionHeight != 0 || s.InclusionGeneration != 0 || s.PriorInclusionCount != 0 || s.Descendants != 0 || s.FinalizedAtMS != 0 || s.ConflictCount != 0 || s.ChainObservedAtMS != 0 || s.CheckpointPresent || s.ConflictOverflow || s.CurrentFinality {
			return false
		}
	} else {
		saved := s.Source.Saved
		if !canonicalManifestID(s.RecordDigest) || !canonicalManifestID(s.SourceRecordDigest) || !canonicalManifestID(s.InputID) || !canonicalManifestID(s.ResultDigest) || !stableTime(s.PreparedAtMS) || s.Phase == "absent" || !saved.Exists || s.SourceRecordDigest != saved.RecordDigest || s.InputID != saved.InputID || s.ResultDigest != saved.ResultDigest || s.PreparedAtMS < saved.CreatedAtMS {
			return false
		}
	}
	if s.FinalizedAtMS != 0 && (!s.CheckpointPresent || s.InclusionHeight == 0 || !stableOne(s.Phase, "finalized", "conflict")) {
		return false
	}
	if s.Phase == "finalized" && s.FinalizedAtMS == 0 || s.Phase == "prepared" && (!stableOne(s.PublicationState, "none", "built") || s.InclusionHeight != 0 || s.FinalizedAtMS != 0) || s.Phase == "provisional" && !stableOne(s.PublicationState, "dispatch_unknown", "submitted") && s.InclusionHeight == 0 {
		return false
	}
	if s.CurrentFinality && (s.FinalizedAtMS == 0 || s.Descendants < 60 || s.ChainObservedAtMS == 0 || s.NodeState != "online" || int64(s.ScanTipHeight)-int64(s.InclusionHeight) != int64(s.Descendants)) {
		return false
	}
	if (s.ConflictCount != 0 || s.ConflictOverflow) && (s.CurrentFinality || s.Phase != "conflict") || s.Exists && !s.Source.Saved.Usable && s.Phase != "conflict" {
		return false
	}
	return resultFits(s, gymresult.MaxStatusSize)
}
func resultReadShape(r gymresult.Record, s gymresult.Status) bool {
	if !resultRecordShape(r) || !resultStatusShape(s) || !s.Exists || s.StoreID != r.StoreID || s.ResultKey != r.ResultKey || s.RecordDigest != resultRecordHash(r) || s.PreparedAtMS != r.PreparedAtMS || r.PreparedAtMS > s.ObservedAtMS || r.Observation.ObservedAtMS > s.ObservedAtMS {
		return false
	}
	src := r.Statement.Statement.Source
	b := s.Source.Saved
	c := src.Source
	state := s.Source.Source
	if s.SourceRecordDigest != src.RecordDigest || s.InputID != src.InputID || s.ResultDigest != src.ResultDigest || s.Source.StoreID != src.StoreID || b.SourceDigest != src.SourceDigest || b.CreatedAtMS != src.CreatedAtMS || b.Bout != c.Bout || b.Winner != src.Summary.Winner || b.Method != src.Summary.Method || b.EndRound != src.Summary.EndRound || b.EndTick != src.Summary.EndTick || state.CardID != c.CardID || state.CardRecordDigest != c.CardRecordDigest || state.CardSourceDigest != c.CardSourceDigest || state.CardCreatedAtMS != c.CardCreatedAtMS || state.Event != c.Event || state.FinalizedAtMS != c.FinalizedAtMS {
		return false
	}
	phase := "prepared"
	if r.Inclusion != nil || r.Publication != nil && r.Publication.DispatchCount > 0 {
		phase = "provisional"
	}
	if r.Finality != nil {
		phase = "finalized"
	}
	if len(r.Conflicts) > 0 || r.ConflictOverflow || r.Equivocation != nil || !b.Usable {
		phase = "conflict"
	}
	pub, tx := "none", ""
	if r.Publication != nil {
		pub = r.Publication.State
		tx = r.Publication.TxID
	}
	var height, generation uint32
	var hash string
	if r.Inclusion != nil {
		height = r.Inclusion.Anchor.Height
		hash = r.Inclusion.Anchor.Hash
		generation = r.Inclusion.Generation
	}
	var finalized int64
	if r.Finality != nil {
		finalized = r.Finality.ObservedAtMS
	}
	tip := int32(-1)
	if r.Observation.Tip != nil {
		tip = int32(r.Observation.Tip.Height)
	}
	return s.Phase == phase && s.PublicationState == pub && s.TxID == tx && s.ScanHeight == r.Cursor.Height && s.ScanTipHeight == tip && s.ScanGeneration == r.Cursor.Generation && s.InclusionHeight == height && s.InclusionHash == hash && s.InclusionGeneration == generation && s.PriorInclusionCount == uint32(len(r.PriorInclusions)) && s.Descendants == r.Observation.Descendants && s.CheckpointPresent == (r.Checkpoint != nil) && s.FinalizedAtMS == finalized && s.ConflictCount == uint32(len(r.Conflicts)) && s.ConflictOverflow == r.ConflictOverflow && s.ChainObservedAtMS == r.Observation.ChainObservedAtMS && s.NodeState == r.Observation.NodeState && s.CurrentFinality == r.Observation.CurrentFinality
}
func resultProofSummaryShape(s gymresult.ProofSummary) bool {
	if s.Version != 1 || s.Bytes == 0 || s.Bytes > 24*1024*1024 || s.HeaderCount == 0 || s.HeaderCount > 512 || s.BlockCount == 0 || s.BlockCount > 3 || s.FinalizedAtMS != 0 && !stableTime(s.FinalizedAtMS) || !resultTipShape(s.ObservedTip) {
		return false
	}
	for _, h := range []string{s.ResultKey, s.CardID, s.BoutID, s.RecordDigest, s.SourceRecordDigest, s.InputID, s.ResultDigest, s.TxID, s.SHA256} {
		if !canonicalManifestID(h) {
			return false
		}
	}
	return resultFits(s, 2048)
}
func resultJobValid(j *ResultJob) bool {
	if j.Version != 1 || !stableViewValid(j.View) || !stableOne(j.Kind, "publish", "reconcile", "attest", "export") || !entryDateValid(j.Saturday) {
		return false
	}
	for _, h := range []string{j.JobID, j.StoreID, j.CardID, j.BoutID, j.ResultKey} {
		if !canonicalManifestID(h) {
			return false
		}
	}
	switch j.State {
	case "queued", "running":
		return j.Error == "" && j.Result == nil && j.Export == nil
	case "failed", "cancelled":
		return j.Result == nil && j.Export == nil && j.Error != "invalid_frame" && resultErrorCode(j.Error) && (j.State != "cancelled" || j.Error == "result_cancelled")
	case "completed":
		if j.Error != "" {
			return false
		}
		if j.Kind == "export" {
			return j.Result == nil && j.Export != nil && resultProofSummaryShape(*j.Export) && j.Export.ResultKey == j.ResultKey && j.Export.CardID == j.CardID && j.Export.BoutID == j.BoutID
		}
		if j.Export != nil || j.Result == nil || j.Result.Version != 1 || !resultReadShape(j.Result.Record, j.Result.Status) {
			return false
		}
		r, s := j.Result.Record, j.Result.Status
		if s.StoreID != j.StoreID || s.CardID != j.CardID || s.BoutID != j.BoutID || s.ResultKey != j.ResultKey || s.Event.Saturday != j.Saturday {
			return false
		}
		if j.Kind == "publish" && (r.Publication == nil || r.Publication.State != "submitted" && r.Inclusion == nil) {
			return false
		}
		if j.Kind == "attest" && r.Finality == nil {
			return false
		}
		return !stableOne(j.Kind, "publish", "attest") || s.Source.Saved.Usable
	}
	return false
}
func resultResponseValid(r *ResultResponse) bool {
	if r.ID > gymstable.MaxSafeInteger {
		return false
	}
	n := 0
	for _, v := range []bool{r.Snapshot != nil, r.Status != nil, r.Record != nil, r.Job != nil} {
		if v {
			n++
		}
	}
	if !r.OK {
		return n == 0 && r.View == nil && r.StoreID == "" && resultErrorCode(r.Error) && (r.ID != 0 || r.Error == "invalid_frame")
	}
	if r.ID == 0 || n != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return resultSnapshotValid(*r.Snapshot) && reflect.DeepEqual(r.View, r.Snapshot.View) && r.StoreID == r.Snapshot.StoreID
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	if r.Status != nil {
		return resultStatusShape(*r.Status) && r.Status.StoreID == r.StoreID
	}
	if r.Record != nil {
		return r.Record.Version == 1 && resultReadShape(r.Record.Record, r.Record.Status) && r.Record.Record.StoreID == r.StoreID
	}
	return resultJobValid(r.Job) && r.Job.View == *r.View && r.Job.StoreID == r.StoreID
}
