// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"sync"
)

type nativeEvidencePending struct {
	selection     *recoveryfile.Selection
	viewID, jobID string
	helper        *bridge.StableBridge
	done          chan struct{}
}
type nativeEvidenceState struct {
	mu      sync.Mutex
	pending *nativeEvidencePending
	action  *nativeEvidenceAction
}
type nativeEvidenceAction struct {
	helper                                        *bridge.StableBridge
	viewID, historyJobID, historyID, jobID, state string
}

func evidenceNativeFailure(code string) pilot.FighterEvidenceResponse {
	return pilot.FighterEvidenceResponse{ID: 1, Version: 1, Error: code}
}
func (g *Gym) evidenceRequest(r pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
	if g.poolHelper != nil {
		return g.poolHelper.FighterEvidenceRequest(context.Background(), r)
	}
	if g.stableHelper != nil {
		return g.stableHelper.FighterEvidenceRequest(context.Background(), r)
	}
	return evidenceNativeFailure("evidence_unavailable")
}
func (g *Gym) releaseEvidenceSelection(p *nativeEvidencePending) error {
	g.evidence.mu.Lock()
	if p == nil {
		p = g.evidence.pending
	}
	if p == nil || g.evidence.pending != p {
		g.evidence.mu.Unlock()
		return nil
	}
	g.evidence.pending = nil
	close(p.done)
	g.evidence.mu.Unlock()
	return p.selection.Close()
}

// Called only after observed helper termination/Close, never an asynchronous
// logout/reset/login acknowledgment.
func (g *Gym) joinedEvidenceSelection() error {
	g.evidence.mu.Lock()
	g.evidence.action = nil
	g.evidence.mu.Unlock()
	return g.releaseEvidenceSelection(nil)
}

