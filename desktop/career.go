// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) CareerInventory(viewID, jobID string, after uint64, limit uint32) pilot.CareerResponse {
	if g.poolHelper != nil {
		return g.poolHelper.CareerRequest(context.Background(), pilot.CareerRequest{Command: "career_inventory", ViewID: viewID, JobID: jobID, After: after, Limit: limit})
	}
	if g.stableHelper == nil {
		return pilot.CareerResponse{ID: 1, Error: "career_inactive"}
	}
	return g.stableHelper.CareerRequest(context.Background(), pilot.CareerRequest{Command: "career_inventory", ViewID: viewID, JobID: jobID, After: after, Limit: limit})
}
func (g *Gym) CareerRoster(viewID, jobID string, firstSlot, limit uint32) pilot.CareerResponse {
	if g.poolHelper != nil {
		return g.poolHelper.CareerRequest(context.Background(), pilot.CareerRequest{Command: "career_roster", ViewID: viewID, JobID: jobID, FirstSlot: firstSlot, Limit: limit})
	}
	if g.stableHelper == nil {
		return pilot.CareerResponse{ID: 1, Error: "career_inactive"}
	}
	return g.stableHelper.CareerRequest(context.Background(), pilot.CareerRequest{Command: "career_roster", ViewID: viewID, JobID: jobID, FirstSlot: firstSlot, Limit: limit})
}

func (g *Gym) CareerMode() bool {
	return g.poolHelper != nil && g.poolHelper.CareerEnabled() || g.stableHelper != nil && g.stableHelper.CareerEnabled()
}
