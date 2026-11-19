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
)

// No whole terminal or extra Capture is retained. The selected page is the
// sole payload; the fixed row array catches changed overlapping/revisited rows.
type careerHistoryTuple struct {
	Version        uint32                `json:"version"`
	View           pilot.ManifestViewPin `json:"view"`
	StoreID        string                `json:"store_id"`
	ActivationID   string                `json:"activation_id"`
	ProspectID     string                `json:"prospect_id"`
	JobID          string                `json:"job_id"`
	HistoryID      string                `json:"history_id"`
	State          string                `json:"state"`
	Error          string                `json:"error"`
	CaptureSHA256  string                `json:"capture_sha256"`
	TerminalSHA256 string                `json:"terminal_sha256"`
}
type careerHistoryCorrelation struct {
	snapshot *pilot.CareerHistorySnapshot
	jobs     [3]careerHistoryTuple
	selected string
	page     *pilot.CareerHistoryPageResult
	limit    uint32
	rows     [256][32]byte
	seen     [32]byte
}

func historyDigest(v any) string {
	raw, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func historyFailure(id uint64, code string) pilot.CareerHistoryResponse {
	if id == 0 {
		id = 1
	}
	return pilot.CareerHistoryResponse{ID: id, Version: 1, Error: code}
}
func (b *StableBridge) clearCareerHistory(fatal bool) {
	b.mu.Lock()
	b.history = nil
	b.historyFatal = b.historyFatal || fatal
	b.mu.Unlock()
}
func (h *careerHistoryCorrelation) tuple(id string) (careerHistoryTuple, int) {
	for i, t := range h.jobs {
		if t.JobID == id && id != "" {
			return t, i
		}
	}
	return careerHistoryTuple{}, -1
}
func (h *careerHistoryCorrelation) discovered(id string) bool {
	s := h.snapshot
	return id != "" && s != nil && (id == s.ActiveJobID || id == s.QueuedJobID || id == s.LastJobID)
}
func (h *careerHistoryCorrelation) room() bool {
	for _, t := range h.jobs {
		if t.JobID == "" || stableTerminal(t.State) {
			return true
		}
	}
	return false
}
func historyTuple(j pilot.CareerHistoryJob) careerHistoryTuple {
	t := careerHistoryTuple{Version: 1, View: j.View, StoreID: j.StoreID, ActivationID: j.ActivationID, ProspectID: j.ProspectID, JobID: j.JobID, State: j.State, Error: j.Error}
	if stableTerminal(j.State) {
		t.TerminalSHA256 = historyDigest(j)
	}
	if j.Capture != nil {
		t.CaptureSHA256 = historyDigest(j.Capture)
		t.HistoryID = j.Capture.HistoryID
	}
	return t
}

func (b *StableBridge) CareerHistoryRequest(ctx context.Context, req pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	return b.careerHistoryRequest(ctx, req, b.ResultRequest)
}

// The caller chooses the concrete source-discovery route; no embedded-method fallback.
func (b *StableBridge) careerHistoryRequest(ctx context.Context, req pilot.CareerHistoryRequest, prepare func(context.Context, pilot.ResultRequest) pilot.ResultResponse) pilot.CareerHistoryResponse {
	if !b.careersEnabled {
		return historyFailure(0, "career_inactive")
	}
	pre := req
	pre.ID, pre.Version = 1, 1
	if _, e := pilot.MarshalCareerHistoryRequest(pre); e != nil {
		return historyFailure(0, "invalid_request")
	}
	if req.Command == "career_history_snapshot" {
		// The existing Result discovery independently establishes the entire
		// original source chain. A History DTO cannot supply its own authority.
		if _, _, ready := b.resultContext(); !ready {
			r := prepare(ctx, pilot.ResultRequest{Command: "result_snapshot"})
			if !r.OK {
				code := "career_inactive"
				if oneOf(r.Error, "busy", "unauthorized", "forbidden", "stale_view", "outcome_unknown") {
					code = r.Error
				}
				return historyFailure(0, code)
			}
		}
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) || errors.Is(e, context.DeadlineExceeded) {
			return historyFailure(0, "busy")
		}
		if errors.Is(e, context.Canceled) {
			return historyFailure(0, "history_cancelled")
		}
		return historyFailure(0, "history_unavailable")
	}
	defer func() { <-b.gate }()
	return b.historyRequestHeld(ctx, id, req)
}

