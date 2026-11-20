// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymstable"
)

const (
	OwnedMaxPinSize       = 64 * 1024
	OwnedMaxMetadataSize  = 96 * 1024
	OwnedMaxMemberSize    = 6 * 1024
	OwnedMaxReceiptSize   = 48 * 1024
	OwnedMaxOperationSize = 56 * 1024
	OwnedMaxCutoffSize    = 52 * 1024
)

type OwnedPin struct {
	Version       uint32             `json:"version"`
	NamespaceID   string             `json:"namespace_id"`
	CalendarID    string             `json:"calendar_id"`
	RulesID       string             `json:"rules_id"`
	StableStoreID string             `json:"stable_store_id"`
	StablePin     gymstable.OwnedPin `json:"stable_pin"`
}
type OwnedMember struct {
	Slot            uint32                          `json:"slot"`
	Birth           gymstable.Assignment            `json:"birth"`
	Ownership       fighterownership.EffectiveState `json:"ownership"`
	Class           string                          `json:"class"`
	ConditionAtSave fightercondition.Snapshot       `json:"condition_at_save"`
}
type OwnedIntent Intent
type OwnedReceipt struct {
	Version           uint32                   `json:"version"`
	StoreID           string                   `json:"store_id"`
	EntryID           string                   `json:"entry_id"`
	Event             Event                    `json:"event"`
	OperationID       string                   `json:"operation_id"`
	OperationSequence uint64                   `json:"operation_sequence"`
	Generation        uint64                   `json:"generation"`
	ViewID            string                   `json:"view_id"`
	Action            string                   `json:"action"`
	PreviousRevision  uint64                   `json:"previous_revision"`
	Revision          uint64                   `json:"revision"`
	RosterRevision    uint64                   `json:"roster_revision"`
	OwnershipHead     fighterownership.HeadRef `json:"ownership_head"`
	CommittedAtMS     int64                    `json:"committed_at_ms"`
	Members           []OwnedMember            `json:"members"`
}
type OwnedOperation struct {
	Version       uint32        `json:"version"`
	Intent        OwnedIntent   `json:"intent"`
	State         string        `json:"state"`
	Receipt       *OwnedReceipt `json:"receipt"`
	AbandonedAtMS int64         `json:"abandoned_at_ms"`
}
type OwnedMutationResult struct {
	Version  uint32       `json:"version"`
	Receipt  OwnedReceipt `json:"receipt"`
	Replayed bool         `json:"replayed"`
}
type OwnedEventStatus EventStatus
type OwnedHistoryPage struct {
	Version      uint32             `json:"version"`
	StoreID      string             `json:"store_id"`
	AfterEventID string             `json:"after_event_id"`
	NextEventID  string             `json:"next_event_id"`
	HasMore      bool               `json:"has_more"`
	ObservedAtMS int64              `json:"observed_at_ms"`
	Events       []OwnedEventStatus `json:"events"`
}
type OwnedCutoffEntry struct {
	Version        uint32        `json:"version"`
	StoreID        string        `json:"store_id"`
	Event          Event         `json:"event"`
	EntryID        string        `json:"entry_id"`
	CommittedCount uint32        `json:"committed_count"`
	LastSequence   uint64        `json:"last_sequence"`
	HistoryDigest  string        `json:"history_digest"`
	LatestReceipt  *OwnedReceipt `json:"latest_receipt"`
}
type OwnedCutoffSnapshot struct {
	Version  uint32                          `json:"version"`
	Cutoff   fighterownership.CutoffRecord   `json:"cutoff"`
	Entry    OwnedCutoffEntry                `json:"entry"`
	Roster   gymstable.CutoffRoster          `json:"roster"`
	Members  []gymstable.OwnedCutoffMember   `json:"members"`
	Excluded []gymstable.OwnedExcludedMember `json:"excluded"`
}
type OwnedSnapshotSourceInfo struct {
	Version      uint32   `json:"version"`
	StoreID      string   `json:"store_id"`
	Pin          OwnedPin `json:"pin"`
	ObservedAtMS int64    `json:"observed_at_ms"`
}
