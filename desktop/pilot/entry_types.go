// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"

	"github.com/taytr-llc/slug/gymentry"
)

const (
	MaxEntryRequestSize  = 8192
	MaxEntryResponseSize = 131072
	MaxEntryCacheSize    = 131072
)

// EntryControllerAPI adds reversible candidate intake to the existing gym.
// It has no lock, seed, pairing, result or award capability.
type EntryControllerAPI interface {
	PreviewControllerAPI
	EntrySurfaceAPI
}

// EntrySurfaceAPI is the closed intake surface shared by the isolated and
// shared-source desktops. It does not require a preview or mining controller.
type EntrySurfaceAPI interface {
	EntrySnapshot() EntrySnapshot
	EntryUpcoming(context.Context, string) (gymentry.EventStatus, error)
	EntryStatus(context.Context, string, string) (gymentry.EventStatus, error)
	EntryHistory(context.Context, string, string, uint32) (gymentry.HistoryPage, error)
	BeginEntryPrepare(context.Context, string, gymentry.Request) (EntryJob, error)
	BeginEntryApply(context.Context, string, string) (EntryJob, error)
	BeginEntryCheck(context.Context, string, string) (EntryJob, error)
	EntryJob(context.Context, string, string) (EntryJob, error)
	CancelEntryJob(context.Context, string, string) (EntryJob, error)
	EntryOperation(context.Context, string, string) (gymentry.OperationResult, error)
	EntryPending(context.Context, string) (gymentry.OperationResult, error)
	AbandonEntryOperation(context.Context, string, string) (gymentry.OperationResult, error)
}

// Each entry wire field is present. This is a separate closed family, not an
// extension of the legacy stable or preview request grammar.
type EntryRequest struct {
	ID           uint64            `json:"id"`
	Command      string            `json:"command"`
	ViewID       string            `json:"view_id"`
	Saturday     string            `json:"saturday"`
	AfterEventID string            `json:"after_event_id"`
	Limit        uint32            `json:"limit"`
	Request      *gymentry.Request `json:"request"`
	OperationID  string            `json:"operation_id"`
	JobID        string            `json:"job_id"`
}

// View and StoreID identify this current query. An embedded historical intent
// or receipt keeps its original author/view without authorizing another Apply.
type EntryResponse struct {
	ID        uint64                    `json:"id"`
	OK        bool                      `json:"ok"`
	View      *ManifestViewPin          `json:"view"`
	StoreID   string                    `json:"store_id"`
	Snapshot  *EntrySnapshot            `json:"snapshot"`
	Status    *gymentry.EventStatus     `json:"status"`
	History   *gymentry.HistoryPage     `json:"history"`
	Job       *EntryJob                 `json:"job"`
	Operation *gymentry.OperationResult `json:"operation"`
	Error     string                    `json:"error"`
}

type EntrySnapshot struct {
	Version            uint32           `json:"version"`
	State              string           `json:"state"`
	Error              string           `json:"error"`
	View               *ManifestViewPin `json:"view"`
	StoreID            string           `json:"store_id"`
	PendingOperationID string           `json:"pending_operation_id"`
	ActiveJobID        string           `json:"active_job_id"`
	QueuedJobID        string           `json:"queued_job_id"`
	LastJobID          string           `json:"last_job_id"`
}

type EntryJob struct {
	IneligibleProspectIDs []string                 `json:"ineligible_prospect_ids,omitempty"`
	Version               uint32                   `json:"version"`
	JobID                 string                   `json:"job_id"`
	Kind                  string                   `json:"kind"`
	State                 string                   `json:"state"`
	View                  ManifestViewPin          `json:"view"`
	StoreID               string                   `json:"store_id"`
	EventID               string                   `json:"event_id"`
	Saturday              string                   `json:"saturday"`
	OperationID           string                   `json:"operation_id"`
	Error                 string                   `json:"error"`
	Intent                *gymentry.Intent         `json:"intent"`
	Mutation              *gymentry.MutationResult `json:"mutation"`
	Check                 *gymentry.CheckResult    `json:"check"`
}
