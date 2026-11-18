// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Closed pipe tests consume the independent modeled reference, never simulate
// or claim a live account. Actual parent authority is a separate acceptance gate.
func TestCareerPreviewBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_CAREER_PREVIEW_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	name := os.Getenv("SLUG_CAREER_PREVIEW_BRIDGE_FIXTURE")
	if name == "" {
		name = "xp1-versus-zero"
	}
	j, s := careerPreviewFixture(t, name)
	reads := 0
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var q pilot.CareerPreviewRequest
		if json.Unmarshal(scan.Bytes(), &q) != nil {
			os.Exit(2)
		}
		r := pilot.CareerPreviewResponse{ID: q.ID, Version: 1, OK: true}
		switch q.Command {
		case "career_preview_snapshot":
			r.Snapshot = &s
			if mode == "foreign-activation" {
				copy := s
				act := *s.Activation
				act.ActivatedAtMS++
				copy.Activation = &act
				r.Snapshot = &copy
			}
			if mode == "mixed-family" {
				raw, _ := json.Marshal(previewTransportFixtures(t)[0].SnapshotResponse)
				fmt.Println(string(raw))
				continue
			}
		case "career_preview_begin":
			if mode == "drop" {
				os.Exit(0)
			}
			if mode == "busy" {
				r = careerPreviewFailure(q.ID, "busy")
				break
			}
			queued := j
			queued.State = "queued"
			queued.Result = nil
			queued.FighterAID = q.FighterAID
			queued.FighterBID = q.FighterBID
			r.Job = &queued
			if mode == "wrong-pair" {
				queued.FighterAID = strings.Repeat("0", 64)
			}
		case "career_preview_job", "career_preview_cancel":
			reads++
			copy := j
			r.Job = &copy
			if mode == "cache-revoke" && reads > 2 {
				r = careerPreviewFailure(q.ID, "stale_view")
				break
			}
			if mode == "status-replace" && reads > 1 {
				value := *j.Result
				copy.Result = &value
				value.Pair.Status.ObservedSourceCount++
				value.Pair.Status.CaughtUp = false
			}
			if q.Command == "career_preview_cancel" {
				copy.State = "cancelled"
				copy.Error = "preview_cancelled"
				copy.Result = nil
			}
		default:
			os.Exit(3)
		}
		raw, _ := json.Marshal(r)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func careerPreviewTestBridge(t *testing.T, mode string) (*StableBridge, pilot.CareerPreviewJob) {
	return careerPreviewTestBridgeNamed(t, mode, "xp1-versus-zero")
}
func careerPreviewTestBridgeNamed(t *testing.T, mode, name string) (*StableBridge, pilot.CareerPreviewJob) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerPreviewBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_CAREER_PREVIEW_BRIDGE_PROCESS="+mode, "SLUG_CAREER_PREVIEW_BRIDGE_FIXTURE="+name)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	j, _ := careerPreviewFixture(t, name)
	model := careerPreviewModelBridge(j)
	b.careersEnabled = true
	b.view = model.view
	b.manifestView = model.manifestView
	b.storeID = model.storeID
	b.stablePin = model.stablePin
	b.resultPin = model.resultPin
	b.result = model.result
	t.Cleanup(func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	})
	return b, j
}
func TestCareerPreviewNativePipeAndPortraitReauthorization(t *testing.T) {
	for _, mode := range []string{"valid", "cache-revoke", "status-replace"} {
		t.Run(mode, func(t *testing.T) {
			b, j := careerPreviewTestBridge(t, mode)
			ctx := context.Background()
			s := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
			if !s.OK {
				t.Fatal(s)
			}
			r := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: j.View.ViewID, JobID: j.JobID})
			if !r.OK || r.Job == nil || r.Job.Result == nil {
				t.Fatal(r)
			}
			p := b.CareerFightPreviewPortrait(ctx, j.View.ViewID, j.JobID, j.FighterAID)
			if mode == "status-replace" {
				if p.OK || p.Portrait != nil {
					t.Fatal("mutable terminal disclosed SVG", p)
				}
				return
			}
			if !p.OK || p.Portrait == nil || p.Portrait.ActivationID != j.ActivationID || p.Portrait.CareerDigest != j.Result.Pair.CareerDigest || p.Portrait.ResultDigest != j.Result.EngineResultDigest {
				t.Fatal(p)
			}
			var expected string
			for _, row := range careerPreviewReference(t).Portraits {
				if row.Accept && row.Response.Portrait != nil && row.Response.Portrait.JobID == j.JobID && row.Response.Portrait.ProspectID == j.FighterAID {
					expected = row.Response.Portrait.SVG
					break
				}
			}
			if p.Portrait.SVG != expected || expected == "" {
				t.Fatal("fixed independent SVG differs")
			}
			hit := b.CareerFightPreviewPortrait(ctx, j.View.ViewID, j.JobID, j.FighterAID)
			if mode == "cache-revoke" {
				if hit.OK || hit.Portrait != nil || hit.Error != "stale_view" {
					t.Fatal("cache bypassed fresh original View", hit)
				}
			} else if !hit.OK || hit.Portrait.SVG != expected {
				t.Fatal(hit)
			}
		})
	}
}
func TestCareerPreviewBeginUnknownAndBusy(t *testing.T) {
	for _, mode := range []string{"valid", "drop", "busy", "wrong-pair", "mixed-family", "foreign-activation"} {
		t.Run(mode, func(t *testing.T) {
			b, j := careerPreviewTestBridge(t, mode)
			ctx := context.Background()
			s := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
			if mode == "mixed-family" || mode == "foreign-activation" {
				if s.OK || s.Snapshot != nil {
					t.Fatal(s)
				}
				return
			}
			if !s.OK {
				t.Fatal(s)
			}
			marker := &previewPresentation{}
			b.presentation = marker
			r := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_begin", ViewID: j.View.ViewID, FighterAID: j.FighterBID, FighterBID: j.FighterAID})
			switch mode {
			case "valid":
				if !r.OK || r.Job.FighterAID != j.FighterAID || b.presentation != nil {
					t.Fatal(r)
				}
			case "busy":
				if r.OK || r.Error != "busy" || b.presentation != marker {
					t.Fatal("definite refusal changed selection", r)
				}
			default:
				if r.OK || r.Error != "outcome_unknown" || r.Job != nil {
					t.Fatal(r)
				}
			}
		})
	}
}

