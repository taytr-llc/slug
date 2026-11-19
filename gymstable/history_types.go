// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const (
	MaxCareerHistoryCaptureSize   = 8192
	MaxCareerHistoryRowSize       = 1536
	MaxCareerHistoryEventSize     = 1344
	MaxCareerHistoryPageSize      = 32768
	MaxCareerHistoryRetainedBytes = 416 * 1024
)

type CareerHistoryCapture struct {
	Version      uint32                 `json:"version"`
	Status       Status                 `json:"status"`
	Activation   careerstate.Activation `json:"activation"`
	CareerDigest string                 `json:"career_digest"`
	AsOfMS       int64                  `json:"as_of_ms"`
	Assignment   Assignment             `json:"assignment"`
	PrefixCount  uint32                 `json:"prefix_count"`
	EventCount   uint32                 `json:"event_count"`
	Totals       fightercareer.Totals   `json:"totals"`
	HistoryID    string                 `json:"history_id"`
}

type CareerHistoryRow = careerstate.HistoryRow

type CareerHistoryPage struct {
	Version uint32               `json:"version"`
	Capture CareerHistoryCapture `json:"capture"`
	After   uint32               `json:"after"`
	Next    uint32               `json:"next"`
	HasMore bool                 `json:"has_more"`
	Items   []CareerHistoryRow   `json:"items"`
}
