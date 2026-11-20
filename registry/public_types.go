// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import "encoding/json"

// PublicManifestPin is independently retained configuration. Parsing it does
// not establish trust in the claimed source or any source-selected frontier.
type PublicManifestPin struct {
	Version        uint32                `json:"version"`
	Source         ManifestAuthorityPin  `json:"source"`
	PublicationID  string                `json:"publication_id"`
	AdmissionKey   string                `json:"admission_key"`
	PublicationKey string                `json:"publication_key"`
	CutoffKey      string                `json:"cutoff_key"`
	CheckpointKey  string                `json:"checkpoint_key"`
	RuleID         string                `json:"rule_id"`
	ScanOrigin     PublicChainPoint      `json:"scan_origin"`
	Limits         PublicOwnershipLimits `json:"limits"`
}
type PublicChainPoint struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
}
type PublicOwnershipLimits struct {
	Admissions   uint32 `json:"admissions"`
	Publications uint32 `json:"publications"`
	Carriers     uint32 `json:"carriers"`
	Checkpoints  uint32 `json:"checkpoints"`
	Cutoffs      uint32 `json:"cutoffs"`
	Headers      uint32 `json:"headers"`
	LogicalBytes uint32 `json:"logical_bytes"`
}
type PublicAdmissionStatement struct {
	Version             uint32         `json:"version"`
	PinID               string         `json:"pin_id"`
	AdmissionSequence   uint32         `json:"admission_sequence"`
	PreviousAdmissionID string         `json:"previous_admission_id"`
	RecordDigest        string         `json:"record_digest"`
	Record              ManifestRecord `json:"record"`
	Signature           string         `json:"signature"`
}

// Payload is decoded by the exact closed kind codec before validation. It is
// retained as canonical bytes to preserve the different payload member sets.
type PublicPublicationEntry struct {
	Version      uint32          `json:"version"`
	PinID        string          `json:"pin_id"`
	Sequence     uint32          `json:"sequence"`
	PreviousID   string          `json:"previous_id"`
	AttestedAtMS int64           `json:"attested_at_ms"`
	Kind         string          `json:"kind"`
	Payload      json.RawMessage `json:"payload"`
	Signature    string          `json:"signature"`
}
type PublicAdmissionReference struct {
	AdmissionSequence uint32 `json:"admission_sequence"`
	AdmissionID       string `json:"admission_id"`
}
type PublicChainReference struct {
	Height               uint32 `json:"height"`
	Hash                 string `json:"hash"`
	PreviousChainEntryID string `json:"previous_chain_entry_id"`
}
type PublicCutoffReference struct {
	CutoffID string `json:"cutoff_id"`
}
type PublicOwnershipHead struct {
	Version             uint32 `json:"version"`
	PinID               string `json:"pin_id"`
	AdmissionSequence   uint32 `json:"admission_sequence"`
	AdmissionHead       string `json:"admission_head"`
	PublicationSequence uint32 `json:"publication_sequence"`
	PublicationHead     string `json:"publication_head"`
	ChainHeight         uint32 `json:"chain_height"`
	ChainHash           string `json:"chain_hash"`
	ReplayDigest        string `json:"replay_digest"`
	FinalizedPrefixID   string `json:"finalized_prefix_id"`
	ConflictID          string `json:"conflict_id"`
}
type PublicBoundaryObservation struct {
	RequestStartMS int64  `json:"request_start_ms"`
	ResponseEndMS  int64  `json:"response_end_ms"`
	InstanceID     string `json:"instance_id"`
	Revision       string `json:"revision"`
	Height         uint32 `json:"height"`
	Hash           string `json:"hash"`
}
type PublicCutoffIntent struct {
	Version                   uint32                    `json:"version"`
	PinID                     string                    `json:"pin_id"`
	EventID                   string                    `json:"event_id"`
	CutoffMS                  int64                     `json:"cutoff_ms"`
	First                     PublicBoundaryObservation `json:"first"`
	Second                    PublicBoundaryObservation `json:"second"`
	AcceptedAdmissionSequence uint32                    `json:"accepted_admission_sequence"`
	PublicationSequence       uint32                    `json:"publication_sequence"`
	PublicationHead           string                    `json:"publication_head"`
	FinalitySequence          uint32                    `json:"finality_sequence"`
	FinalityBodyHead          string                    `json:"finality_body_head"`
	PriorCutoffID             string                    `json:"prior_cutoff_id"`
}
type PublicSignedBoundary struct {
	Version   uint32             `json:"version"`
	PinID     string             `json:"pin_id"`
	Intent    PublicCutoffIntent `json:"intent"`
	Signature string             `json:"signature"`
}
type PublicCutoffRecord struct {
	Version           uint32               `json:"version"`
	PinID             string               `json:"pin_id"`
	EventID           string               `json:"event_id"`
	CutoffMS          int64                `json:"cutoff_ms"`
	Boundary          PublicSignedBoundary `json:"boundary"`
	AdmissionSequence uint32               `json:"admission_sequence"`
	AdmissionHead     string               `json:"admission_head"`
	Head              PublicOwnershipHead  `json:"head"`
	OwnershipDigest   string               `json:"ownership_digest"`
	Signature         string               `json:"signature"`
}
type PublicAdmissionPage struct {
	Version         uint32                     `json:"version"`
	PinID           string                     `json:"pin_id"`
	ThroughSequence uint32                     `json:"through_sequence"`
	ThroughHead     string                     `json:"through_head"`
	After           uint32                     `json:"after"`
	Next            uint32                     `json:"next"`
	Complete        bool                       `json:"complete"`
	Items           []PublicAdmissionStatement `json:"items"`
}
type PublicPublicationPage struct {
	Version         uint32                   `json:"version"`
	PinID           string                   `json:"pin_id"`
	ThroughSequence uint32                   `json:"through_sequence"`
	ThroughHead     string                   `json:"through_head"`
	After           uint32                   `json:"after"`
	Next            uint32                   `json:"next"`
	Complete        bool                     `json:"complete"`
	Items           []PublicPublicationEntry `json:"items"`
}

// PublicationStatus distinguishes durable admission from portable publication.
// Accepted ownership is immediate; a signing gap is not pending ownership.
type PublicationStatus struct {
	Version                 uint32 `json:"version"`
	PinID                   string `json:"pin_id"`
	AcceptedSequence        uint32 `json:"accepted_sequence"`
	SignedAdmissionSequence uint32 `json:"signed_admission_sequence"`
	AdmissionHead           string `json:"admission_head"`
	PublicationSequence     uint32 `json:"publication_sequence"`
	PublicationHead         string `json:"publication_head"`
	PublicEvidencePending   bool   `json:"public_evidence_pending"`
}
