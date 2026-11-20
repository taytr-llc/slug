// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymcard"
)

const MaxCardRequestSize = 8192
const MaxCardResponseSize = 262144
const MaxCardCacheSize = 294912
const MaxCardJobSize = 245760
const MaxCardMetadataSize = 49152

type CardControllerAPI interface {
	SeedControllerAPI
	CardSurfaceAPI
}

// CardSurfaceAPI is the closed card family shared by solo and pool routes.
type CardSurfaceAPI interface {
	CardSnapshot() CardSnapshot
	CardStatus(context.Context, string, string) (gymcard.Status, error)
	CardRecord(context.Context, string, string) (gymcard.ReadResult, error)
	CardHistory(context.Context, string, string, uint32) (gymcard.HistoryPage, error)
	BeginCardBuild(context.Context, string, string) (CardJob, error)
	CardJob(context.Context, string, string) (CardJob, error)
	CancelCardJob(context.Context, string, string) (CardJob, error)
}
type CardRequest struct {
	ID          uint64 `json:"id"`
	Command     string `json:"command"`
	ViewID      string `json:"view_id"`
	Saturday    string `json:"saturday"`
	AfterCardID string `json:"after_card_id"`
	Limit       uint32 `json:"limit"`
	JobID       string `json:"job_id"`
}
type CardResponse struct {
	ID       uint64               `json:"id"`
	OK       bool                 `json:"ok"`
	View     *ManifestViewPin     `json:"view"`
	StoreID  string               `json:"store_id"`
	Snapshot *CardSnapshot        `json:"snapshot"`
	Status   *gymcard.Status      `json:"status"`
	Record   *gymcard.ReadResult  `json:"record"`
	History  *gymcard.HistoryPage `json:"history"`
	Job      *CardJob             `json:"job"`
	Error    string               `json:"error"`
}
type CardSnapshot struct {
	Version           uint32           `json:"version"`
	State             string           `json:"state"`
	Error             string           `json:"error"`
	View              *ManifestViewPin `json:"view"`
	StoreID           string           `json:"store_id"`
	Pin               *gymcard.Pin     `json:"pin"`
	SourceCreatedAtMS int64            `json:"source_created_at_ms"`
	CreatedAtMS       int64            `json:"created_at_ms"`
	ActiveJobID       string           `json:"active_job_id"`
	QueuedJobID       string           `json:"queued_job_id"`
	LastJobID         string           `json:"last_job_id"`
}
type CardJob struct {
	IneligibleProspectIDs []string             `json:"ineligible_prospect_ids,omitempty"`
	Version               uint32               `json:"version"`
	JobID                 string               `json:"job_id"`
	Kind                  string               `json:"kind"`
	State                 string               `json:"state"`
	View                  ManifestViewPin      `json:"view"`
	StoreID               string               `json:"store_id"`
	CardID                string               `json:"card_id"`
	Saturday              string               `json:"saturday"`
	Error                 string               `json:"error"`
	Result                *gymcard.BuildResult `json:"result"`
}
