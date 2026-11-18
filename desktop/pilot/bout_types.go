// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
)

const MaxBoutRequestSize = 8192
const MaxBoutResponseSize = 262144
const MaxBoutJobSize = 253952
const MaxBoutMetadataSize = 73728
const MaxBoutPortraitBytes = 65536
const MaxBoutCacheSize = 393216

type BoutControllerAPI interface {
	CardControllerAPI
	BoutSnapshot() BoutSnapshot
	BoutCardStatus(context.Context, string, string) (gymbout.CardStatus, error)
	BoutStatus(context.Context, string, string, string) (gymbout.Status, error)
	BoutRecord(context.Context, string, string, string) (gymbout.ReadResult, error)
	BeginBoutRun(context.Context, string, string, string) (BoutJob, error)
	BoutJob(context.Context, string, string) (BoutJob, error)
	CancelBoutJob(context.Context, string, string) (BoutJob, error)
}
type BoutRequest struct {
	ID       uint64 `json:"id"`
	Command  string `json:"command"`
	ViewID   string `json:"view_id"`
	Saturday string `json:"saturday"`
	BoutID   string `json:"bout_id"`
	JobID    string `json:"job_id"`
}
type BoutResponse struct {
	ID         uint64              `json:"id"`
	OK         bool                `json:"ok"`
	View       *ManifestViewPin    `json:"view"`
	StoreID    string              `json:"store_id"`
	Snapshot   *BoutSnapshot       `json:"snapshot"`
	CardStatus *gymbout.CardStatus `json:"card_status"`
	Status     *gymbout.Status     `json:"status"`
	Record     *gymbout.ReadResult `json:"record"`
	Job        *BoutJob            `json:"job"`
	Error      string              `json:"error"`
}
type BoutSnapshot struct {
	Version           uint32           `json:"version"`
	State             string           `json:"state"`
	Error             string           `json:"error"`
	View              *ManifestViewPin `json:"view"`
	StoreID           string           `json:"store_id"`
	Pin               *gymbout.Pin     `json:"pin"`
	SourceCreatedAtMS int64            `json:"source_created_at_ms"`
	CreatedAtMS       int64            `json:"created_at_ms"`
	ActiveJobID       string           `json:"active_job_id"`
	QueuedJobID       string           `json:"queued_job_id"`
	LastJobID         string           `json:"last_job_id"`
}
type BoutJob struct {
	Version  uint32             `json:"version"`
	JobID    string             `json:"job_id"`
	Kind     string             `json:"kind"`
	State    string             `json:"state"`
	View     ManifestViewPin    `json:"view"`
	StoreID  string             `json:"store_id"`
	CardID   string             `json:"card_id"`
	Saturday string             `json:"saturday"`
	BoutID   string             `json:"bout_id"`
	Error    string             `json:"error"`
	Result   *gymbout.RunResult `json:"result"`
}
