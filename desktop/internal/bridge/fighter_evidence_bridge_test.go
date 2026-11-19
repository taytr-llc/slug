//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"path/filepath"
	"strings"
	"testing"
)

func evidenceBridgeHistory(t *testing.T) pilot.CareerHistoryJob {
	t.Helper()
	for _, p := range historyReference(t).Wire.Protocols {
		if p.Context.Selected != nil && p.Context.Selected.State == "done" && p.Context.Selected.Capture != nil {
			return *p.Context.Selected
		}
	}
	t.Fatal("no frozen completed history")
	return pilot.CareerHistoryJob{}
}
func TestFighterEvidenceBridgeKnownOperationAndTerminalBinding(t *testing.T) {
	history := evidenceBridgeHistory(t)
	b := historyModelBridge(t, history)
	pin := recoveryfile.Pin{Version: 1, Mode: recoveryfile.CreateMode, Path: filepath.Join(t.TempDir(), "public.json"), Parent: recoveryfile.Identity{Device: "0000000000000001", Inode: "0000000000000002"}}
	id := strings.Repeat("9", 64)
	if e := b.RememberFighterEvidence(history, id, pin); e != nil {
		t.Fatal(e)
	}
	expected := b.evidenceKnownLocked(id)
	job := expected.job
	req := pilot.FighterEvidenceRequest{ID: 3, Version: 1, Command: "fighter_evidence_job", ViewID: job.View.ViewID, JobID: id}
	r := pilot.FighterEvidenceResponse{ID: 3, Version: 1, OK: true, Job: &job}
	if !b.correlateFighterEvidence(req, r) {
		t.Fatal("known original job refused")
	}
	for _, mutate := range []func(*pilot.FighterEvidenceJob){func(j *pilot.FighterEvidenceJob) { j.HistoryID = strings.Repeat("8", 64) }, func(j *pilot.FighterEvidenceJob) { j.View.Generation++ }, func(j *pilot.FighterEvidenceJob) { j.ProspectID = strings.Repeat("7", 64) }, func(j *pilot.FighterEvidenceJob) { j.JobID = strings.Repeat("6", 64) }} {
		changed := job
		mutate(&changed)
		r.Job = &changed
		if b.correlateFighterEvidence(req, r) {
			t.Fatal("changed job accepted")
		}
	}
	job.State = "done"
	job.Summary = &pilot.FighterEvidenceSummary{Version: 1, FileSHA256: strings.Repeat("a", 64), FileBytes: 100, ProspectID: job.ProspectID, HistoryID: job.HistoryID, SourceRecordDigest: expected.sourceDigest, EventCount: expected.count, CapturedAsOfMS: expected.asOf}
	r.Job = &job
	if !b.correlateFighterEvidence(req, r) {
		t.Fatal("bound summary refused")
	}
	job.Summary.FileBytes++
	if b.correlateFighterEvidence(req, r) {
		t.Fatal("terminal summary changed")
	}
	snapshot := pilot.FighterEvidenceSnapshot{Version: 1, State: "idle", View: &history.View, StoreID: history.StoreID, LastJobID: strings.Repeat("f", 64)}
	sr := pilot.FighterEvidenceRequest{ID: 4, Version: 1, Command: "fighter_evidence_snapshot"}
	if b.correlateFighterEvidence(sr, pilot.FighterEvidenceResponse{ID: 4, Version: 1, OK: true, Snapshot: &snapshot}) {
		t.Fatal("adopted arbitrary latest operation")
	}
}
func TestFighterEvidenceBridgeRequestAndSnapshotClosedBounds(t *testing.T) {
	r := pilot.FighterEvidenceRequest{ID: 1, Version: 1, Command: "fighter_evidence_snapshot"}
	raw, e := pilot.MarshalFighterEvidenceRequest(r)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"id":1`, `"id":9007199254740992`, 1), strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(raw), `"job_id":""`, `"job_id":null`, 1), strings.Replace(string(raw), `"selection":null`, `"selection":null,"path":"/tmp/x"`, 1)} {
		if _, e := pilot.ParseFighterEvidenceRequest([]byte(bad)); e == nil {
			t.Fatal("nonclosed request accepted")
		}
	}
	s := pilot.BlankFighterEvidenceSnapshot()
	if pilot.ValidateFighterEvidenceSnapshot(s) != nil {
		t.Fatal("blank refused")
	}
	s.Error = "unauthorized"
	if pilot.ValidateFighterEvidenceSnapshot(s) == nil {
		t.Fatal("authority error smuggled into snapshot")
	}
}

func TestFighterEvidenceBridgeViewClearReleasesOnlyMetadata(t *testing.T) {
	history := evidenceBridgeHistory(t)
	b := historyModelBridge(t, history)
	b.evidence[0] = &evidenceExpectation{job: pilot.FighterEvidenceJob{JobID: strings.Repeat("a", 64), View: history.View}}
	b.evidence[1] = &evidenceExpectation{job: pilot.FighterEvidenceJob{JobID: strings.Repeat("b", 64), View: history.View}}
	b.clearStableView()
	if b.evidence[0] != nil || b.evidence[1] != nil || b.view != nil {
		t.Fatal("old metadata exhausted replacement view")
	}
}
