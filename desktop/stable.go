// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"

	"github.com/taytr-llc/slug/desktop/pilot"
)

// Both implementations retain the old closed manifest protocol. This private
// interface shares native dialogs without weakening any old response grammar.
type manifestTransport interface {
	Request(context.Context, string, uint64) (pilot.ManifestResponse, error)
	RequestFixture(context.Context, string) (pilot.ManifestResponse, error)
	RequestPath(context.Context, string, string) (pilot.ManifestResponse, error)
	ExportManifest(context.Context, uint64, string, string, string) (pilot.ManifestResponse, error)
	ReleaseManifest(context.Context, string, string) (pilot.ManifestResponse, error)
	Close() error
}

// Stable methods accept only typed navigation/selection values. Errors stay in
// the closed result envelope so an uncertain edit cannot become a plain thrown
// rejection. No native path or generic request body is exposed to JavaScript.
func (g *Gym) stableRequest(r pilot.StableRequest) pilot.StableResponse {
	if g.stableHelper == nil {
		return pilot.StableResponse{ID: 1, Error: "stable_unavailable"}
	}
	return g.stableHelper.StableRequest(context.Background(), r)
}
func (g *Gym) StableSnapshot() pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_snapshot"})
}
func (g *Gym) BeginStableSync(viewID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_sync", ViewID: viewID})
}
func (g *Gym) BeginStableInventory(viewID string, after uint64, limit uint32) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_inventory", ViewID: viewID, After: after, Limit: limit})
}
func (g *Gym) BeginStableRoster(viewID string, firstSlot, limit uint32) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_roster", ViewID: viewID, FirstSlot: firstSlot, Limit: limit})
}
func (g *Gym) BeginStablePrepare(viewID string, expectedRevision uint64, slot uint32, prospectID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_prepare_slot", ViewID: viewID, ExpectedRevision: expectedRevision, Slot: slot, ProspectID: prospectID})
}
func (g *Gym) BeginStableApply(viewID, operationID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_apply_slot", ViewID: viewID, OperationID: operationID})
}
func (g *Gym) StableJob(viewID, jobID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) StableOperation(viewID, operationID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_operation", ViewID: viewID, OperationID: operationID})
}
func (g *Gym) StablePending(viewID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_pending", ViewID: viewID})
}
func (g *Gym) AbandonStableOperation(viewID, operationID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_abandon_operation", ViewID: viewID, OperationID: operationID})
}
func (g *Gym) CancelStableJob(viewID, jobID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_cancel", ViewID: viewID, JobID: jobID})
}

func (g *Gym) BeginStableRecent(viewID string) pilot.StableResponse {
	return g.stableRequest(pilot.StableRequest{Command: "stable_recent", ViewID: viewID})
}
