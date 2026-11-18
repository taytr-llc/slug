// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymevent"
)

const MaxEventRequestSize = 8192
const MaxEventResponseSize = 131072
const MaxEventCacheSize = 131072

type EventControllerAPI interface {
	EntryControllerAPI
	EventSnapshot() EventSnapshot
	EventStatus(context.Context, string, string) (gymevent.EventStatus, error)
	EventRecord(context.Context, string, string) (gymevent.SnapshotReceipt, error)
	EventHistory(context.Context, string, string, uint32) (gymevent.HistoryPage, error)
	BeginEventCapture(context.Context, string, string) (EventJob, error)
	EventJob(context.Context, string, string) (EventJob, error)
	CancelEventJob(context.Context, string, string) (EventJob, error)
}
type EventRequest struct {
	ID           uint64 `json:"id"`
	Command      string `json:"command"`
	ViewID       string `json:"view_id"`
	Saturday     string `json:"saturday"`
	AfterEventID string `json:"after_event_id"`
	Limit        uint32 `json:"limit"`
	JobID        string `json:"job_id"`
}
type EventResponse struct {
	ID       uint64                    `json:"id"`
	OK       bool                      `json:"ok"`
	View     *ManifestViewPin          `json:"view"`
	StoreID  string                    `json:"store_id"`
	Snapshot *EventSnapshot            `json:"snapshot"`
	Status   *gymevent.EventStatus     `json:"status"`
	Record   *gymevent.SnapshotReceipt `json:"record"`
	History  *gymevent.HistoryPage     `json:"history"`
	Job      *EventJob                 `json:"job"`
	Error    string                    `json:"error"`
}
type EventSnapshot struct {
	Version                uint32           `json:"version"`
	State                  string           `json:"state"`
	Error                  string           `json:"error"`
	View                   *ManifestViewPin `json:"view"`
	StoreID                string           `json:"store_id"`
	Pin                    *gymevent.Pin    `json:"pin"`
	CoordinatorCreatedAtMS int64            `json:"coordinator_created_at_ms"`
	ParticipantID          string           `json:"participant_id"`
	ActiveJobID            string           `json:"active_job_id"`
	QueuedJobID            string           `json:"queued_job_id"`
	LastJobID              string           `json:"last_job_id"`
}
type EventJob struct {
	IneligibleProspectIDs []string                `json:"ineligible_prospect_ids,omitempty"`
	Version               uint32                  `json:"version"`
	JobID                 string                  `json:"job_id"`
	Kind                  string                  `json:"kind"`
	State                 string                  `json:"state"`
	View                  ManifestViewPin         `json:"view"`
	StoreID               string                  `json:"store_id"`
	EventID               string                  `json:"event_id"`
	Saturday              string                  `json:"saturday"`
	ParticipantID         string                  `json:"participant_id"`
	Error                 string                  `json:"error"`
	Result                *gymevent.CaptureResult `json:"result"`
}
