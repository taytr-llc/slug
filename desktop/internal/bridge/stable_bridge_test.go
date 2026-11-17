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
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

// Transport examples exercise correlation only, not proof or digest goldens.
func stableBridgeSnapshot(t *testing.T) pilot.StableSnapshot {
	t.Helper()
	old := manifestBridgeSnapshot(t)
	identity, err := gymaccount.FixtureIdentity("zero256")
	if err != nil {
		t.Fatal(err)
	}
	pin := gymstable.Pin{Version: 1, AccountAuthorityID: old.View.AccountAuthorityID, AccountID: old.View.AccountID, Identity: identity, Source: old.View.AuthorityPin, ConditionVersion: 1, ProjectorID: fightercondition.ProjectorID}
	id, err := gymstable.ID(pin)
	if err != nil {
		t.Fatal(err)
	}
	status := gymstable.Status{Version: 1, StoreID: id, Pin: pin, CaughtUp: true}
	return pilot.StableSnapshot{Version: 1, State: "ready", View: old.View, Status: &status}
}
func stableTestBridge(t *testing.T, mode string) *StableBridge {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStableBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_STABLE_BRIDGE_PROCESS="+mode)
	b, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error("owned helper did not drain", err)
		}
	})
	return b
}
func TestStableBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_STABLE_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	var asyncRequest pilot.StableRequest
	for scanner.Scan() {
		var req pilot.StableRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		if req.Command == "snapshot" {
			s := manifestBridgeSnapshot(t)
			raw, _ := json.Marshal(pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &s})
			fmt.Println(string(raw))
			continue
		}
		s := stableBridgeSnapshot(t)
		r := pilot.StableResponse{ID: req.ID, OK: true}
		if strings.HasPrefix(mode, "async-") && req.Command != "stable_snapshot" {
			if stableStartsJob(req.Command) {
				asyncRequest = req
				r.Job = stableAsyncResponse(s, req, mode, false)
			} else {
				r.Job = stableAsyncResponse(s, asyncRequest, mode, true)
			}
			if mode == "async-malformed" && oneOf(req.Command, "stable_job", "stable_cancel") {
				fmt.Printf("{\"id\":%d,\"ok\":true,\"job\":{}}\n", req.ID)
			} else {
				raw, _ := json.Marshal(r)
				fmt.Println(string(raw))
			}
			continue
		}
		switch req.Command {
		case "stable_snapshot":
			r.Snapshot = &s
		case "stable_recent":
			r.Job = &pilot.StableJob{Version: 1, JobID: strings.Repeat("f", 64), Kind: "inventory", State: "queued", View: *s.View, StoreID: s.Status.StoreID}
		case "stable_pending", "stable_operation":
			r.Operation = &pilot.StableOperation{Version: 1, View: *s.View, StoreID: s.Status.StoreID}
		case "stable_apply_slot", "stable_abandon_operation":
			if mode == "drop-apply" {
				os.Exit(0)
			}
			if mode == "malformed-apply" {
				fmt.Println(`{"id":2,"ok":true,"job":{}}`)
				continue
			}
			if mode == "stale-apply" {
				r.OK = false
				r.Error = "stale_view"
				break
			}
			if mode == "busy-apply" {
				r.OK = false
				r.Error = "busy"
				break
			}
			r.Job = &pilot.StableJob{Version: 1, JobID: strings.Repeat("f", 64), Kind: "apply", State: "queued", View: *s.View, StoreID: s.Status.StoreID, OperationID: req.OperationID}
			if mode == "wrong-operation" {
				r.Job.OperationID = strings.Repeat("a", 64)
			}
			if mode == "wrong-view" {
				r.Job.View.ViewID = strings.Repeat("1", 64)
			}
		default:
			os.Exit(3)
		}
		raw, _ := json.Marshal(r)
		if mode == "mixed-family" && req.Command == "stable_snapshot" {
			s := manifestBridgeSnapshot(t)
			raw, _ = json.Marshal(pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &s})
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func TestStableBridgeClosedFamiliesAndPendingNull(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	b := stableTestBridge(t, "valid")
	ctx := context.Background()
	r := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !r.OK || r.Snapshot == nil {
		t.Fatal(r)
	}
	view := r.Snapshot.View.ViewID
	recent := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_recent", ViewID: view})
	if !recent.OK || recent.Job == nil || recent.Job.Kind != "inventory" {
		t.Fatal("explicit Recent ACK", recent)
	}

	if old, err := b.Request(ctx, "snapshot", 0); err != nil || old.Snapshot == nil {
		t.Fatal("old exact route", err)
	}
	r = b.StableRequest(ctx, pilot.StableRequest{Command: "stable_pending", ViewID: view})
	if !r.OK || r.Operation == nil || r.Operation.Operation != nil {
		t.Fatal("empty Pending was not a valid typed result", r)
	}
	r = b.StableRequest(ctx, pilot.StableRequest{Command: "stable_operation", ViewID: view, OperationID: strings.Repeat("e", 64)})
	if r.OK || r.Error != "stable_unavailable" {
		t.Fatal("null Operation accepted", r)
	}
}
func TestStableBridgeApplyUnknownAndDefiniteBusy(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	for _, mode := range []string{"drop-apply", "malformed-apply", "stale-apply", "wrong-operation", "wrong-view", "busy-apply", "valid"} {
		t.Run(mode, func(t *testing.T) {
			b := stableTestBridge(t, mode)
			ctx := context.Background()
			r := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
			if !r.OK {
				t.Fatal(r)
			}
			operation := strings.Repeat("e", 64)
			r = b.StableRequest(ctx, pilot.StableRequest{Command: "stable_apply_slot", ViewID: r.Snapshot.View.ViewID, OperationID: operation})
			switch mode {
			case "valid":
				if !r.OK || r.Job == nil || r.Job.OperationID != operation {
					t.Fatal(r)
				}
			case "busy-apply":
				if r.OK || r.Error != "busy" {
					t.Fatal("definite queued refusal changed", r)
				}
			default:
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal("potential edit became a definite rejection", r)
				}
			}
		})
	}
}
func TestStableBridgeMixedFamilyAndPreflight(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	b := stableTestBridge(t, "mixed-family")
	r := b.StableRequest(context.Background(), pilot.StableRequest{Command: "stable_snapshot"})
	if r.OK || r.Error != "stable_unavailable" {
		t.Fatal("old response accepted for new command", r)
	}
	// No pipe is touched before either of these local preflight rejections.
	local := &StableBridge{gate: make(chan struct{}, 1), failed: make(chan struct{})}
	r = local.StableRequest(context.Background(), pilot.StableRequest{Command: "stable_inventory", ViewID: strings.Repeat("d", 64), Limit: 0})
	if r.Error != "invalid_request" {
		t.Fatal(r)
	}
	r = local.StableRequest(context.Background(), pilot.StableRequest{Command: "stable_apply_slot", ViewID: strings.Repeat("d", 64), OperationID: strings.Repeat("e", 64)})
	if r.Error != "stale_view" {
		t.Fatal("undispatched stale view should be definite", r)
	}
	local.gate <- struct{}{}
	r = local.StableRequest(context.Background(), pilot.StableRequest{Command: "stable_snapshot"})
	if r.Error != "busy" {
		t.Fatal(r)
	}
}
func TestStableBridgeHistoricalCurrentWrapper(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	s := stableBridgeSnapshot(t)
	b := &StableBridge{view: s.View, storeID: s.Status.StoreID}
	opID := strings.Repeat("e", 64)
	old := &gymstable.Operation{Version: 1, State: "prepared", Intent: gymstable.Intent{Version: 1, OperationID: opID, Generation: 1, ViewID: strings.Repeat("1", 64)}}
	result := pilot.StableResponse{ID: 1, OK: true, Operation: &pilot.StableOperation{Version: 1, View: *s.View, StoreID: s.Status.StoreID, Operation: old}}
	req := pilot.StableRequest{Command: "stable_operation", ViewID: s.View.ViewID, OperationID: opID}
	if !b.correlateStable(req, result) {
		t.Fatal("historical author was confused with current wrapper")
	}
	result.Operation.View.ViewID = old.Intent.ViewID
	if b.correlateStable(req, result) {
		t.Fatal("old wrapper accepted")
	}
}