func TestCareerPreviewAllFrozenTypedPortraitOutputs(t *testing.T) {
	all := careerPreviewReference(t)
	count := 0
	for _, row := range all.DTOs {
		if row.Kind != "CareerPreviewJob" || strings.Contains(row.Name, "/") {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			b, j := careerPreviewTestBridgeNamed(t, "valid", row.Name)
			ctx := context.Background()
			if r := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"}); !r.OK {
				t.Fatal(r)
			}
			if r := b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: j.View.ViewID, JobID: j.JobID}); !r.OK {
				t.Fatal(r)
			}
			for _, id := range []string{j.FighterAID, j.FighterBID} {
				out := b.CareerFightPreviewPortrait(ctx, j.View.ViewID, j.JobID, id)
				if !out.OK {
					t.Fatal(out)
				}
				raw, e := json.Marshal(out)
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, d := range all.DTOs {
					if d.Kind == "CareerPreviewPortraitResponse" && d.Name == row.Name+"/"+id {
						found = true
						count++
						if string(raw) != d.Canonical {
							t.Fatal("native typed portrait differs from independent canonical DTO")
						}
						break
					}
				}
				if !found {
					t.Fatal("missing expected portrait")
				}
			}
		})
	}
	if count != 26 {
		t.Fatal("typed portrait count", count)
	}
}

func TestCareerPreviewWaitingSourceRefusal(t *testing.T) {
	j, _ := careerPreviewFixture(t, "xp1-versus-zero")
	b := careerPreviewModelBridge(j)
	b.careerPreviewReady = false
	b.careerActivation = nil
	b.gate = make(chan struct{}, 1)
	b.failed = make(chan struct{})
	for _, view := range []string{j.View.ViewID, strings.Repeat("f", 64)} {
		want := "career_inactive"
		if view != j.View.ViewID {
			want = "stale_view"
		}
		r := b.careerPreviewRequestHeld(context.Background(), 1, pilot.CareerPreviewRequest{Command: "career_preview_begin", ViewID: view, FighterAID: j.FighterAID, FighterBID: j.FighterBID})
		if r.OK || r.Error != want || r.Job != nil || r.Snapshot != nil {
			t.Fatal(r)
		}
		p := b.CareerFightPreviewPortrait(context.Background(), view, j.JobID, j.FighterAID)
		if p.OK || p.Error != want || p.Portrait != nil {
			t.Fatal(p)
		}
	}
	if len(b.careerPreviewJobs) != 0 || b.presentation != nil {
		t.Fatal("waiting source dispatched/selected")
	}
}

func TestCareerPreviewDetachedViewRefusal(t *testing.T) {
	j, _ := careerPreviewFixture(t, "xp1-versus-zero")
	b := careerPreviewModelBridge(j)
	b.gate = make(chan struct{}, 1)
	b.failed = make(chan struct{})
	if !b.rememberCareerPreviewJob(j) || b.presentation == nil {
		t.Fatal("terminal presentation setup")
	}
	// This is the same invalidation used by a successful signed-out account
	// snapshot. No pipe writer is installed: an attempted dispatch would fail.
	b.clearStableView()
	for _, command := range []string{"career_preview_begin", "career_preview_job", "career_preview_cancel"} {
		q := pilot.CareerPreviewRequest{Command: command, ViewID: j.View.ViewID}
		if command == "career_preview_begin" {
			q.FighterAID, q.FighterBID = j.FighterAID, j.FighterBID
		} else {
			q.JobID = j.JobID
		}
		r := b.CareerPreviewRequest(context.Background(), q)
		if r.OK || r.Error != "stale_view" || r.Job != nil || r.Snapshot != nil {
			t.Fatal("detached original session", command, r)
		}
	}
	p := b.CareerFightPreviewPortrait(context.Background(), j.View.ViewID, j.JobID, j.FighterAID)
	if p.OK || p.Portrait != nil || p.Error != "stale_view" {
		t.Fatal("detached presentation", p)
	}
	snapshot := b.careerPreviewRequestHeld(context.Background(), 1, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
	if snapshot.OK || snapshot.Error != "preview_unavailable" || snapshot.Job != nil || snapshot.Snapshot != nil {
		t.Fatal("unavailable discovery classification", snapshot)
	}
	if b.presentation != nil || b.previewSelection != nil || len(b.careerPreviewJobs) != 0 || len(b.careerPreviewSnapshotJobs) != 0 || len(b.gate) != 0 {
		t.Fatal("detached data or gate retained")
	}
	// A closed transport is not a successful logout acknowledgement and stays
	// unavailable, without attempting to send or returning a prior payload.
	close(b.failed)
	r := b.CareerPreviewRequest(context.Background(), pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: j.View.ViewID, JobID: j.JobID})
	if r.OK || r.Error != "preview_unavailable" || r.Job != nil || r.Snapshot != nil || len(b.gate) != 0 {
		t.Fatal("closed transport classification", r)
	}
}
