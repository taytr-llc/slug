// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) careerPreviewRequest(r pilot.CareerPreviewRequest) pilot.CareerPreviewResponse {
	if g.poolHelper != nil || g.stableHelper == nil {
		return pilot.CareerPreviewResponse{ID: 1, Version: 1, Error: "career_inactive"}
	}
	return g.stableHelper.CareerPreviewRequest(context.Background(), r)
}
func (g *Gym) CareerPreviewSnapshot() pilot.CareerPreviewResponse {
	return g.careerPreviewRequest(pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
}
func (g *Gym) BeginCareerFightPreview(viewID, aID, bID string) pilot.CareerPreviewResponse {
	return g.careerPreviewRequest(pilot.CareerPreviewRequest{Command: "career_preview_begin", ViewID: viewID, FighterAID: aID, FighterBID: bID})
}
func (g *Gym) CareerFightPreviewJob(viewID, jobID string) pilot.CareerPreviewResponse {
	return g.careerPreviewRequest(pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelCareerFightPreview(viewID, jobID string) pilot.CareerPreviewResponse {
	return g.careerPreviewRequest(pilot.CareerPreviewRequest{Command: "career_preview_cancel", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CareerFightPreviewPortrait(viewID, jobID, prospectID string) bridge.CareerPreviewPortraitResponse {
	if g.poolHelper != nil || g.stableHelper == nil {
		return bridge.CareerPreviewPortraitResponse{Version: 1, Error: "career_inactive"}
	}
	return g.stableHelper.CareerFightPreviewPortrait(context.Background(), viewID, jobID, prospectID)
}
