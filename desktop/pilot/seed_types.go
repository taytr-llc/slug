// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymseed"
)

const MaxSeedRequestSize = 8192
const MaxSeedResponseSize = 262144
const MaxSeedCacheSize = 262144
const MaxSeedJobSize = 245760
const MaxSeedMetadataSize = 16384

type SeedControllerAPI interface {
	EventControllerAPI
	SeedSurfaceAPI
}

// SeedSurfaceAPI is the closed seed family shared by solo and pool routes.
type SeedSurfaceAPI interface {
	SeedSnapshot() SeedSnapshot
	SeedStatus(context.Context, string, string) (gymseed.Status, error)
	SeedRecord(context.Context, string, string) (gymseed.Record, error)
	SeedHistory(context.Context, string, string, uint32) (gymseed.HistoryPage, error)
	BeginSeedPrepare(context.Context, string, string) (SeedJob, error)
	BeginSeedPublish(context.Context, string, string) (SeedJob, error)
	BeginSeedReconcile(context.Context, string, string) (SeedJob, error)
	BeginSeedAttest(context.Context, string, string) (SeedJob, error)
	SeedJob(context.Context, string, string) (SeedJob, error)
	CancelSeedJob(context.Context, string, string) (SeedJob, error)
}
type SeedRequest struct {
	ID            uint64 `json:"id"`
	Command       string `json:"command"`
	ViewID        string `json:"view_id"`
	Saturday      string `json:"saturday"`
	AfterEventKey string `json:"after_event_key"`
	Limit         uint32 `json:"limit"`
	JobID         string `json:"job_id"`
}
type SeedResponse struct {
	ID       uint64               `json:"id"`
	OK       bool                 `json:"ok"`
	View     *ManifestViewPin     `json:"view"`
	StoreID  string               `json:"store_id"`
	Snapshot *SeedSnapshot        `json:"snapshot"`
	Status   *gymseed.Status      `json:"status"`
	Record   *gymseed.Record      `json:"record"`
	History  *gymseed.HistoryPage `json:"history"`
	Job      *SeedJob             `json:"job"`
	Error    string               `json:"error"`
}
type SeedSnapshot struct {
	Version            uint32           `json:"version"`
	State              string           `json:"state"`
	Error              string           `json:"error"`
	View               *ManifestViewPin `json:"view"`
	StoreID            string           `json:"store_id"`
	Pin                *gymseed.Pin     `json:"pin"`
	CreatedAtMS        int64            `json:"created_at_ms"`
	PublisherAccountID string           `json:"publisher_account_id"`
	NodeState          string           `json:"node_state"`
	ActiveJobID        string           `json:"active_job_id"`
	QueuedJobID        string           `json:"queued_job_id"`
	LastJobID          string           `json:"last_job_id"`
}
type SeedJob struct {
	Version  uint32          `json:"version"`
	JobID    string          `json:"job_id"`
	Kind     string          `json:"kind"`
	State    string          `json:"state"`
	View     ManifestViewPin `json:"view"`
	StoreID  string          `json:"store_id"`
	EventKey string          `json:"event_key"`
	Saturday string          `json:"saturday"`
	Error    string          `json:"error"`
	Result   *gymseed.Record `json:"result"`
}
