// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymresult"
)

const MaxResultRequestSize = 8192
const MaxResultResponseSize = 262144
const MaxResultJobSize = 245760
const MaxResultMetadataSize = 49152
const MaxResultCacheSize = 294912

type ResultControllerAPI interface {
	BoutControllerAPI
	ResultSnapshot() ResultSnapshot
	ResultStatus(context.Context, string, string, string) (gymresult.Status, error)
	ResultRecord(context.Context, string, string, string) (gymresult.ReadResult, error)
	BeginResultPublish(context.Context, string, string, string) (ResultJob, error)
	BeginResultReconcile(context.Context, string, string, string) (ResultJob, error)
	BeginResultAttest(context.Context, string, string, string) (ResultJob, error)
	ResultJob(context.Context, string, string) (ResultJob, error)
	CancelResultJob(context.Context, string, string) (ResultJob, error)
	BeginResultExport(context.Context, string, string, string, string) (ResultJob, error)
}
type ResultRequest struct {
	ID       uint64 `json:"id"`
	Command  string `json:"command"`
	ViewID   string `json:"view_id"`
	Saturday string `json:"saturday"`
	BoutID   string `json:"bout_id"`
	JobID    string `json:"job_id"`
	Path     string `json:"path"`
}
type ResultResponse struct {
	ID       uint64                `json:"id"`
	OK       bool                  `json:"ok"`
	View     *ManifestViewPin      `json:"view"`
	StoreID  string                `json:"store_id"`
	Snapshot *ResultSnapshot       `json:"snapshot"`
	Status   *gymresult.Status     `json:"status"`
	Record   *gymresult.ReadResult `json:"record"`
	Job      *ResultJob            `json:"job"`
	Error    string                `json:"error"`
}
type ResultSnapshot struct {
	Version            uint32           `json:"version"`
	State              string           `json:"state"`
	Error              string           `json:"error"`
	View               *ManifestViewPin `json:"view"`
	StoreID            string           `json:"store_id"`
	Pin                *gymresult.Pin   `json:"pin"`
	SourceCreatedAtMS  int64            `json:"source_created_at_ms"`
	CreatedAtMS        int64            `json:"created_at_ms"`
	PublisherAccountID string           `json:"publisher_account_id"`
	NodeState          string           `json:"node_state"`
	NodeOwner          string           `json:"node_owner"`
	ActiveJobID        string           `json:"active_job_id"`
	QueuedJobID        string           `json:"queued_job_id"`
	LastJobID          string           `json:"last_job_id"`
}
type ResultJob struct {
	Version   uint32                  `json:"version"`
	JobID     string                  `json:"job_id"`
	Kind      string                  `json:"kind"`
	State     string                  `json:"state"`
	View      ManifestViewPin         `json:"view"`
	StoreID   string                  `json:"store_id"`
	CardID    string                  `json:"card_id"`
	Saturday  string                  `json:"saturday"`
	BoutID    string                  `json:"bout_id"`
	ResultKey string                  `json:"result_key"`
	Error     string                  `json:"error"`
	Result    *gymresult.ActionResult `json:"result"`
	Export    *gymresult.ProofSummary `json:"export"`
}