func TestStableBridgeAbandonUnknownAndAuthCacheRevocation(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate profile")
	}
	for _, command := range []string{"stable_apply_slot", "stable_abandon_operation"} {
		t.Run(command, func(t *testing.T) {
			b := stableTestBridge(t, "stale-apply")
			ctx := context.Background()
			s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
			if !s.OK {
				t.Fatal(s)
			}
			req := pilot.StableRequest{Command: command, ViewID: s.Snapshot.View.ViewID, OperationID: strings.Repeat("e", 64)}
			r := b.StableRequest(ctx, req)
			if r.Error != "outcome_unknown" {
				t.Fatal(r)
			}
			if b.view != nil || b.storeID != "" {
				t.Fatal("expired current view retained")
			}
			r = b.StableRequest(ctx, req)
			if r.Error != "stale_view" {
				t.Fatal("revoked view reached transport again", r)
			}
		})
	}
}

func TestStableRecentCorrelationUsesExecutedCheckpoint(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate profile")
	}
	s := stableBridgeSnapshot(t)
	b := &StableBridge{view: s.View, storeID: s.Status.StoreID}
	page := &gymstable.Page{Status: *s.Status, After: 4, NextAfter: 12}
	page.Status.Checkpoint = 12
	j := &pilot.StableJob{Kind: "inventory", View: *s.View, StoreID: s.Status.StoreID, Page: page}
	req := pilot.StableRequest{Command: "stable_recent", ViewID: s.View.ViewID}
	if !b.correlateStable(req, pilot.StableResponse{Job: j}) {
		t.Fatal("new current checkpoint refused")
	}
	page.After = 3
	if b.correlateStable(req, pilot.StableResponse{Job: j}) {
		t.Fatal("Recent prefix accepted")
	}
	page.Status.Checkpoint = 3
	page.After = 0
	page.NextAfter = 3
	if !b.correlateStable(req, pilot.StableResponse{Job: j}) {
		t.Fatal("initial partial Recent refused")
	}
}

