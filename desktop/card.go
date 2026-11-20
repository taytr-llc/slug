// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) cardRequest(r pilot.CardRequest) pilot.CardResponse {
	if g.poolHelper != nil {
		return g.poolHelper.CardRequest(context.Background(), r)
	}
	if g.stableHelper == nil {
		return pilot.CardResponse{ID: 1, Error: "card_unavailable"}
	}
	return g.stableHelper.CardRequest(context.Background(), r)
}
func (g *Gym) CardSnapshot() pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_snapshot"})
}
func (g *Gym) CardStatus(viewID, saturday string) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_status", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) CardRecord(viewID, saturday string) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_record", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) CardHistory(viewID, afterCardID string, limit uint32) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_history", ViewID: viewID, AfterCardID: afterCardID, Limit: limit})
}
func (g *Gym) BeginCardBuild(viewID, saturday string) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_build", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) CardJob(viewID, jobID string) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelCardJob(viewID, jobID string) pilot.CardResponse {
	return g.cardRequest(pilot.CardRequest{Command: "card_cancel", ViewID: viewID, JobID: jobID})
}
