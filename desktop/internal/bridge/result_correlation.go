// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
)

// Only compact immutable commitments are retained; the selected transcript and
// the 24 MiB proof never enter this family's cache.
type resultSourceExpectation struct {
	set, exists                   bool
	record, input, result, source string
	created                       int64
}
type resultCommitments struct {
	statement, txid, raw, checkpoint, finality string
	finalizedMS                                int64
}
type resultSelection struct {
	date, bout string
	known      resultCommitments
}

func resultSourceFrom(s gymbout.Status) resultSourceExpectation {
	v := s.Saved
	return resultSourceExpectation{true, v.Exists, v.RecordDigest, v.InputID, v.ResultDigest, v.SourceDigest, v.CreatedAtMS}
}
func (e resultSourceExpectation) matches(s gymbout.Status) bool {
	if !e.set {
		return true
	}
	return e == resultSourceFrom(s)
}
func resultDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func resultCommitment(r gymresult.Record) resultCommitments {
	id, _ := gymresult.StatementID(r.Statement.Statement)
	v := resultCommitments{statement: id}
	if r.Publication != nil {
		v.txid = r.Publication.TxID
		v.raw = resultDigest(r.Publication.RawTx)
	}
	if r.Checkpoint != nil {
		v.checkpoint = resultDigest(r.Checkpoint)
	}
	if r.Finality != nil {
		v.finality = resultDigest(r.Finality)
		v.finalizedMS = r.Finality.ObservedAtMS
	}
	return v
}
func (k resultCommitments) matchesStatus(s gymresult.Status) bool {
	return (k.txid == "" || s.TxID == k.txid) && (k.checkpoint == "" || s.CheckpointPresent) && (k.finality == "" || s.FinalizedAtMS == k.finalizedMS)
}
func (k resultCommitments) matches(r gymresult.Record) bool {
	n := resultCommitment(r)
	return (k.statement == "" || n.statement == k.statement) && (k.txid == "" || n.txid == k.txid && n.raw == k.raw) && (k.checkpoint == "" || n.checkpoint == k.checkpoint) && (k.finality == "" || n.finality == k.finality)
}
func (b *StableBridge) resultKnown(date, bid string) resultCommitments {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.result != nil && b.result.selected != nil && b.result.selected.date == date && b.result.selected.bout == bid {
		return b.result.selected.known
	}
	return resultCommitments{}
}
func (b *StableBridge) rememberResultSelection(v pilot.ManifestViewPin, r gymresult.Record, fromJob bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.result == nil || b.result.view != v {
		return
	}
	date, bid := r.Statement.Statement.Source.Source.Event.Saturday, r.Statement.Statement.Source.Source.Bout.BoutID
	if fromJob && b.result.selected != nil {
		// A retained terminal stays immutable even after an explicit read learns
		// more. Accept that old job without forgetting this selection's first
		// transaction, certificate, or finality commitments.
		if b.result.selected.date != date || b.result.selected.bout != bid || !b.result.selected.known.matches(r) {
			return
		}
	}
	b.result.selected = &resultSelection{date, bid, resultCommitment(r)}
}
