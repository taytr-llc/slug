//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymresult"
)

// Exact original IDs from the frozen winner/loser-assignment.json inputs.
const historyActualWinner = "067121d1f9be9eee0eab884de24e24df996e3b15d4385f5123930c6405d71fa3"
const historyActualLoser = "22a2c799c122ac002a3ca277e0f7f84dd89c8b5f1fe1e1a18e098b1aad27e717"

func historyActualOwnedRoot(root string) error {
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		return fmt.Errorf("canonical history root required")
	}
	raw, e := os.ReadFile(filepath.Join(root, "SET51-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(raw, &m) != nil || m.Version != 1 || m.Scope != "Set51 owned cold copy" || m.SourceRoot == root || !filepath.IsAbs(m.SourceRoot) {
		return fmt.Errorf("Set51 copy declaration required")
	}
	return careerActualOwnedRoot(root) // The original Set49 declaration is retained.
}
func TestCareerHistoryActualNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_CAREER_HISTORY_ACTUAL_CHILD")
	if root == "" {
		return
	}
	fail := func(e error) {
		_ = os.WriteFile(filepath.Join(root, "career-history-child-error.txt"), []byte(e.Error()+"\n"), 0600)
		os.Exit(2)
	}
	if e := historyActualOwnedRoot(root); e != nil {
		fail(e)
	}
	var p boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &p); e != nil || !p.NormalPoW || !p.TaggedClock || p.Saturday != "2026-12-05" || p.ClockMS != 1796500801000 {
		fail(fmt.Errorf("unchanged original clock declaration: %v", e))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	c, e := accountpilot.NewCareersForTestClock(ctx, pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(p.ClockMS + 1) })
	if e != nil {
		fail(e)
	}
	e = pilot.ServeCareers(ctx, c, os.Stdin, os.Stdout)
	closed := c.Close()
	if e != nil && e != context.Canceled {
		fail(e)
	}
	if closed != nil {
		fail(closed)
	}
	os.Exit(0)
}

type historyActualNative struct{ *resultActualNative }

func (h *historyActualNative) open() {
	h.t.Helper()
	if e := historyActualOwnedRoot(h.root); e != nil {
		h.t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerHistoryActualNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_CAREER_HISTORY_ACTUAL_CHILD="+h.root)
	var e error
	h.b, e = startStable(cmd)
	if e != nil {
		h.t.Fatal(e)
	}
	h.b.entriesEnabled, h.b.eventsEnabled, h.b.seedsEnabled, h.b.cardsEnabled, h.b.boutsEnabled, h.b.resultsEnabled, h.b.careersEnabled = true, true, true, true, true, true, true
	h.processes = newManifestProcessInventory(filepath.Join(h.root, "data"), h.node, cmd.Process.Pid)
}
func (h *historyActualNative) history(q pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	h.t.Helper()
	var r pilot.CareerHistoryResponse
	// Functional readiness only: retry definite busy, never an unknown action.
	seedNativeClockWait(h.t, h.ctx, func() bool { r = h.b.CareerHistoryRequest(h.ctx, q); return r.Error != "busy" })
	return r
}
func (h *historyActualNative) capture(pid string) (pilot.CareerHistoryResponse, pilot.CareerHistoryResponse, pilot.CareerHistoryResponse, int64) {
	h.t.Helper()
	view := *h.view.View
	s := h.history(pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
	if !s.OK || s.Snapshot == nil || s.Snapshot.View == nil || *s.Snapshot.View != view || s.Snapshot.Activation == nil || s.Snapshot.State == "unavailable" {
		h.t.Fatal("history discovery", s.Error)
	}
	started := time.Now()
	ack := h.history(pilot.CareerHistoryRequest{Command: "career_history_begin", ViewID: view.ViewID, ProspectID: pid})
	if !ack.OK || ack.Job == nil || ack.Job.State != "queued" {
		h.t.Fatal("explicit history ACK", ack.Error)
	}
	var terminal pilot.CareerHistoryResponse
	seedNativeClockWait(h.t, h.ctx, func() bool {
		terminal = h.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: view.ViewID, JobID: ack.Job.JobID})
		if !terminal.OK || terminal.Job == nil {
			h.t.Fatal("history poll", terminal.Error)
		}
		return terminal.Job.State != "queued" && terminal.Job.State != "running"
	})
	if terminal.Job.State != "done" || terminal.Job.Capture == nil {
		h.t.Fatal("capture failed", terminal.Job.State, terminal.Job.Error)
	}
	return s, ack, terminal, time.Since(started).Milliseconds()
}
func (h *historyActualNative) page(job string, after uint32) pilot.CareerHistoryResponse {
	h.t.Helper()
	r := h.history(pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: h.view.View.ViewID, JobID: job, After: after, Limit: 8})
	if !r.OK || r.Page == nil {
		h.t.Fatal("history page", r.Error)
	}
	return r
}

