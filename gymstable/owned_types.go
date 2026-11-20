// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"errors"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const (
	OwnedMaxPinSize              = 16 * 1024
	OwnedMaxStatusSize           = 24 * 1024
	OwnedMaxItemSize             = 88 * 1024
	OwnedMaxPageSize             = 736 * 1024
	OwnedMaxOperationSize        = 16 * 1024
	OwnedMaxHistoryCaptureSize   = 32 * 1024
	OwnedMaxHistoryPageSize      = 64 * 1024
	OwnedMaxHistoryRetainedBytes = 512 * 1024
)

var ErrOwnedCareerConflict = errors.New("owned_career_conflict")

type OwnedPin struct {
	Version   uint32               `json:"version"`
	Stable    Pin                  `json:"stable"`
	Ownership fighterownership.Pin `json:"ownership"`
}
type OwnedStatus struct {
	Version            uint32                   `json:"version"`
	StoreID            string                   `json:"store_id"`
	Pin                OwnedPin                 `json:"pin"`
	Head               fighterownership.HeadRef `json:"head"`
	RosterRevision     uint64                   `json:"roster_revision"`
	Slots              [10]string               `json:"slots"`
	OperationCount     uint32                   `json:"operation_count"`
	PendingOperationID string                   `json:"pending_operation_id"`
	LastOperationID    string                   `json:"last_operation_id"`
	LastAsOfMS         int64                    `json:"last_as_of_ms"`
}
type OwnedCareer struct {
	Activation     careerstate.Activation `json:"activation"`
	CareerDigest   string                 `json:"career_digest"`
	Totals         fightercareer.Totals   `json:"totals"`
	CurrentGymWins uint32                 `json:"current_gym_wins"`
}
type OwnedItem struct {
	Birth     Assignment                          `json:"birth"`
	Admission fighterownership.AdmissionStatement `json:"admission"`
	Ownership fighterownership.EffectiveState     `json:"ownership"`
	Manifest  fightermanifest.Manifest            `json:"manifest"`
	Condition fightercondition.Snapshot           `json:"condition"`
	Career    OwnedCareer                         `json:"career"`
}
type OwnedPage struct {
	Version   uint32                   `json:"version"`
	Status    OwnedStatus              `json:"status"`
	Head      fighterownership.HeadRef `json:"head"`
	AsOfMS    int64                    `json:"as_of_ms"`
	After     uint32                   `json:"after"`
	NextAfter uint32                   `json:"next_after"`
	HasMore   bool                     `json:"has_more"`
	Items     []OwnedItem              `json:"items"`
}
type OwnedSlotRequest struct {
	ExpectedRevision uint64                   `json:"expected_revision"`
	Slot             uint32                   `json:"slot"`
	ProspectID       string                   `json:"prospect_id"`
	ExpectedStateID  string                   `json:"expected_state_id"`
	Head             fighterownership.HeadRef `json:"head"`
}
type OwnedIntent struct {
	Version      uint32           `json:"version"`
	Sequence     uint64           `json:"sequence"`
	OperationID  string           `json:"operation_id"`
	Generation   uint64           `json:"generation"`
	ViewID       string           `json:"view_id"`
	Request      OwnedSlotRequest `json:"request"`
	PreparedAtMS int64            `json:"prepared_at_ms"`
}
type OwnedMutationReceipt struct {
	Version            uint32           `json:"version"`
	OperationID        string           `json:"operation_id"`
	Generation         uint64           `json:"generation"`
	ViewID             string           `json:"view_id"`
	Request            OwnedSlotRequest `json:"request"`
	PreviousProspectID string           `json:"previous_prospect_id"`
	Changed            bool             `json:"changed"`
	PreviousRevision   uint64           `json:"previous_revision"`
	Revision           uint64           `json:"revision"`
	Slots              [10]string       `json:"slots"`
	CommittedAtMS      int64            `json:"committed_at_ms"`
}
type OwnedOperation struct {
	Version       uint32                `json:"version"`
	Intent        OwnedIntent           `json:"intent"`
	State         string                `json:"state"`
	Receipt       *OwnedMutationReceipt `json:"receipt"`
	AbandonedAtMS int64                 `json:"abandoned_at_ms"`
}
type OwnedMutationResult struct {
	Version  uint32               `json:"version"`
	Receipt  OwnedMutationReceipt `json:"receipt"`
	Replayed bool                 `json:"replayed"`
}
type OwnedCareerBindings struct {
	Current careerstate.Activation
	Legacy  *careerstate.Activation
}
type OwnedCareerHistoryCapture struct {
	Version        uint32                          `json:"version"`
	Status         OwnedStatus                     `json:"status"`
	Activation     careerstate.Activation          `json:"activation"`
	CareerDigest   string                          `json:"career_digest"`
	AsOfMS         int64                           `json:"as_of_ms"`
	Birth          Assignment                      `json:"birth"`
	Ownership      fighterownership.EffectiveState `json:"ownership"`
	PrefixCount    uint32                          `json:"prefix_count"`
	EventCount     uint32                          `json:"event_count"`
	Totals         fightercareer.Totals            `json:"totals"`
	CurrentGymWins uint32                          `json:"current_gym_wins"`
	HistoryID      string                          `json:"history_id"`
}
type OwnedCareerHistoryPage struct {
	Version uint32                    `json:"version"`
	Capture OwnedCareerHistoryCapture `json:"capture"`
	After   uint32                    `json:"after"`
	Next    uint32                    `json:"next"`
	HasMore bool                      `json:"has_more"`
	Items   []CareerHistoryRow        `json:"items"`
}
