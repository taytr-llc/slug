// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) careerHistoryRequest(r pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	if g.stableHelper == nil {
		return pilot.CareerHistoryResponse{ID: 1, Version: 1, Error: "career_inactive"}
	}
	return g.stableHelper.CareerHistoryRequest(context.Background(), r)
}
func (g *Gym) CareerHistorySnapshot() pilot.CareerHistoryResponse {
	return g.careerHistoryRequest(pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
}
func (g *Gym) BeginCareerHistory(viewID, prospectID string) pilot.CareerHistoryResponse {
	return g.careerHistoryRequest(pilot.CareerHistoryRequest{Command: "career_history_begin", ViewID: viewID, ProspectID: prospectID})
}
func (g *Gym) CareerHistoryJob(viewID, jobID string) pilot.CareerHistoryResponse {
	return g.careerHistoryRequest(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelCareerHistory(viewID, jobID string) pilot.CareerHistoryResponse {
	return g.careerHistoryRequest(pilot.CareerHistoryRequest{Command: "career_history_cancel", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CareerHistoryPage(viewID, jobID string, after, limit uint32) pilot.CareerHistoryResponse {
	return g.careerHistoryRequest(pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: viewID, JobID: jobID, After: after, Limit: limit})
}
func (g *Gym) CareerHistoryResult(viewID, jobID string, ordinal uint32) pilot.ResultResponse {
	if g.stableHelper == nil {
		return pilot.ResultResponse{ID: 1, Error: "result_unavailable"}
	}
	return g.stableHelper.CareerHistoryResult(context.Background(), viewID, jobID, ordinal)
}
