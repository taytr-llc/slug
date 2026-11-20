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
)

// Closed private-pipe fixtures use independent public/model data. They do not
// stand in for a real account lease, one-proof capture or concurrency evidence.
func TestCareerHistoryBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_HISTORY_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	p, j := historyPageFixture(t, "capacity256")
	model := historyModelBridge(t, j)
	pageReads, jobReads := 0, 0
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var q pilot.CareerHistoryRequest
		if json.Unmarshal(scan.Bytes(), &q) != nil {
			os.Exit(2)
		}
		if q.Command == "stop" {
			raw, _ := json.Marshal(pilot.ManifestResponse{ID: q.ID, OK: false, Error: "account_unavailable"})
			fmt.Println(string(raw))
			continue
		}
		r := pilot.CareerHistoryResponse{ID: q.ID, Version: 1, OK: true}
		switch q.Command {
		case "career_history_snapshot":
			r.Snapshot = model.history.snapshot
			if mode == "mixed" {
				fmt.Printf("{\"id\":%d,\"ok\":false,\"error\":\"busy\"}\n", q.ID)
				continue
			}
		case "career_history_begin":
			if mode == "drop" {
				os.Exit(0)
			}
			if mode == "busy" {
				r = historyFailure(q.ID, "busy")
				break
			}
			copy := j
			copy.State = "queued"
			copy.Capture = nil
			copy.JobID = strings.Repeat("e", 64)
			r.Job = &copy
			if mode == "web-ack" {
				s := model.history.snapshot
				s.State, s.LastJobID, s.QueuedJobID = "busy", "", copy.JobID
			}
		case "career_history_job", "career_history_cancel":
			jobReads++
			copy := j
			copy.JobID = q.JobID
			r.Job = &copy
			if mode == "regress" && jobReads > 1 {
				copy.State = "running"
				copy.Capture = nil
			}
			if mode == "fatal" {
				copy.State = "failed"
				copy.Error = "history_corrupt"
				copy.Capture = nil
			}
		case "career_history_page":
			pageReads++
			if mode == "revoke" && pageReads > 1 {
				r = historyFailure(q.ID, "stale_view")
				break
			}
			copy := p
			copy.JobID = q.JobID
			copy.Page.Items = append(copy.Page.Items[:0:0], p.Page.Items...)
			if mode == "change-row" && pageReads > 1 {
				copy.Page.Items[0].Event.ResultDigest = strings.Repeat("f", 64)
			}
			r.Page = &copy
		default:
			os.Exit(3)
		}
		raw, _ := json.Marshal(r)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}

func TestCareerHistoryLostWebACKRecoversAdmittedJob(t *testing.T) {
	b, j := historyTestBridge(t, "web-ack")
	historyReadFirst(t, b, j)
	ctx := context.Background()
	// Native received a valid Begin ACK, but the web caller never receives it.
	// Unlike a failed helper pipe, this original channel remains usable.
	begin := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_begin", ViewID: j.View.ViewID, ProspectID: j.ProspectID})
	if !begin.OK || begin.Job == nil {
		t.Fatal(begin)
	}
	id := begin.Job.JobID
	begin = pilot.CareerHistoryResponse{}
	s := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
	if !s.OK || s.Snapshot == nil || s.Snapshot.QueuedJobID != id {
		t.Fatal(s)
	}
	r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: j.View.ViewID, JobID: id})
	if !r.OK || r.Job == nil || r.Job.State != "done" || b.history.selected != id {
		t.Fatal(r)
	}
	// Exactly snapshot/job recovery: no second Begin is issued.
}
func historyTestBridge(t *testing.T, mode string) (*StableBridge, pilot.CareerHistoryJob) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerHistoryBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_HISTORY_BRIDGE_PROCESS="+mode)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	_, j := historyPageFixture(t, "capacity256")
	model := historyModelBridge(t, j)
	b.careersEnabled = true
	b.view = model.view
	b.manifestView = model.manifestView
	b.storeID = model.storeID
	b.stablePin = model.stablePin
	b.resultPin = model.resultPin
	b.result = model.result
	t.Cleanup(func() {
		if e := b.Close(); e != nil {
			t.Error("owned pipe did not drain", e)
		}
	})
	return b, j
}
func historyReadFirst(t *testing.T, b *StableBridge, j pilot.CareerHistoryJob) {
	t.Helper()
	ctx := context.Background()
	if r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_snapshot"}); !r.OK {
		t.Fatal(r)
	}
	if r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: j.View.ViewID, JobID: j.JobID}); !r.OK || r.Job == nil {
		t.Fatal(r)
	}
}
func TestCareerHistoryPipeFreshPagesAndTerminal(t *testing.T) {
	for _, mode := range []string{"valid", "revoke", "change-row", "regress"} {
		t.Run(mode, func(t *testing.T) {
			b, j := historyTestBridge(t, mode)
			historyReadFirst(t, b, j)
			ctx := context.Background()
			q := pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: j.View.ViewID, JobID: j.JobID, After: 0, Limit: 8}
			if r := b.CareerHistoryRequest(ctx, q); !r.OK || r.Page == nil || len(r.Page.Page.Items) != 8 {
				t.Fatal(r)
			}
			if mode == "regress" {
				q = pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: j.View.ViewID, JobID: j.JobID}
			}
			r := b.CareerHistoryRequest(ctx, q)
			if mode == "valid" {
				if !r.OK || r.Page == nil {
					t.Fatal(r)
				}
				return
			}
			want := "outcome_unknown"
			if mode == "revoke" {
				want = "stale_view"
			}
			if r.OK || r.Page != nil || r.Job != nil || r.Error != want {
				t.Fatal("changed/revoked reply disclosed payload", r)
			}
		})
	}
}
func TestCareerHistoryPipeDefiniteBusyLostACKAndDetached(t *testing.T) {
	for _, mode := range []string{"busy", "drop", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			b, j := historyTestBridge(t, mode)
			ctx := context.Background()
			if mode == "mixed" {
				r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal(r)
				}
				return
			}
			historyReadFirst(t, b, j)
			r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_begin", ViewID: j.View.ViewID, ProspectID: j.ProspectID})
			want := "outcome_unknown"
			if mode == "busy" {
				want = "busy"
				if b.history == nil || b.history.selected != j.JobID {
					t.Fatal("busy changed selection")
				}
			}
			if r.OK || r.Job != nil || r.Error != want {
				t.Fatal(r)
			}
			if mode == "busy" {
				b.clearStableView()
				r = b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: j.View.ViewID, JobID: j.JobID})
				if r.Error != "stale_view" || r.Job != nil {
					t.Fatal("detached old session", r)
				}
			}
		})
	}
}
func TestCareerHistoryPipeStopAndFatalInvalidation(t *testing.T) {
	b, j := historyTestBridge(t, "valid")
	historyReadFirst(t, b, j)
	_, _ = b.Request(context.Background(), "stop", 0)
	if b.history != nil {
		t.Fatal("Stop retained owner-bound capture")
	}
	r := b.CareerHistoryRequest(context.Background(), pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: j.View.ViewID, JobID: j.JobID, Limit: 8})
	if r.OK || r.Page != nil || r.Error != "history_not_found" {
		t.Fatal(r)
	}
	f, k := historyTestBridge(t, "fatal")
	historyReadFirst(t, f, k)
	if !f.historyFatal || f.history != nil {
		t.Fatal("failed job did not latch owner failure")
	}
	r = f.CareerHistoryRequest(context.Background(), pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
	if r.OK || r.Error != "history_unavailable" {
		t.Fatal("snapshot repaired failed owner", r)
	}
}
