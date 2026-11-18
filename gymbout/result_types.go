// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import "github.com/taytr-llc/slug/gymcard"

const MaxResultSourceSize = 20 * 1024

type ResultSummary struct {
	Winner   string `json:"winner"`
	Method   string `json:"method"`
	EndRound uint32 `json:"end_round"`
	EndTick  uint32 `json:"end_tick"`
}
type ResultSource struct {
	Version          uint32             `json:"version"`
	StoreID          string             `json:"store_id"`
	StoreCreatedAtMS int64              `json:"store_created_at_ms"`
	RecordDigest     string             `json:"record_digest"`
	SourceDigest     string             `json:"source_digest"`
	InputID          string             `json:"input_id"`
	ResultDigest     string             `json:"result_digest"`
	CreatedAtMS      int64              `json:"created_at_ms"`
	Source           gymcard.BoutSource `json:"source"`
	Summary          ResultSummary      `json:"summary"`
}
type ResultView struct { // owned in-process view; never an imported verdict
	Version              uint32
	StoreID              string
	Pin                  Pin
	CreatedAtMS          int64
	ParticipantAccountID string
	ParticipantGymID     string
	ObservedAtMS         int64
	Status               Status
	Source               *ResultSource // nil only if booked but not yet saved
}
