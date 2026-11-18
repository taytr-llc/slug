//go:build !slugtestpow

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
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// These empty modeled pages exercise the native pipe/correlation boundary only.
// They do not claim an actual award or authenticated source admission.
func careerPipePage(t *testing.T) pilot.CareerResponse {
	s := stableBridgeSnapshot(t)
	s.Status.LastAsOfMS = 1800000000000
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("a", 64), ResultCreatedAtMS: 1799999999000, SourceCreatedAtMS: 1799999998000, ActivatedAtMS: 1799999999500}
	a.ActivationID = careerstate.ActivationID(a)
	p := gymstable.CareerPage{Version: 1, Activation: a, CareerDigest: strings.Repeat("b", 64), AsOfMS: s.Status.LastAsOfMS, Source: gymstable.Page{Version: 1, Status: *s.Status, AsOfMS: s.Status.LastAsOfMS, Items: []gymstable.Item{}}, Items: []gymstable.CareerItem{}}
	return pilot.CareerResponse{ID: 1, JobID: strings.Repeat("f", 64), OK: true, View: s.View, StoreID: s.Status.StoreID, Page: &p}
}
func TestCareerBridgePipeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_CAREER_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	count := 0
	for scan.Scan() {
		var req pilot.CareerRequest
		if json.Unmarshal(scan.Bytes(), &req) != nil {
			os.Exit(2)
		}
		count++
		r := careerPipePage(t)
		r.ID = req.ID
		switch mode {
		case "wrong-job":
			r.JobID = strings.Repeat("e", 64)
		case "wrong-view":
			r.View.ViewID = strings.Repeat("e", 64)
		case "wrong-incarnation":
			r.Page.Activation.ResultCreatedAtMS--
			r.Page.Activation.ActivationID = careerstate.ActivationID(r.Page.Activation)
		case "changed-activation":
			if count > 1 {
				r.Page.Activation.ActivatedAtMS++
				r.Page.Activation.ActivationID = careerstate.ActivationID(r.Page.Activation)
			}
		case "wrong-page":
			r.Page.Source.After = 1
			r.Page.Source.NextAfter = 1
			r.Page.Source.Status.Checkpoint = 1
			r.Page.Source.Status.ObservedSourceCount = 1
		case "wrong-family":
			fmt.Printf("{\"id\":%d,\"ok\":false,\"error\":\"busy\"}\n", req.ID)
			continue
		case "busy":
			r = pilot.CareerResponse{ID: req.ID, Error: "busy"}
		case "lost":
			os.Exit(0)
		}
		raw, e := pilot.MarshalCareerResponse(r)
		if e != nil {
			t.Fatal(e)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func TestCareerBridgeOriginalViewIncarnationAndClosedFamily(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-job", "wrong-view", "wrong-incarnation", "changed-activation", "wrong-page", "wrong-family", "busy", "lost"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestCareerBridgePipeProcess$")
			cmd.Env = append(os.Environ(), "SLUG_CAREER_BRIDGE_PROCESS="+mode)
			b, e := startStable(cmd)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = b.Close() })
			p := careerPipePage(t)
			v := *p.View
			a := p.Page.Activation
			b.mu.Lock()
			b.careersEnabled = true
			b.resultsEnabled = true
			b.view = &v
			b.manifestView = &v
			b.storeID = p.StoreID
			b.stablePin = &p.Page.Source.Status.Pin
			b.resultPin = &resultAuthority{storeID: a.ResultStoreID, createdMS: a.ResultCreatedAtMS, sourceCreatedMS: a.SourceCreatedAtMS}
			b.result = &resultCorrelation{view: v}
			b.careerSelection = &careerPageExpectation{jobID: p.JobID, view: v, after: 0, limit: 8, ready: true, sourceDigest: careerSourceDigest(p.Page.Source)}
			b.mu.Unlock()
			req := pilot.CareerRequest{Command: "career_inventory", ViewID: v.ViewID, JobID: p.JobID, Limit: 8}
			r := b.CareerRequest(context.Background(), req)
			switch mode {
			case "valid", "changed-activation":
				if !r.OK || r.Page == nil {
					t.Fatal(r)
				}
			case "busy":
				if r.OK || r.Error != "busy" {
					t.Fatal(r)
				}
			default:
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal(r)
				}
			}
			if mode == "changed-activation" {
				r = b.CareerRequest(t.Context(), req)
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal("changed retained activation", r)
				}
			}
			if mode == "valid" {
				req.ViewID = strings.Repeat("e", 64)
				if r = b.CareerRequest(t.Context(), req); r.OK || r.Error != "stale_view" {
					t.Fatal("old view", r)
				}
			}
		})
	}
}

