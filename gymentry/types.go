// Copyright (c) 2026 The SLUG developers. ISC license.

// Package gymentry retains authenticated candidate Saturday entry intake.
// It does not lock events, select entropy, book bouts, or establish ownership.
package gymentry

import (
	"errors"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymstable"
)

const (
	Version1           uint32 = 1
	MaxEvents                 = 256
	MaxOperations             = 4096
	MaxMembers                = 5
	MaxPageSize               = 8
	MaxMarkerSize             = 8192
	MaxMetadataSize           = 8192
	MaxEventSize              = 2048
	MaxOperationSize          = 32768
	MaxRequestSize            = 8192
	MaxResponseSize           = 131072
	MaxMemberSize             = 4096
	MaxCheckMemberSize        = 10240
	MaxSafeInteger     uint64 = 9007199254740991
	MaxTimestampMS     int64  = 8640000000000000
)

var (
	ErrInvalid        = errors.New("invalid_request")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrStaleView      = errors.New("stale_view")
	ErrBusy           = errors.New("busy")
	ErrUnavailable    = errors.New("entry_unavailable")
	ErrCorrupt        = errors.New("entry_corrupt")
	ErrCapacity       = errors.New("entry_capacity")
	ErrClock          = errors.New("entry_clock")
	ErrCalendar       = errors.New("entry_calendar")
	ErrClosed         = errors.New("entry_closed")
	ErrConflict       = errors.New("entry_conflict")
	ErrPending        = errors.New("entry_pending")
	ErrNotFound       = errors.New("entry_not_found")
	ErrSyncRequired   = errors.New("entry_sync_required")
	ErrNotRostered    = errors.New("entry_not_rostered")
	ErrRetired        = errors.New("entry_retired")
	ErrClassConflict  = errors.New("entry_class_conflict")
	ErrCancelled      = errors.New("entry_cancelled")
	ErrOutcomeUnknown = errors.New("outcome_unknown")
	ErrPoisoned       = errors.New("entry write uncertain; reopen required")
	ErrProfile        = errors.New("entry requires production PoW")
)

type Session = gymstable.Session

type Pin struct {
	Version       uint32        `json:"version"`
	NamespaceID   string        `json:"namespace_id"`
	CalendarID    string        `json:"calendar_id"`
	RulesID       string        `json:"rules_id"`
	StableStoreID string        `json:"stable_store_id"`
	StablePin     gymstable.Pin `json:"stable_pin"`
}
type Event struct {
	Version     uint32 `json:"version"`
	EventID     string `json:"event_id"`
	NamespaceID string `json:"namespace_id"`
	CalendarID  string `json:"calendar_id"`
	RulesID     string `json:"rules_id"`
	Saturday    string `json:"saturday"`
	CutoffMS    int64  `json:"cutoff_ms"`
}
type Request struct {
	Version                uint32   `json:"version"`
	EventID                string   `json:"event_id"`
	Saturday               string   `json:"saturday"`
	Action                 string   `json:"action"`
	ExpectedRevision       uint64   `json:"expected_revision"`
	ExpectedRosterRevision uint64   `json:"expected_roster_revision"`
	ProspectIDs            []string `json:"prospect_ids"`
}
type Intent struct {
	Version      uint32  `json:"version"`
	Sequence     uint64  `json:"sequence"`
	OperationID  string  `json:"operation_id"`
	Generation   uint64  `json:"generation"`
	ViewID       string  `json:"view_id"`
	Request      Request `json:"request"`
	PreparedAtMS int64   `json:"prepared_at_ms"`
}
type Member struct {
	Slot            uint32                    `json:"slot"`
	Assignment      gymstable.Assignment      `json:"assignment"`
	Class           string                    `json:"class"`
	ConditionAtSave fightercondition.Snapshot `json:"condition_at_save"`
}
type Receipt struct {
	Version             uint32   `json:"version"`
	StoreID             string   `json:"store_id"`
	EntryID             string   `json:"entry_id"`
	Event               Event    `json:"event"`
	OperationID         string   `json:"operation_id"`
	OperationSequence   uint64   `json:"operation_sequence"`
	Generation          uint64   `json:"generation"`
	ViewID              string   `json:"view_id"`
	Action              string   `json:"action"`
	PreviousRevision    uint64   `json:"previous_revision"`
	Revision            uint64   `json:"revision"`
	RosterRevision      uint64   `json:"roster_revision"`
	StableCheckpoint    uint64   `json:"stable_checkpoint"`
	ObservedSourceCount uint64   `json:"observed_source_count"`
	CommittedAtMS       int64    `json:"committed_at_ms"`
	Members             []Member `json:"members"`
}
type Operation struct {
	Version       uint32   `json:"version"`
	Intent        Intent   `json:"intent"`
	State         string   `json:"state"`
	Receipt       *Receipt `json:"receipt"`
	AbandonedAtMS int64    `json:"abandoned_at_ms"`
}
type MutationResult struct {
	Version  uint32  `json:"version"`
	Receipt  Receipt `json:"receipt"`
	Replayed bool    `json:"replayed"`
}
type OperationResult struct {
	Version   uint32     `json:"version"`
	StoreID   string     `json:"store_id"`
	Operation *Operation `json:"operation"`
}
type EventStatus struct {
	Version               uint32   `json:"version"`
	StoreID               string   `json:"store_id"`
	Event                 Event    `json:"event"`
	Exists                bool     `json:"exists"`
	Phase                 string   `json:"phase"`
	EntryID               string   `json:"entry_id"`
	Revision              uint64   `json:"revision"`
	LatestOperationID     string   `json:"latest_operation_id"`
	LatestReceiptDigest   string   `json:"latest_receipt_digest"`
	SavedIDs              []string `json:"saved_ids"`
	SavedAtMS             int64    `json:"saved_at_ms"`
	SavedRosterRevision   uint64   `json:"saved_roster_revision"`
	CurrentRosterRevision uint64   `json:"current_roster_revision"`
	RosterChanged         bool     `json:"roster_changed"`
	Eligibility           string   `json:"eligibility"`
	PendingOperationID    string   `json:"pending_operation_id"`
	ObservedAtMS          int64    `json:"observed_at_ms"`
	ClosedObservedAtMS    int64    `json:"closed_observed_at_ms"`
}
type CheckMember struct {
	Slot        uint32         `json:"slot"`
	Item        gymstable.Item `json:"item"`
	Eligibility string         `json:"eligibility"`
}
type CheckResult struct {
	Version        uint32        `json:"version"`
	Status         EventStatus   `json:"status"`
	AsOfMS         int64         `json:"as_of_ms"`
	RosterRevision uint64        `json:"roster_revision"`
	Members        []CheckMember `json:"members"`
}
type HistoryPage struct {
	Version      uint32        `json:"version"`
	StoreID      string        `json:"store_id"`
	AfterEventID string        `json:"after_event_id"`
	NextEventID  string        `json:"next_event_id"`
	HasMore      bool          `json:"has_more"`
	ObservedAtMS int64         `json:"observed_at_ms"`
	Events       []EventStatus `json:"events"`
}
