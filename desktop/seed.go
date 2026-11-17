// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) seedRequest(r pilot.SeedRequest) pilot.SeedResponse {
	if g.stableHelper == nil {
		return pilot.SeedResponse{ID: 1, Error: "seed_unavailable"}
	}
	return g.stableHelper.SeedRequest(context.Background(), r)
}
func (g *Gym) SeedSnapshot() pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_snapshot"})
}
func (g *Gym) SeedStatus(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_status", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) SeedRecord(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_record", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) SeedHistory(viewID, afterEventKey string, limit uint32) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_history", ViewID: viewID, AfterEventKey: afterEventKey, Limit: limit})
}
func (g *Gym) BeginSeedPrepare(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_prepare", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) BeginSeedPublish(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_publish", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) BeginSeedReconcile(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_reconcile", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) BeginSeedAttest(viewID, saturday string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_attest", ViewID: viewID, Saturday: saturday})
}
func (g *Gym) SeedJob(viewID, jobID string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelSeedJob(viewID, jobID string) pilot.SeedResponse {
	return g.seedRequest(pilot.SeedRequest{Command: "seed_cancel", ViewID: viewID, JobID: jobID})
}
