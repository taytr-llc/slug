// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"path/filepath"
	"strings"
	"testing"
)

func nativeEvidenceFixture(t *testing.T) (*Gym, *nativeEvidencePending) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	selection, e := recoveryfile.Capture(filepath.Join(root, "public.json"), nil, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	helper := &bridge.StableBridge{}
	p := &nativeEvidencePending{selection: selection, viewID: strings.Repeat("a", 64), jobID: strings.Repeat("b", 64), helper: helper, done: make(chan struct{})}
	g := &Gym{stableHelper: helper}
	g.evidence.pending = p
	t.Cleanup(func() { _ = g.releaseEvidenceSelection(p) })
	return g, p
}
func TestFighterEvidenceNativePendingSelectionWaitsForJoinedObservation(t *testing.T) {
	for _, state := range []string{"signing_out", "resetting", "signing_in", "expired", "failed"} {
		t.Run(state, func(t *testing.T) {
			g, p := nativeEvidenceFixture(t)
			if e := g.observeEvidenceAccount(p.helper, state, nil); e != nil {
				t.Fatal(e)
			}
			if p.selection.Pin().Version != 1 || g.evidence.pending != p {
				t.Fatal("async acknowledgment released directory")
			}
			if e := g.observeEvidenceAccount(&bridge.StableBridge{}, "signed_out", nil); e != nil {
				t.Fatal(e)
			}
			if p.selection.Pin().Version != 1 {
				t.Fatal("different helper released original selection")
			}
			if e := g.observeEvidenceAccount(p.helper, "signed_out", nil); e != nil {
				t.Fatal(e)
			}
			if p.selection.Pin().Version != 0 || g.evidence.pending != nil {
				t.Fatal("joined observation retained selection")
			}
			select {
			case <-p.done:
			default:
				t.Fatal("pending lifetime not closed")
			}
		})
	}
}
func TestFighterEvidenceNativeReplacementViewAndOriginalPin(t *testing.T) {
	g, p := nativeEvidenceFixture(t)
	same := pilot.ManifestViewPin{ViewID: p.viewID}
	if e := g.observeEvidenceAccount(p.helper, "signed_in", &same); e != nil {
		t.Fatal(e)
	}
	if p.selection.Pin().Version != 1 {
		t.Fatal("same view treated as joined replacement")
	}
	next := same
	next.ViewID = strings.Repeat("c", 64)
	if e := g.observeEvidenceAccount(p.helper, "signed_in", &next); e != nil {
		t.Fatal(e)
	}
	if p.selection.Pin().Version != 0 {
		t.Fatal("replacement did not release old handoff")
	}
}