func stableAsyncResponse(s pilot.StableSnapshot, req pilot.StableRequest, mode string, terminal bool) *pilot.StableJob {
	kind := map[string]string{"stable_recent": "inventory", "stable_inventory": "inventory", "stable_roster": "roster", "stable_prepare_slot": "prepare", "stable_apply_slot": "apply", "stable_sync": "sync"}[req.Command]
	j := &pilot.StableJob{Version: 1, JobID: strings.Repeat("f", 64), Kind: kind, State: "queued", View: *s.View, StoreID: s.Status.StoreID}
	if kind == "apply" {
		j.OperationID = req.OperationID
	}
	if !terminal {
		return j
	}
	j.State = "done"
	status := *s.Status
	status.LastAsOfMS = 1
	switch kind {
	case "inventory":
		j.Page = &gymstable.Page{Version: 1, Status: status, AsOfMS: 1, Items: []gymstable.Item{}}
		if mode != "async-valid" {
			j.Page.Status.Checkpoint = 2
			j.Page.Status.ObservedSourceCount = 2
			j.Page.After = 1
			j.Page.NextAfter = 1
			j.Page.HasMore = true
		}
	case "roster":
		j.Roster = &gymstable.RosterPage{Version: 1, Status: status, AsOfMS: 1, FirstSlot: req.FirstSlot, NextSlot: req.FirstSlot + req.Limit, Items: []gymstable.RosterItem{}}
		if j.Roster.NextSlot > 10 {
			j.Roster.NextSlot = 0
		}
		if mode != "async-valid" {
			j.Roster.NextSlot = 3
		}
	case "prepare":
		j.Intent = &gymstable.Intent{Version: 1, Sequence: 1, OperationID: strings.Repeat("e", 64), Generation: s.View.Generation, ViewID: s.View.ViewID, Request: gymstable.SlotRequest{ExpectedRevision: req.ExpectedRevision, Slot: req.Slot, ProspectID: req.ProspectID}, PreparedAtMS: 1}
		if mode != "async-valid" {
			j.Intent.Request.Slot = 2
		}
	case "apply":
		j.State = "failed"
		j.Error = "stable_conflict"
		if mode != "async-valid" {
			j.OperationID = strings.Repeat("a", 64)
		}
	case "sync":
		j.Sync = &gymstable.SyncResult{Version: 1, Status: status, AsOfMS: 1, Items: []gymstable.Item{}}
	}
	return j
}

