// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const MaxCareerHistoryRequestSize = 1024
const MaxCareerHistoryResponseSize = 36864
const MaxCareerHistoryCaptureSize = 8192
const MaxCareerHistoryPageSize = 32768
const MaxCareerHistoryJobSize = 12288
const MaxCareerHistorySnapshotSize = 4096
const careerHistoryAllowance = 512 * 1024
const careerHistoryCaptureAllowance = 416 * 1024

type CareerHistoryControllerAPI interface {
	CareerControllerAPI
	CareerHistorySurfaceAPI
}

// CareerHistorySurfaceAPI is the closed careerhistory family shared by solo and pool routes.
type CareerHistorySurfaceAPI interface {
	CareerHistorySnapshot(context.Context) (CareerHistorySnapshot, error)
	BeginCareerHistory(context.Context, string, string) (CareerHistoryJob, error)
	CareerHistoryJob(context.Context, string, string) (CareerHistoryJob, error)
	CancelCareerHistory(context.Context, string, string) (CareerHistoryJob, error)
	CareerHistoryPage(context.Context, string, string, uint32, uint32) (CareerHistoryPageResult, error)
}
type CareerHistoryRequest struct {
	ID         uint64 `json:"id"`
	Version    uint32 `json:"version"`
	Command    string `json:"command"`
	ViewID     string `json:"view_id"`
	ProspectID string `json:"prospect_id"`
	JobID      string `json:"job_id"`
	After      uint32 `json:"after"`
	Limit      uint32 `json:"limit"`
}
type CareerHistorySnapshot struct {
	Version     uint32                  `json:"version"`
	State       string                  `json:"state"`
	Error       string                  `json:"error"`
	View        *ManifestViewPin        `json:"view"`
	StoreID     string                  `json:"store_id"`
	Activation  *careerstate.Activation `json:"activation"`
	ActiveJobID string                  `json:"active_job_id"`
	QueuedJobID string                  `json:"queued_job_id"`
	LastJobID   string                  `json:"last_job_id"`
}
type CareerHistoryJob struct {
	Version      uint32                          `json:"version"`
	JobID        string                          `json:"job_id"`
	State        string                          `json:"state"`
	View         ManifestViewPin                 `json:"view"`
	StoreID      string                          `json:"store_id"`
	ActivationID string                          `json:"activation_id"`
	ProspectID   string                          `json:"prospect_id"`
	Error        string                          `json:"error"`
	Capture      *gymstable.CareerHistoryCapture `json:"capture"`
}
type CareerHistoryPageResult struct {
	View    ManifestViewPin             `json:"view"`
	StoreID string                      `json:"store_id"`
	JobID   string                      `json:"job_id"`
	Page    gymstable.CareerHistoryPage `json:"page"`
}
type CareerHistoryResponse struct {
	ID       uint64                   `json:"id"`
	Version  uint32                   `json:"version"`
	OK       bool                     `json:"ok"`
	Snapshot *CareerHistorySnapshot   `json:"snapshot"`
	Job      *CareerHistoryJob        `json:"job"`
	Page     *CareerHistoryPageResult `json:"page"`
	Error    string                   `json:"error"`
}
