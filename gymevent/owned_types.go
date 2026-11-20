// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

const (
	OwnedMaxPinSize         = 64 * 1024
	OwnedMaxMetadataSize    = 96 * 1024
	OwnedMaxParticipantSize = 104 * 1024
	OwnedMaxInputSize       = 224 * 1024
	OwnedMaxReceiptSize     = 232 * 1024
)

type OwnedPin struct {
	Version      uint32               `json:"version"`
	CalendarID   string               `json:"calendar_id"`
	RulesID      string               `json:"rules_id"`
	Ownership    fighterownership.Pin `json:"ownership"`
	Participants []gymentry.OwnedPin  `json:"participants"`
}
type OwnedInfo struct {
	Version     uint32   `json:"version"`
	StoreID     string   `json:"store_id"`
	Pin         OwnedPin `json:"pin"`
	CreatedAtMS int64    `json:"created_at_ms"`
	EventCount  uint32   `json:"event_count"`
}
type OwnedMember = gymstable.OwnedCutoffMember
type OwnedExcludedMember = gymstable.OwnedExcludedMember
type OwnedParticipantSnapshot struct {
	Version       uint32                    `json:"version"`
	ParticipantID string                    `json:"participant_id"`
	EntryStoreID  string                    `json:"entry_store_id"`
	Owner         AssignmentOwner           `json:"owner"`
	Roster        gymstable.CutoffRoster    `json:"roster"`
	Entry         gymentry.OwnedCutoffEntry `json:"entry"`
	Members       []OwnedMember             `json:"members"`
	Excluded      []OwnedExcludedMember     `json:"excluded"`
}
type OwnedCaptureEvidence struct {
	Version       uint32                `json:"version"`
	ParticipantID string                `json:"participant_id"`
	StableFence   gymstable.CutoffFence `json:"stable_fence"`
	EntryFence    gymstable.CutoffFence `json:"entry_fence"`
	CutoffID      string                `json:"cutoff_id"`
	CapturedAtMS  int64                 `json:"captured_at_ms"`
}
type OwnedParticipantCapture struct {
	Version  uint32                   `json:"version"`
	Snapshot OwnedParticipantSnapshot `json:"snapshot"`
	Evidence OwnedCaptureEvidence     `json:"evidence"`
}
type OwnedSnapshotInput struct {
	Version      uint32                        `json:"version"`
	Event        Event                         `json:"event"`
	Cutoff       fighterownership.CutoffRecord `json:"cutoff"`
	Participants []OwnedParticipantSnapshot    `json:"participants"`
}
type OwnedSnapshotReceipt struct {
	Version        uint32                 `json:"version"`
	Input          OwnedSnapshotInput     `json:"input"`
	SnapshotDigest string                 `json:"snapshot_digest"`
	Captures       []OwnedCaptureEvidence `json:"captures"`
	CompletedAtMS  int64                  `json:"completed_at_ms"`
}
type OwnedEventStatus EventStatus
type OwnedCaptureResult struct {
	Version  uint32                  `json:"version"`
	Status   OwnedEventStatus        `json:"status"`
	Capture  OwnedParticipantCapture `json:"capture"`
	Replayed bool                    `json:"replayed"`
}
type OwnedHistoryPage struct {
	Version      uint32             `json:"version"`
	StoreID      string             `json:"store_id"`
	AfterEventID string             `json:"after_event_id"`
	NextEventID  string             `json:"next_event_id"`
	HasMore      bool               `json:"has_more"`
	ObservedAtMS int64              `json:"observed_at_ms"`
	Events       []OwnedEventStatus `json:"events"`
}