// Callers supply a bridge-validated account snapshot from this exact helper.
// Transitional/failed/expired states do not prove old-child cleanup has joined.
func (g *Gym) observeEvidenceAccount(helper *bridge.StableBridge, state string, view *pilot.ManifestViewPin) error {
	g.evidence.mu.Lock()
	p := g.evidence.pending
	joined := p != nil && p.helper == helper && (state == "signed_out" && view == nil || state == "signed_in" && view != nil && view.ViewID != p.viewID)
	if a := g.evidence.action; a != nil && a.helper == helper && (state == "signed_out" && view == nil || state == "signed_in" && view != nil && view.ViewID != a.viewID) {
		g.evidence.action = nil
	}
	g.evidence.mu.Unlock()
	if joined {
		return g.releaseEvidenceSelection(p)
	}
	return nil
}
func (g *Gym) BeginFighterEvidenceExport(viewID, historyJobID string) (*pilot.FighterEvidenceResponse, error) {
	if g.stableHelper == nil {
		r := evidenceNativeFailure("evidence_unavailable")
		return &r, nil
	}
	if !g.exportMu.TryLock() {
		r := evidenceNativeFailure("busy")
		return &r, nil
	}
	defer g.exportMu.Unlock()
	g.evidence.mu.Lock()
	pending := g.evidence.pending != nil
	g.evidence.mu.Unlock()
	if pending {
		r := evidenceNativeFailure("busy")
		return &r, nil
	}
	// Even a pre-dialog/pre-Remember failure is a new action. It must never
	// recover a previous successful file for the same history selection.
	action := &nativeEvidenceAction{helper: g.stableHelper, viewID: viewID, historyJobID: historyJobID, state: "preparing"}
	g.evidence.mu.Lock()
	g.evidence.action = action
	g.evidence.mu.Unlock()
	defer func() {
		g.evidence.mu.Lock()
		if g.evidence.action == action && action.state == "preparing" {
			action.state = "not_started"
		}
		g.evidence.mu.Unlock()
	}()
	// Capture the original account and completed history before the native dialog.
	if g.poolHelper != nil {
		s, e := g.PoolSnapshot()
		if e != nil || s.View == nil || s.View.ViewID != viewID {
			return nil, bridge.ErrUnavailable
		}
		if s.State != "stopped" && s.State != "failed" || s.BackupState == "running" {
			return nil, errors.New("Stop mining and wait for the current task to finish before exporting public evidence.")
		}
	} else {
		s, e := g.ManifestSnapshot()
		if e != nil || s.View == nil || s.View.ViewID != viewID {
			return nil, bridge.ErrUnavailable
		}
		if s.State != "stopped" && s.State != "failed" {
			return nil, errors.New("Stop mining and wait for the current task to finish before exporting public evidence.")
		}
	}
	var j pilot.CareerHistoryJob
	var e error
	if g.poolHelper != nil {
		j, e = g.poolHelper.CaptureFighterEvidence(context.Background(), viewID, historyJobID)
	} else {
		j, e = g.stableHelper.CaptureFighterEvidence(context.Background(), viewID, historyJobID)
	}
	if e != nil {
		return nil, e
	}
	var nonce [32]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return nil, bridge.ErrUnavailable
	}
	id := hex.EncodeToString(nonce[:])
	path, e := g.app.Dialog.SaveFile().AttachToWindow(g.window).SetMessage("Save the original public receipt and this captured fighter history.").SetFilename("slug-fighter-evidence-"+j.ProspectID[:12]+".json").AddFilter("Public fighter evidence", "*.json").AllowsOtherFileTypes(false).CanCreateDirectories(true).PromptForSingleSelection()
	if e != nil {
		return nil, errors.New("The save dialog is unavailable.")
	}
	if path == "" {
		return nil, nil
	}
	selection, e := recoveryfile.Capture(path, g.protectedRoots, recoveryfile.CreateMode)
	if e != nil {
		return nil, errors.New("Choose a new evidence destination outside application data.")
	}
	pin := selection.Pin()
	if e = g.stableHelper.RememberFighterEvidence(j, id, pin); e != nil {
		_ = selection.Close()
		return nil, e
	}
	p := &nativeEvidencePending{selection: selection, viewID: viewID, jobID: id, helper: g.stableHelper, done: make(chan struct{})}
	g.evidence.mu.Lock()
	if g.evidence.action != action {
		g.evidence.mu.Unlock()
		_ = selection.Close()
		r := evidenceNativeFailure("stale_view")
		return &r, nil
	}
	action.historyID, action.jobID, action.state = j.Capture.HistoryID, id, "unknown"
	g.evidence.pending = p
	g.evidence.mu.Unlock()
	// A transport failure closes the helper's input. Retain the pinned directory
	// until observed process exit proves that no late Open can still happen.
	go func() {
		select {
		case <-p.helper.HelperDone():
			_ = g.releaseEvidenceSelection(p)
		case <-p.done:
		}
	}()
	r := g.evidenceRequest(pilot.FighterEvidenceRequest{Command: "fighter_evidence_begin", ViewID: viewID, HistoryJobID: j.JobID, HistoryID: j.Capture.HistoryID, JobID: id, Selection: &pin})
	g.evidence.mu.Lock()
	if g.evidence.action == action {
		if r.OK {
			action.state = "admitted"
		} else if r.Error != "outcome_unknown" {
			action.state = "not_started"
		}
	}
	g.evidence.mu.Unlock()
	if r.OK || r.Error != "outcome_unknown" {
		if e = g.releaseEvidenceSelection(p); e != nil {
			r = evidenceNativeFailure("outcome_unknown")
		}
	}
	return &r, nil
}
func (g *Gym) FighterEvidenceSnapshot() (pilot.FighterEvidenceResponse, error) {
	g.evidence.mu.Lock()
	p := g.evidence.pending
	g.evidence.mu.Unlock()
	if p != nil {
		// Snapshot observation can prove an asynchronous account transition drained.
		if g.poolHelper != nil {
			_, _ = g.PoolSnapshot()
		} else {
			_, _ = g.ManifestSnapshot()
		}
	}
	g.evidence.mu.Lock()
	current := g.evidence.action
	var a nativeEvidenceAction
	if current != nil {
		a = *current
	}
	g.evidence.mu.Unlock()
	r, resolved := nativeEvidenceActionSnapshot(a, g.evidenceRequest)
	g.evidence.mu.Lock()
	same := g.evidence.action == current
	if same && current != nil && resolved != "" {
		current.state = resolved
	}
	g.evidence.mu.Unlock()
	if !same {
		return evidenceNativeFailure("stale_view"), nil
	}
	if resolved != "" && p != nil && p.jobID == a.jobID && p.viewID == a.viewID && p.helper == a.helper {
		if e := g.releaseEvidenceSelection(p); e != nil {
			return evidenceNativeFailure("outcome_unknown"), nil
		}
	}
	return r, nil
}
func (g *Gym) FighterEvidenceJob(viewID, jobID string) (pilot.FighterEvidenceResponse, error) {
	if !g.currentEvidenceAction(viewID, jobID) {
		return evidenceNativeFailure("evidence_not_found"), nil
	}
	return g.evidenceRequest(pilot.FighterEvidenceRequest{Command: "fighter_evidence_job", ViewID: viewID, JobID: jobID}), nil
}
func (g *Gym) CancelFighterEvidence(viewID, jobID string) (pilot.FighterEvidenceResponse, error) {
	if !g.currentEvidenceAction(viewID, jobID) {
		return evidenceNativeFailure("evidence_not_found"), nil
	}
	return g.evidenceRequest(pilot.FighterEvidenceRequest{Command: "fighter_evidence_cancel", ViewID: viewID, JobID: jobID}), nil
}
func (g *Gym) currentEvidenceAction(v, id string) bool {
	g.evidence.mu.Lock()
	defer g.evidence.mu.Unlock()
	a := g.evidence.action
	return a != nil && a.helper == g.stableHelper && a.viewID == v && a.jobID == id && (a.state == "unknown" || a.state == "admitted")
}

