// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymcard"
)

const (
	OwnedMaxPinSize          = 64 * 1024
	OwnedMaxMetadataSize     = 96 * 1024
	OwnedMaxRecordSize       = 272 * 1024
	OwnedMaxResponseSize     = 304 * 1024
	OwnedMaxResultSourceSize = 32 * 1024
)

type OwnedPin struct {
	Version       uint32           `json:"version"`
	SourceStoreID string           `json:"source_store_id"`
	SourcePin     gymcard.OwnedPin `json:"source_pin"`
	RulesID       string           `json:"rules_id"`
}

type OwnedInfo struct {
	Version           uint32   `json:"version"`
	StoreID           string   `json:"store_id"`
	Pin               OwnedPin `json:"pin"`
	SourceCreatedAtMS int64    `json:"source_created_at_ms"`
	CreatedAtMS       int64    `json:"created_at_ms"`
	BoutCount         uint32   `json:"bout_count"`
}

type OwnedRecord struct {
	Version      uint32                  `json:"version"`
	StoreID      string                  `json:"store_id"`
	CardID       string                  `json:"card_id"`
	BoutID       string                  `json:"bout_id"`
	Source       gymcard.OwnedBoutSource `json:"source"`
	SourceDigest string                  `json:"source_digest"`
	Input        fighterbout.Input       `json:"input"`
	InputID      string                  `json:"input_id"`
	Result       fighterbout.Result      `json:"result"`
	ResultDigest string                  `json:"result_digest"`
	CreatedAtMS  int64                   `json:"created_at_ms"`
}

type OwnedSavedBout struct {
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

type OwnedStatus struct {
	Version      uint32                 `json:"version"`
	StoreID      string                 `json:"store_id"`
	CardID       string                 `json:"card_id"`
	ObservedAtMS int64                  `json:"observed_at_ms"`
	Source       gymcard.OwnedBoutState `json:"source"`
	Saved        OwnedSavedBout         `json:"saved"`
}

type OwnedCardStatus struct {
	Version      uint32                 `json:"version"`
	StoreID      string                 `json:"store_id"`
	CardID       string                 `json:"card_id"`
	ObservedAtMS int64                  `json:"observed_at_ms"`
	Source       gymcard.OwnedBoutState `json:"source"`
	Bouts        []OwnedSavedBout       `json:"bouts"`
}

type OwnedRunResult struct {
	Version  uint32      `json:"version"`
	Record   OwnedRecord `json:"record"`
	Status   OwnedStatus `json:"status"`
	Replayed bool        `json:"replayed"`
}

type OwnedReadResult struct {
	Version uint32      `json:"version"`
	Record  OwnedRecord `json:"record"`
	Status  OwnedStatus `json:"status"`
}

type OwnedResultSource struct {
	Version          uint32                  `json:"version"`
	StoreID          string                  `json:"store_id"`
	StoreCreatedAtMS int64                   `json:"store_created_at_ms"`
	RecordDigest     string                  `json:"record_digest"`
	SourceDigest     string                  `json:"source_digest"`
	InputID          string                  `json:"input_id"`
	ResultDigest     string                  `json:"result_digest"`
	CreatedAtMS      int64                   `json:"created_at_ms"`
	Source           gymcard.OwnedBoutSource `json:"source"`
	Summary          ResultSummary           `json:"summary"`
}

type OwnedResultView struct {
	Version              uint32
	StoreID              string
	Pin                  OwnedPin
	CreatedAtMS          int64
	ParticipantAccountID string
	ParticipantGymID     string
	ObservedAtMS         int64
	Status               OwnedStatus
	Source               *OwnedResultSource
}
