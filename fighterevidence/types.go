// Copyright (c) 2026 The SLUG developers. ISC license.
// Package fighterevidence checks supplied public evidence against separately
// selected source trust. It grants no ownership, admission or import authority.
package fighterevidence

import (
	"errors"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

const (
	Format                 = "slug-public-fighter-evidence"
	Version         uint32 = 1
	MaxSize                = 512 * 1024
	MaxPinSize             = 4096
	MaxRows                = 256
	SuccessHeadline        = "Supplied evidence is consistent with the selected source."
	Limitations            = "Issuance and birth/acceptance times, actual result awards/finality, complete history and current ownership are not established."
)

var (
	ErrProfile        = errors.New("public fighter evidence requires the normal proof-of-work profile")
	ErrInvalid        = errors.New("invalid public fighter evidence")
	ErrSourceMismatch = errors.New("public fighter evidence source mismatch")
	ErrBusy           = errors.New("public fighter evidence verification busy")
	ErrCancelled      = errors.New("public fighter evidence verification cancelled")
)

type Archive struct {
	Format    string                         `json:"format"`
	Version   uint32                         `json:"version"`
	Admission registry.ManifestRecord        `json:"admission"`
	Capture   gymstable.CareerHistoryCapture `json:"capture"`
	Rows      []gymstable.CareerHistoryRow   `json:"rows"`
}

type Checks struct {
	ExactSourceMatch            bool `json:"exact_source_match"`
	SuppliedWorkConsistent      bool `json:"supplied_work_consistent"`
	GymAuthorizationValid       bool `json:"gym_authorization_valid"`
	StaticAppearanceConsistent  bool `json:"static_appearance_consistent"`
	ReceiptAssignmentConsistent bool `json:"receipt_assignment_consistent"`
	SuppliedHistoryConsistent   bool `json:"supplied_history_consistent"`
}

type Unverified struct {
	IssuanceAndAdmissionTimes       bool `json:"issuance_and_admission_times"`
	ActualAwardsAndResultFinality   bool `json:"actual_awards_and_result_finality"`
	CanonicalChain                  bool `json:"canonical_chain"`
	CurrentOwnership                bool `json:"current_ownership"`
	HistoryCompletenessAndFreshness bool `json:"history_completeness_and_freshness"`
}

type Report struct {
	Version        uint32     `json:"version"`
	Status         string     `json:"status"`
	FileSHA256     string     `json:"file_sha256"`
	FileBytes      int        `json:"file_bytes"`
	SourceStoreID  string     `json:"source_store_id"`
	ProspectID     string     `json:"prospect_id"`
	HistoryID      string     `json:"history_id"`
	CapturedAsOfMS int64      `json:"captured_as_of_ms"`
	EventCount     uint32     `json:"event_count"`
	Checks         Checks     `json:"checks"`
	Unverified     Unverified `json:"unverified"`
}
