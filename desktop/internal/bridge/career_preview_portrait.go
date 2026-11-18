// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
)

type CareerPreviewPortrait struct {
	Version        uint32                `json:"version"`
	View           pilot.ManifestViewPin `json:"view"`
	StoreID        string                `json:"store_id"`
	JobID          string                `json:"job_id"`
	ProspectID     string                `json:"prospect_id"`
	ActivationID   string                `json:"activation_id"`
	CareerDigest   string                `json:"career_digest"`
	ResultDigest   string                `json:"result_digest"`
	DescriptorID   string                `json:"descriptor_id"`
	ManifestDigest string                `json:"manifest_digest"`
	PortraitDigest string                `json:"portrait_digest"`
	SVG            string                `json:"svg"`
}
type CareerPreviewPortraitResponse struct {
	Version  uint32                 `json:"version"`
	OK       bool                   `json:"ok"`
	Portrait *CareerPreviewPortrait `json:"portrait,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

// CareerFightPreviewPortrait authorizes every disclosure after rendering, including
// cache hits. It sends only the existing job read and holds no helper lease.
func (b *StableBridge) CareerFightPreviewPortrait(ctx context.Context, viewID, jobID, prospectID string) CareerPreviewPortraitResponse {
	fail := func(code string) CareerPreviewPortraitResponse {
		return CareerPreviewPortraitResponse{Version: 1, Error: code}
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
	if !b.careersEnabled {
		return fail("career_inactive")
	}
	if !b.previewPinned() || b.view.ViewID != viewID {
		return fail("stale_view")
	}
	if !b.careerPreviewReady || b.careerActivation == nil {
		return fail("career_inactive")
	}
	b.mu.Lock()
	p := b.presentation
	if p == nil || p.metadata.Family != careerPreviewFamily || p.metadata.View != *b.view || p.metadata.StoreID != b.storeID || p.metadata.JobID != jobID {
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
	r := b.careerPreviewRequestHeld(ctx, id, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: viewID, JobID: jobID})
	if !r.OK {
		return fail(r.Error)
	}
	if !careerPreviewPortraitFresh(meta, *b.view, r.Job) {
		b.clearCareerPreviewPresentation()
		return fail("preview_not_found")
	}
	b.mu.Lock()
	if b.failure != nil || b.presentation != p || p.metadata != meta || ctx.Err() != nil {
		b.mu.Unlock()
		return fail("preview_unavailable")
	}
	p.svgs[n] = svg
	b.mu.Unlock()
	out := CareerPreviewPortraitResponse{Version: 1, OK: true, Portrait: &CareerPreviewPortrait{Version: 1, View: meta.View, StoreID: meta.StoreID, JobID: jobID, ProspectID: prospectID, ActivationID: meta.ActivationID, CareerDigest: meta.CareerDigest, ResultDigest: meta.ResultDigest, DescriptorID: meta.DescriptorID, ManifestDigest: m.ManifestDigest, PortraitDigest: m.PortraitDigest, SVG: svg}}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > 98304 || !careerPreviewPortraitMatches(out, meta) {
		return fail("preview_capacity")
	}
	return out
}

// This validates a native-produced typed value, not a public JSON import API.
// The caller must separately complete the fresh exact-job authorization below.
func careerPreviewPortraitMatches(out CareerPreviewPortraitResponse, meta previewPresentationMetadata) bool {
	p := out.Portrait
	if out.Version != 1 || !out.OK || out.Error != "" || p == nil || p.Version != 1 || meta.Family != careerPreviewFamily || p.View != meta.View || p.StoreID != meta.StoreID || p.JobID != meta.JobID || p.ActivationID != meta.ActivationID || p.CareerDigest != meta.CareerDigest || p.ResultDigest != meta.ResultDigest || p.DescriptorID != meta.DescriptorID || !validManifestView(p.View) || !manifestASCII(p.SVG, 1, 32768) {
		return false
	}
	var member *previewMember
	for i := range meta.Members {
		if meta.Members[i].ID == p.ProspectID {
			member = &meta.Members[i]
		}
	}
	if member == nil || p.ManifestDigest != member.ManifestDigest || p.PortraitDigest != member.PortraitDigest {
		return false
	}
	for _, id := range []string{p.StoreID, p.JobID, p.ProspectID, p.ActivationID, p.CareerDigest, p.ResultDigest, p.DescriptorID, p.ManifestDigest, p.PortraitDigest} {
		if !publicHex.MatchString(id) {
			return false
		}
	}
	h := sha256.Sum256(append([]byte("SLUG fighter portrait SVG\x00v1\x00"), []byte(p.SVG)...))
	if hex.EncodeToString(h[:]) != p.PortraitDigest {
		return false
	}
	raw, e := json.Marshal(out)
	return e == nil && len(raw) <= 98304
}
func careerPreviewPortraitFresh(meta previewPresentationMetadata, current pilot.ManifestViewPin, j *pilot.CareerPreviewJob) bool {
	return j != nil && j.State == "done" && j.Result != nil && current == meta.View && j.View == meta.View && j.StoreID == meta.StoreID && j.JobID == meta.JobID && j.ActivationID == meta.ActivationID && j.FighterAID == meta.Members[0].ID && j.FighterBID == meta.Members[1].ID && j.Result.EngineResultDigest == meta.ResultDigest && j.Result.Pair.CareerDigest == meta.CareerDigest && careerPreviewCapturedDigest(j.Result) == meta.CapturedResultDigest
}