func TestFighterEvidenceNativeRecoveryOnlyCurrentAction(t *testing.T) {
	// Both exports select the same history. A previous successful file must not
	// answer a new request whose acknowledgment was lost before dispatch.
	a := nativeEvidenceAction{viewID: strings.Repeat("a", 64), historyJobID: strings.Repeat("b", 64), historyID: strings.Repeat("c", 64), jobID: strings.Repeat("d", 64), state: "unknown"}
	oldID := strings.Repeat("e", 64)
	old := pilot.FighterEvidenceSnapshot{Version: 1, State: "idle", View: &pilot.ManifestViewPin{ViewID: a.viewID}, StoreID: strings.Repeat("f", 64), LastJobID: oldID}
	for _, state := range []string{"unknown", "not_started", ""} {
		t.Run(state, func(t *testing.T) {
			current := a
			current.state = state
			calls := 0
			r, resolved := nativeEvidenceActionSnapshot(current, func(q pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
				calls++
				if q.Command == "fighter_evidence_job" {
					if state != "unknown" || q.JobID != a.jobID || q.ViewID != a.viewID {
						t.Fatal("lookup did not use the exact latest action")
					}
					return evidenceNativeFailure("evidence_not_found")
				}
				if q.Command != "fighter_evidence_snapshot" {
					t.Fatal(q.Command)
				}
				return pilot.FighterEvidenceResponse{ID: 1, Version: 1, OK: true, Snapshot: &old}
			})
			if !r.OK || r.Snapshot == nil || r.Snapshot.State != "idle" || r.Snapshot.ActiveJobID != "" || r.Snapshot.LastJobID != "" || old.LastJobID != oldID {
				t.Fatal("new action exposed or mutated the previous successful export")
			}
			if state == "unknown" && (calls != 2 || resolved != "not_started") || state != "unknown" && (calls != 1 || resolved != "") {
				t.Fatalf("calls=%d resolved=%q", calls, resolved)
			}
		})
	}
	for _, state := range []string{"running", "done"} {
		t.Run(state, func(t *testing.T) {
			calls := 0
			r, resolved := nativeEvidenceActionSnapshot(a, func(q pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
				calls++
				if q.Command != "fighter_evidence_job" || q.JobID != a.jobID {
					t.Fatal("exact recovery fell back to generic latest job")
				}
				j := pilot.FighterEvidenceJob{Version: 1, JobID: a.jobID, State: state, View: *old.View, StoreID: old.StoreID, HistoryJobID: a.historyJobID, HistoryID: a.historyID}
				return pilot.FighterEvidenceResponse{ID: 1, Version: 1, OK: true, Job: &j}
			})
			if calls != 1 || resolved != "admitted" || !r.OK || r.Snapshot == nil || r.Job != nil {
				t.Fatal("exact action recovery failed")
			}
			if state == "running" && (r.Snapshot.State != "busy" || r.Snapshot.ActiveJobID != a.jobID || r.Snapshot.LastJobID != "") || state == "done" && (r.Snapshot.State != "idle" || r.Snapshot.LastJobID != a.jobID || r.Snapshot.ActiveJobID != "") {
				t.Fatal("snapshot did not expose only the exact current action")
			}
		})
	}
	preparing := a
	preparing.state = "preparing"
	r, _ := nativeEvidenceActionSnapshot(preparing, func(pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
		t.Fatal("preparing queried old helper history")
		return pilot.FighterEvidenceResponse{}
	})
	if r.OK || r.Error != "outcome_unknown" || r.Snapshot != nil {
		t.Fatal("pre-dialog action exposed a prior export")
	}
	r, _ = nativeEvidenceActionSnapshot(a, func(pilot.FighterEvidenceRequest) pilot.FighterEvidenceResponse {
		j := pilot.FighterEvidenceJob{Version: 1, JobID: oldID, State: "done", View: *old.View, StoreID: old.StoreID, HistoryJobID: a.historyJobID, HistoryID: a.historyID}
		return pilot.FighterEvidenceResponse{ID: 1, Version: 1, OK: true, Job: &j}
	})
	if r.OK || r.Error != "outcome_unknown" || r.Snapshot != nil {
		t.Fatal("foreign exact lookup became an export result")
	}
}

func TestFighterEvidenceNativeOldActionCannotBePolledOrCancelled(t *testing.T) {
	g, p := nativeEvidenceFixture(t)
	g.evidence.action = &nativeEvidenceAction{helper: p.helper, viewID: p.viewID, jobID: p.jobID, state: "admitted"}
	if !g.currentEvidenceAction(p.viewID, p.jobID) || g.currentEvidenceAction(p.viewID, strings.Repeat("c", 64)) {
		t.Fatal("current action guard is not exact")
	}
	g.evidence.action.state = "not_started"
	for _, f := range []func(string, string) (pilot.FighterEvidenceResponse, error){g.FighterEvidenceJob, g.CancelFighterEvidence} {
		r, e := f(p.viewID, p.jobID)
		if e != nil || r.OK || r.Error != "evidence_not_found" {
			t.Fatal("definite no-start dispatched a previous action")
		}
	}
}
