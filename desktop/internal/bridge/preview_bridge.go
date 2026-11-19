// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
)

type previewJobExpectation struct {
	a, b, state, seed, inputID, digest, capturedResultDigest string
}

// Unlike the engine digest, this private correlation digest also binds the
// captured roster/observation metadata. A completed read must not be relabelled
// with a later status even when its deterministic fight recipe is unchanged.
func previewCapturedDigest(r *pilot.PreviewResult) string {
	raw, err := json.Marshal(r)
	if err != nil || r == nil || len(raw) > pilot.MaxPreviewResultSize {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// The presentation cache contains no second transcript, source receipt or
// condition. Its one tuple belongs to a checked completed job in this view.
type previewMember struct {
	ID, ManifestDigest, PortraitDigest string
	Manifest                           fightermanifest.Manifest
}
type previewPresentationMetadata struct {
	Family, ActivationID, CareerDigest, CapturedResultDigest string
	AsOfMS                                                   int64
	View                                                     pilot.ManifestViewPin
	StoreID, JobID                                           string
	EngineVersion                                            uint32
	EngineID, Seed, InputID, ResultDigest, DescriptorID      string
	Members                                                  [2]previewMember
}
type previewPresentation struct {
	metadata previewPresentationMetadata
	svgs     [2]string
}

type PreviewPortrait struct {
	Version        uint32                `json:"version"`
	View           pilot.ManifestViewPin `json:"view"`
	StoreID        string                `json:"store_id"`
	JobID          string                `json:"job_id"`
	ProspectID     string                `json:"prospect_id"`
	ResultDigest   string                `json:"result_digest"`
	DescriptorID   string                `json:"descriptor_id"`
	ManifestDigest string                `json:"manifest_digest"`
	PortraitDigest string                `json:"portrait_digest"`
	SVG            string                `json:"svg"`
}
type PreviewPortraitResponse struct {
	Version  uint32           `json:"version"`
	OK       bool             `json:"ok"`
	Portrait *PreviewPortrait `json:"portrait,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func previewFailure(id uint64, code string) pilot.PreviewResponse {
	if id == 0 {
		id = 1
	}
	return pilot.PreviewResponse{ID: id, Version: 1, Error: code}
}
func (b *StableBridge) clearPreviewPresentation() {
	b.mu.Lock()
	if b.presentation != nil && b.presentation.metadata.Family != careerPreviewFamily {
		b.presentation = nil
	}
	b.mu.Unlock()
}
func (b *StableBridge) clearPreviewView() {
	b.previewReady = false
	b.previewJobs = nil
	b.previewSnapshotJobs = nil
	b.clearPreviewPresentation()
}
func (b *StableBridge) previewPinned() bool {
	return b.view != nil && b.manifestView != nil && *b.view == *b.manifestView && b.storeID != "" && b.stablePin != nil
}

// PreviewRequest shares the existing pipe gate and request counter. Parsing
// verifies bounded shapes and digest bindings; it never evaluates work or runs
// the fight engine in the native process.
func (b *StableBridge) PreviewRequest(ctx context.Context, req pilot.PreviewRequest) pilot.PreviewResponse {
	if b.poolMode {
		return previewFailure(0, "preview_unavailable")
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return previewFailure(0, "busy")
		}
		return previewFailure(0, "preview_unavailable")
	}
	defer func() { <-b.gate }()
	return b.previewRequestHeld(ctx, id, req)
}
func (b *StableBridge) previewRequestHeld(ctx context.Context, id uint64, req pilot.PreviewRequest) pilot.PreviewResponse {
	req.ID, req.Version = id, 1
	if req.Command == "preview_begin" && req.FighterBID < req.FighterAID {
		req.FighterAID, req.FighterBID = req.FighterBID, req.FighterAID
	}
	raw, err := pilot.MarshalPreviewRequest(req)
	if err != nil {
		return previewFailure(id, "invalid_request")
	}
	if !b.previewPinned() {
		return previewFailure(id, "preview_unavailable")
	}
	if req.Command != "preview_snapshot" && (!b.previewReady || req.ViewID != b.view.ViewID) {
		return previewFailure(id, "stale_view")
	}
	if oneOf(req.Command, "preview_job", "preview_cancel") {
		if _, ok := b.previewJobs[req.JobID]; !ok && !b.previewSnapshotJobs[req.JobID] {
			return previewFailure(id, "preview_not_found")
		}
	}
	if req.Command == "preview_begin" && !b.canTrackPreview() {
		return previewFailure(id, "busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, err := b.exchangeFamily(ctx, id, false, true, raw)
	if err != nil || f.preview == nil {
		b.clearPreviewView()
		if req.Command == "preview_begin" {
			return previewFailure(id, "outcome_unknown")
		}
		return previewFailure(id, "preview_unavailable")
	}
	r := *f.preview
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if oneOf(r.Error, "preview_not_found", "preview_unavailable") {
			b.mu.Lock()
			if b.presentation != nil && b.presentation.metadata.Family != careerPreviewFamily && (req.JobID == b.presentation.metadata.JobID || r.Error == "preview_unavailable") {
				b.presentation = nil
			}
			b.mu.Unlock()
		}
		return r
	}
	if !b.correlatePreview(req, r) {
		b.fail(ErrUnavailable)
		b.clearPreviewView()
		if req.Command == "preview_begin" {
			return previewFailure(id, "outcome_unknown")
		}
		return previewFailure(id, "preview_unavailable")
	}
	if r.Snapshot != nil {
		b.rememberPreviewSnapshot(*r.Snapshot)
		if !b.previewMetadataBound() {
			b.fail(ErrUnavailable)
			return previewFailure(id, "preview_capacity")
		}
	}
	if r.Job != nil {
		if req.Command == "preview_begin" {
			// Only successful internal acceptance replaces the previous replay.
			b.selectPreview(legacyPreviewFamily, r.Job.JobID)
			for key, e := range b.previewJobs {
				if stableTerminal(e.state) {
					delete(b.previewJobs, key)
				}
			}
			b.previewSnapshotJobs = nil
		}
		if !b.rememberPreviewJob(*r.Job) {
			b.fail(ErrUnavailable)
			return previewFailure(id, "preview_unavailable")
		}
	}
	return r
}
func (b *StableBridge) correlatePreview(req pilot.PreviewRequest, r pilot.PreviewResponse) bool {
	if req.Command == "preview_snapshot" {
		s := r.Snapshot
		if s == nil || r.Job != nil {
			return false
		}
		if s.View == nil {
			return s.State == "unavailable"
		}
		return b.previewPinned() && *s.View == *b.view && s.StoreID == b.storeID && s.EngineVersion == fighterbout.Version1 && s.EngineID == fighterbout.EngineID
	}
	j := r.Job
	if j == nil || r.Snapshot != nil || !b.previewPinned() || j.View != *b.view || j.StoreID != b.storeID || j.EngineVersion != fighterbout.Version1 || j.EngineID != fighterbout.EngineID {
		return false
	}
	if req.Command == "preview_begin" {
		if j.State != "queued" || j.FighterAID != req.FighterAID || j.FighterBID != req.FighterBID {
			return false
		}
		if _, exists := b.previewJobs[j.JobID]; exists {
			return false
		}
	} else {
		if j.JobID != req.JobID {
			return false
		}
		if e, ok := b.previewJobs[j.JobID]; ok {
			if j.FighterAID != e.a || j.FighterBID != e.b || stableTerminal(e.state) && j.State != e.state || e.state == "running" && j.State == "queued" {
				return false
			}
			if e.digest != "" && (j.Result == nil || e.digest != j.Result.EngineResultDigest || e.seed != j.Result.Seed || e.inputID != j.Result.EngineResult.InputID || e.capturedResultDigest != previewCapturedDigest(j.Result)) {
				return false
			}
		} else if !b.previewSnapshotJobs[j.JobID] {
			return false
		}
	}
	if j.Result != nil {
		p := j.Result.Pair
		if p.Status.StoreID != b.storeID || p.Status.Pin != *b.stablePin || p.Fighters[0].Assignment.ProspectID != j.FighterAID || p.Fighters[1].Assignment.ProspectID != j.FighterBID {
			return false
		}
	}
	return true
}
func (b *StableBridge) rememberPreviewSnapshot(s pilot.PreviewSnapshot) {
	if s.View == nil {
		b.clearPreviewView()
		return
	}
	b.previewReady = true
	b.previewSnapshotJobs = make(map[string]bool, 3)
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.previewSnapshotJobs[id] = true
		}
	}
	for id := range b.previewJobs {
		if !b.previewSnapshotJobs[id] {
			delete(b.previewJobs, id)
		}
	}
	b.mu.Lock()
	if b.presentation != nil && b.presentation.metadata.Family != careerPreviewFamily && b.presentation.metadata.JobID != s.LastJobID {
		b.presentation = nil
	}
	b.mu.Unlock()
}
func (b *StableBridge) rememberPreviewJob(j pilot.PreviewJob) bool {
	if b.previewJobs == nil {
		b.previewJobs = make(map[string]previewJobExpectation)
	}
	if stableTerminal(j.State) {
		for id, e := range b.previewJobs {
			if id != j.JobID && stableTerminal(e.state) {
				delete(b.previewJobs, id)
			}
		}
	}
	if _, exists := b.previewJobs[j.JobID]; !exists && len(b.previewJobs)+len(b.careerPreviewJobs) >= 3 {
		return false
	}
	e := previewJobExpectation{a: j.FighterAID, b: j.FighterBID, state: j.State}
	if j.Result != nil {
		r := j.Result
		e.seed, e.inputID, e.digest = r.Seed, r.EngineResult.InputID, r.EngineResultDigest
		e.capturedResultDigest = previewCapturedDigest(r)
		if e.capturedResultDigest == "" {
			return false
		}
		meta := previewPresentationMetadata{View: j.View, StoreID: j.StoreID, JobID: j.JobID, EngineVersion: j.EngineVersion, EngineID: j.EngineID, Seed: e.seed, InputID: e.inputID, ResultDigest: e.digest, DescriptorID: r.Pair.Status.Pin.Source.DescriptorID}
		for n, item := range r.Pair.Fighters {
			meta.Members[n] = previewMember{ID: item.Assignment.ProspectID, ManifestDigest: item.Assignment.ManifestDigest, PortraitDigest: item.Assignment.PortraitDigest, Manifest: item.Manifest}
		}
		raw, err := json.Marshal(meta)
		if err != nil || len(raw) > 24576 {
			return false
		}
		b.mu.Lock()
		if b.failure != nil {
			b.mu.Unlock()
			return false
		}
		if b.previewMayPresentLocked(legacyPreviewFamily, j.JobID) && (b.presentation == nil || b.presentation.metadata != meta) {
			b.presentation = &previewPresentation{metadata: meta}
		}
		b.mu.Unlock()
	}
	b.previewJobs[j.JobID] = e
	return b.previewMetadataBound()
}

// FightPreviewPortrait authorizes every disclosure after rendering, including
// cache hits. It sends only the existing job read and holds no helper lease.
func (b *StableBridge) FightPreviewPortrait(ctx context.Context, viewID, jobID, prospectID string) PreviewPortraitResponse {
	fail := func(code string) PreviewPortraitResponse { return PreviewPortraitResponse{Version: 1, Error: code} }
	if b.poolMode {
		return fail("preview_unavailable")
	}
	if !publicHex.MatchString(viewID) || !publicHex.MatchString(jobID) || !publicHex.MatchString(prospectID) {
		return fail("invalid_request")
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return fail("busy")
		}
		return fail("preview_unavailable")
	}
	defer func() { <-b.gate }()
	if !b.previewPinned() || !b.previewReady || b.view.ViewID != viewID {
		return fail("stale_view")
	}
	b.mu.Lock()
	p := b.presentation
	if p == nil || p.metadata.Family == careerPreviewFamily || p.metadata.View != *b.view || p.metadata.StoreID != b.storeID || p.metadata.JobID != jobID {
		b.mu.Unlock()
		return fail("preview_not_found")
	}
	n := -1
	for i, m := range p.metadata.Members {
		if m.ID == prospectID {
			n = i
		}
	}
	if n < 0 {
		b.mu.Unlock()
		return fail("preview_not_found")
	}
	meta, svg := p.metadata, p.svgs[n]
	b.mu.Unlock()
	m := meta.Members[n]
	md, err := fightermanifest.Digest(m.Manifest)
	descriptor, descriptorErr := fighterdescriptor.ResolveCandidate(meta.DescriptorID)
	if err != nil || md != m.ManifestDigest || descriptorErr != nil || descriptor.RendererID != fighterportrait.RendererID || descriptor.RendererVersion != fighterportrait.Version1 {
		return fail("preview_unavailable")
	}
	if svg == "" {
		raw, err := fighterportrait.Render(fighterportrait.Version1, m.Manifest)
		if err != nil {
			return fail("preview_unavailable")
		}
		svg = string(raw)
	}
	if len(svg) == 0 || len(svg) > 32768 {
		return fail("preview_capacity")
	}
	for i := range len(svg) {
		if svg[i] < 32 || svg[i] > 126 {
			return fail("preview_unavailable")
		}
	}
	h := sha256.Sum256(append([]byte("SLUG fighter portrait SVG\x00v1\x00"), []byte(svg)...))
	if hex.EncodeToString(h[:]) != m.PortraitDigest {
		return fail("preview_unavailable")
	}
	// Already gate-held: recursively calling PreviewRequest would deadlock or
	// return busy. The fresh reply is fully decoded and correlated as usual.
	r := b.previewRequestHeld(ctx, id, pilot.PreviewRequest{Command: "preview_job", ViewID: viewID, JobID: jobID})
	if !r.OK {
		return fail(r.Error)
	}
	if r.Job == nil || r.Job.State != "done" || r.Job.Result == nil || r.Job.Result.EngineResultDigest != meta.ResultDigest {
		b.clearPreviewPresentation()
		return fail("preview_not_found")
	}
	b.mu.Lock()
	if b.failure != nil || b.presentation != p || p.metadata != meta || ctx.Err() != nil {
		b.mu.Unlock()
		return fail("preview_unavailable")
	}
	p.svgs[n] = svg
	b.mu.Unlock()
	out := PreviewPortraitResponse{Version: 1, OK: true, Portrait: &PreviewPortrait{Version: 1, View: meta.View, StoreID: meta.StoreID, JobID: jobID, ProspectID: prospectID, ResultDigest: meta.ResultDigest, DescriptorID: meta.DescriptorID, ManifestDigest: m.ManifestDigest, PortraitDigest: m.PortraitDigest, SVG: svg}}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > 98304 {
		return fail("preview_capacity")
	}
	return out
}
