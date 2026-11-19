// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

var errHistoryReplyUnknown = errors.New("history reply outcome unknown")

func careerHistoryProtocolError(e error) string {
	if errors.Is(e, errHistoryReplyUnknown) {
		return "outcome_unknown"
	}
	if errors.Is(e, gymstable.ErrOutcomeUnknown) {
		return "history_observation_unknown"
	}
	switch {
	case errors.Is(e, gymstable.ErrInvalid), errors.Is(e, careerstate.ErrInvalid):
		return "invalid_request"
	case errors.Is(e, gymstable.ErrUnauthorized), errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired):
		return "unauthorized"
	case errors.Is(e, gymstable.ErrForbidden), errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymaccount.ErrHeld):
		return "forbidden"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymstable.ErrBusy):
		return "busy"
	case errors.Is(e, careerstate.ErrInactive):
		return "career_inactive"
	case errors.Is(e, gymstable.ErrCorrupt), errors.Is(e, careerstate.ErrCorrupt), errors.Is(e, careerstate.ErrUnfenced):
		return "history_corrupt"
	case errors.Is(e, gymstable.ErrClock):
		return "history_clock"
	case errors.Is(e, gymstable.ErrCapacity):
		return "history_capacity"
	case errors.Is(e, gymstable.ErrNotFound):
		return "history_not_found"
	case errors.Is(e, gymstable.ErrConflict):
		return "history_source_changed"
	case errors.Is(e, gymstable.ErrClosed), errors.Is(e, gymstable.ErrPoisoned), errors.Is(e, careerstate.ErrClosed), errors.Is(e, careerstate.ErrPoisoned):
		return "history_unavailable"
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded), errors.Is(e, gymstable.ErrCancelled), errors.Is(e, careerstate.ErrCancelled):
		return "history_cancelled"
	}
	return "history_unavailable"
}
func historyMetadataError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	code := careerHistoryProtocolError(e)
	// Only known cancellation/deadline may be narrowed to the metadata busy arm.
	// Opaque failures and known auth/fatal/uncertain results retain their meaning.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && (code == "history_cancelled" || code == "busy") {
		return gymstable.ErrBusy
	}
	return e
}
func handleCareerHistoryRequest(ctx context.Context, r CareerHistoryRequest, c CareerHistorySurfaceAPI) CareerHistoryResponse {
	out := CareerHistoryResponse{ID: r.ID, Version: 1, OK: true}
	var e error
	switch r.Command {
	case "career_history_snapshot":
		v, x := c.CareerHistorySnapshot(ctx)
		out.Snapshot, e = &v, x
	case "career_history_begin":
		v, x := c.BeginCareerHistory(ctx, r.ViewID, r.ProspectID)
		out.Job, e = &v, x
	case "career_history_job":
		v, x := c.CareerHistoryJob(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "career_history_cancel":
		v, x := c.CancelCareerHistory(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "career_history_page":
		v, x := c.CareerHistoryPage(ctx, r.ViewID, r.JobID, r.After, r.Limit)
		out.Page, e = &v, x
	default:
		e = gymstable.ErrInvalid
	}
	if e != nil {
		return CareerHistoryResponse{ID: r.ID, Version: 1, Error: careerHistoryProtocolError(e)}
	}
	return out
}
