// Copyright (c) 2026 The SLUG developers. ISC license.

// Package gymstable retains candidate gym assignment and reversible roster
// choices. It is not a transferable ownership or official league authority.
package gymstable

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

const (
	Version1          uint32 = 1
	RosterSlots              = 10
	MaxPageSize              = 8
	MaxAssignments           = 1000
	MaxOperations            = 4096
	MaxMarkerSize            = 4096
	MaxMetadataSize          = 4096
	MaxAssignmentSize        = 2048
	MaxOperationSize         = 8192
	MaxRequestSize           = 8192
	MaxResponseSize          = 131072
	MaxItemSize              = 8192
	MaxSafeInteger    uint64 = 9007199254740991
	MaxTimestampMS    int64  = 8640000000000000
)

var (
	ErrInvalid        = errors.New("invalid_request")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrStaleView      = errors.New("stale_view")
	ErrBusy           = errors.New("busy")
	ErrCorrupt        = errors.New("stable_corrupt")
	ErrCapacity       = errors.New("stable_capacity")
	ErrClock          = errors.New("stable_clock")
	ErrSyncRequired   = errors.New("stable_sync_required")
	ErrConflict       = errors.New("stable_conflict")
	ErrPending        = errors.New("stable_pending")
	ErrNotFound       = errors.New("stable_not_found")
	ErrCancelled      = errors.New("stable_cancelled")
	ErrOutcomeUnknown = errors.New("outcome_unknown")
	ErrClosed         = errors.New("stable store closed")
	ErrPoisoned       = errors.New("stable write uncertain; reopen required")
	ErrProfile        = errors.New("stable requires production PoW")
)

// Lease is a trusted synchronous callback capturing one original account view.
// It is not a public assertion or a caller-supplied ownership claim.
type Lease func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error

type Session struct {
	Generation uint64
	ViewID     string
	Lease      Lease
}

type Pin struct {
	Version            uint32                        `json:"version"`
	AccountAuthorityID string                        `json:"account_authority_id"`
	AccountID          string                        `json:"account_id"`
	Identity           gymaccount.Identity           `json:"identity"`
	Source             registry.ManifestAuthorityPin `json:"source"`
	ConditionVersion   uint32                        `json:"condition_version"`
	ProjectorID        string                        `json:"projector_id"`
}
type Assignment struct {
	Version            uint32 `json:"version"`
	Sequence           uint64 `json:"sequence"`
	ProspectID         string `json:"prospect_id"`
	GymID              string `json:"gym_id"`
	WorkHash           string `json:"work_hash"`
	BornAtMS           int64  `json:"born_at_ms"`
	AcceptedAtMS       int64  `json:"accepted_at_ms"`
	DescriptorID       string `json:"descriptor_id"`
	ManifestDigest     string `json:"manifest_digest"`
	PortraitDigest     string `json:"portrait_digest"`
	SourceRecordDigest string `json:"source_record_digest"`
}
type Status struct {
	Version             uint32     `json:"version"`
	StoreID             string     `json:"store_id"`
	Pin                 Pin        `json:"pin"`
	Checkpoint          uint64     `json:"checkpoint"`
	ObservedSourceCount uint64     `json:"observed_source_count"`
	CaughtUp            bool       `json:"caught_up"`
	RosterRevision      uint64     `json:"roster_revision"`
	Slots               [10]string `json:"slots"`
	OperationCount      uint32     `json:"operation_count"`
	PendingOperationID  string     `json:"pending_operation_id"`
	LastOperationID     string     `json:"last_operation_id"`
	LastAsOfMS          int64      `json:"last_as_of_ms"`
}
type Item struct {
	Assignment Assignment                `json:"assignment"`
	Manifest   fightermanifest.Manifest  `json:"manifest"`
	Condition  fightercondition.Snapshot `json:"condition"`
	record     registry.ManifestRecord
}

type SyncResult struct {
	Version uint32 `json:"version"`
	Status  Status `json:"status"`
	AsOfMS  int64  `json:"as_of_ms"`
	Items   []Item `json:"items"`
}
type Page struct {
	Version   uint32 `json:"version"`
	Status    Status `json:"status"`
	AsOfMS    int64  `json:"as_of_ms"`
	After     uint64 `json:"after"`
	NextAfter uint64 `json:"next_after"`
	HasMore   bool   `json:"has_more"`
	Items     []Item `json:"items"`
}
type RosterItem struct {
	Slot uint32 `json:"slot"`
	Item Item   `json:"item"`
}
type RosterPage struct {
	Version   uint32       `json:"version"`
	Status    Status       `json:"status"`
	AsOfMS    int64        `json:"as_of_ms"`
	FirstSlot uint32       `json:"first_slot"`
	NextSlot  uint32       `json:"next_slot"`
	Items     []RosterItem `json:"items"`
}
type SlotRequest struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	Slot             uint32 `json:"slot"`
	ProspectID       string `json:"prospect_id"`
}
type Intent struct {
	Version      uint32      `json:"version"`
	Sequence     uint64      `json:"sequence"`
	OperationID  string      `json:"operation_id"`
	Generation   uint64      `json:"generation"`
	ViewID       string      `json:"view_id"`
	Request      SlotRequest `json:"request"`
	PreparedAtMS int64       `json:"prepared_at_ms"`
}
type MutationReceipt struct {
	Version            uint32      `json:"version"`
	OperationID        string      `json:"operation_id"`
	Generation         uint64      `json:"generation"`
	ViewID             string      `json:"view_id"`
	Request            SlotRequest `json:"request"`
	PreviousProspectID string      `json:"previous_prospect_id"`
	Changed            bool        `json:"changed"`
	PreviousRevision   uint64      `json:"previous_revision"`
	Revision           uint64      `json:"revision"`
	Slots              [10]string  `json:"slots"`
	CommittedAtMS      int64       `json:"committed_at_ms"`
}
type Operation struct {
	Version       uint32           `json:"version"`
	Intent        Intent           `json:"intent"`
	State         string           `json:"state"`
	Receipt       *MutationReceipt `json:"receipt"`
	AbandonedAtMS int64            `json:"abandoned_at_ms"`
}
type MutationResult struct {
	Version  uint32          `json:"version"`
	Receipt  MutationReceipt `json:"receipt"`
	Replayed bool            `json:"replayed"`
}

// Receipt returns owned evidence for a bounded private appearance/export cache.
// No gymstable API accepts this value as authoritative input.
func (i Item) Receipt() registry.ManifestRecord {
	r := i.record
	r.Proof.MerkleBranch = append([]string{}, r.Proof.MerkleBranch...)
	return r
}