func (b *StableBridge) historyRequestHeld(ctx context.Context, id uint64, req pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	req.ID, req.Version = id, 1
	raw, e := pilot.MarshalCareerHistoryRequest(req)
	if e != nil {
		return historyFailure(id, "invalid_request")
	}
	if req.Command != "career_history_snapshot" && (!b.previewPinned() || req.ViewID != b.view.ViewID) {
		return historyFailure(id, "stale_view")
	}
	if !b.previewPinned() {
		return historyFailure(id, "history_unavailable")
	}
	b.mu.Lock()
	fatal, h := b.historyFatal, b.history
	code := ""
	if fatal {
		code = "history_unavailable"
	} else if req.Command != "career_history_snapshot" {
		if h == nil || h.snapshot == nil || h.snapshot.Activation == nil {
			code = "career_inactive"
			if b.careerActivation != nil && req.Command != "career_history_begin" {
				code = "history_not_found"
			}
		} else if req.Command == "career_history_begin" {
			if !h.room() {
				code = "busy"
			}
		} else {
			t, n := h.tuple(req.JobID)
			if req.Command == "career_history_page" {
				if n < 0 || t.State != "done" || h.selected != req.JobID {
					code = "history_not_found"
				}
			} else if n < 0 && !h.discovered(req.JobID) {
				code = "history_not_found"
			}
		}
	}
	b.mu.Unlock()
	if code != "" {
		return historyFailure(id, code)
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return historyFailure(id, "busy")
		}
		return historyFailure(id, "history_cancelled")
	}
	// The helper owns the500ms metadata budget; this outer pipe wait cannot
	// extend an earlier caller deadline and does not start the capture worker.
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, e := b.exchangeHistoryFamily(work, id, false, false, false, false, false, false, false, false, false, false, true, raw)
	if e != nil || f.history == nil {
		b.clearCareerHistory(true)
		return historyFailure(id, "outcome_unknown")
	}
	r := *f.history
	if !r.OK {
		if oneOf(r.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		} else if historyFatalCode(r.Error) {
			b.clearCareerHistory(true)
		} else if oneOf(r.Error, "history_not_found", "history_source_changed") {
			b.clearCareerHistory(false)
		}
		return r
	}
	if !b.correlateCareerHistory(req, r) {
		b.fail(ErrUnavailable)
		return historyFailure(id, "outcome_unknown")
	}
	return r
}

func historyFatalCode(code string) bool {
	return oneOf(code, "history_unavailable", "history_corrupt", "history_clock", "history_observation_unknown")
}

