// Copyright (c) 2026 The SLUG developers. ISC license.
// Package gymbout retains candidate transcripts without result or award authority.
package gymbout

import (
	"errors"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymcard"
)

const (
	Version1          uint32 = 1
	MaxSavedBouts            = 256
	MaxPinSize               = 24 * 1024
	MaxInfoSize              = 28 * 1024
	MaxMarkerSize            = 28 * 1024
	MaxMetadataSize          = 28 * 1024
	MaxInputSize             = 16 * 1024
	MaxResultSize            = 192 * 1024
	MaxRecordSize            = 232 * 1024
	MaxSavedBoutSize         = 2 * 1024
	MaxStatusSize            = 8 * 1024
	MaxCardStatusSize        = 16 * 1024
	MaxRunResultSize         = 244 * 1024
	MaxReadResultSize        = 244 * 1024
	MaxResponseSize          = 256 * 1024
	MaxIndexSize             = 384 * 1024
	MaxTimestampMS    int64  = 8640000000000000
	MaxSafeInteger    uint64 = 9007199254740991
)

var (
	ErrInvalid          = errors.New("invalid_request")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrStaleView        = errors.New("stale_view")
	ErrBusy             = errors.New("busy")
	ErrOutcomeUnknown   = errors.New("outcome_unknown")
	ErrUnavailable      = errors.New("bout_unavailable")
	ErrCorrupt          = errors.New("bout_corrupt")
	ErrCapacity         = errors.New("bout_capacity")
	ErrClock            = errors.New("bout_clock")
	ErrCalendar         = errors.New("bout_calendar")
	ErrNotFound         = errors.New("bout_not_found")
	ErrSourceIncomplete = errors.New("bout_source_incomplete")
	ErrSourceConflict   = errors.New("bout_source_conflict")
	ErrSourceChanged    = errors.New("bout_source_changed")
	ErrCancelled        = errors.New("bout_cancelled")
	ErrClosed           = errors.New("bout store closed")
	ErrPoisoned         = errors.New("bout write uncertain; reopen required")
	ErrProfile          = errors.New("bout requires production PoW")
)

type Pin struct {
	Version       uint32      `json:"version"`
	SourceStoreID string      `json:"source_store_id"`
	SourcePin     gymcard.Pin `json:"source_pin"`
	RulesID       string      `json:"rules_id"`
}
type Info struct {
	Version           uint32 `json:"version"`
	StoreID           string `json:"store_id"`
	Pin               Pin    `json:"pin"`
	SourceCreatedAtMS int64  `json:"source_created_at_ms"`
	CreatedAtMS       int64  `json:"created_at_ms"`
	BoutCount         uint32 `json:"bout_count"`
}
type Record struct {
	Version      uint32             `json:"version"`
	StoreID      string             `json:"store_id"`
	CardID       string             `json:"card_id"`
	BoutID       string             `json:"bout_id"`
	Source       gymcard.BoutSource `json:"source"`
	SourceDigest string             `json:"source_digest"`
	Input        fighterbout.Input  `json:"input"`
	InputID      string             `json:"input_id"`
	Result       fighterbout.Result `json:"result"`
	ResultDigest string             `json:"result_digest"`
	CreatedAtMS  int64              `json:"created_at_ms"`
}
type SavedBout struct {
	Bout         gymcard.Bout `json:"bout"`
	SourceDigest string       `json:"source_digest"`
	Exists       bool         `json:"exists"`
	RecordDigest string       `json:"record_digest"`
	CreatedAtMS  int64        `json:"created_at_ms"`
	InputID      string       `json:"input_id"`
	ResultDigest string       `json:"result_digest"`
	Winner       string       `json:"winner"`
	Method       string       `json:"method"`
	EndRound     uint32       `json:"end_round"`
	EndTick      uint32       `json:"end_tick"`
	Usable       bool         `json:"usable"`
}
type Status struct {
	Version      uint32            `json:"version"`
	StoreID      string            `json:"store_id"`
	CardID       string            `json:"card_id"`
	ObservedAtMS int64             `json:"observed_at_ms"`
	Source       gymcard.BoutState `json:"source"`
	Saved        SavedBout         `json:"saved"`
}
type CardStatus struct {
	Version      uint32            `json:"version"`
	StoreID      string            `json:"store_id"`
	CardID       string            `json:"card_id"`
	ObservedAtMS int64             `json:"observed_at_ms"`
	Source       gymcard.BoutState `json:"source"`
	Bouts        []SavedBout       `json:"bouts"`
}
type RunResult struct {
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
