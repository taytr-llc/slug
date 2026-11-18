// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymstable"
)

type StableRequest struct {
	ID               uint64 `json:"id"`
	Command          string `json:"command"`
	ViewID           string `json:"view_id,omitempty"`
	After            uint64 `json:"after,omitempty"`
	Limit            uint32 `json:"limit,omitempty"`
	FirstSlot        uint32 `json:"first_slot,omitempty"`
	ExpectedRevision uint64 `json:"expected_revision,omitempty"`
	Slot             uint32 `json:"slot,omitempty"`
	ProspectID       string `json:"prospect_id,omitempty"`
	OperationID      string `json:"operation_id,omitempty"`
	JobID            string `json:"job_id,omitempty"`
}
type StableResponse struct {
	ID        uint64           `json:"id"`
	OK        bool             `json:"ok"`
	Snapshot  *StableSnapshot  `json:"snapshot,omitempty"`
	Job       *StableJob       `json:"job,omitempty"`
	Operation *StableOperation `json:"operation,omitempty"`
	Error     string           `json:"error,omitempty"`
}
type StableSnapshot struct {
	Version            uint32            `json:"version"`
	State              string            `json:"state"`
	Error              string            `json:"error"`
	View               *ManifestViewPin  `json:"view"`
	Status             *gymstable.Status `json:"status"`
	ActiveJobID        string            `json:"active_job_id"`
	QueuedJobID        string            `json:"queued_job_id"`
	LastJobID          string            `json:"last_job_id"`
	PendingOperationID string            `json:"pending_operation_id"`
}
type StableJob struct {
	Version     uint32                    `json:"version"`
	JobID       string                    `json:"job_id"`
	Kind        string                    `json:"kind"`
	State       string                    `json:"state"`
	View        ManifestViewPin           `json:"view"`
	StoreID     string                    `json:"store_id"`
	OperationID string                    `json:"operation_id"`
	Error       string                    `json:"error"`
	Sync        *gymstable.SyncResult     `json:"sync"`
	Page        *gymstable.Page           `json:"page"`
	Roster      *gymstable.RosterPage     `json:"roster"`
	Intent      *gymstable.Intent         `json:"intent"`
	Mutation    *gymstable.MutationResult `json:"mutation"`
}
type StableOperation struct {
	Version   uint32               `json:"version"`
	View      ManifestViewPin      `json:"view"`
	StoreID   string               `json:"store_id"`
	Operation *gymstable.Operation `json:"operation"`
}
type StableAccountConfig struct {
	Manifest ManifestAccountConfig
	Lease    gymstable.Lease
	// ResultNodeLease fences only explicit Result node readiness admission.
	ResultNodeLease ManifestViewLease
}

type StableControllerAPI interface {
	ManifestControllerAPI
	StableSnapshot() StableSnapshot
	BeginStableSync(context.Context, string) (StableJob, error)
	BeginStableRecent(context.Context, string) (StableJob, error)
	BeginStableInventory(context.Context, string, uint64, uint32) (StableJob, error)
	BeginStableRoster(context.Context, string, uint32, uint32) (StableJob, error)
	BeginStablePrepare(context.Context, string, gymstable.SlotRequest) (StableJob, error)
	BeginStableApply(context.Context, string, string) (StableJob, error)
	StableJob(context.Context, string, string) (StableJob, error)
	StableOperation(context.Context, string, string) (StableOperation, error)
	StablePending(context.Context, string) (StableOperation, error)
	AbandonStableOperation(context.Context, string, string) (StableOperation, error)
	CancelStableJob(context.Context, string, string) (StableJob, error)
}
