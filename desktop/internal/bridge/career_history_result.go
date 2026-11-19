// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// CareerHistoryResult resolves only a row in the selected native page. The
// caller supplies no Event, activation, source or result identity. A fresh
// History page read precedes the existing explicit Result read; neither starts
// node readiness, Check, Finalize or export.
func (b *StableBridge) CareerHistoryResult(ctx context.Context, viewID, jobID string, ordinal uint32) pilot.ResultResponse {
	if !b.careersEnabled {
		return resultFailure(0, "result_unavailable")
	}
	if !publicHex.MatchString(viewID) || !publicHex.MatchString(jobID) || ordinal < 1 || ordinal > 256 {
		return resultFailure(0, "invalid_request")
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) || errors.Is(e, context.DeadlineExceeded) {
			return resultFailure(0, "busy")
		}
		return resultFailure(0, "result_unavailable")
	}
	defer func() { <-b.gate }()
	if !b.previewPinned() || b.view.ViewID != viewID {
		return resultFailure(id, "stale_view")
	}
	b.mu.Lock()
	h := b.history
	if h == nil || b.historyFatal || h.page == nil || h.selected != jobID || h.page.JobID != jobID {
		b.mu.Unlock()
		return resultFailure(id, "result_not_found")
	}
	after, limit := h.page.Page.After, h.limit
	b.mu.Unlock()
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	fresh := b.historyRequestHeld(c, id, pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: viewID, JobID: jobID, After: after, Limit: limit})
	if !fresh.OK {
		code := fresh.Error
		switch code {
		case "history_observation_unknown":
			code = "outcome_unknown"
		case "history_cancelled":
			code = "result_cancelled"
		case "history_not_found":
			code = "result_not_found"
		case "history_source_changed":
			code = "result_source_changed"
		default:
			if !oneOf(code, "busy", "stale_view", "unauthorized", "forbidden", "outcome_unknown") {
				code = "result_unavailable"
			}
		}
		return resultFailure(fresh.ID, code)
	}
	var event careerstate.Event
	found := false
	for _, row := range fresh.Page.Page.Items {
		if row.Ordinal == ordinal {
			event = row.Event
			found = true
			break
		}
	}
	if !found {
		return resultFailure(id, "result_not_found")
	}
	activation := fresh.Page.Page.Capture.Activation
	authority, view, ready := b.resultContext()
	if !ready || view != fresh.Page.View || !historyResultIncarnation(activation, authority) {
		return resultFailure(id, "stale_view")
	}
	id = b.nextEntryIDHeld()
	if id == 0 {
		return resultFailure(0, "outcome_unknown")
	}
	r := b.resultRequestHeld(c, id, pilot.ResultRequest{Command: "result_record", ViewID: viewID, Saturday: event.Saturday, BoutID: event.BoutID})
	if !r.OK {
		return r
	}
	current, currentView, currentReady := b.resultContext()
	b.mu.Lock()
	stillSelected := b.failure == nil && !b.historyFatal && b.history == h && h.selected == jobID && h.page != nil && h.page.Page.Capture.Activation == activation
	b.mu.Unlock()
	if !stillSelected || !currentReady || currentView != view || !historyResultIncarnation(activation, current) || r.View == nil || *r.View != view || r.StoreID != activation.ResultStoreID || r.Record == nil || !historyResultMatches(event, activation, r.Record.Record) {
		b.fail(ErrUnavailable)
		return resultFailure(r.ID, "outcome_unknown")
	}
	return r
}

func historyResultIncarnation(a careerstate.Activation, r resultAuthority) bool {
	return a.ResultStoreID == r.storeID && a.ResultCreatedAtMS == r.createdMS && a.SourceCreatedAtMS == r.sourceCreatedMS
}
func historyResultMatches(want careerstate.Event, a careerstate.Activation, r gymresult.Record) bool {
	if r.Finality == nil || len(r.Statement.Statement.Source.Source.Corners) != 2 {
		return false
	}
	v := r.Statement.Statement.Source
	s := v.Source
	var corners [2]careerstate.Corner
	for i, c := range s.Corners {
		corners[i] = careerstate.Corner{ProspectID: c.Member.Assignment.ProspectID, AccountID: c.Owner.AccountID, GymID: c.Owner.GymID}
	}
	raw, e := json.Marshal(r.Finality)
	if e != nil {
		return false
	}
	got := careerstate.Event{Version: 1, AwardID: careerstate.AwardID(a, r.ResultKey), ResultKey: r.ResultKey, CardID: s.CardID, BoutID: s.Bout.BoutID, SourceRecordDigest: v.RecordDigest, InputID: v.InputID, ResultDigest: v.ResultDigest, FinalityDigest: careerstate.FinalityDigest(raw), EffectiveAtMS: r.Finality.ObservedAtMS, Saturday: s.Event.Saturday, CornerA: corners[0], CornerB: corners[1], Winner: v.Summary.Winner, Method: v.Summary.Method}
	return got == want
}
