// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymbout"
	"time"
)

type boutPresentation struct {
	view   pilot.ManifestViewPin
	record gymbout.Record
	digest string
	svgs   [2]string
}
type BoutPortrait struct {
	Version        uint32                `json:"version"`
	View           pilot.ManifestViewPin `json:"view"`
	StoreID        string                `json:"store_id"`
	CardID         string                `json:"card_id"`
	Saturday       string                `json:"saturday"`
	BoutID         string                `json:"bout_id"`
	ProspectID     string                `json:"prospect_id"`
	RecordDigest   string                `json:"record_digest"`
	InputID        string                `json:"input_id"`
	ResultDigest   string                `json:"result_digest"`
	DescriptorID   string                `json:"descriptor_id"`
	ManifestDigest string                `json:"manifest_digest"`
	PortraitDigest string                `json:"portrait_digest"`
	SVG            string                `json:"svg"`
}
type BoutPortraitResponse struct {
	Version  uint32          `json:"version"`
	OK       bool            `json:"ok"`
	Portrait *BoutPortrait   `json:"portrait"`
	Status   *gymbout.Status `json:"status"`
	Error    string          `json:"error"`
}

func boutRecordDigest(r gymbout.Record) string {
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > gymbout.MaxRecordSize {
		return ""
	}
	h := sha256.New()
	h.Write([]byte("SLUG candidate bout record\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func (b *StableBridge) clearBoutPresentation() { b.mu.Lock(); b.boutPresentation = nil; b.mu.Unlock() }
func (b *StableBridge) selectBoutRecord(view pilot.ManifestViewPin, r gymbout.Record, fromJob ...bool) bool {
	digest := boutRecordDigest(r)
	if digest == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.bout == nil || b.bout.view != view {
		return false
	}
	if p := b.boutPresentation; p != nil && p.view == view {
		if p.record.BoutID == r.BoutID && p.record.CardID == r.CardID {
			return p.digest == digest
		}
		// A cached Job remains valid historical evidence, but cannot replace
		// a different transcript selected by a newer explicit Read. Accepted
		// Run already clears the tuple; first/discovered completion may select.
		if len(fromJob) == 1 && fromJob[0] {
			return true
		}
	}
	// Exactly one owned transcript; no duplicate Job/Read cache or copied manifests.
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > pilot.MaxBoutJobSize {
		return false
	}
	var owned gymbout.Record
	if json.Unmarshal(raw, &owned) != nil {
		return false
	}
	b.boutPresentation = &boutPresentation{view: view, record: owned, digest: digest}
	return true
}
func (b *StableBridge) BoutPortrait(ctx context.Context, viewID, saturday, boutID, prospectID string) BoutPortraitResponse {
	fail := func(code string) BoutPortraitResponse { return BoutPortraitResponse{Version: 1, Error: code} }
	if !publicHex.MatchString(viewID) || !publicHex.MatchString(boutID) || !publicHex.MatchString(prospectID) {
		return fail("invalid_request")
	}
	if _, e := pilot.MarshalBoutRequest(pilot.BoutRequest{ID: 1, Command: "bout_status", ViewID: viewID, Saturday: saturday, BoutID: boutID}); e != nil {
		return fail("bout_calendar")
	}
	if !b.boutsEnabled {
		return fail("bout_unavailable")
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return fail("busy")
		}
		b.clearBoutPresentation()
		return fail("bout_unavailable")
	}
	defer func() { <-b.gate }()
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	a, view, ok := b.boutContext()
	if !ok || view.ViewID != viewID {
		b.clearBoutPresentation()
		return fail("stale_view")
	}
	b.mu.Lock()
	p := b.boutPresentation
	if p == nil || p.view != view || p.record.StoreID != a.storeID || p.record.BoutID != boutID || p.record.Source.Event.Saturday != saturday {
		b.mu.Unlock()
		return fail("bout_not_found")
	}
	n := -1
	for i, c := range p.record.Source.Corners {
		if c.Member.Assignment.ProspectID == prospectID {
			n = i
		}
	}
	if n < 0 {
		b.mu.Unlock()
		return fail("bout_not_found")
	}
	svg := p.svgs[n]
	assignment := p.record.Source.Corners[n].Member.Assignment
	manifest := p.record.Input.A.Manifest
	if n == 1 {
		manifest = p.record.Input.B.Manifest
	}
	b.mu.Unlock()
	descriptor, err := fighterdescriptor.ResolveCandidate(assignment.DescriptorID)
	domain, ok := manifestPortraitDomain(assignment.DescriptorID)
	if err != nil || !ok || !b.validBoutRecord(a, p.record) {
		b.clearBoutPresentation()
		return fail("bout_corrupt")
	}
	if svg == "" {
		raw, e := fighterportrait.Render(descriptor.RendererVersion, manifest)
		if e != nil {
			b.clearBoutPresentation()
			return fail("bout_corrupt")
		}
		svg = string(raw)
	}
	raw, e := json.Marshal(svg)
	if e != nil || len(svg) == 0 || len(svg) > 32768 || len(raw) > 32768 {
		b.clearBoutPresentation()
		return fail("bout_capacity")
	}
	h := sha256.Sum256(append([]byte(domain), []byte(svg)...))
	if hex.EncodeToString(h[:]) != assignment.PortraitDigest {
		b.clearBoutPresentation()
		return fail("bout_corrupt")
	}
	// Every disclosure, including a cached SVG, obtains a new original-view Status.
	r := b.boutRequestHeld(work, id, pilot.BoutRequest{Command: "bout_status", ViewID: viewID, Saturday: saturday, BoutID: boutID})
	if !r.OK {
		code := r.Error
		if code == "bout_setup_incomplete" {
			code = "bout_unavailable"
		}
		if !oneOf(code, "busy", "invalid_request", "bout_calendar", "bout_cancelled") {
			b.clearBoutPresentation()
		}
		return fail(code)
	}
	s := r.Status
	if s == nil || !s.Saved.Exists || s.Saved.RecordDigest != p.digest || s.Saved.SourceDigest != p.record.SourceDigest || s.Saved.InputID != p.record.InputID || s.Saved.ResultDigest != p.record.ResultDigest || s.Source.CardRecordDigest != p.record.Source.CardRecordDigest || s.Source.CardSourceDigest != p.record.Source.CardSourceDigest || s.Source.CardCreatedAtMS != p.record.Source.CardCreatedAtMS || s.Source.FinalizedAtMS != p.record.Source.FinalizedAtMS || s.Saved.Usable != s.Source.Usable || !s.Saved.Usable && s.Source.ConflictCount == 0 && !s.Source.ConflictOverflow {
		b.clearBoutPresentation()
		return fail("bout_source_changed")
	}
	b.mu.Lock()
	if b.failure != nil || b.boutPresentation != p || b.bout == nil || b.bout.view != view || work.Err() != nil {
		b.boutPresentation = nil
		b.mu.Unlock()
		return fail("bout_unavailable")
	}
	p.svgs[n] = svg
	b.mu.Unlock()
	out := BoutPortraitResponse{Version: 1, OK: true, Portrait: &BoutPortrait{Version: 1, View: view, StoreID: a.storeID, CardID: p.record.CardID, Saturday: saturday, BoutID: boutID, ProspectID: prospectID, RecordDigest: p.digest, InputID: p.record.InputID, ResultDigest: p.record.ResultDigest, DescriptorID: assignment.DescriptorID, ManifestDigest: assignment.ManifestDigest, PortraitDigest: assignment.PortraitDigest, SVG: svg}, Status: s}
	raw, e = json.Marshal(out)
	if e != nil || len(raw) > 49152 {
		b.clearBoutPresentation()
		return fail("bout_capacity")
	}
	return out
}
