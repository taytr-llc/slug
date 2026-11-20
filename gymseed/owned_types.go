// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymevent"
)

const (
	OwnedMaxPinSize        = 64 * 1024
	OwnedMaxMetadataSize   = 96 * 1024
	OwnedMaxRecordSize     = 384 * 1024
	OwnedMaxResponseSize   = 448 * 1024
	OwnedMaxCardSourceSize = 272 * 1024
	OwnedMaxCardStateSize  = 12 * 1024
	OwnedMaxCardViewSize   = 160 * 1024
)

type OwnedPin struct {
	Version                    uint32            `json:"version"`
	SourceStoreID              string            `json:"source_store_id"`
	SourcePin                  gymevent.OwnedPin `json:"source_pin"`
	PublisherGymID             string            `json:"publisher_gym_id"`
	PublisherAccountID         string            `json:"publisher_account_id"`
	PublisherSourceAuthorityID string            `json:"publisher_source_authority_id"`
	PublisherChainID           string            `json:"publisher_chain_id"`
	Network                    uint32            `json:"network"`
	Genesis                    string            `json:"genesis"`
	FloorHeight                uint32            `json:"floor_height"`
	FloorHash                  string            `json:"floor_hash"`
	PublisherPublicKey         string            `json:"publisher_public_key"`
	CheckpointPublicKey        string            `json:"checkpoint_public_key"`
	FundingPublicKey           string            `json:"funding_public_key"`
	FundingScript              string            `json:"funding_script"`
	RulesID                    string            `json:"rules_id"`
}

type OwnedInfo struct {
	Version             uint32   `json:"version"`
	StoreID             string   `json:"store_id"`
	Pin                 OwnedPin `json:"pin"`
	CreatedAtMS         int64    `json:"created_at_ms"`
	EventCount          uint32   `json:"event_count"`
	FundingHeight       int32    `json:"funding_height"`
	FundingHash         string   `json:"funding_hash"`
	FundingComplete     bool     `json:"funding_complete"`
	FundingGeneration   uint32   `json:"funding_generation"`
	FundingRestartCount uint32   `json:"funding_restart_count"`
}

type OwnedDeclaration struct {
	Version             uint32         `json:"version"`
	EventKey            string         `json:"event_key"`
	StoreID             string         `json:"store_id"`
	SourceStoreID       string         `json:"source_store_id"`
	Event               gymevent.Event `json:"event"`
	SnapshotDigest      string         `json:"snapshot_digest"`
	ReceiptDigest       string         `json:"receipt_digest"`
	PublisherChainID    string         `json:"publisher_chain_id"`
	Network             uint32         `json:"network"`
	Genesis             string         `json:"genesis"`
	FloorHeight         uint32         `json:"floor_height"`
	FloorHash           string         `json:"floor_hash"`
	RulesID             string         `json:"rules_id"`
	PublisherPublicKey  string         `json:"publisher_public_key"`
	CheckpointPublicKey string         `json:"checkpoint_public_key"`
}

type OwnedSignedDeclaration struct {
	Version     uint32           `json:"version"`
	Declaration OwnedDeclaration `json:"declaration"`
	Signature   string           `json:"signature"`
}

type OwnedConflict struct {
	Kind           string                     `json:"kind"`
	ObservedAtMS   int64                      `json:"observed_at_ms"`
	Declaration    *OwnedSignedDeclaration    `json:"declaration"`
	Checkpoint     *chaincfg.SignedCheckpoint `json:"checkpoint"`
	ExpectedHeight uint32                     `json:"expected_height"`
	ExpectedHash   string                     `json:"expected_hash"`
	ObservedHash   string                     `json:"observed_hash"`
	ObservedTip    *Tip                       `json:"observed_tip"`
}

