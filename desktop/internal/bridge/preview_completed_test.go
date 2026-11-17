// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

type previewTransportFixture struct {
	Name             string                `json:"name"`
	Status           gymstable.Status      `json:"status"`
	View             pilot.ManifestViewPin `json:"view"`
	Response         pilot.PreviewResponse `json:"response"`
	SnapshotResponse pilot.PreviewResponse `json:"snapshot_response"`
	Portraits        []struct {
		ID     string `json:"prospect_id"`
		SVG    string `json:"svg"`
		Digest string `json:"portrait_digest"`
	} `json:"portrait_evidence"`
}

func previewTransportFixtures(t *testing.T) []previewTransportFixture {
	t.Helper()
	raw, e := os.ReadFile("../../../fighterbout/testdata/reference/preview_transport_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var all struct {
		Cases []previewTransportFixture `json:"cases"`
	}
	if e = json.Unmarshal(raw, &all); e != nil || len(all.Cases) != 4 {
		t.Fatal("independent fixtures", e)
	}
	return all.Cases
}
func TestPreviewCompletedBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_PREVIEW_COMPLETED_PROCESS")
	if mode == "" {
		return
	}
	index, _ := strconv.Atoi(os.Getenv("SLUG_PREVIEW_COMPLETED_INDEX"))
	c := previewTransportFixtures(t)[index]
	reads := 0
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req pilot.PreviewRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		var response any
		switch req.Command {
		case "snapshot":
			m := manifestBridgeSnapshot(t)
			m.View = &c.View
			m.Account.ID = c.View.AccountID
			m.Account.Generation = c.View.Generation
			m.Account.Fixture = strings.Split(c.Name, "-")[0]
			m.GymID = c.Status.Pin.Identity.GymID
			m.GymPublicKey = c.Status.Pin.Identity.GymPublicKey
			response = pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &m}
		case "stable_snapshot":
			response = pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &pilot.StableSnapshot{Version: 1, State: "ready", View: &c.View, Status: &c.Status}}
		case "preview_snapshot":
			r := c.SnapshotResponse
			r.ID = req.ID
			response = r
		case "preview_job":
			reads++
			r := c.Response
			r.ID = req.ID
			j := *r.Job
			r.Job = &j
			if mode == "drop" && reads > 1 {
				os.Exit(0)
			}
			if (mode == "not-found" && reads > 1) || (mode == "cache-revoke" && reads > 2) {
				r = previewFailure(req.ID, "preview_not_found")
			}
			if reads > 1 {
				switch mode {
				case "generation":
					j.View.Generation++
				case "job":
					j.JobID = strings.Repeat("9", 64)
				case "appearance":
					result := *j.Result
					j.Result = &result
					result.Pair.Fighters[0].Assignment.PortraitDigest = strings.Repeat("9", 64)
				case "status":
					result := *j.Result
					j.Result = &result
					result.Pair.Status.RosterRevision = 1
					result.Pair.Status.OperationCount = 1
					result.Pair.Status.LastOperationID = strings.Repeat("9", 64)
					result.Pair.Status.Slots[0] = result.Pair.Fighters[0].Assignment.ProspectID
				}
			}
			response = r
		default:
			os.Exit(3)
		}
		raw, _ := json.Marshal(response)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func completedPreviewBridge(t *testing.T, mode string, index int) (*StableBridge, previewTransportFixture) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPreviewCompletedBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_PREVIEW_COMPLETED_PROCESS="+mode, "SLUG_PREVIEW_COMPLETED_INDEX="+strconv.Itoa(index))
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	})
	c := previewTransportFixtures(t)[index]
	establishPreview(t, b)
	r := b.PreviewRequest(context.Background(), pilot.PreviewRequest{Command: "preview_job", ViewID: c.View.ViewID, JobID: c.Response.Job.JobID})
	if !r.OK || r.Job.State != "done" {
		t.Fatal("complete independent fixture", r.Error)
	}
	return b, c
}
func TestPreviewCompletedIndependentPortraitsAndCacheHit(t *testing.T) {
	for n, c := range previewTransportFixtures(t) {
		t.Run(c.Name, func(t *testing.T) {
			b, c := completedPreviewBridge(t, "valid", n)
			for _, p := range c.Portraits {
				for repeat := 0; repeat < 2; repeat++ {
					r := b.FightPreviewPortrait(context.Background(), c.View.ViewID, c.Response.Job.JobID, p.ID)
					if !r.OK || r.Portrait == nil || r.Portrait.SVG != p.SVG || r.Portrait.PortraitDigest != p.Digest || r.Portrait.ResultDigest != c.Response.Job.Result.EngineResultDigest {
						t.Fatal("independent portrait or cached disclosure", r.Error)
					}
				}
			}
			b.mu.Lock()
			meta, e := json.Marshal(b.presentation.metadata)
			retained := len(meta) + len(b.presentation.svgs[0]) + len(b.presentation.svgs[1])
			b.mu.Unlock()
			if e != nil || len(meta) > 24576 || retained > 90112 {
				t.Fatal("presentation bound", retained, e)
			}
		})
	}
}
func TestPreviewPortraitFreshAuthorizationRefusesEveryStaleTuple(t *testing.T) {
	for _, mode := range []string{"not-found", "cache-revoke", "generation", "job", "appearance", "status", "drop"} {
		t.Run(mode, func(t *testing.T) {
			b, c := completedPreviewBridge(t, mode, 0)
			p := c.Portraits[0]
			if mode == "cache-revoke" {
				r := b.FightPreviewPortrait(context.Background(), c.View.ViewID, c.Response.Job.JobID, p.ID)
				if !r.OK {
					t.Fatal("first render", r.Error)
				}
			}
			r := b.FightPreviewPortrait(context.Background(), c.View.ViewID, c.Response.Job.JobID, p.ID)
			if r.OK || r.Portrait != nil {
				t.Fatal("stale portrait disclosure", mode)
			}
		})
	}
}
