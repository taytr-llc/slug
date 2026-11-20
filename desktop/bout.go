// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) boutRequest(r pilot.BoutRequest) pilot.BoutResponse {
	if g.poolHelper != nil {
		return g.poolHelper.BoutRequest(context.Background(), r)
	}
	if g.stableHelper == nil {
		return pilot.BoutResponse{ID: 1, Error: "bout_unavailable"}
	}
	return g.stableHelper.BoutRequest(context.Background(), r)
}
func (g *Gym) BoutSnapshot() pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_snapshot"})
}
func (g *Gym) BoutStatus(viewID, saturday, boutID string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_status", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) BoutRecord(viewID, saturday, boutID string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_record", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) BoutCardStatus(viewID, saturday string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_card_status", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) BeginBoutRun(viewID, saturday, boutID string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_run", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) BoutJob(viewID, jobID string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelBoutJob(viewID, jobID string) pilot.BoutResponse {
	return g.boutRequest(pilot.BoutRequest{Command: "bout_cancel", ViewID: viewID, JobID: jobID})
}

func (g *Gym) BoutPortrait(viewID, saturday, boutID, prospectID string) bridge.BoutPortraitResponse {
	if g.poolHelper != nil {
		return g.poolHelper.BoutPortrait(context.Background(), viewID, saturday, boutID, prospectID)
	}
	if g.stableHelper == nil {
		return bridge.BoutPortraitResponse{Version: 1, Error: "bout_unavailable"}
	}
	return g.stableHelper.BoutPortrait(context.Background(), viewID, saturday, boutID, prospectID)
}
