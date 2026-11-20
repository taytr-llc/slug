// Copyright (c) 2026 The SLUG developers. ISC license.
// Package gymcard retains immutable two-fixture candidate cards. It is not a
// professional record, production matching or fight-result authority.
package gymcard

import (
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

const (
	Version1           uint32 = 1
	MaxEvents                 = 256
	MaxPageSize               = 8
	MaxEntrants               = 10
	MaxBouts                  = 5
	MaxPinSize                = 20 * 1024
	MaxInfoSize               = 24 * 1024
	MaxMarkerSize             = 24 * 1024
	MaxMetadataSize           = 24 * 1024
	MaxPlanSize               = 16 * 1024
	MaxRecordSize             = 192 * 1024
	MaxStatusSize             = 16 * 1024
	MaxHistoryPageSize        = 136 * 1024
	MaxBuildResultSize        = 224 * 1024
	MaxReadResultSize         = 224 * 1024
	MaxResponseSize           = 256 * 1024
	MaxTimestampMS     int64  = 8640000000000000
	MaxSafeInteger     uint64 = 9007199254740991
	RecordBasis               = "candidate_zero_record_no_results_authority"
)

var (
	ErrInvalid          = errors.New("invalid_request")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrStaleView        = errors.New("stale_view")
	ErrBusy             = errors.New("busy")
	ErrOutcomeUnknown   = errors.New("outcome_unknown")
	ErrUnavailable      = errors.New("card_unavailable")
	ErrCorrupt          = errors.New("card_corrupt")
	ErrCapacity         = errors.New("card_capacity")
	ErrClock            = errors.New("card_clock")
	ErrCalendar         = errors.New("card_calendar")
	ErrNotFound         = errors.New("card_not_found")
	ErrSourceIncomplete = errors.New("card_source_incomplete")
	ErrSourceConflict   = errors.New("card_source_conflict")
	ErrSourceChanged    = errors.New("card_source_changed")
	ErrCancelled        = errors.New("card_cancelled")
	ErrClosed           = errors.New("card store closed")
	ErrPoisoned         = errors.New("card write uncertain; reopen required")
	ErrProfile          = errors.New("card requires production PoW")
)

type Pin struct {
	Version       uint32      `json:"version"`
	SourceStoreID string      `json:"source_store_id"`
	SourcePin     gymseed.Pin `json:"source_pin"`
	RulesID       string      `json:"rules_id"`
}
type Info struct {
	Version           uint32 `json:"version"`
	StoreID           string `json:"store_id"`
	Pin               Pin    `json:"pin"`
	SourceCreatedAtMS int64  `json:"source_created_at_ms"`
	CreatedAtMS       int64  `json:"created_at_ms"`
	EventCount        uint32 `json:"event_count"`
}
type ProfessionalRecord struct {
	Wins   uint32 `json:"wins"`
	Losses uint32 `json:"losses"`
	Draws  uint32 `json:"draws"`
}
type ReasonCounts struct {
	RetiredAtCutoff            uint32 `json:"retired_at_cutoff"`
	NotRosteredAtCutoff        uint32 `json:"not_rostered_at_cutoff"`
	NoOpponentInClass          uint32 `json:"no_opponent_in_class"`
	OpponentIneligibleAtCutoff uint32 `json:"opponent_ineligible_at_cutoff"`
	OutsideRatingBand          uint32 `json:"outside_rating_band"`
}
type Counts struct {
	TotalCount     uint32       `json:"total_count"`
	EligibleCount  uint32       `json:"eligible_count"`
	PairedCount    uint32       `json:"paired_count"`
	UnmatchedCount uint32       `json:"unmatched_count"`
	Reasons        ReasonCounts `json:"reasons"`
}
type GymCounts struct {
	GymID  string `json:"gym_id"`
	Counts Counts `json:"counts"`
}
type Entrant struct {
	ProspectID         string             `json:"prospect_id"`
	GymID              string             `json:"gym_id"`
	AccountID          string             `json:"account_id"`
	DisplayName        string             `json:"display_name"`
	Class              string             `json:"class"`
	RatingHundredths   uint32             `json:"rating_hundredths"`
	CutoffEligibility  string             `json:"cutoff_eligibility"`
	ProfessionalRecord ProfessionalRecord `json:"professional_record"`
	State              string             `json:"state"`
	Reason             string             `json:"reason"`
	BoutID             string             `json:"bout_id"`
}
type Bout struct {
	BoutID  string `json:"bout_id"`
	Class   string `json:"class"`
	CornerA string `json:"corner_a"`
	CornerB string `json:"corner_b"`
	Seed    string `json:"seed"`
}
type Plan struct {
	Version     uint32      `json:"version"`
	CardID      string      `json:"card_id"`
	InputID     string      `json:"input_id"`
	Seed        string      `json:"seed"`
	RecordBasis string      `json:"record_basis"`
	Counts      Counts      `json:"counts"`
	BoutCount   uint32      `json:"bout_count"`
	Gyms        []GymCounts `json:"gyms"`
	Entrants    []Entrant   `json:"entrants"`
	Bouts       []Bout      `json:"bouts"`
}
type Record struct {
	Version      uint32             `json:"version"`
	StoreID      string             `json:"store_id"`
	CardID       string             `json:"card_id"`
	Source       gymseed.CardSource `json:"source"`
	SourceDigest string             `json:"source_digest"`
	Plan         Plan               `json:"plan"`
	CreatedAtMS  int64              `json:"created_at_ms"`
}
type Status struct {
	Version      uint32            `json:"version"`
	StoreID      string            `json:"store_id"`
	CardID       string            `json:"card_id"`
	Event        gymevent.Event    `json:"event"`
	Exists       bool              `json:"exists"`
	RecordDigest string            `json:"record_digest"`
	CreatedAtMS  int64             `json:"created_at_ms"`
	ObservedAtMS int64             `json:"observed_at_ms"`
	Counts       Counts            `json:"counts"`
	BoutCount    uint32            `json:"bout_count"`
	Source       gymseed.CardState `json:"source"`
	Usable       bool              `json:"usable"`
}
type BuildResult struct {
	Version  uint32 `json:"version"`
	Record   Record `json:"record"`
	Status   Status `json:"status"`
	Replayed bool   `json:"replayed"`
}
type ReadResult struct {
	Version uint32 `json:"version"`
	Record  Record `json:"record"`
	Status  Status `json:"status"`
}
type HistoryPage struct {
	Version      uint32   `json:"version"`
	StoreID      string   `json:"store_id"`
	AfterCardID  string   `json:"after_card_id"`
	NextCardID   string   `json:"next_card_id"`
	HasMore      bool     `json:"has_more"`
	ObservedAtMS int64    `json:"observed_at_ms"`
	Cards        []Status `json:"cards"`
}
