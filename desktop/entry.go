// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
)

func (g *Gym) entryRequest(r pilot.EntryRequest) pilot.EntryResponse {
	if g.poolHelper != nil {
		if g.poolHelper.StableBridge == nil {
			return pilot.EntryResponse{ID: 1, Error: "entry_unavailable"}
		}
		return g.poolHelper.EntryRequest(context.Background(), r)
	}
	if g.stableHelper == nil {
		return pilot.EntryResponse{ID: 1, Error: "entry_unavailable"}
	}
	return g.stableHelper.EntryRequest(context.Background(), r)
}

func (g *Gym) EntrySnapshot() pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_snapshot"})
}
func (g *Gym) EntryUpcoming(viewID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_upcoming", ViewID: viewID})
}
func (g *Gym) EntryStatus(viewID, saturday string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_status", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) EntryHistory(viewID, afterEventID string, limit uint32) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_history", ViewID: viewID, AfterEventID: afterEventID, Limit: limit})
}
func (g *Gym) BeginEntryPrepare(viewID string, request gymentry.Request) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_prepare", ViewID: viewID, Request: &request})
}
func (g *Gym) BeginEntryApply(viewID, operationID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_apply", ViewID: viewID, OperationID: operationID})
}
func (g *Gym) BeginEntryCheck(viewID, saturday string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_check", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) EntryJob(viewID, jobID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelEntryJob(viewID, jobID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_cancel", ViewID: viewID, JobID: jobID})
}
func (g *Gym) EntryOperation(viewID, operationID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_operation", ViewID: viewID, OperationID: operationID})
}
func (g *Gym) EntryPending(viewID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_pending", ViewID: viewID})
}
func (g *Gym) AbandonEntryOperation(viewID, operationID string) pilot.EntryResponse {
	return g.entryRequest(pilot.EntryRequest{Command: "entry_abandon", ViewID: viewID, OperationID: operationID})
}
