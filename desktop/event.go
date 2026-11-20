// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) eventRequest(r pilot.EventRequest) pilot.EventResponse {
	if g.poolHelper != nil {
		return g.poolHelper.EventRequest(context.Background(), r)
	}
	if g.stableHelper == nil {
		return pilot.EventResponse{ID: 1, Error: "event_unavailable"}
	}
	return g.stableHelper.EventRequest(context.Background(), r)
}

func (g *Gym) EventSnapshot() pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_snapshot"})
}
func (g *Gym) EventStatus(viewID, saturday string) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_status", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) EventRecord(viewID, saturday string) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_record", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) EventHistory(viewID, afterEventID string, limit uint32) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_history", ViewID: viewID, AfterEventID: afterEventID, Limit: limit})
}
func (g *Gym) BeginEventCapture(viewID, saturday string) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_capture", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) EventJob(viewID, jobID string) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelEventJob(viewID, jobID string) pilot.EventResponse {
	return g.eventRequest(pilot.EventRequest{Command: "event_cancel", ViewID: viewID, JobID: jobID})
}
