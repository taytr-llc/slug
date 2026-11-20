// Copyright (c) 2026 The SLUG developers. ISC license.

// Package gymevent retains candidate cutoff snapshots from concrete authenticated
// entry/stable sources. Candidate assignment is not transferable ownership.
package gymevent

import (
	"errors"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

const (
	Version1           uint32 = 1
	MaxParticipants           = 2
	MaxMembers                = 5
	MaxEvents                 = 256
	MaxPageSize               = 8
	MaxPinSize                = 16384
	MaxMarkerSize             = 16384
	MaxMetadataSize           = 20480
	MaxEventSize              = 2048
	MaxRosterSize             = 4096
	MaxEntrySize              = 36864
	MaxMemberSize             = 4096
	MaxParticipantSize        = 57344
	MaxEvidenceSize           = 2048
	MaxCaptureSize            = 61440
	MaxInputSize              = 122880
	MaxReceiptSize            = 126976
	MaxRequestSize            = 8192
	MaxResponseSize           = 131072
	MaxSafeInteger     uint64 = 9007199254740991
	MaxTimestampMS     int64  = 8640000000000000
)

var (
	ErrInvalid        = errors.New("invalid_request")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrStaleView      = errors.New("stale_view")
	ErrBusy           = errors.New("busy")
	ErrUnavailable    = errors.New("event_unavailable")
	ErrCorrupt        = errors.New("event_corrupt")
	ErrCapacity       = errors.New("event_capacity")
	ErrClock          = errors.New("event_clock")
	ErrCalendar       = errors.New("event_calendar")
	ErrNotDue         = errors.New("event_not_due")
	ErrCreatedTooLate = errors.New("event_created_too_late")
	ErrIncomplete     = errors.New("event_incomplete")
	ErrConflict       = errors.New("event_conflict")
	ErrNotFound       = errors.New("event_not_found")
	ErrCancelled      = errors.New("event_cancelled")
	ErrOutcomeUnknown = errors.New("outcome_unknown")
	ErrClosed         = errors.New("event store closed")
	ErrPoisoned       = errors.New("event write uncertain; reopen required")
	ErrProfile        = errors.New("event requires production PoW")
)

type Pin struct {
	Version      uint32         `json:"version"`
	CalendarID   string         `json:"calendar_id"`
	RulesID      string         `json:"rules_id"`
	Participants []gymentry.Pin `json:"participants"`
}
type Info struct {
	Version     uint32 `json:"version"`
	StoreID     string `json:"store_id"`
	Pin         Pin    `json:"pin"`
	CreatedAtMS int64  `json:"created_at_ms"`
	EventCount  uint32 `json:"event_count"`
}
type Event struct {
	Version            uint32 `json:"version"`
	EventID            string `json:"event_id"`
	SourceEventID      string `json:"source_event_id"`
	StoreID            string `json:"store_id"`
	NamespaceID        string `json:"namespace_id"`
	CalendarID         string `json:"calendar_id"`
	RulesID            string `json:"rules_id"`
	Saturday           string `json:"saturday"`
	CutoffMS           int64  `json:"cutoff_ms"`
	IncidentDeadlineMS int64  `json:"incident_deadline_ms"`
}
type AssignmentOwner struct {
	OwnershipBasis     string `json:"ownership_basis"`
	AccountAuthorityID string `json:"account_authority_id"`
	AccountID          string `json:"account_id"`
	GymID              string `json:"gym_id"`
}
type Member struct {
	Slot              uint32                    `json:"slot"`
	SlotAssignedAtMS  int64                     `json:"slot_assigned_at_ms"`
	Assignment        gymstable.Assignment      `json:"assignment"`
	Class             string                    `json:"class"`
	ConditionAtCutoff fightercondition.Snapshot `json:"condition_at_cutoff"`
	Eligibility       string                    `json:"eligibility"`
}
type ParticipantSnapshot struct {
	Version       uint32                 `json:"version"`
	ParticipantID string                 `json:"participant_id"`
	EntryStoreID  string                 `json:"entry_store_id"`
	Owner         AssignmentOwner        `json:"owner"`
	Roster        gymstable.CutoffRoster `json:"roster"`
	Entry         gymentry.CutoffEntry   `json:"entry"`
	Members       []Member               `json:"members"`
}
type CaptureEvidence struct {
	Version       uint32                `json:"version"`
	ParticipantID string                `json:"participant_id"`
	StableFence   gymstable.CutoffFence `json:"stable_fence"`
	EntryFence    gymstable.CutoffFence `json:"entry_fence"`
	CapturedAtMS  int64                 `json:"captured_at_ms"`
}
type ParticipantCapture struct {
	Version  uint32              `json:"version"`
	Snapshot ParticipantSnapshot `json:"snapshot"`
	Evidence CaptureEvidence     `json:"evidence"`
}
type SnapshotInput struct {
	Version      uint32                `json:"version"`
	Event        Event                 `json:"event"`
	Participants []ParticipantSnapshot `json:"participants"`
}
type SnapshotReceipt struct {
	Version        uint32            `json:"version"`
	Input          SnapshotInput     `json:"input"`
	SnapshotDigest string            `json:"snapshot_digest"`
	Captures       []CaptureEvidence `json:"captures"`
	CompletedAtMS  int64             `json:"completed_at_ms"`
}
type EventStatus struct {
	Version                uint32   `json:"version"`
	StoreID                string   `json:"store_id"`
	Event                  Event    `json:"event"`
	Exists                 bool     `json:"exists"`
	Phase                  string   `json:"phase"`
	CapturedParticipants   []string `json:"captured_participants"`
	SnapshotDigest         string   `json:"snapshot_digest"`
	CoordinatorCreatedAtMS int64    `json:"coordinator_created_at_ms"`
	CreatedAtMS            int64    `json:"created_at_ms"`
	CompletedAtMS          int64    `json:"completed_at_ms"`
	ObservedAtMS           int64    `json:"observed_at_ms"`
	SeedAuthority          string   `json:"seed_authority"`
}
type CaptureResult struct {
	Version  uint32             `json:"version"`
	Status   EventStatus        `json:"status"`
	Capture  ParticipantCapture `json:"capture"`
	Replayed bool               `json:"replayed"`
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