// Gate serializes exchanges; mu also fences failure/Close invalidation. All
// prospective changes are checked before publication into retained native state.
func (b *StableBridge) correlateCareerHistory(req pilot.CareerHistoryRequest, r pilot.CareerHistoryResponse) bool {
	if req.ID != r.ID || r.Version != 1 || !r.OK {
		return false
	}
	if r.Snapshot != nil {
		s := *r.Snapshot
		if req.Command != "career_history_snapshot" || r.Job != nil || r.Page != nil || pilot.ValidateCareerHistorySnapshot(s) != nil {
			return false
		}
		if s.View != nil && (!b.previewPinned() || *s.View != *b.view || s.StoreID != b.storeID || s.Activation == nil || !b.careerPreviewActivationValid(*s.Activation)) {
			return false
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.failure != nil || b.historyFatal {
			return false
		}
		if s.View == nil {
			b.history = nil
			b.historyFatal = oneOf(s.Error, "history_unavailable", "history_corrupt", "history_clock")
			return true
		}
		a, v := *s.Activation, *s.View
		s.Activation, s.View = &a, &v
		b.careerActivation = &a
		if b.history == nil {
			b.history = &careerHistoryCorrelation{}
		}
		h := b.history
		h.snapshot = &s
		if historyFatalCode(s.Error) {
			b.history, b.historyFatal = nil, true
			return true
		}
		// Old unselected tuples can be forgotten, never silently selected.
		for i, t := range h.jobs {
			if t.JobID != h.selected && !h.discovered(t.JobID) {
				h.jobs[i] = careerHistoryTuple{}
			}
		}
		return historyMetadataBound(h)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.history
	if b.failure != nil || b.historyFatal || h == nil || h.snapshot == nil || h.snapshot.Activation == nil || b.view == nil || h.snapshot.View == nil || *h.snapshot.View != *b.view || req.ViewID != b.view.ViewID {
		return false
	}
	if j := r.Job; j != nil {
		if r.Page != nil || !oneOf(req.Command, "career_history_begin", "career_history_job", "career_history_cancel") || pilot.ValidateCareerHistoryJob(*j) != nil || j.View != *b.view || j.StoreID != b.storeID || j.ActivationID != h.snapshot.Activation.ActivationID {
			return false
		}
		old, slot := h.tuple(j.JobID)
		if req.Command == "career_history_begin" {
			if j.State != "queued" || j.ProspectID != req.ProspectID || slot >= 0 || h.discovered(j.JobID) {
				return false
			}
		} else {
			if j.JobID != req.JobID || slot < 0 && !h.discovered(j.JobID) {
				return false
			}
			if slot >= 0 && (old.View != j.View || old.StoreID != j.StoreID || old.ActivationID != j.ActivationID || old.ProspectID != j.ProspectID || old.State == "running" && j.State == "queued" || old.TerminalSHA256 != "" && old.TerminalSHA256 != historyDigest(*j)) {
				return false
			}
		}
		if j.Capture != nil && (b.stablePin == nil || j.Capture.Status.Pin != *b.stablePin || j.Capture.Activation != *h.snapshot.Activation) {
			return false
		}
		if slot < 0 {
			for i, t := range h.jobs {
				if t.JobID == "" || t.JobID != h.selected && !h.discovered(t.JobID) {
					slot = i
					break
				}
			}
		}
		if req.Command == "career_history_begin" {
			// The acknowledged queued job is the only point that replaces intent.
			for i, t := range h.jobs {
				if stableTerminal(t.State) {
					h.jobs[i] = careerHistoryTuple{}
					if slot < 0 {
						slot = i
					}
				}
			}
		}
		if slot < 0 {
			return false
		}
		h.jobs[slot] = historyTuple(*j)
		if req.Command == "career_history_begin" || h.selected == "" {
			h.selected = j.JobID
			h.page = nil
			h.rows, h.seen = [256][32]byte{}, [32]byte{}
		}
		if historyFatalCode(j.Error) {
			b.history, b.historyFatal = nil, true
			return true
		}
		return historyMetadataBound(h)
	}
	p := r.Page
	if req.Command != "career_history_page" || p == nil || pilot.ValidateCareerHistoryPageResult(*p) != nil || p.View != *b.view || p.StoreID != b.storeID || p.JobID != req.JobID || h.selected != req.JobID {
		return false
	}
	t, slot := h.tuple(req.JobID)
	page := p.Page
	if slot < 0 || t.State != "done" || t.CaptureSHA256 == "" || historyDigest(page.Capture) != t.CaptureSHA256 || page.After != req.After || req.Limit < 1 || req.Limit > 8 || page.After > page.Capture.EventCount {
		return false
	}
	n := min(req.Limit, page.Capture.EventCount-page.After)
	if len(page.Items) != int(n) || page.Next != page.After+n || page.HasMore != (page.Next < page.Capture.EventCount) {
		return false
	}
	rows, seen := h.rows, h.seen
	for i, row := range page.Items {
		if row.Ordinal != page.Capture.EventCount-page.After-uint32(i) || row.Ordinal == 0 || row.Ordinal > 256 {
			return false
		}
		raw, e := json.Marshal(row)
		if e != nil {
			return false
		}
		d := sha256.Sum256(raw)
		index := row.Ordinal - 1
		mask := byte(1 << (index % 8))
		if seen[index/8]&mask != 0 && rows[index] != d {
			return false
		}
		rows[index], seen[index/8] = d, seen[index/8]|mask
	}
	// Marshal/unmarshal also detaches every slice from the caller's returned DTO.
	raw, e := json.Marshal(p.Page)
	if e != nil || len(raw) > pilot.MaxCareerHistoryPageSize {
		return false
	}
	owned := pilot.CareerHistoryPageResult{View: p.View, StoreID: p.StoreID, JobID: p.JobID}
	if json.Unmarshal(raw, &owned.Page) != nil {
		return false
	}
	h.rows, h.seen, h.page, h.limit = rows, seen, &owned, req.Limit
	return historyMetadataBound(h)
}

// Encoded retained accounting, not a claim about Go heap or transient decode
// allocations. Bound includes the fixed8224B commitments and1024B bookkeeping.
func historyMetadataBound(h *careerHistoryCorrelation) bool {
	if h == nil {
		return true
	}
	ids := make(map[string]bool, 3)
	charged := 8224 + 1024
	for _, t := range h.jobs {
		if t.JobID != "" {
			ids[t.JobID] = true
			raw, e := json.Marshal(t)
			if e != nil {
				return false
			}
			charged += len(raw)
		}
	}
	if h.snapshot != nil {
		raw, e := json.Marshal(h.snapshot)
		if e != nil {
			return false
		}
		charged += len(raw)
		for _, id := range []string{h.snapshot.ActiveJobID, h.snapshot.QueuedJobID, h.snapshot.LastJobID} {
			if id != "" {
				ids[id] = true
			}
		}
	}
	return len(ids) <= 3 && charged <= 16*1024
}