type OwnedRecord struct {
	Version          uint32                        `json:"version"`
	StoreID          string                        `json:"store_id"`
	EventKey         string                        `json:"event_key"`
	SourceReceipt    gymevent.OwnedSnapshotReceipt `json:"source_receipt"`
	Declaration      OwnedSignedDeclaration        `json:"declaration"`
	PreparedAtMS     int64                         `json:"prepared_at_ms"`
	Publication      *Publication                  `json:"publication"`
	Cursor           Cursor                        `json:"cursor"`
	ScanRestarts     []ScanRestart                 `json:"scan_restarts"`
	Anchor           *Anchor                       `json:"anchor"`
	Target           *Target                       `json:"target"`
	Checkpoint       *chaincfg.SignedCheckpoint    `json:"checkpoint"`
	Finality         *Finality                     `json:"finality"`
	Conflicts        []OwnedConflict               `json:"conflicts"`
	ConflictOverflow bool                          `json:"conflict_overflow"`
	IncidentAtMS     int64                         `json:"incident_at_ms"`
	Observation      Observation                   `json:"observation"`
}

type OwnedStatus struct {
	Version           uint32         `json:"version"`
	StoreID           string         `json:"store_id"`
	EventKey          string         `json:"event_key"`
	Event             gymevent.Event `json:"event"`
	Exists            bool           `json:"exists"`
	Phase             string         `json:"phase"`
	PublicationState  string         `json:"publication_state"`
	ScanHeight        int32          `json:"scan_height"`
	ScanTipHeight     int32          `json:"scan_tip_height"`
	AnchorHeight      uint32         `json:"anchor_height"`
	AnchorHash        string         `json:"anchor_hash"`
	TargetHeight      uint32         `json:"target_height"`
	TargetHash        string         `json:"target_hash"`
	TargetDescendants uint32         `json:"target_descendants"`
	CheckpointPresent bool           `json:"checkpoint_present"`
	FinalizedAtMS     int64          `json:"finalized_at_ms"`
	ConflictCount     uint32         `json:"conflict_count"`
	ConflictOverflow  bool           `json:"conflict_overflow"`
	IncidentAtMS      int64          `json:"incident_at_ms"`
	PreparedAtMS      int64          `json:"prepared_at_ms"`
	ObservedAtMS      int64          `json:"observed_at_ms"`
	ChainObservedAtMS int64          `json:"chain_observed_at_ms"`
	NodeState         string         `json:"node_state"`
	FundingState      string         `json:"funding_state"`
	CurrentFinality   bool           `json:"current_finality"`
	EscalationDue     bool           `json:"escalation_due"`
	ScanGeneration    uint32         `json:"scan_generation"`
	FundingGeneration uint32         `json:"funding_generation"`
}

type OwnedHistoryPage struct {
	Version       uint32        `json:"version"`
	StoreID       string        `json:"store_id"`
	AfterEventKey string        `json:"after_event_key"`
	NextEventKey  string        `json:"next_event_key"`
	HasMore       bool          `json:"has_more"`
	ObservedAtMS  int64         `json:"observed_at_ms"`
	Events        []OwnedStatus `json:"events"`
}

type OwnedCardSource struct {
	Version       uint32                        `json:"version"`
	StoreID       string                        `json:"store_id"`
	EventKey      string                        `json:"event_key"`
	SourceReceipt gymevent.OwnedSnapshotReceipt `json:"source_receipt"`
	Declaration   OwnedSignedDeclaration        `json:"declaration"`
	Anchor        Anchor                        `json:"anchor"`
	Target        Target                        `json:"target"`
	Finality      Finality                      `json:"finality"`
}

type OwnedCardState struct {
	Version      uint32      `json:"version"`
	Status       OwnedStatus `json:"status"`
	SourceDigest string      `json:"source_digest"`
}

type OwnedCardView struct {
	Version              uint32           `json:"version"`
	StoreID              string           `json:"store_id"`
	Pin                  OwnedPin         `json:"pin"`
	CreatedAtMS          int64            `json:"created_at_ms"`
	ParticipantAccountID string           `json:"participant_account_id"`
	ParticipantGymID     string           `json:"participant_gym_id"`
	ObservedAtMS         int64            `json:"observed_at_ms"`
	States               []OwnedCardState `json:"states"`
}
