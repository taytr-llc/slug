// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"sync"
)

const (
	MaxFighterEvidenceRequestSize  = 4096
	MaxFighterEvidenceResponseSize = 8192
	MaxFighterEvidenceJobSize      = 4096
	FighterEvidenceScratchBytes    = 4 * 1024 * 1024
)

var (
	ErrEvidenceUnavailable       = errors.New("evidence unavailable")
	ErrEvidenceNotFound          = errors.New("evidence operation not found")
	ErrEvidenceOperationConflict = errors.New("evidence operation ID already used")
	ErrEvidenceReplyUnknown      = errors.New("evidence dispatch reply unknown")
)

// EvidencePermit accounts for one export across all children of one parent.
// Its fixed reservation is not a measured hard Go heap cap. The owner releases
// only after proof and file cleanup join. A standalone controller uses its own.
type EvidencePermit struct {
	mu      sync.Mutex
	owner   any
	failure error
}

func (p *EvidencePermit) reserve(owner any) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if owner == nil || p.owner != nil || p.failure != nil {
		return false
	}
	p.owner = owner
	return true
}
func (p *EvidencePermit) release(owner any, cleanup error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.owner != owner {
		return
	}
	p.failure = errors.Join(p.failure, cleanup)
	p.owner = nil
}

type FighterEvidenceSurfaceAPI interface {
	FighterEvidenceSnapshot(context.Context) (FighterEvidenceSnapshot, error)
	BeginFighterEvidence(context.Context, string, string, string, string, recoveryfile.Pin) (FighterEvidenceJob, error)
	FighterEvidenceJob(context.Context, string, string) (FighterEvidenceJob, error)
	CancelFighterEvidence(context.Context, string, string) (FighterEvidenceJob, error)
}
type FighterEvidenceRequest struct {
	ID           uint64            `json:"id"`
	Version      uint32            `json:"version"`
	Command      string            `json:"command"`
	ViewID       string            `json:"view_id"`
	HistoryJobID string            `json:"history_job_id"`
	HistoryID    string            `json:"history_id"`
	JobID        string            `json:"job_id"`
	Selection    *recoveryfile.Pin `json:"selection"`
}
type FighterEvidenceResponse struct {
	ID       uint64                   `json:"id"`
	Version  uint32                   `json:"version"`
	OK       bool                     `json:"ok"`
	Snapshot *FighterEvidenceSnapshot `json:"snapshot"`
	Job      *FighterEvidenceJob      `json:"job"`
	Error    string                   `json:"error"`
}
type FighterEvidenceSnapshot struct {
	Version     uint32           `json:"version"`
	State       string           `json:"state"`
	Error       string           `json:"error"`
	View        *ManifestViewPin `json:"view"`
	StoreID     string           `json:"store_id"`
	ActiveJobID string           `json:"active_job_id"`
	LastJobID   string           `json:"last_job_id"`
}
type FighterEvidenceJob struct {
	Version      uint32                  `json:"version"`
	JobID        string                  `json:"job_id"`
	State        string                  `json:"state"`
	View         ManifestViewPin         `json:"view"`
	StoreID      string                  `json:"store_id"`
	HistoryJobID string                  `json:"history_job_id"`
	HistoryID    string                  `json:"history_id"`
	ProspectID   string                  `json:"prospect_id"`
	Error        string                  `json:"error"`
	Summary      *FighterEvidenceSummary `json:"summary"`
}

// Summary is exported-file metadata, never an offline verification Report.
type FighterEvidenceSummary struct {
	Version            uint32 `json:"version"`
	FileSHA256         string `json:"file_sha256"`
	FileBytes          uint32 `json:"file_bytes"`
	ProspectID         string `json:"prospect_id"`
	HistoryID          string `json:"history_id"`
	SourceRecordDigest string `json:"source_record_digest"`
	EventCount         uint32 `json:"event_count"`
	CapturedAsOfMS     int64  `json:"captured_as_of_ms"`
}

func BlankFighterEvidenceSnapshot() FighterEvidenceSnapshot {
	return FighterEvidenceSnapshot{Version: 1, State: "unavailable", Error: "evidence_unavailable"}
}