func TestCareerHistoryActualFinalized(t *testing.T) {
	root, node := os.Getenv("SLUG_CAREER_HISTORY_ACTUAL_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit Set51 owned cold copy of finalized Set49 evidence")
	}
	if e := historyActualOwnedRoot(root); e != nil {
		t.Fatal(e)
	}
	var prepared boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &prepared); e != nil || prepared.Build.Record.Plan.BoutCount != 1 || prepared.Saturday != "2026-12-05" {
		t.Fatal("preserved source", e)
	}
	booked := prepared.Build.Record.Plan.Bouts[0]
	if !publicHex.MatchString(historyActualWinner) || !publicHex.MatchString(historyActualLoser) || historyActualWinner == historyActualLoser || !((booked.CornerA == historyActualWinner && booked.CornerB == historyActualLoser) || (booked.CornerB == historyActualWinner && booked.CornerA == historyActualLoser)) {
		t.Fatal("frozen original participant IDs do not match the retained booking")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	h := &historyActualNative{&resultActualNative{t: t, ctx: ctx, root: root, node: node, date: prepared.Saturday, bid: prepared.Build.Record.Plan.Bouts[0].BoutID, baseline: map[string]pilot.ManifestSnapshot{}}}
	defer h.close()
	h.open()
	rows := []map[string]any{}
	var original gymresult.Record
	var oldView, oldJob string
	for n, fixture := range []string{"zero256", "incrementing256", "zero256"} {
		if n == 2 {
			h.close()
			h.open()
		}
		h.login(fixture)
		view := *h.view.View
		before := h.read()
		if before.Record.Finality == nil || before.Record.Checkpoint == nil || before.Status.Phase != "conflict" {
			t.Fatal("retained own first Finality and later conflict required")
		}
		if n == 0 {
			original = before.Record
		} else if !reflect.DeepEqual(original, before.Record) {
			t.Fatal("login/restart changed permanent result")
		}
		var oldRefusal *pilot.CareerHistoryResponse
		if oldView != "" {
			r := h.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: oldView, JobID: oldJob})
			if r.OK || r.Job != nil || r.Error != "stale_view" {
				t.Fatal("old View", r.Error)
			}
			oldRefusal = &r
		}
		// These two immutable original participants and totals are pinned in the
		// independent Set49/Set51 public evidence, not inferred from a new result.
		pid := historyActualWinner
		want := fightercareer.Totals{Wins: 1, XP: 1, BasePoints: 14}
		if fixture == "incrementing256" {
			pid = historyActualLoser
			want = fightercareer.Totals{Losses: 1, XP: 1}
		}
		snap, ack, terminal, elapsed := h.capture(pid)
		j := terminal.Job
		c := j.Capture
		if c.AsOfMS != 1796500801001 || c.EventCount != 1 || c.PrefixCount != 1 || c.Totals != want || c.Assignment.ProspectID != pid {
			t.Fatal("effective captured history")
		}
		first := h.page(j.JobID, 0)
		repeat := h.page(j.JobID, 0)
		if len(first.Page.Page.Items) != 1 || first.Page.Page.HasMore || first.Page.Page.Next != 1 || !reflect.DeepEqual(first.Page, repeat.Page) {
			t.Fatal("fixed first/repeat page")
		}
		row := first.Page.Page.Items[0]
		if row.Ordinal != 1 || row.XPBefore != 0 || row.XPApplied != 1 || row.Earned.XP != 1 || row.TotalsAfter != want || row.Event.EffectiveAtMS != before.Record.Finality.ObservedAtMS {
			t.Fatal("effective contribution")
		}
		var linked pilot.ResultResponse
		seedNativeClockWait(t, ctx, func() bool {
			linked = h.b.CareerHistoryResult(ctx, view.ViewID, j.JobID, row.Ordinal)
			return linked.Error != "busy"
		})
		if !linked.OK || linked.Record == nil || linked.Record.Status.Phase != "conflict" || !reflect.DeepEqual(linked.Record.Record, before.Record) {
			t.Fatal("explicit retained Result link", linked.Error)
		}
		end := h.page(j.JobID, 1)
		if end.Page.Page.Items == nil || len(end.Page.Page.Items) != 0 || end.Page.Page.Next != 1 || end.Page.Page.HasMore {
			t.Fatal("empty captured end")
		}
		if _, e := h.b.Request(ctx, "stop", 0); e != nil {
			t.Fatal("Stop", e)
		}
		stoppedJob := h.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: view.ViewID, JobID: j.JobID})
		stoppedPage := h.history(pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: view.ViewID, JobID: j.JobID, Limit: 8})
		for _, r := range []pilot.CareerHistoryResponse{stoppedJob, stoppedPage} {
			if r.OK || r.Job != nil || r.Page != nil || r.Error != "history_not_found" {
				t.Fatal("Stop must invalidate exact concrete owner", r.Error)
			}
		}
		freshSnap, freshACK, freshJob, freshElapsed := h.capture(pid)
		freshPage := h.page(freshJob.Job.JobID, 0)
		if freshJob.Job.JobID == j.JobID || freshJob.Job.Capture.Activation != c.Activation || freshJob.Job.Capture.CareerDigest != c.CareerDigest || freshJob.Job.Capture.Totals != want || !reflect.DeepEqual(freshPage.Page.Page.Items, first.Page.Page.Items) {
			t.Fatal("fresh capture after Stop")
		}
		after := h.read()
		if !reflect.DeepEqual(before.Record, after.Record) {
			t.Fatal("history mutated saved result")
		}
		account := h.account("signed_in")
		h.unchanged(h.baseline[fixture], account)
		h.processes.capture(t, fixture, "history-complete", view.AccountID, view.AuthorityOrigin, false)
		rows = append(rows, map[string]any{"fixture": fixture, "reopened": n == 2, "held": h.view.Account.Held, "view": view, "prospect_id": pid, "result_before": before, "snapshot": snap, "ack": ack, "terminal": terminal, "pages": []pilot.CareerHistoryResponse{first, repeat, end}, "result_link": linked, "stop_job_refusal": stoppedJob, "stop_page_refusal": stoppedPage, "fresh_snapshot": freshSnap, "fresh_ack": freshACK, "fresh_terminal": freshJob, "fresh_page": freshPage, "old_view_refusal": oldRefusal, "result_after": after, "account_after": account, "capture_elapsed_ms": elapsed, "fresh_capture_elapsed_ms": freshElapsed})
		oldView, oldJob = view.ViewID, freshJob.Job.JobID
		if _, e := h.b.Request(ctx, "logout", 0); e != nil {
			t.Fatal(e)
		}
		h.account("signed_out")
	}
	h.close()
	resultActualSave(t, root, "public-career-history-native-evidence.json", map[string]any{"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": true, "clock_ms": int64(1796500801001), "automatic_admissions": 0, "new_awards": 0, "capture_jobs": 6, "result_links": 3, "proof_call_count_measured": false, "rows": rows, "source_observations": h.rows, "baseline": h.baseline, "scope": "Actual original fixture login and private helper pipe; winner, loser and process restart finalized captures, immutable pages, retained conflicted Result links, Stop/new-owner capture and old-View refusal. No mining, result mutation, Check/Finalize/export, node readiness, GUI or production authority. Busy waits are functional readiness, not latency acceptance; proof call counts are measured separately."})
}
