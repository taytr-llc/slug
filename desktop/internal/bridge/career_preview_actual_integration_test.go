//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
// Explicit owned-copy acceptance; no mining or result mutation is performed.
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

func careerPreviewActualOwnedRoot(root string) error {
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		return fmt.Errorf("canonical preview root required")
	}
	raw, e := os.ReadFile(filepath.Join(root, "SET50-OWNED-COPY.json"))
	var m struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	if e != nil || json.Unmarshal(raw, &m) != nil || m.Version != 1 || m.Scope != "Set50 owned cold copy" || m.SourceRoot == root || !filepath.IsAbs(m.SourceRoot) {
		return fmt.Errorf("Set50 copy declaration required")
	}
	return nil
}

func TestCareerPreviewActualNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_CAREER_PREVIEW_ACTUAL_CHILD")
	if root == "" {
		return
	}
	fail := func(e error) {
		_ = os.WriteFile(filepath.Join(root, "career-preview-child-error.txt"), []byte(e.Error()+"\n"), 0600)
		os.Exit(2)
	}
	if e := careerPreviewActualOwnedRoot(root); e != nil {
		fail(e)
	}
	var prepared boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &prepared); e != nil || !prepared.NormalPoW || !prepared.TaggedClock || prepared.Saturday != "2026-12-05" || prepared.ClockMS != 1796500801000 {
		fail(fmt.Errorf("unchanged Set49 clock declaration: %v", e))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	c, e := accountpilot.NewCareersForTestClock(ctx, pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(prepared.ClockMS + 1) })
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

type careerPreviewActualNative struct{ *careerActualNative }

