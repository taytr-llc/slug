// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) previewRequest(r pilot.PreviewRequest) pilot.PreviewResponse {
	if g.stableHelper == nil {
		return pilot.PreviewResponse{ID: 1, Version: 1, Error: "preview_unavailable"}
	}
	return g.stableHelper.PreviewRequest(context.Background(), r)
}
func (g *Gym) PreviewSnapshot() pilot.PreviewResponse {
	return g.previewRequest(pilot.PreviewRequest{Command: "preview_snapshot"})
}
func (g *Gym) BeginFightPreview(viewID, aID, bID string) pilot.PreviewResponse {
	return g.previewRequest(pilot.PreviewRequest{Command: "preview_begin", ViewID: viewID, FighterAID: aID, FighterBID: bID})
}
func (g *Gym) FightPreviewJob(viewID, jobID string) pilot.PreviewResponse {
	return g.previewRequest(pilot.PreviewRequest{Command: "preview_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelFightPreview(viewID, jobID string) pilot.PreviewResponse {
	return g.previewRequest(pilot.PreviewRequest{Command: "preview_cancel", ViewID: viewID, JobID: jobID})
}
func (g *Gym) FightPreviewPortrait(viewID, jobID, prospectID string) bridge.PreviewPortraitResponse {
	if g.stableHelper == nil {
		return bridge.PreviewPortraitResponse{Version: 1, Error: "preview_unavailable"}
	}
	return g.stableHelper.FightPreviewPortrait(context.Background(), viewID, jobID, prospectID)
}
