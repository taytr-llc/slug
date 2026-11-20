// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

var errEvidenceInvalid = errors.New("invalid live public evidence")

func evidenceProtocolError(e error) string {
	switch {
	case errors.Is(e, ErrEvidenceReplyUnknown), errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, ErrEvidenceNotFound):
		return "evidence_not_found"
	case errors.Is(e, ErrEvidenceOperationConflict):
		return "evidence_operation_conflict"
	case errors.Is(e, gymstable.ErrInvalid), errors.Is(e, recoveryfile.ErrInvalid):
		return "invalid_request"
	case errors.Is(e, gymstable.ErrUnauthorized), errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired):
		return "unauthorized"
	case errors.Is(e, gymstable.ErrForbidden), errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymaccount.ErrHeld):
		return "forbidden"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymstable.ErrBusy), errors.Is(e, recoveryfile.ErrBusy), errors.Is(e, context.DeadlineExceeded):
		return "busy"
	case errors.Is(e, gymstable.ErrNotFound):
		return "history_not_found"
	case errors.Is(e, gymstable.ErrCapacity):
		return "evidence_capacity"
	case errors.Is(e, gymstable.ErrProfile):
		return "evidence_profile"
	}
	return "evidence_unavailable"
}
func evidenceTaskError(e error) string {
	switch {
	case errors.Is(e, recoveryfile.ErrOutcomeUnknown):
		return "evidence_export_uncertain"
	case errors.Is(e, recoveryfile.ErrCleanup):
		return "evidence_storage_failed"
	case errors.Is(e, recoveryfile.ErrConflict):
		return "evidence_destination_conflict"
	case errors.Is(e, gymstable.ErrConflict), errors.Is(e, gymstable.ErrStaleView):
		return "evidence_source_changed"
	case errors.Is(e, errEvidenceInvalid), errors.Is(e, gymstable.ErrCorrupt), errors.Is(e, recoveryfile.ErrInvalid):
		return "evidence_invalid"
	}
	return "evidence_unavailable"
}
func handleFighterEvidenceRequest(ctx context.Context, r FighterEvidenceRequest, c FighterEvidenceSurfaceAPI) FighterEvidenceResponse {
	out := FighterEvidenceResponse{ID: r.ID, Version: 1, OK: true}
	var e error
	switch r.Command {
	case "fighter_evidence_snapshot":
		v, x := c.FighterEvidenceSnapshot(ctx)
		out.Snapshot, e = &v, x
		if e == nil && r.ViewID != "" && (v.View == nil || v.View.ViewID != r.ViewID) {
			e = gymstable.ErrStaleView
		}
	case "fighter_evidence_begin":
		v, x := c.BeginFighterEvidence(ctx, r.ViewID, r.HistoryJobID, r.HistoryID, r.JobID, *r.Selection)
		out.Job, e = &v, x
	case "fighter_evidence_job":
		v, x := c.FighterEvidenceJob(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "fighter_evidence_cancel":
		v, x := c.CancelFighterEvidence(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	default:
		e = gymstable.ErrInvalid
	}
	if e != nil {
		return FighterEvidenceResponse{ID: r.ID, Version: 1, Error: evidenceProtocolError(e)}
	}
	return out
}
