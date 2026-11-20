// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

const (
	OwnedMaxPinSize        = 64 * 1024
	OwnedMaxMetadataSize   = 96 * 1024
	OwnedMaxRecordSize     = 320 * 1024
	OwnedMaxResponseSize   = 384 * 1024
	OwnedMaxBoutSourceSize = 28 * 1024
	OwnedMaxBoutStateSize  = 16 * 1024
	OwnedMaxBoutViewSize   = 192 * 1024
)

type OwnedPin struct {
	Version       uint32           `json:"version"`
	SourceStoreID string           `json:"source_store_id"`
	SourcePin     gymseed.OwnedPin `json:"source_pin"`
	RulesID       string           `json:"rules_id"`
}

type OwnedInfo struct {
	Version           uint32   `json:"version"`
	StoreID           string   `json:"store_id"`
	Pin               OwnedPin `json:"pin"`
	SourceCreatedAtMS int64    `json:"source_created_at_ms"`
	CreatedAtMS       int64    `json:"created_at_ms"`
	EventCount        uint32   `json:"event_count"`
}

type OwnedRecord struct {
	Version      uint32                  `json:"version"`
	StoreID      string                  `json:"store_id"`
	CardID       string                  `json:"card_id"`
	Source       gymseed.OwnedCardSource `json:"source"`
	SourceDigest string                  `json:"source_digest"`
	Plan         Plan                    `json:"plan"`
	CreatedAtMS  int64                   `json:"created_at_ms"`
}

type OwnedStatus struct {
	Version      uint32                 `json:"version"`
	StoreID      string                 `json:"store_id"`
	CardID       string                 `json:"card_id"`
	Event        gymevent.Event         `json:"event"`
	Exists       bool                   `json:"exists"`
	RecordDigest string                 `json:"record_digest"`
	CreatedAtMS  int64                  `json:"created_at_ms"`
	ObservedAtMS int64                  `json:"observed_at_ms"`
	Counts       Counts                 `json:"counts"`
	BoutCount    uint32                 `json:"bout_count"`
	Source       gymseed.OwnedCardState `json:"source"`
	Usable       bool                   `json:"usable"`
}

type OwnedBuildResult struct {
	Version  uint32      `json:"version"`
	Record   OwnedRecord `json:"record"`
	Status   OwnedStatus `json:"status"`
	Replayed bool        `json:"replayed"`
}

type OwnedReadResult struct {
	Version uint32      `json:"version"`
	Record  OwnedRecord `json:"record"`
	Status  OwnedStatus `json:"status"`
}

type OwnedHistoryPage struct {
	Version      uint32        `json:"version"`
	StoreID      string        `json:"store_id"`
	AfterCardID  string        `json:"after_card_id"`
	NextCardID   string        `json:"next_card_id"`
	HasMore      bool          `json:"has_more"`
	ObservedAtMS int64         `json:"observed_at_ms"`
	Cards        []OwnedStatus `json:"cards"`
}

type OwnedBoutCorner struct {
	Owner  gymevent.AssignmentOwner `json:"owner"`
	Member gymevent.OwnedMember     `json:"member"`
}

type OwnedBoutSource struct {
	Version          uint32            `json:"version"`
	StoreID          string            `json:"store_id"`
	CardID           string            `json:"card_id"`
	CardRecordDigest string            `json:"card_record_digest"`
	CardSourceDigest string            `json:"card_source_digest"`
	CardCreatedAtMS  int64             `json:"card_created_at_ms"`
	Event            gymevent.Event    `json:"event"`
	SnapshotDigest   string            `json:"snapshot_digest"`
	SeedEventKey     string            `json:"seed_event_key"`
	TargetHeight     uint32            `json:"target_height"`
	TargetHash       string            `json:"target_hash"`
	FinalizedAtMS    int64             `json:"finalized_at_ms"`
	Bout             Bout              `json:"bout"`
	Corners          []OwnedBoutCorner `json:"corners"`
}

type OwnedBoutState struct {
	Version           uint32         `json:"version"`
	CardID            string         `json:"card_id"`
	Event             gymevent.Event `json:"event"`
	CardExists        bool           `json:"card_exists"`
	CardRecordDigest  string         `json:"card_record_digest"`
	CardSourceDigest  string         `json:"card_source_digest"`
	CardCreatedAtMS   int64          `json:"card_created_at_ms"`
	FinalizedAtMS     int64          `json:"finalized_at_ms"`
	ConflictCount     uint32         `json:"conflict_count"`
	ConflictOverflow  bool           `json:"conflict_overflow"`
	IncidentAtMS      int64          `json:"incident_at_ms"`
	EscalationDue     bool           `json:"escalation_due"`
	ChainObservedAtMS int64          `json:"chain_observed_at_ms"`
	TargetDescendants uint32         `json:"target_descendants"`
	CurrentFinality   bool           `json:"current_finality"`
	NodeState         string         `json:"node_state"`
	ObservedAtMS      int64          `json:"observed_at_ms"`
	Usable            bool           `json:"usable"`
}

type OwnedBoutView struct {
	Version              uint32         `json:"version"`
	StoreID              string         `json:"store_id"`
	Pin                  OwnedPin       `json:"pin"`
	CreatedAtMS          int64          `json:"created_at_ms"`
	ParticipantAccountID string         `json:"participant_account_id"`
	ParticipantGymID     string         `json:"participant_gym_id"`
	ObservedAtMS         int64          `json:"observed_at_ms"`
	State                OwnedBoutState `json:"state"`
	Bouts                []BoutRef      `json:"bouts"`
}
