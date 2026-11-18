// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/internal/careerstate"
	"time"
)

type careerPreviewJobExpectation struct {
	View                                                            pilot.ManifestViewPin
	StoreID, ActivationID, A, B, state, terminalDigest              string
	CareerDigest, Seed, InputID, ResultDigest, CapturedResultDigest string
	AsOfMS                                                          int64
}

func careerPreviewCapturedDigest(r *pilot.CareerPreviewResult) string {
	if r == nil {
		return ""
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > pilot.MaxCareerPreviewResultSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func careerPreviewTerminalDigest(j pilot.CareerPreviewJob) string {
	raw, e := json.Marshal(j)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func careerPreviewFailure(id uint64, code string) pilot.CareerPreviewResponse {
	if id == 0 {
		id = 1
	}
	return pilot.CareerPreviewResponse{ID: id, Version: 1, Error: code}
}
func (b *StableBridge) clearCareerPreviewView() {
	b.careerPreviewReady = false
	b.careerPreviewJobs = nil
	b.careerPreviewSnapshotJobs = nil
	b.clearCareerPreviewPresentation()
}

func (b *StableBridge) CareerPreviewRequest(ctx context.Context, req pilot.CareerPreviewRequest) pilot.CareerPreviewResponse {
	if !b.careersEnabled {
		return careerPreviewFailure(0, "career_inactive")
	}
	if req.Command == "career_preview_begin" && req.FighterBID < req.FighterAID {
		req.FighterAID, req.FighterBID = req.FighterBID, req.FighterAID
	}
	pre := req
	pre.ID = 1
	pre.Version = 1
	if _, err := pilot.MarshalCareerPreviewRequest(pre); err != nil {
		return careerPreviewFailure(0, "invalid_request")
	}
	if req.Command == "career_preview_snapshot" {
		// Result discovery itself binds every earlier source and the original full
		// View. An activation embedded in a preview cannot create that authority.
		if _, _, ready := b.resultContext(); !ready {
			r := b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_snapshot"})
			if !r.OK {
				code := "career_inactive"
				if oneOf(r.Error, "busy", "unauthorized", "forbidden", "stale_view", "outcome_unknown") {
					code = r.Error
				}
				return careerPreviewFailure(0, code)
			}
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return careerPreviewFailure(0, "busy")
		}
		return careerPreviewFailure(0, "preview_unavailable")
	}
	defer func() { <-b.gate }()
	return b.careerPreviewRequestHeld(ctx, id, req)
}
func (b *StableBridge) careerPreviewRequestHeld(ctx context.Context, id uint64, req pilot.CareerPreviewRequest) pilot.CareerPreviewResponse {
	req.ID, req.Version = id, 1
	raw, err := pilot.MarshalCareerPreviewRequest(req)
	if err != nil {
		return careerPreviewFailure(id, "invalid_request")
	}
	// An original-session command remains stale after logout clears its pinned
	// View. Snapshot discovery and transport failure keep their own classification.
	if req.Command != "career_preview_snapshot" && (!b.previewPinned() || req.ViewID != b.view.ViewID) {
		return careerPreviewFailure(id, "stale_view")
	}
	if !b.previewPinned() {
		return careerPreviewFailure(id, "preview_unavailable")
	}
	if req.Command != "career_preview_snapshot" {
		if !b.careerPreviewReady || b.careerActivation == nil {
			return careerPreviewFailure(id, "career_inactive")
		}
	}
	if oneOf(req.Command, "career_preview_job", "career_preview_cancel") {
		if _, ok := b.careerPreviewJobs[req.JobID]; !ok && !b.careerPreviewSnapshotJobs[req.JobID] {
			return careerPreviewFailure(id, "preview_not_found")
		}
	}
	if req.Command == "career_preview_begin" && !b.canTrackPreview() {
		return careerPreviewFailure(id, "busy")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	frame, err := b.exchangeCareerPreviewFamily(work, id, false, false, false, false, false, false, false, false, false, true, raw)
	if err != nil || frame.careerPreview == nil {
		b.clearCareerPreviewView()
		if req.Command == "career_preview_begin" {
			return careerPreviewFailure(id, "outcome_unknown")
		}
		return careerPreviewFailure(id, "preview_unavailable")
	}
	r := *frame.careerPreview
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if oneOf(r.Error, "preview_unavailable", "career_inactive", "career_closed", "career_poisoned", "career_corrupt") {
			b.clearCareerPreviewView()
		}
		if r.Error == "preview_not_found" {
			b.mu.Lock()
			if b.presentation != nil && b.presentation.metadata.Family == careerPreviewFamily && b.presentation.metadata.JobID == req.JobID {
				b.presentation = nil
			}
			b.mu.Unlock()
		}
		return r
	}
	if !b.correlateCareerPreview(req, r) {
		b.fail(ErrUnavailable)
		b.clearCareerPreviewView()
		if req.Command == "career_preview_begin" {
			return careerPreviewFailure(id, "outcome_unknown")
		}
		return careerPreviewFailure(id, "preview_unavailable")
	}
	if r.Snapshot != nil {
		b.rememberCareerPreviewSnapshot(*r.Snapshot)
	}
	if r.Job != nil {
		if req.Command == "career_preview_begin" {
			b.selectPreview(careerPreviewFamily, r.Job.JobID)
		}
		if !b.rememberCareerPreviewJob(*r.Job) {
			b.fail(ErrUnavailable)
			return careerPreviewFailure(id, "preview_unavailable")
		}
	}
	if !b.previewMetadataBound() {
		b.fail(ErrUnavailable)
		return careerPreviewFailure(id, "preview_capacity")
	}
	return r
}
func (b *StableBridge) careerPreviewActivationValid(a careerstate.Activation) bool {
	authority, v, ready := b.resultContext()
	return ready && b.view != nil && v == *b.view && a.ResultStoreID == authority.storeID && a.ResultCreatedAtMS == authority.createdMS && a.SourceCreatedAtMS == authority.sourceCreatedMS && (b.careerActivation == nil || *b.careerActivation == a)
}
func (b *StableBridge) correlateCareerPreview(req pilot.CareerPreviewRequest, r pilot.CareerPreviewResponse) bool {
	if req.ID != r.ID {
		return false
	}
	if req.Command == "career_preview_snapshot" {
		s := r.Snapshot
		if s == nil || r.Job != nil {
			return false
		}
		if s.View == nil {
			return s.State == "unavailable"
		}
		return b.previewPinned() && *s.View == *b.view && s.StoreID == b.storeID && s.EngineVersion == fighterbout.Version1 && s.EngineID == fighterbout.EngineID && s.Activation != nil && b.careerPreviewActivationValid(*s.Activation)
	}
	j := r.Job
	if j == nil || r.Snapshot != nil || !b.previewPinned() || b.careerActivation == nil || j.View != *b.view || j.StoreID != b.storeID || j.EngineVersion != fighterbout.Version1 || j.EngineID != fighterbout.EngineID || j.ActivationID != b.careerActivation.ActivationID {
		return false
	}
	if req.Command == "career_preview_begin" {
		if j.State != "queued" || j.FighterAID != req.FighterAID || j.FighterBID != req.FighterBID {
			return false
		}
		if _, ok := b.careerPreviewJobs[j.JobID]; ok {
			return false
		}
	} else {
		if j.JobID != req.JobID {
			return false
		}
		if e, ok := b.careerPreviewJobs[j.JobID]; ok {
			if j.View != e.View || j.StoreID != e.StoreID || j.ActivationID != e.ActivationID || j.FighterAID != e.A || j.FighterBID != e.B || e.state == "running" && j.State == "queued" {
				return false
			}
			if e.terminalDigest != "" && careerPreviewTerminalDigest(*j) != e.terminalDigest {
				return false
			}
			if e.CapturedResultDigest != "" && (j.Result == nil || careerPreviewCapturedDigest(j.Result) != e.CapturedResultDigest) {
				return false
			}
		} else if !b.careerPreviewSnapshotJobs[j.JobID] {
			return false
		}
	}
	if j.Result != nil {
		p := j.Result.Pair
		if p.Status.StoreID != b.storeID || b.stablePin == nil || p.Status.Pin != *b.stablePin || p.Activation != *b.careerActivation || p.Fighters[0].Assignment.ProspectID != j.FighterAID || p.Fighters[1].Assignment.ProspectID != j.FighterBID {
			return false
		}
	}
	return true
}
func (b *StableBridge) rememberCareerPreviewSnapshot(s pilot.CareerPreviewSnapshot) {
	if s.View == nil {
		b.clearCareerPreviewView()
		return
	}
	a := *s.Activation
	b.careerActivation = &a
	b.careerPreviewReady = true
	b.careerPreviewSnapshotJobs = make(map[string]bool, 3)
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.careerPreviewSnapshotJobs[id] = true
		}
	}
	for id := range b.careerPreviewJobs {
		if !b.careerPreviewSnapshotJobs[id] {
			delete(b.careerPreviewJobs, id)
		}
	}
	b.mu.Lock()
	if b.presentation != nil && b.presentation.metadata.Family == careerPreviewFamily && b.presentation.metadata.JobID != s.LastJobID {
		b.presentation = nil
	}
	b.mu.Unlock()
}
func (b *StableBridge) rememberCareerPreviewJob(j pilot.CareerPreviewJob) bool {
	if b.careerPreviewJobs == nil {
		b.careerPreviewJobs = make(map[string]careerPreviewJobExpectation)
	}
	if stableTerminal(j.State) {
		for id, e := range b.careerPreviewJobs {
			if id != j.JobID && stableTerminal(e.state) {
				delete(b.careerPreviewJobs, id)
			}
		}
	}
	if _, ok := b.careerPreviewJobs[j.JobID]; !ok && len(b.previewJobs)+len(b.careerPreviewJobs) >= 3 {
		return false
	}
	e := careerPreviewJobExpectation{View: j.View, StoreID: j.StoreID, ActivationID: j.ActivationID, A: j.FighterAID, B: j.FighterBID, state: j.State}
	if stableTerminal(j.State) {
		e.terminalDigest = careerPreviewTerminalDigest(j)
		if e.terminalDigest == "" {
			return false
		}
	}
	if j.Result != nil {
		r := j.Result
		e.Seed, e.InputID, e.ResultDigest = r.Seed, r.EngineResult.InputID, r.EngineResultDigest
		e.CareerDigest, e.AsOfMS = r.Pair.CareerDigest, r.Pair.AsOfMS
		e.CapturedResultDigest = careerPreviewCapturedDigest(r)
		if e.CapturedResultDigest == "" {
			return false
		}
		meta := previewPresentationMetadata{Family: careerPreviewFamily, ActivationID: j.ActivationID, CareerDigest: e.CareerDigest, CapturedResultDigest: e.CapturedResultDigest, AsOfMS: e.AsOfMS, View: j.View, StoreID: j.StoreID, JobID: j.JobID, EngineVersion: j.EngineVersion, EngineID: j.EngineID, Seed: e.Seed, InputID: e.InputID, ResultDigest: e.ResultDigest, DescriptorID: r.Pair.Status.Pin.Source.DescriptorID}
		for i, f := range r.Pair.Fighters {
			meta.Members[i] = previewMember{ID: f.Assignment.ProspectID, ManifestDigest: f.Assignment.ManifestDigest, PortraitDigest: f.Assignment.PortraitDigest, Manifest: f.Manifest}
		}
		b.mu.Lock()
		if b.failure != nil {
			b.mu.Unlock()
			return false
		}
		if b.previewMayPresentLocked(careerPreviewFamily, j.JobID) && (b.presentation == nil || b.presentation.metadata != meta) {
			b.presentation = &previewPresentation{metadata: meta}
		}
		b.mu.Unlock()
	}
	b.careerPreviewJobs[j.JobID] = e
	return b.previewMetadataBound()
}
