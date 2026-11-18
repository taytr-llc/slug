// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerPreviewProtocolError(e error) string {
	if errors.Is(e, gymstable.ErrOutcomeUnknown) {
		return "preview_observation_unknown"
	}
	switch {
	case errors.Is(e, careerstate.ErrInvalid), errors.Is(e, careerstate.ErrInactive), errors.Is(e, careerstate.ErrUnfenced), errors.Is(e, careerstate.ErrClosed), errors.Is(e, careerstate.ErrPoisoned), errors.Is(e, careerstate.ErrCancelled), errors.Is(e, careerstate.ErrCorrupt):
		for _, known := range []error{careerstate.ErrInvalid, careerstate.ErrInactive, careerstate.ErrUnfenced, careerstate.ErrClosed, careerstate.ErrPoisoned, careerstate.ErrCancelled, careerstate.ErrCorrupt} {
			if errors.Is(e, known) {
				return known.Error()
			}
		}
	case errors.Is(e, gymstable.ErrNotFound):
		return "preview_not_found"
	case errors.Is(e, gymstable.ErrClassMismatch):
		return "preview_class_mismatch"
	case errors.Is(e, gymstable.ErrCapacity):
		return "preview_capacity"
	case errors.Is(e, gymstable.ErrCancelled), errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return "preview_cancelled"
	case errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "preview_observation_unknown"
	}
	code := stableProtocolError(e)
	if careerPreviewErrorCode(code) {
		return code
	}
	return "preview_unavailable"
}
func handleCareerPreviewRequest(ctx context.Context, r CareerPreviewRequest, c CareerPreviewControllerAPI) CareerPreviewResponse {
	out := CareerPreviewResponse{ID: r.ID, Version: 1, OK: true}
	var err error
	switch r.Command {
	case "career_preview_snapshot":
		v := c.CareerPreviewSnapshot()
		out.Snapshot = &v
	case "career_preview_begin":
		v, e := c.BeginCareerFightPreview(ctx, r.ViewID, r.FighterAID, r.FighterBID)
		out.Job, err = &v, e
	case "career_preview_job":
		v, e := c.CareerFightPreviewJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "career_preview_cancel":
		v, e := c.CancelCareerFightPreview(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymstable.ErrInvalid
	}
	if err != nil {
		return CareerPreviewResponse{ID: r.ID, Version: 1, Error: careerPreviewProtocolError(err)}
	}
	return out
}
