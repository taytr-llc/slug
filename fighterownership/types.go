// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fighterownership verifies pinned public admission and chain evidence.
// Parsed values alone never grant a complete head, signing capability or owner.
package fighterownership

import (
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
)

const (
	Version        uint32 = 1
	MaxTimestampMS int64  = 253402300799999
	MaxPageSize    uint32 = 8
	MaxPageBytes          = 655360
	MaxStoreBytes         = 268435456
	MaxStates             = 5096
	MaxMaps               = 5097
	MaxWitnesses          = 8192
)

var (
	ErrInvalid        = errors.New("public ownership invalid input")
	ErrSource         = errors.New("public ownership source mismatch")
	ErrNotFound       = errors.New("public ownership not found")
	ErrUnavailable    = errors.New("public ownership required evidence unavailable")
	ErrBusy           = errors.New("public ownership busy")
	ErrCapacity       = errors.New("public ownership capacity")
	ErrStale          = errors.New("public ownership stale head")
	ErrClock          = errors.New("public ownership clock")
	ErrConflict       = errors.New("public ownership conflict")
	ErrCancelled      = errors.New("public ownership cancelled")
	ErrOutcomeUnknown = errors.New("public ownership commit outcome unknown")
	ErrClosed         = errors.New("public ownership closed")
	ErrCorrupt        = errors.New("public ownership corrupt storage")
	ErrProfile        = errors.New("public ownership requires normal proof profile")
)

type Pin = registry.PublicManifestPin
type ChainPoint = registry.PublicChainPoint
type Limits = registry.PublicOwnershipLimits
type AdmissionStatement = registry.PublicAdmissionStatement
type PublicationEntry = registry.PublicPublicationEntry
type HeadRef = registry.PublicOwnershipHead
type BoundaryObservation = registry.PublicBoundaryObservation
type CutoffIntent = registry.PublicCutoffIntent
type SignedBoundary = registry.PublicSignedBoundary
type CutoffRecord = registry.PublicCutoffRecord

type StoreIdentity struct {
	StoreID string `json:"store_id"`
}
type InitialStateEvidence struct {
	Version        uint32 `json:"version"`
	PinID          string `json:"pin_id"`
	ProspectID     string `json:"prospect_id"`
	AdmissionID    string `json:"admission_id"`
	OwnerPublicKey string `json:"owner_public_key"`
}
type FinalStateEvidence struct {
	Version                  uint32 `json:"version"`
	PinID                    string `json:"pin_id"`
	ProspectID               string `json:"prospect_id"`
	PriorStateID             string `json:"prior_state_id"`
	TransferID               string `json:"transfer_id"`
	CheckpointPublicationID  string `json:"checkpoint_publication_id"`
	SixtiethDescendantHeight uint32 `json:"sixtieth_descendant_height"`
	SixtiethDescendantHash   string `json:"sixtieth_descendant_hash"`
	OwnerPublicKey           string `json:"owner_public_key"`
}
type EffectiveState struct {
	Version          uint32          `json:"version"`
	PinID            string          `json:"pin_id"`
	ProspectID       string          `json:"prospect_id"`
	StateID          string          `json:"state_id"`
	OwnerPublicKey   string          `json:"owner_public_key"`
	BirthAdmissionID string          `json:"birth_admission_id"`
	Generation       uint32          `json:"generation"`
	Evidence         json.RawMessage `json:"evidence"`
}
type OwnedItem struct {
	Admission AdmissionStatement `json:"admission"`
	State     EffectiveState     `json:"state"`
}
type OwnedPage struct {
	Version      uint32      `json:"version"`
	PinID        string      `json:"pin_id"`
	Head         HeadRef     `json:"head"`
	GymPublicKey string      `json:"gym_public_key"`
	After        uint32      `json:"after"`
	Next         uint32      `json:"next"`
	Complete     bool        `json:"complete"`
	Items        []OwnedItem `json:"items"`
}
type CutoffOwners struct {
	Cutoff CutoffRecord `json:"cutoff"`
	Head   HeadRef      `json:"head"`
	Items  []OwnedItem  `json:"items"`
}
type MapItem struct {
	AdmissionSequence uint32 `json:"admission_sequence"`
	ProspectID        string `json:"prospect_id"`
	StateID           string `json:"state_id"`
}
type OwnershipMap struct {
	Version uint32    `json:"version"`
	PinID   string    `json:"pin_id"`
	Items   []MapItem `json:"items"`
}
type FrontierEvidence struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type FinalityWitness struct {
	Version                  uint32           `json:"version"`
	PinID                    string           `json:"pin_id"`
	StateID                  string           `json:"state_id"`
	Frontier                 FrontierEvidence `json:"frontier"`
	CheckpointPublicationID  string           `json:"checkpoint_publication_id"`
	SixtiethDescendantHeight uint32           `json:"sixtieth_descendant_height"`
	SixtiethDescendantHash   string           `json:"sixtieth_descendant_hash"`
}
type FinalityFrameBody struct {
	Version             uint32 `json:"version"`
	PinID               string `json:"pin_id"`
	Sequence            uint32 `json:"sequence"`
	PreviousBodyID      string `json:"previous_body_id"`
	AdmissionSequence   uint32 `json:"admission_sequence"`
	AdmissionHead       string `json:"admission_head"`
	PublicationSequence uint32 `json:"publication_sequence"`
	PublicationHead     string `json:"publication_head"`
	ChainHeight         uint32 `json:"chain_height"`
	ChainHash           string `json:"chain_hash"`
	OwnershipMapID      string `json:"ownership_map_id"`
	FinalizedPrefixID   string `json:"finalized_prefix_id"`
}
type FinalityFrame struct {
	FinalityFrameBody
	Signature string `json:"signature"`
}
type FundingOutpoint struct {
	TxID  string `json:"txid"`
	Index uint32 `json:"index"`
}
type FundingTuple []FundingOutpoint

// TransferIntent is an immutable projection, not the PreparedTransfer capability.
type TransferIntent struct {
	Version       uint32       `json:"version"`
	PinID         string       `json:"pin_id"`
	ProspectID    string       `json:"prospect_id"`
	PriorStateID  string       `json:"prior_state_id"`
	FromGym       string       `json:"from_gym"`
	Recipient     string       `json:"recipient"`
	Funding       FundingTuple `json:"funding"`
	FundingDigest string       `json:"funding_digest"`
	TransferID    string       `json:"transfer_id"`
	IntentHex     string       `json:"intent"`
}
type TransferStatus struct {
	TransferID            string          `json:"transfer_id"`
	State                 string          `json:"state"`
	EffectiveState        *EffectiveState `json:"effective_state"`
	PublicEvidencePending bool            `json:"public_evidence_pending"`
	ConflictID            string          `json:"conflict_id"`
}