func (h *careerPreviewActualNative) open() {
	h.t.Helper()
	if e := careerPreviewActualOwnedRoot(h.root); e != nil {
		h.t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerPreviewActualNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_CAREER_PREVIEW_ACTUAL_CHILD="+h.root)
	var e error
	h.b, e = startStable(cmd)
	if e != nil {
		h.t.Fatal(e)
	}
	h.b.entriesEnabled, h.b.eventsEnabled, h.b.seedsEnabled, h.b.cardsEnabled, h.b.boutsEnabled, h.b.resultsEnabled, h.b.careersEnabled = true, true, true, true, true, true, true
	h.processes = newManifestProcessInventory(filepath.Join(h.root, "data"), h.node, cmd.Process.Pid)
}

func TestCareerPreviewActualAwardedPractice(t *testing.T) {
	root, node := os.Getenv("SLUG_CAREER_PREVIEW_ACTUAL_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit Set50 owned copy of finalized Set49 evidence")
	}
	if e := careerPreviewActualOwnedRoot(root); e != nil {
		t.Fatal(e)
	}
	var prepared boutNativePublicCard
	if e := boutNativeLoad(root, "public-result-prepared-card.json", &prepared); e != nil || prepared.Build.Record.Plan.BoutCount != 1 || prepared.Saturday != "2026-12-05" {
		t.Fatal("preserved source", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	h := &careerPreviewActualNative{&careerActualNative{resultActualNative: &resultActualNative{t: t, ctx: ctx, root: root, node: node, date: prepared.Saturday, bid: prepared.Build.Record.Plan.Bouts[0].BoutID, baseline: map[string]pilot.ManifestSnapshot{}}}}
	defer h.close()
	h.open()
	rows := []map[string]any{}
	var originalRecord gymresult.Record
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
			t.Fatal("actual retained finality/conflict required")
		}
		if n == 0 {
			originalRecord = before.Record
		} else if !reflect.DeepEqual(before.Record, originalRecord) {
			t.Fatal("login/reopen changed saved result")
		}
		if oldView != "" {
			refused := h.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: oldView, JobID: oldJob})
			if refused.OK || refused.Job != nil || refused.Error != "stale_view" {
				t.Fatal("previous login disclosed practice", refused.Error)
			}
		}
		pair := [2]string{"067121d1f9be9eee0eab884de24e24df996e3b15d4385f5123930c6405d71fa3", "164913fbdff8365f9187dbdfa22c4a6b667399f5fbde0e4f4ddd718daa1cbb3c"}
		want := fightercareer.Totals{Wins: 1, XP: 1, BasePoints: 14}
		if fixture == "incrementing256" {
			pair = [2]string{"22a2c799c122ac002a3ca277e0f7f84dd89c8b5f1fe1e1a18e098b1aad27e717", "ebfddc68ebcaac81047e4d409458575f41b9509b7c330bfdd0f087e13c5288db"}
			want = fightercareer.Totals{Losses: 1, XP: 1}
		}
		// Establish actual current Career page authority from fresh source jobs.
		pages := []pilot.CareerResponse{}
		pageJobs := []pilot.StableJob{}
		found := [2]int{}
		var after uint64
		for pageNumber := 0; pageNumber < 4; pageNumber++ {
			r, sourceJob := h.careerRequest(pilot.CareerRequest{Command: "career_inventory", After: after, Limit: 8})
			pages = append(pages, r)
			pageJobs = append(pageJobs, sourceJob)
			for _, item := range r.Page.Items {
				for i, id := range pair {
					if item.ProspectID == id {
						found[i]++
						expected := fightercareer.Totals{}
						if i == 0 {
							expected = want
						}
						if item.Totals != expected {
							t.Fatal("preserved effective totals")
						}
					}
				}
			}
			if !r.Page.Source.HasMore {
				break
			}
			after = r.Page.Source.NextAfter
		}
		if found != [2]int{1, 1} {
			t.Fatal("exact actual same-gym pair absent", found)
		}
		var snap pilot.CareerPreviewResponse
		seedNativeClockWait(t, ctx, func() bool {
			snap = h.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
			return snap.Error != "busy"
		})
		if !snap.OK || snap.Snapshot == nil || snap.Snapshot.State == "unavailable" || snap.Snapshot.View == nil || *snap.Snapshot.View != view || snap.Snapshot.Activation == nil {
			t.Fatal("actual ready preview", snap.Error)
		}
		started := time.Now()
		ack := h.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_begin", ViewID: view.ViewID, FighterAID: pair[0], FighterBID: pair[1]})
		if !ack.OK || ack.Job == nil || ack.Job.State != "queued" {
			t.Fatal("explicit practice ACK", ack.Error)
		}
		job := *ack.Job
		seedNativeClockWait(t, ctx, func() bool {
			r := h.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: view.ViewID, JobID: job.JobID})
			if !r.OK || r.Job == nil {
				t.Fatal("actual practice poll", r.Error)
			}
			job = *r.Job
			return job.State != "queued" && job.State != "running"
		})
		elapsed := time.Since(started)
		if job.State != "done" || job.Result == nil || job.Result.Pair.Fighters[0].Totals != want || job.Result.Pair.Fighters[1].Totals != (fightercareer.Totals{}) || job.Result.Pair.AsOfMS != 1796500801001 {
			t.Fatal("actual accounted-XP practice", job.State, job.Error)
		}
		portraits := []CareerPreviewPortraitResponse{}
		for _, id := range pair {
			first := h.b.CareerFightPreviewPortrait(ctx, view.ViewID, job.JobID, id)
			cached := h.b.CareerFightPreviewPortrait(ctx, view.ViewID, job.JobID, id)
			if !first.OK || !cached.OK || !reflect.DeepEqual(first, cached) {
				t.Fatal("actual portrait/render-cache", first.Error, cached.Error)
			}
			portraits = append(portraits, first)
		}
		if _, e := h.b.Request(ctx, "stop", 0); e != nil {
			t.Fatal(e)
		}
		stopped := h.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: view.ViewID, JobID: job.JobID})
		if !stopped.OK || stopped.Job == nil || !reflect.DeepEqual(*stopped.Job, job) {
			t.Fatal("completed practice did not survive Stop", stopped.Error)
		}
		end := h.read()
		if !reflect.DeepEqual(end.Record, before.Record) {
			t.Fatal("unrecorded practice mutated permanent result")
		}
		// Read effective career state again through fresh authenticated source
		// jobs. Result bytes alone would not detect an accidentally added award.
		pagesAfter := []pilot.CareerResponse{}
		pageJobsAfter := []pilot.StableJob{}
		for _, previous := range pages {
			next, sourceJob := h.careerRequest(pilot.CareerRequest{Command: "career_inventory", After: previous.Page.Source.After, Limit: 8})
			p, q := previous.Page, next.Page
			if q == nil || p.Activation != q.Activation || p.CareerDigest != q.CareerDigest || p.AsOfMS != q.AsOfMS || !reflect.DeepEqual(p.Items, q.Items) || !reflect.DeepEqual(p.Source.Items, q.Source.Items) || p.Source.NextAfter != q.Source.NextAfter || p.Source.HasMore != q.Source.HasMore {
				t.Fatal("unrecorded practice changed effective career prefix or totals")
			}
			pagesAfter = append(pagesAfter, next)
			pageJobsAfter = append(pageJobsAfter, sourceJob)
		}
		last := h.account("signed_in")
		h.unchanged(h.baseline[fixture], last)
		h.processes.capture(t, fixture, "career-preview-complete", view.AccountID, view.AuthorityOrigin, false)
		rows = append(rows, map[string]any{"fixture": fixture, "reopened": n == 2, "held": h.view.Account.Held, "view": view, "pages_before": pages, "source_jobs_before": pageJobs, "pages_after": pagesAfter, "source_jobs_after": pageJobsAfter, "snapshot": snap, "ack": ack, "job": job, "portraits": portraits, "stop_replay": stopped, "elapsed_ms": elapsed.Milliseconds(), "result_before": before, "result_after": end, "account_after": last})
		oldView, oldJob = view.ViewID, job.JobID
		if _, e := h.b.Request(ctx, "logout", 0); e != nil {
			t.Fatal(e)
		}
		h.account("signed_out")
	}
	h.close()
	resultActualSave(t, root, "public-career-preview-native-evidence.json", map[string]any{"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": true, "clock_ms": int64(1796500801001), "automatic_admissions": 0, "rows": rows, "baseline": h.baseline, "scope": "Actual finalized Set49 awarded pairs through fresh fixture login, private helper pipe and native preview/portrait guards. Captured result is unrecorded. No mining, new admission, result mutation, GUI or production authority."})
}
