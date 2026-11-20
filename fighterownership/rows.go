// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
)

type HeaderRow struct {
	Version uint32 `json:"version"`
	PinID   string `json:"pin_id"`
	Height  uint32 `json:"height"`
	Hash    string `json:"hash"`
	Header  string `json:"header"`
}
type TransactionWitness struct {
	Height           uint32   `json:"height"`
	BlockHash        string   `json:"block_hash"`
	TransactionIndex uint32   `json:"transaction_index"`
	TransactionCount uint32   `json:"transaction_count"`
	TxID             string   `json:"txid"`
	Raw              string   `json:"raw"`
	MerkleBranch     []string `json:"merkle_branch"`
}
type FundingPrevout struct {
	TxID   string `json:"txid"`
	Index  uint32 `json:"index"`
	Atoms  int64  `json:"atoms"`
	Script string `json:"script"`
}
type CarrierWitness struct {
	Version         uint32             `json:"version"`
	PinID           string             `json:"pin_id"`
	TransferID      string             `json:"transfer_id"`
	PriorStateID    string             `json:"prior_state_id"`
	Commit          TransactionWitness `json:"commit"`
	Spend           TransactionWitness `json:"spend"`
	FundingPrevouts []FundingPrevout   `json:"funding_prevouts"`
	Envelope        string             `json:"envelope"`
	Kind            string             `json:"kind"`
}
type ConflictRecord struct {
	Version               uint32            `json:"version"`
	PinID                 string            `json:"pin_id"`
	Kind                  string            `json:"kind"`
	ProspectID            string            `json:"prospect_id"`
	RetainedStateID       string            `json:"retained_state_id"`
	RetainedFrontier      *FrontierEvidence `json:"retained_frontier"`
	ContradictoryFrontier *FrontierEvidence `json:"contradictory_frontier"`
	EvidenceIDs           []string          `json:"evidence_ids"`
}
type ReplayTarget struct {
	AdmissionSequence   uint32            `json:"admission_sequence"`
	AdmissionHead       string            `json:"admission_head"`
	PublicationSequence uint32            `json:"publication_sequence"`
	PublicationHead     string            `json:"publication_head"`
	ChainHeight         uint32            `json:"chain_height"`
	ChainHash           string            `json:"chain_hash"`
	Frontier            *FrontierEvidence `json:"frontier"`
}
type replayWork struct {
	Version         uint32       `json:"version"`
	PinID           string       `json:"pin_id"`
	WorkID          string       `json:"work_id"`
	BaseRevision    uint32       `json:"base_revision"`
	Target          ReplayTarget `json:"target"`
	Phase           string       `json:"phase"`
	NextAdmission   uint32       `json:"next_admission"`
	NextPublication uint32       `json:"next_publication"`
	NextHeight      uint32       `json:"next_height"`
	ScanDigest      string       `json:"scan_digest"`
	WorkingMapID    string       `json:"working_map_id"`
	ConflictID      string       `json:"conflict_id"`
}

func validFrontier(v FrontierEvidence) bool {
	return (v.Kind == "publication" || v.Kind == "boundary" || v.Kind == "finality_frame") && hexValue(v.ID, 32, true)
}
func validHeader(id string, v HeaderRow) bool {
	return v.Version == 1 && v.PinID == id && v.Height <= math.MaxInt32 && hexValue(v.Hash, 32, true) && hexValue(v.Header, 80, false)
}
func validTransactionWitness(v TransactionWitness, max int) bool {
	if v.Height > math.MaxInt32 || !hexValue(v.BlockHash, 32, true) || !hexValue(v.TxID, 32, true) || v.TransactionCount < 1 || v.TransactionCount > 1000000 || v.TransactionIndex >= v.TransactionCount || len(v.Raw) > 2*max || len(v.Raw) < 20 || len(v.Raw)%2 != 0 || !hexValue(v.Raw, len(v.Raw)/2, false) || v.MerkleBranch == nil || len(v.MerkleBranch) > 20 {
		return false
	}
	for _, x := range v.MerkleBranch {
		if !hexValue(x, 32, false) {
			return false
		}
	}
	return true
}
func validateWitness(p Pin, v CarrierWitness) error {
	return (ownershipCodec{}).validateWitness(p, v)
}
func validateFrameBody(p Pin, v FinalityFrameBody) error {
	return (ownershipCodec{}).validateFrameBody(p, v)
}
func signObject(domain string, v any) (string, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return "", ErrInvalid
	}
	at := strings.LastIndex(string(raw), `,"signature":`)
	if at < 0 {
		return "", ErrInvalid
	}
	digest, e := objectDigest(domain, json.RawMessage(append(append([]byte{}, raw[:at]...), '}')))
	if e != nil {
		return "", e
	}
	d, _ := hex.DecodeString(digest)
	key := ed25519.NewKeyFromSeed([]byte(strings.Repeat("s", 32)))
	return hex.EncodeToString(ed25519.Sign(key, d)), nil
}
func verifyObject(domain, key string, v any, signature string) error {
	if !hexValue(signature, 64, false) {
		return ErrInvalid
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return ErrInvalid
	}
	at := strings.LastIndex(string(raw), `,"signature":`)
	if at < 0 {
		return ErrInvalid
	}
	digest, e := objectDigest(domain, json.RawMessage(append(append([]byte{}, raw[:at]...), '}')))
	if e != nil {
		return e
	}
	d, _ := hex.DecodeString(digest)
	pub, _ := hex.DecodeString(key)
	sig, _ := hex.DecodeString(signature)
	if !ed25519.Verify(pub, d, sig) {
		return ErrInvalid
	}
	return nil
}
func ValidateFinalityFrame(p Pin, v FinalityFrame) error {
	return (ownershipCodec{}).ValidateFinalityFrame(p, v)
}
func FinalityFrameID(p Pin, v FinalityFrame) (string, error) {
	return (ownershipCodec{}).FinalityFrameID(p, v)
}
func validateFinalityWitness(p Pin, v FinalityWitness) error {
	return (ownershipCodec{}).validateFinalityWitness(p, v)
}
func validateConflict(p Pin, v ConflictRecord) error {
	return (ownershipCodec{}).validateConflict(p, v)
}
func validTarget(v ReplayTarget) bool {
	return v.AdmissionSequence <= 1000 && v.PublicationSequence <= 16384 && validPrefix(v.AdmissionSequence, v.AdmissionHead) && validPrefix(v.PublicationSequence, v.PublicationHead) && v.ChainHeight <= math.MaxInt32 && hexValue(v.ChainHash, 32, true) && (v.Frontier == nil || validFrontier(*v.Frontier))
}
func validateWork(id string, v replayWork) error {
	if v.Version != 1 || v.PinID != id || !hexValue(v.WorkID, 32, true) || !validTarget(v.Target) || v.NextAdmission < 1 || v.NextAdmission > v.Target.AdmissionSequence+1 || v.NextPublication < 1 || v.NextPublication > v.Target.PublicationSequence+1 || v.NextHeight > v.Target.ChainHeight+1 || !hexValue(v.ScanDigest, 32, true) || (v.WorkingMapID != "" && !hexValue(v.WorkingMapID, 32, true)) || (v.ConflictID != "" && !hexValue(v.ConflictID, 32, true)) {
		return ErrInvalid
	}
	switch v.Phase {
	case "admissions", "publications", "chain", "ready":
	default:
		return ErrInvalid
	}
	return nil
}
