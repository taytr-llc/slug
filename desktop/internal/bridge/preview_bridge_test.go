// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/wire/v2"
)

// These synthetic queued-job transport cases exercise no engine or work
// evaluation. Full completed results use the separate independent corpus.
func TestPreviewBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_PREVIEW_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	s := stableBridgeSnapshot(t)
	ps := pilot.PreviewSnapshot{Version: 1, State: "idle", View: s.View, StoreID: s.Status.StoreID, EngineVersion: 1, EngineID: fighterbout.EngineID}
	j := pilot.PreviewJob{Version: 1, JobID: strings.Repeat("e", 64), State: "queued", View: *s.View, StoreID: s.Status.StoreID, EngineVersion: 1, EngineID: fighterbout.EngineID, FighterAID: strings.Repeat("1", 64), FighterBID: strings.Repeat("2", 64)}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req pilot.PreviewRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		var reply any
		switch req.Command {
		case "snapshot":
			m := manifestBridgeSnapshot(t)
			reply = pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &m}
		case "stable_snapshot":
			reply = pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &s}
		case "preview_snapshot":
			if mode == "discover" {
				ps.State = "busy"
				ps.ActiveJobID = j.JobID
			}
			if mode == "wrong-store" {
				ps.StoreID = strings.Repeat("a", 64)
			}
			if mode == "wrong-generation" {
				v := *ps.View
				v.Generation++
				ps.View = &v
			}
			if mode == "mixed-family" {
				reply = pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &s}
			} else {
				reply = pilot.PreviewResponse{ID: req.ID, Version: 1, OK: true, Snapshot: &ps}
			}
		case "preview_begin":
			if mode == "drop" {
				os.Exit(0)
			}
			if mode == "busy" {
				reply = pilot.PreviewResponse{ID: req.ID, Version: 1, Error: "busy"}
				break
			}
			j.FighterAID, j.FighterBID = req.FighterAID, req.FighterBID
			if mode == "wrong-pair" {
				j.FighterAID = strings.Repeat("0", 64)
			}
			reply = pilot.PreviewResponse{ID: req.ID, Version: 1, OK: true, Job: &j}
		case "preview_job", "preview_cancel":
			if mode == "stale" {
				reply = pilot.PreviewResponse{ID: req.ID, Version: 1, Error: "stale_view"}
				break
			}
			j.State = "running"
			if req.Command == "preview_cancel" {
				j.State = "cancelled"
				j.Error = "preview_cancelled"
			}
			if mode == "changed-pair" {
				j.FighterAID = strings.Repeat("0", 64)
			}
			reply = pilot.PreviewResponse{ID: req.ID, Version: 1, OK: true, Job: &j}
		default:
			os.Exit(3)
		}
		raw, _ := json.Marshal(reply)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func previewTestBridge(t *testing.T, mode string) *StableBridge {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPreviewBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_PREVIEW_BRIDGE_PROCESS="+mode)
	b, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b
}
func establishPreview(t *testing.T, b *StableBridge) string {
	t.Helper()
	ctx := context.Background()
	if _, err := b.Request(ctx, "snapshot", 0); err != nil {
		t.Fatal(err)
	}
	s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !s.OK {
		t.Fatal(s)
	}
	p := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_snapshot"})
	if !p.OK {
		t.Fatal(p)
	}
	return s.Snapshot.View.ViewID
}
func TestPreviewRequiresFullEstablishedIdentity(t *testing.T) {
	b := previewTestBridge(t, "valid")
	ctx := context.Background()
	if r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_snapshot"}); r.Error != "preview_unavailable" {
		t.Fatal(r)
	}
	s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !s.OK {
		t.Fatal(s)
	}
	if r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_snapshot"}); r.Error != "preview_unavailable" {
		t.Fatal("stable alone blessed preview", r)
	}
	if _, err := b.Request(ctx, "snapshot", 0); err != nil {
		t.Fatal(err)
	}
	if r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_snapshot"}); !r.OK {
		t.Fatal(r)
	}
}
func TestPreviewNativeSortsAndCorrelatesOnlyKnownJobs(t *testing.T) {
	b := previewTestBridge(t, "valid")
	view := establishPreview(t, b)
	ctx := context.Background()
	unknown := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: view, JobID: strings.Repeat("f", 64)})
	if unknown.Error != "preview_not_found" {
		t.Fatal(unknown)
	}
	r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_begin", ViewID: view, FighterAID: strings.Repeat("2", 64), FighterBID: strings.Repeat("1", 64)})
	if !r.OK || r.Job.FighterAID != strings.Repeat("1", 64) {
		t.Fatal(r)
	}
	r = b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_cancel", ViewID: view, JobID: r.Job.JobID})
	if !r.OK || r.Job.State != "cancelled" {
		t.Fatal(r)
	}
	if len(b.jobs) != 0 || len(b.previewJobs) != 1 {
		t.Fatal("family correlation leaked")
	}
}
func TestPreviewSnapshotCanDiscoverExactAdvertisedJob(t *testing.T) {
	b := previewTestBridge(t, "discover")
	view := establishPreview(t, b)
	r := b.PreviewRequest(context.Background(), pilot.PreviewRequest{Command: "preview_job", ViewID: view, JobID: strings.Repeat("e", 64)})
	if !r.OK || r.Job.State != "running" || len(b.previewJobs) != 1 {
		t.Fatal(r)
	}
}
func TestPreviewRefusesForeignPinsAndOldFamilyFrames(t *testing.T) {
	for _, mode := range []string{"wrong-store", "wrong-generation", "mixed-family"} {
		t.Run(mode, func(t *testing.T) {
			b := previewTestBridge(t, mode)
			ctx := context.Background()
			if _, err := b.Request(ctx, "snapshot", 0); err != nil {
				t.Fatal(err)
			}
			if r := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"}); !r.OK {
				t.Fatal(r)
			}
			if r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_snapshot"}); r.OK || r.Error != "preview_unavailable" {
				t.Fatal(r)
			}
		})
	}
}
func TestPreviewUncertainBeginAndDefiniteBusyStayDistinct(t *testing.T) {
	for _, mode := range []string{"drop", "wrong-pair", "busy"} {
		t.Run(mode, func(t *testing.T) {
			b := previewTestBridge(t, mode)
			view := establishPreview(t, b)
			marker := &previewPresentation{}
			b.mu.Lock()
			b.presentation = marker
			b.mu.Unlock()
			r := b.PreviewRequest(context.Background(), pilot.PreviewRequest{Command: "preview_begin", ViewID: view, FighterAID: strings.Repeat("1", 64), FighterBID: strings.Repeat("2", 64)})
			if mode == "busy" {
				if r.Error != "busy" || b.presentation != marker {
					t.Fatal("preflight refusal discarded replay", r)
				}
			} else if r.Error != "outcome_unknown" || b.presentation != nil {
				t.Fatal("uncertain dispatch retained replay", r)
			}
		})
	}
}
func TestPreviewChangedPairAndRevokedViewRefuse(t *testing.T) {
	for _, mode := range []string{"changed-pair", "stale"} {
		t.Run(mode, func(t *testing.T) {
			b := previewTestBridge(t, mode)
			view := establishPreview(t, b)
			ctx := context.Background()
			r := b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_begin", ViewID: view, FighterAID: strings.Repeat("1", 64), FighterBID: strings.Repeat("2", 64)})
			if !r.OK {
				t.Fatal(r)
			}
			r = b.PreviewRequest(ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: view, JobID: r.Job.JobID})
			if r.OK {
				t.Fatal("foreign result published", r)
			}
			if mode == "stale" && (b.view != nil || b.stablePin != nil || b.previewReady) {
				t.Fatal("revoked pins retained")
			}
		})
	}
}
func TestPreviewPortraitMissingMemberDoesNotDispatch(t *testing.T) {
	b := previewTestBridge(t, "valid")
	view := establishPreview(t, b)
	r := b.FightPreviewPortrait(context.Background(), view, strings.Repeat("e", 64), strings.Repeat("1", 64))
	if r.OK || r.Error != "preview_not_found" {
		t.Fatal(r)
	}
	// A second snapshot succeeding proves no unexpected job/portrait command
	// was emitted to the helper after the local cache miss.
	if r := b.PreviewRequest(context.Background(), pilot.PreviewRequest{Command: "preview_snapshot"}); !r.OK {
		t.Fatal(r)
	}
}
