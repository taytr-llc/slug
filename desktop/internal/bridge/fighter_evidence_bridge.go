// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"time"
)

type evidenceExpectation struct {
	ordinal         uint64
	job             pilot.FighterEvidenceJob
	sourceDigest    string
	count           uint32
	asOf            int64
	selectionDigest string
	terminal        string
	rejected        bool
}

func evidenceFailure(id uint64, code string) pilot.FighterEvidenceResponse {
	if id == 0 {
		id = 1
	}
	return pilot.FighterEvidenceResponse{ID: id, Version: 1, Error: code}
}

// HelperDone is observed termination, including helper-owned cleanup.
func (b *StableBridge) HelperDone() <-chan struct{} { return b.done }
func (b *StableBridge) CaptureFighterEvidence(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	return b.captureFighterEvidence(ctx, v, id, b.CareerHistoryRequest)
}
func (b *PoolBridge) CaptureFighterEvidence(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	return b.captureFighterEvidence(ctx, v, id, b.CareerHistoryRequest)
}
func (b *StableBridge) captureFighterEvidence(ctx context.Context, v, id string, request func(context.Context, pilot.CareerHistoryRequest) pilot.CareerHistoryResponse) (pilot.CareerHistoryJob, error) {
	r := request(ctx, pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: v, JobID: id})
	if !r.OK || r.Job == nil || r.Job.State != "done" || r.Job.Capture == nil {
		return pilot.CareerHistoryJob{}, ErrUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.history == nil || b.history.selected != id || b.view == nil || r.Job.View != *b.view {
		return pilot.CareerHistoryJob{}, ErrUnavailable
	}
	return *r.Job, nil
}