func TestCareerBridgeWaitingSourcePreservesOriginalView(t *testing.T) {
	// waiting_source and waiting_participants are private parent phases. Both
	// produce this exact validated setup snapshot, without Result authority.
	for _, phase := range []string{"waiting_source", "waiting_participants"} {
		t.Run(phase, func(t *testing.T) {
			v := *stableBridgeSnapshot(t).View
			b := &StableBridge{view: &v}
			s := &pilot.ResultSnapshot{Version: 1, State: "setup_incomplete", Error: "result_setup_incomplete", View: &v, NodeState: "offline", NodeOwner: "none"}
			if !b.rememberResultSnapshot(pilot.ResultResponse{ID: 1, OK: true, View: &v, Snapshot: s}) {
				t.Fatal("original-view setup refused")
			}
			if _, _, ready := b.resultContext(); ready {
				t.Fatal("setup manufactured Result authority")
			}
			if got := b.careerSetupError(v.ViewID, s); got != "career_inactive" {
				t.Fatal("matching original view", got)
			}
			if got := b.careerSetupError(strings.Repeat("e", 64), s); got != "stale_view" {
				t.Fatal("wrong requested view", got)
			}
			changed := v
			changed.Generation++
			b.view = &changed
			if got := b.careerSetupError(v.ViewID, s); got != "stale_view" {
				t.Fatal("replaced full view", got)
			}
			b.view = nil
			if got := b.careerSetupError(v.ViewID, s); got != "stale_view" {
				t.Fatal("revoked view", got)
			}
			b.view = &v
			b.failure = ErrUnavailable
			if got := b.careerSetupError(v.ViewID, s); got != "stale_view" {
				t.Fatal("failed connection", got)
			}
		})
	}
}

func TestCareerSelectionRejectsOlderTerminalAndChangedSource(t *testing.T) {
	p := careerPipePage(t)
	view := *p.View
	oldID, newID := strings.Repeat("1", 64), strings.Repeat("2", 64)
	old := pilot.StableRequest{Command: "stable_inventory", ViewID: view.ViewID, Limit: 8}
	recent := pilot.StableRequest{Command: "stable_recent", ViewID: view.ViewID}
	b := &StableBridge{careersEnabled: true, view: &view, jobs: map[string]stableJobExpectation{oldID: {request: old}, newID: {request: recent}}}
	j := pilot.StableJob{Version: 1, JobID: oldID, View: view, Kind: "inventory", State: "queued", StoreID: p.StoreID}
	if !b.rememberCareerStableJob(old, j) {
		t.Fatal("old admission")
	}
	j.State = "done"
	j.Page = &p.Page.Source
	if !b.rememberCareerStableJob(pilot.StableRequest{Command: "stable_job"}, j) || !b.careerSelection.ready {
		t.Fatal("old terminal")
	}
	next := j
	next.JobID = newID
	next.State = "queued"
	next.Page = nil
	if !b.rememberCareerStableJob(recent, next) {
		t.Fatal("new admission")
	}
	if !b.rememberCareerStableJob(pilot.StableRequest{Command: "stable_job"}, j) || b.careerSelection.jobID != newID || b.careerSelection.ready {
		t.Fatal("late old terminal replaced new selection")
	}
	next.State = "done"
	source := p.Page.Source
	source.After = 7
	next.Page = &source
	// Upstream Stable correlation supplies the actual recent page tuple. The
	// selected tuple must never be replaced by stable_recent's request After=0.
	if !b.rememberCareerStableJob(pilot.StableRequest{Command: "stable_job"}, next) || b.careerSelection.after != 7 || b.careerSelection.limit != 8 {
		t.Fatal("recent actual tuple")
	}
	source.AsOfMS++
	if b.rememberCareerStableJob(pilot.StableRequest{Command: "stable_job"}, next) {
		t.Fatal("changed terminal source accepted")
	}
	b.clearStableView()
	if b.careerSelection != nil {
		t.Fatal("logout retained selection")
	}
}
