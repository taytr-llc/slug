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
	"time"
)

func (r boutReference) job(t *testing.T, state string) pilot.BoutJob {
	t.Helper()
	for _, d := range r.DTOs {
		if d.Kind == "BoutJob" && d.Name == state+"-0" {
			var j pilot.BoutJob
			if json.Unmarshal(d.Value, &j) != nil {
				t.Fatal("job")
			}
			return j
		}
	}
	t.Fatal("missing job", state)
	return pilot.BoutJob{}
}

// Only the separately frozen reference values cross this real child pipe.
// This is transport coverage, not journal or owned-chain execution.
func TestBoutBridgeReferencePipeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_BOUT_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	ref := boutRef(t)
	queued, completed := ref.job(t, "queued"), ref.job(t, "completed")
	scan := bufio.NewScanner(os.Stdin)
	count := 0
	for scan.Scan() {
		var q pilot.BoutRequest
		if json.Unmarshal(scan.Bytes(), &q) != nil {
			os.Exit(2)
		}
		if mode == "drop" || mode == "drop-read" {
			os.Exit(0)
		}
		if q.Command == "bout_status" && strings.HasPrefix(mode, "portrait-") {
			count++
			status := completed.Result.Status
			status.ObservedAtMS += int64(count)
			status.Source.ObservedAtMS = status.ObservedAtMS
			out := pilot.BoutResponse{ID: q.ID, OK: true, View: &completed.View, StoreID: completed.StoreID, Status: &status}
			if count > 1 {
				switch mode {
				case "portrait-conflict":
					status.Source.ConflictCount = 1
					status.Source.Usable = false
					status.Saved.Usable = false
					status.Source.CurrentFinality = false
					status.Source.EscalationDue = status.ObservedAtMS >= status.Source.Event.IncidentDeadlineMS
				case "portrait-changed":
					status.Saved.RecordDigest = strings.Repeat("e", 64)
				case "portrait-unauthorized":
					out = pilot.BoutResponse{ID: q.ID, Error: "unauthorized"}
				case "portrait-clock":
					out = pilot.BoutResponse{ID: q.ID, Error: "bout_clock"}
				}
			}
			raw, e := pilot.MarshalBoutResponse(out)
			if e != nil {
				os.Exit(4)
			}
			fmt.Println(string(raw))
			continue
		}
		j := queued
		out := pilot.BoutResponse{ID: q.ID, OK: true, View: &j.View, StoreID: j.StoreID, Job: &j}
		if mode == "wrong-error-view" {
			wrong := j.View
			wrong.Generation++
			out = pilot.BoutResponse{ID: q.ID, Error: "busy", View: &wrong, StoreID: j.StoreID}
		} else if mode == "busy" {
			out = pilot.BoutResponse{ID: q.ID, Error: "busy"}
		} else if mode == "refusal" {
			out = pilot.BoutResponse{ID: q.ID, Error: "bout_source_incomplete"}
		} else if q.Command == "bout_job" {
			count++
			j = completed
			if mode == "wrong-view" {
				j.View.Generation++
			}
			if mode == "changed-terminal" && count > 1 {
				j.Result.Status.ObservedAtMS++
			}
			if mode == "backwards" {
				j = queued
				if count == 1 {
					j.State = "running"
				}
			}
			out.View = &j.View
		} else if mode == "bad-ack" {
			j = completed
		}
		if mode == "wrong-family" && q.Command == "bout_job" {
			fmt.Printf("{\"id\":%d,\"ok\":false,\"error\":\"seed_unavailable\"}\n", q.ID)
			continue
		}
		raw, e := pilot.MarshalBoutResponse(out)
		if e != nil {
			os.Exit(3)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func boutPipeBridge(t *testing.T, mode string) (*StableBridge, pilot.BoutJob) {
	t.Helper()
	ref := boutRef(t)
	j := ref.job(t, "queued")
	var s pilot.BoutSnapshot
	for _, d := range ref.DTOs {
		if d.Kind == "BoutSnapshot" && d.Name == "ready-0" {
			if json.Unmarshal(d.Value, &s) != nil {
				t.Fatal("snapshot")
			}
		}
	}
	if s.Pin == nil {
		t.Fatal("reference snapshot")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoutBridgeReferencePipeProcess$")
	cmd.Env = append(os.Environ(), "SLUG_BOUT_BRIDGE_PROCESS="+mode)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = b.Close() })
	v := j.View
	b.mu.Lock()
	b.boutsEnabled = true
	b.view = &v
	b.boutPin = &boutAuthority{pin: *s.Pin, storeID: s.StoreID, createdMS: s.CreatedAtMS, sourceCreatedMS: s.SourceCreatedAtMS}
	b.bout = &boutCorrelation{view: v, jobs: map[string]boutJobExpectation{}, snapshotJobs: map[string]bool{}}
	b.mu.Unlock()
	return b, j
}
func TestBoutBridgePipeCorrelationAndUncertainty(t *testing.T) {
	for _, mode := range []string{"good", "drop", "wrong-error-view", "busy", "refusal", "bad-ack", "wrong-view", "wrong-family", "changed-terminal", "backwards"} {
		t.Run(mode, func(t *testing.T) {
			b, q := boutPipeBridge(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r := b.BoutRequest(ctx, pilot.BoutRequest{Command: "bout_run", ViewID: q.View.ViewID, Saturday: q.Saturday, BoutID: q.BoutID})
			switch mode {
			case "drop", "bad-ack", "wrong-error-view":
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal(r.Error)
				}
				return
			case "busy", "refusal":
				want := "busy"
				if mode == "refusal" {
					want = "bout_source_incomplete"
				}
				if r.OK || r.Error != want || len(b.bout.jobs) != 0 {
					t.Fatal("definite refusal", r.Error)
				}
				return
			}
			if !r.OK || r.Job.State != "queued" {
				t.Fatal("ACK", r.Error)
			}
			req := pilot.BoutRequest{Command: "bout_job", ViewID: q.View.ViewID, JobID: r.Job.JobID}
			r = b.BoutRequest(ctx, req)
			if mode == "wrong-view" || mode == "wrong-family" {
				if r.OK || r.Error != "bout_unavailable" {
					t.Fatal("known job failure", r.Error)
				}
				return
			}
			if !r.OK {
				t.Fatal("job", r.Error)
			}
			again := b.BoutRequest(ctx, req)
			if mode == "changed-terminal" || mode == "backwards" {
				if again.OK || again.Error != "bout_unavailable" {
					t.Fatal("changed known job accepted", again.Error)
				}
			} else if !again.OK || boutJobDigest(*again.Job) != boutJobDigest(*r.Job) {
				t.Fatal("terminal changed")
			}
		})
	}
	for _, command := range []string{"bout_status", "bout_record", "bout_card_status"} {
		t.Run(command+"-lost-observation", func(t *testing.T) {
			b, j := boutPipeBridge(t, "drop-read")
			q := pilot.BoutRequest{Command: command, ViewID: j.View.ViewID, Saturday: j.Saturday, BoutID: j.BoutID}
			if command == "bout_card_status" {
				q.BoutID = ""
			}
			r := b.BoutRequest(t.Context(), q)
			if r.OK || r.Error != "outcome_unknown" {
				t.Fatal("observation uncertainty", r.Error)
			}
		})
	}
}
func TestBoutNativeCancellationAfterSuccessfulPrefetch(t *testing.T) {
	b, j := boutPipeBridge(t, "good")
	b.bout.snapshotJobs[j.JobID] = true
	recorded := &entryRecordingInput{WriteCloser: b.input}
	b.input = recorded
	base, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, e := b.begin(base)
	if e != nil {
		t.Fatal(e)
	}
	ctx := &entryCancelAfterPrefetchContext{Context: base, cancel: cancel, bridge: b, afterID: id}
	r := b.boutRequestHeld(ctx, id, pilot.BoutRequest{Command: "bout_cancel", ViewID: j.View.ViewID, JobID: j.JobID})
	<-b.gate
	if r.OK || r.Error != "bout_cancelled" || base.Err() != context.Canceled {
		t.Fatal(r.Error, base.Err())
	}
	if len(recorded.commands) != 1 || recorded.commands[0] != "bout_job" {
		t.Fatal("dispatched after cancellation", recorded.commands)
	}
}

// These are real private pipes carrying frozen public DTOs. They isolate native
// disclosure/correlation; the separate root-owned test composes actual journals.
func TestBoutPortraitFreshStatusOnEveryDisclosure(t *testing.T) {
	for _, mode := range []string{"portrait-good", "portrait-conflict", "portrait-changed", "portrait-unauthorized", "portrait-clock"} {
		t.Run(mode, func(t *testing.T) {
			b, j := boutPipeBridge(t, mode)
			completed := boutRef(t).job(t, "completed")
			if !b.selectBoutRecord(j.View, completed.Result.Record) {
				t.Fatal("select")
			}
			recorded := &entryRecordingInput{WriteCloser: b.input}
			b.input = recorded
			prospect := completed.Result.Record.Source.Corners[0].Member.Assignment.ProspectID
			one := b.BoutPortrait(t.Context(), j.View.ViewID, j.Saturday, j.BoutID, prospect)
			if !one.OK || one.Portrait == nil || one.Status == nil || one.Portrait.SVG == "" {
				t.Fatal("first portrait", one.Error)
			}
			if b.boutPresentation == nil || b.boutPresentation.svgs[0] == "" {
				t.Fatal("image not retained")
			}
			two := b.BoutPortrait(t.Context(), j.View.ViewID, j.Saturday, j.BoutID, prospect)
			if len(recorded.commands) != 2 || recorded.commands[0] != "bout_status" || recorded.commands[1] != "bout_status" {
				t.Fatal("cache bypassed fresh status", recorded.commands)
			}
			switch mode {
			case "portrait-good":
				if !two.OK || two.Portrait.SVG != one.Portrait.SVG || two.Status.ObservedAtMS <= one.Status.ObservedAtMS {
					t.Fatal("cache not freshly authorized", two.Error)
				}
			case "portrait-conflict":
				if !two.OK || two.Status.Source.ConflictCount != 1 || two.Status.Saved.Usable || two.Portrait.SVG != one.Portrait.SVG {
					t.Fatal("historical conflict hidden", two.Error)
				}
			default:
				if two.OK || two.Portrait != nil || two.Status != nil || b.boutPresentation != nil {
					t.Fatal("failure disclosed or retained tuple", two.Error)
				}
			}
		})
	}
}
func TestBoutPortraitSelectionAndViewInvalidation(t *testing.T) {
	b, j := boutPipeBridge(t, "portrait-good")
	done := boutRef(t).job(t, "completed")
	if !b.selectBoutRecord(j.View, done.Result.Record) {
		t.Fatal("select")
	}
	wrong := strings.Repeat("f", 64)
	if r := b.BoutPortrait(t.Context(), j.View.ViewID, j.Saturday, wrong, done.Result.Record.Source.Corners[0].Member.Assignment.ProspectID); r.OK || r.Portrait != nil || r.Status != nil || r.Error != "bout_not_found" {
		t.Fatal("selection bypass", r.Error)
	}
	b.clearStableView()
	if b.boutPresentation != nil || b.bout != nil {
		t.Fatal("view loss retained portrait authority")
	}
	r := b.BoutPortrait(t.Context(), j.View.ViewID, j.Saturday, j.BoutID, done.Result.Record.Source.Corners[0].Member.Assignment.ProspectID)
	if r.OK || r.Portrait != nil || r.Status != nil {
		t.Fatal("view loss disclosed")
	}
}

func TestBoutLateJobPreservesNewerExplicitRecordSelection(t *testing.T) {
	ref := boutRef(t)
	first, next := ref.Compositions[1], ref.Compositions[2]
	if first.Pin.SourceStoreID != next.Pin.SourceStoreID {
		t.Fatal("modeled card fixture mismatch")
	}
	v := first.Views[0]
	a := boutAuthority{pin: first.Pin, storeID: first.Record.StoreID, createdMS: first.Created, sourceCreatedMS: first.SourceCreated}
	b := &StableBridge{view: &v, boutPin: &a, bout: &boutCorrelation{view: v, jobs: map[string]boutJobExpectation{}}}
	if !b.validBoutRecord(a, first.Record) || !b.validBoutRecord(a, next.Record) {
		t.Fatal("independent valid source records")
	}
	if !b.selectBoutRecord(v, first.Record, true) || b.boutPresentation.record.BoutID != first.Record.BoutID {
		t.Fatal("first discovered completion must select")
	}
	if !b.selectBoutRecord(v, next.Record) || b.boutPresentation.record.BoutID != next.Record.BoutID {
		t.Fatal("explicit read must replace")
	}
	selected := b.boutPresentation
	if !b.selectBoutRecord(v, first.Record, true) || b.boutPresentation != selected {
		t.Fatal("late job displaced explicit selection")
	}
	if !b.selectBoutRecord(v, next.Record, true) || b.boutPresentation != selected {
		t.Fatal("same transcript changed")
	}
	b.clearBoutPresentation()
	if !b.selectBoutRecord(v, first.Record, true) || b.boutPresentation == nil {
		t.Fatal("new run completion cannot establish tuple")
	}
}
