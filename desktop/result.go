// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) resultRequest(r pilot.ResultRequest) pilot.ResultResponse {
	if g.poolHelper != nil {
		return g.poolHelper.ResultRequest(context.Background(), r)
	}
	if g.stableHelper == nil {
		return pilot.ResultResponse{ID: 1, Error: "result_unavailable"}
	}
	return g.stableHelper.ResultRequest(context.Background(), r)
}
func (g *Gym) ResultSnapshot() pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_snapshot"})
}
func (g *Gym) ResultStatus(viewID, saturday, boutID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_status", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) ResultRecord(viewID, saturday, boutID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_record", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) BeginResultPublish(viewID, saturday, boutID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_publish", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) ResultJob(viewID, jobID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_job", ViewID: viewID, JobID: jobID})
}
func (g *Gym) CancelResultJob(viewID, jobID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_cancel", ViewID: viewID, JobID: jobID})
}

func (g *Gym) BeginResultReconcile(viewID, saturday, boutID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_reconcile", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}
func (g *Gym) BeginResultAttest(viewID, saturday, boutID string) pilot.ResultResponse {
	return g.resultRequest(pilot.ResultRequest{Command: "result_attest", ViewID: viewID, Saturday: saturday, BoutID: boutID})
}

func (g *Gym) ExportResultProof(viewID, saturday, boutID string) pilot.ResultResponse {
	failure := func(code string) pilot.ResultResponse { return pilot.ResultResponse{ID: 1, Error: code} }
	if g.stableHelper == nil && g.poolHelper == nil || g.app == nil || g.window == nil {
		return failure("result_unavailable")
	}
	if !g.exportMu.TryLock() {
		return failure("busy")
	}
	defer g.exportMu.Unlock()
	// Do not hold the helper pipe gate while a native modal is open: Stop/logout
	// remain available. The captured original View is rechecked after selection.
	status := g.resultRequest(pilot.ResultRequest{Command: "result_status", ViewID: viewID, Saturday: saturday, BoutID: boutID})
	if !status.OK {
		return status
	}
	if status.Status == nil || !status.Status.Exists || status.Status.InclusionHeight == 0 {
		return failure("result_proof_unavailable")
	}
	path, e := g.app.Dialog.SaveFile().AttachToWindow(g.window).SetMessage("Save this candidate result's public proof.").SetFilename("slug-candidate-result-"+boutID+".json").AddFilter("JSON proof", "*.json").AllowsOtherFileTypes(false).CanCreateDirectories(true).PromptForSingleSelection()
	if e != nil {
		return failure("result_unavailable")
	}
	if path == "" {
		return failure("result_cancelled")
	}
	path, e = selectedExportPath(path, g.protectedRoots)
	if e != nil {
		return failure("invalid_request")
	}
	if g.poolHelper != nil {
		return g.poolHelper.ExportResultProof(context.Background(), viewID, saturday, boutID, path)
	}
	return g.stableHelper.ExportResultProof(context.Background(), viewID, saturday, boutID, path)
}