// Resolve one exact native action. A generic helper snapshot is used only for
// source presentation after definite no-start, with every operation ID removed.
func nativeEvidenceActionSnapshot(a nativeEvidenceAction, request func(pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse) (pilot.FighterEvidenceResponse, string) {
	if a.state == "preparing" {
		return evidenceNativeFailure("outcome_unknown"), ""
	}
	resolved := ""
	if a.jobID != "" && (a.state == "unknown" || a.state == "admitted") {
		r := request(pilot.FighterEvidenceRequest{Command: "fighter_evidence_job", ViewID: a.viewID, JobID: a.jobID})
		if r.OK && r.Job != nil {
			j := r.Job
			if j.JobID != a.jobID || j.View.ViewID != a.viewID || j.HistoryJobID != a.historyJobID || j.HistoryID != a.historyID {
				return evidenceNativeFailure("outcome_unknown"), ""
			}
			view := j.View
			s := pilot.FighterEvidenceSnapshot{Version: 1, State: "idle", View: &view, StoreID: j.StoreID, LastJobID: j.JobID}
			if j.State == "running" {
				s.State = "busy"
				s.ActiveJobID = j.JobID
				s.LastJobID = ""
			}
			return pilot.FighterEvidenceResponse{ID: r.ID, Version: 1, OK: true, Snapshot: &s}, "admitted"
		}
		if r.Error != "evidence_not_found" {
			if a.state == "unknown" {
				return evidenceNativeFailure("outcome_unknown"), ""
			}
			return r, ""
		}
		resolved = "not_started"
	}
	r := request(pilot.FighterEvidenceRequest{Command: "fighter_evidence_snapshot"})
	if r.OK && r.Snapshot != nil {
		owned := *r.Snapshot
		owned.ActiveJobID, owned.LastJobID = "", ""
		if owned.State != "unavailable" {
			owned.State = "idle"
		}
		r.Snapshot = &owned
	}
	return r, resolved
}