func TestStableAsyncTerminalRequestCorrelation(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate profile")
	}
	for _, command := range []string{"stable_recent", "stable_inventory", "stable_roster", "stable_prepare_slot", "stable_apply_slot"} {
		for _, poll := range []string{"stable_job", "stable_cancel"} {
			t.Run(command+"/"+poll, func(t *testing.T) {
				b := stableTestBridge(t, "async-wrong")
				ctx := context.Background()
				snapshot := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
				if !snapshot.OK {
					t.Fatal(snapshot)
				}
				req := pilot.StableRequest{Command: command, ViewID: snapshot.Snapshot.View.ViewID}
				switch command {
				case "stable_inventory":
					req.Limit = 1
				case "stable_roster":
					req.FirstSlot = 1
					req.Limit = 1
				case "stable_prepare_slot":
					req.Slot = 1
				case "stable_apply_slot":
					req.OperationID = strings.Repeat("e", 64)
				}
				ack := b.StableRequest(ctx, req)
				if !ack.OK || ack.Job == nil || ack.Job.State != "queued" {
					t.Fatal(ack)
				}
				result := b.StableRequest(ctx, pilot.StableRequest{Command: poll, ViewID: req.ViewID, JobID: ack.Job.JobID})
				want := "stable_unavailable"
				if stableMutation(command) {
					want = "outcome_unknown"
				}
				if result.OK || result.Error != want {
					t.Fatal("async tuple bypass", result)
				}
			})
		}
	}
	for _, mode := range []string{"async-valid", "async-malformed"} {
		t.Run(mode, func(t *testing.T) {
			b := stableTestBridge(t, mode)
			ctx := context.Background()
			s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
			view := s.Snapshot.View.ViewID
			ack := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_prepare_slot", ViewID: view, Slot: 1})
			if !ack.OK {
				t.Fatal(ack)
			}
			result := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_job", ViewID: view, JobID: ack.Job.JobID})
			if mode == "async-valid" {
				if !result.OK || result.Job.Intent == nil || result.Job.Intent.Request.Slot != 1 {
					t.Fatal(result)
				}
			} else if result.OK || result.Error != "outcome_unknown" {
				t.Fatal("malformed terminal lost uncertainty", result)
			}
		})
	}
}

func TestStableJobMetadataBoundPreservesLiveExpectations(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate profile")
	}
	s := stableBridgeSnapshot(t)
	b := &StableBridge{view: s.View, storeID: s.Status.StoreID, gate: make(chan struct{}, 1), failed: make(chan struct{})}
	req := pilot.StableRequest{Command: "stable_inventory", ViewID: s.View.ViewID, After: 0, Limit: 1}
	job := func(id string, state string) pilot.StableJob {
		return pilot.StableJob{JobID: id, Kind: "inventory", State: state, View: *s.View, StoreID: s.Status.StoreID}
	}
	active := strings.Repeat("a", 64)
	if !b.trackStableJob(req, job(active, "running")) {
		t.Fatal("active")
	}
	for n := 1; n <= 10; n++ {
		id := fmt.Sprintf("%064x", n)
		if !b.trackStableJob(req, job(id, "queued")) {
			t.Fatal("queue", n)
		}
		if !b.trackStableJob(pilot.StableRequest{Command: "stable_cancel", JobID: id}, job(id, "cancelled")) {
			t.Fatal("cancel", n)
		}
		if _, ok := b.jobs[active]; !ok || len(b.jobs) > 2 {
			t.Fatal("cancelled replacements evicted active correlation", len(b.jobs))
		}
	}
	// Three unclassified live ACKs may reflect an unobserved completion. Do not
	// guess which one is old or dispatch a fourth action without a snapshot.
	b.jobs = map[string]stableJobExpectation{}
	for _, id := range []string{active, strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		if !b.trackStableJob(req, job(id, "queued")) {
			t.Fatal("fill")
		}
	}
	r := b.StableRequest(context.Background(), req)
	if r.Error != "busy" {
		t.Fatal("capacity touched uninitialized pipe", r)
	}
	s.ActiveJobID = strings.Repeat("b", 64)
	s.QueuedJobID = strings.Repeat("c", 64)
	b.rememberStableSnapshot(s)
	if len(b.jobs) != 2 || !b.canTrackStableJob() {
		t.Fatal("authoritative snapshot did not reconcile")
	}
	if _, ok := b.jobs[active]; ok {
		t.Fatal("evicted stale result retained")
	}
	// An automatically discovered result may be Sync, never an untracked edit.
	auto := strings.Repeat("d", 64)
	s.LastJobID = auto
	b.rememberStableSnapshot(s)
	poll := pilot.StableRequest{Command: "stable_job", JobID: auto, ViewID: s.View.ViewID}
	j := job(auto, "running")
	j.Kind = "sync"
	if !b.correlateStable(poll, pilot.StableResponse{Job: &j}) {
		t.Fatal("automatic sync refused")
	}
	j.Kind = "prepare"
	if b.correlateStable(poll, pilot.StableResponse{Job: &j}) {
		t.Fatal("unknown mutation accepted")
	}
	j.Kind = "sync"
	j.JobID = strings.Repeat("e", 64)
	poll.JobID = j.JobID
	if b.correlateStable(poll, pilot.StableResponse{Job: &j}) {
		t.Fatal("undiscovered sync accepted")
	}
	b.clearStableView()
	if b.jobs != nil || b.snapshotJobs != nil {
		t.Fatal("old identity metadata retained")
	}
}