// Remember captures trusted selected metadata before the native begin request.
// The browser cannot provide any of these expected claims or destination data.
func (b *StableBridge) RememberFighterEvidence(j pilot.CareerHistoryJob, id string, pin recoveryfile.Pin) error {
	if pilot.ValidateCareerHistoryJob(j) != nil || j.State != "done" || j.Capture == nil || recoveryfile.ValidatePin(pin, recoveryfile.CreateMode) != nil {
		return ErrUnavailable
	}
	req := pilot.FighterEvidenceRequest{ID: 1, Version: 1, Command: "fighter_evidence_begin", ViewID: j.View.ViewID, HistoryJobID: j.JobID, HistoryID: j.Capture.HistoryID, JobID: id, Selection: &pin}
	if _, e := pilot.MarshalFighterEvidenceRequest(req); e != nil {
		return ErrUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.history == nil || b.view == nil || j.View != *b.view || b.history.selected != j.JobID || j.StoreID != b.storeID {
		return ErrUnavailable
	}
	t, n := b.history.tuple(j.JobID)
	if n < 0 || t.CaptureSHA256 != historyDigest(*j.Capture) || t.HistoryID != j.Capture.HistoryID {
		return ErrUnavailable
	}
	slot := -1
	for i, e := range b.evidence {
		if e != nil && e.job.JobID == id {
			return ErrUnavailable
		}
		if slot < 0 && (e == nil || e.rejected) {
			slot = i
		}
	}
	if slot < 0 {
		for i, e := range b.evidence {
			if e.terminal != "" && (slot < 0 || e.ordinal < b.evidence[slot].ordinal) {
				slot = i
			}
		}
	}
	if slot < 0 {
		return ErrBusy
	}
	b.evidenceSerial++
	b.evidence[slot] = &evidenceExpectation{ordinal: b.evidenceSerial, job: pilot.FighterEvidenceJob{Version: 1, JobID: id, State: "running", View: j.View, StoreID: j.StoreID, HistoryJobID: j.JobID, HistoryID: j.Capture.HistoryID, ProspectID: j.ProspectID}, sourceDigest: j.Capture.Assignment.SourceRecordDigest, count: j.Capture.EventCount, asOf: j.Capture.AsOfMS, selectionDigest: historyDigest(pin)}
	return nil
}
func (b *StableBridge) evidenceKnownLocked(id string) *evidenceExpectation {
	for _, e := range b.evidence {
		if e != nil && !e.rejected && e.job.JobID == id {
			return e
		}
	}
	return nil
}
func (b *StableBridge) FighterEvidenceRequest(ctx context.Context, r pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
	return b.fighterEvidenceRequest(ctx, r, b.ResultRequest)
}
func (b *PoolBridge) FighterEvidenceRequest(ctx context.Context, r pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
	return b.fighterEvidenceRequest(ctx, r, b.ResultRequest)
}
func (b *StableBridge) fighterEvidenceRequest(ctx context.Context, r pilot.FighterEvidenceRequest, prepare func(context.Context, pilot.ResultRequest) pilot.ResultResponse) pilot.FighterEvidenceResponse {
	pre := r
	pre.ID, pre.Version = 1, 1
	if ctx == nil {
		return evidenceFailure(1, "invalid_request")
	}
	if _, e := pilot.MarshalFighterEvidenceRequest(pre); e != nil {
		return evidenceFailure(1, "invalid_request")
	}
	if !b.careersEnabled {
		return evidenceFailure(1, "evidence_unavailable")
	}
	if r.Command == "fighter_evidence_snapshot" {
		if _, _, ready := b.resultContext(); !ready {
			p := prepare(ctx, pilot.ResultRequest{Command: "result_snapshot"})
			if !p.OK {
				return evidenceFailure(1, "evidence_unavailable")
			}
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) || errors.Is(e, context.DeadlineExceeded) {
			return evidenceFailure(1, "busy")
		}
		return evidenceFailure(1, "evidence_unavailable")
	}
	defer func() { <-b.gate }()
	r.ID, r.Version = id, 1
	b.mu.Lock()
	valid := b.evidenceRequestBoundLocked(r)
	expected := b.evidenceKnownLocked(r.JobID)
	b.mu.Unlock()
	if !valid {
		return evidenceFailure(id, "stale_view")
	}
	raw, e := pilot.MarshalFighterEvidenceRequest(r)
	if e != nil {
		return evidenceFailure(id, "invalid_request")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, e := b.exchangeExpected(work, &stableExpected{id: id, evidence: true}, raw)
	if e != nil || f.evidence == nil {
		return evidenceFailure(id, "outcome_unknown")
	}
	out := *f.evidence
	if !out.OK {
		if r.Command == "fighter_evidence_begin" && out.Error != "outcome_unknown" {
			b.mu.Lock()
			if expected != nil {
				expected.rejected = true
			}
			b.mu.Unlock()
		}
		return out
	}
	if !b.correlateFighterEvidence(r, out) {
		b.fail(ErrUnavailable)
		return evidenceFailure(id, "outcome_unknown")
	}
	return out
}
func (b *StableBridge) correlateFighterEvidence(r pilot.FighterEvidenceRequest, out pilot.FighterEvidenceResponse) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || out.ID != r.ID || !out.OK {
		return false
	}
	if s := out.Snapshot; s != nil {
		if r.Command != "fighter_evidence_snapshot" || out.Job != nil || pilot.ValidateFighterEvidenceSnapshot(*s) != nil {
			return false
		}
		if s.View == nil {
			return true
		}
		if b.view == nil || *s.View != *b.view || s.StoreID != b.storeID || r.ViewID != "" && r.ViewID != s.View.ViewID {
			return false
		}
		for _, id := range []string{s.ActiveJobID, s.LastJobID} {
			if id != "" {
				e := b.evidenceKnownLocked(id)
				if e == nil || e.job.View != *s.View || e.job.StoreID != s.StoreID {
					return false
				}
			}
		}
		return true
	}
	j := out.Job
	if j == nil || r.Command == "fighter_evidence_snapshot" || pilot.ValidateFighterEvidenceJob(*j) != nil || j.JobID != r.JobID {
		return false
	}
	e := b.evidenceKnownLocked(j.JobID)
	if e == nil || b.view == nil || j.View != *b.view || j.View != e.job.View || j.StoreID != e.job.StoreID || j.HistoryJobID != e.job.HistoryJobID || j.HistoryID != e.job.HistoryID || j.ProspectID != e.job.ProspectID {
		return false
	}
	if e.terminal != "" && e.terminal != historyDigest(*j) {
		return false
	}
	if s := j.Summary; s != nil {
		if s.SourceRecordDigest != e.sourceDigest || s.EventCount != e.count || s.CapturedAsOfMS != e.asOf {
			return false
		}
	}
	if j.State != "running" {
		e.terminal = historyDigest(*j)
	}
	e.job = *j
	return true
}

// The native-held operation, never helper latest-job discovery, supplies trust.
func (b *StableBridge) evidenceRequestBoundLocked(r pilot.FighterEvidenceRequest) bool {
	if b.failure != nil {
		return false
	}
	if r.Command == "fighter_evidence_snapshot" {
		return true
	}
	expected := b.evidenceKnownLocked(r.JobID)
	if expected == nil || b.view == nil || r.ViewID != b.view.ViewID || expected.job.View != *b.view {
		return false
	}
	if r.Command == "fighter_evidence_begin" {
		return r.Selection != nil && expected.job.HistoryJobID == r.HistoryJobID && expected.job.HistoryID == r.HistoryID && historyDigest(*r.Selection) == expected.selectionDigest
	}
	return true
}
