// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import "github.com/taytr-llc/slug/gymevent"

const (
	MaxBoutSourceSize = 16 * 1024
	MaxBoutStateSize  = 4 * 1024
	MaxBoutViewSize   = 32 * 1024
)

type BoutCorner struct {
	Owner  gymevent.AssignmentOwner `json:"owner"`
	Member gymevent.Member          `json:"member"`
}
type BoutSource struct {
	Version          uint32         `json:"version"`
	StoreID          string         `json:"store_id"`
	CardID           string         `json:"card_id"`
	CardRecordDigest string         `json:"card_record_digest"`
	CardSourceDigest string         `json:"card_source_digest"`
	CardCreatedAtMS  int64          `json:"card_created_at_ms"`
	Event            gymevent.Event `json:"event"`
	SnapshotDigest   string         `json:"snapshot_digest"`
	SeedEventKey     string         `json:"seed_event_key"`
	TargetHeight     uint32         `json:"target_height"`
	TargetHash       string         `json:"target_hash"`
	FinalizedAtMS    int64          `json:"finalized_at_ms"`
	Bout             Bout           `json:"bout"`
	Corners          []BoutCorner   `json:"corners"`
}
type BoutState struct {
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
type BoutRef struct {
	Bout         Bout   `json:"bout"`
	SourceDigest string `json:"source_digest"`
}
type BoutView struct {
	Version              uint32    `json:"version"`
	StoreID              string    `json:"store_id"`
	Pin                  Pin       `json:"pin"`
	CreatedAtMS          int64     `json:"created_at_ms"`
	ParticipantAccountID string    `json:"participant_account_id"`
	ParticipantGymID     string    `json:"participant_gym_id"`
	ObservedAtMS         int64     `json:"observed_at_ms"`
	State                BoutState `json:"state"`
	Bouts                []BoutRef `json:"bouts"`
}
