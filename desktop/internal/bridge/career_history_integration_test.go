//go:build desktopintegration && !slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

type historyPackageAcceptance struct{ *careerAcceptance }

func (p *historyPackageAcceptance) history(q pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.CareerHistoryRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		p.pause() // Functional readiness only; never retry an uncertain action.
	}
	p.t.Fatal("history metadata remained busy", q.Command)
	return pilot.CareerHistoryResponse{}
}
func (p *historyPackageAcceptance) capture(view pilot.ManifestViewPin, id string) (pilot.CareerHistoryResponse, pilot.CareerHistoryResponse, pilot.CareerHistoryResponse) {
	p.t.Helper()
	s := p.history(pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
	if !s.OK || s.Snapshot == nil || s.Snapshot.State != "idle" || s.Snapshot.View == nil || *s.Snapshot.View != view || s.Snapshot.Activation == nil || *s.Snapshot.Activation != p.activation {
		p.t.Fatal("fresh history authority", s.Error)
	}
	a := p.history(pilot.CareerHistoryRequest{Command: "career_history_begin", ViewID: view.ViewID, ProspectID: id})
	if !a.OK || a.Job == nil || a.Job.State != "queued" || a.Job.View != view || a.Job.ProspectID != id || a.Job.Capture != nil {
		p.t.Fatal("explicit history queued ACK", a.Error)
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		j := p.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: view.ViewID, JobID: a.Job.JobID})
		if !j.OK || j.Job == nil || j.Job.JobID != a.Job.JobID || j.Job.View != view {
			p.t.Fatal("original history job", j.Error)
		}
		if j.Job.State != "queued" && j.Job.State != "running" {
			if j.Job.State != "done" || j.Job.Capture == nil || j.Job.Capture.Activation != p.activation || j.Job.Capture.EventCount != 0 || j.Job.Capture.PrefixCount != 0 || j.Job.Capture.Totals != (fightercareer.Totals{}) {
				p.t.Fatal("normal calendar invented finalized history", j.Job.State, j.Job.Error)
			}
			return s, a, j
		}
		p.pause()
	}
	p.t.Fatal("history capture timeout")
	return s, a, pilot.CareerHistoryResponse{}
}
func (p *historyPackageAcceptance) emptyPage(view pilot.ManifestViewPin, job string) pilot.CareerHistoryResponse {
	p.t.Helper()
	r := p.history(pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: view.ViewID, JobID: job, After: 0, Limit: 8})
	if !r.OK || r.Page == nil || r.Page.View != view || r.Page.JobID != job || r.Page.Page.Items == nil || len(r.Page.Page.Items) != 0 || r.Page.Page.After != 0 || r.Page.Page.Next != 0 || r.Page.Page.HasMore || r.Page.Page.Capture.EventCount != 0 || r.Page.Page.Capture.Totals != (fightercareer.Totals{}) {
		p.t.Fatal("exact captured empty/end page", r.Error)
	}
	return r
}

// This phase uses the retained normal bundle and disposable journals produced
// by the unchanged ten-family package flow. It adds no mining or clock override.
func TestCareerHistoryPackagedHelperNormalPoW(t *testing.T) {
	path, helper, node := os.Getenv("SLUG_CAREER_HISTORY_PACKAGE_RESULT"), os.Getenv("SLUG_STABLE_HELPER"), os.Getenv("SLUG_STABLE_NODE")
	if path == "" || helper == "" || node == "" {
		t.Skip("explicit completed package evidence and frozen normal bundle required")
	}
	if real, e := filepath.EvalSymlinks(path); e != nil || !filepath.IsAbs(path) || real != path || filepath.Base(path) != "public-result.json" {
		t.Fatal("canonical completed package evidence required")
	}
	root := filepath.Dir(path)
	read := func(name string, max int64) ([]byte, string) {
		t.Helper()
		p := filepath.Join(root, name)
		info, e := os.Lstat(p)
		if e != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > max {
			t.Fatal("regular bounded package input", name)
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.Sum256(raw)
		return raw, hex.EncodeToString(h[:])
	}
	raw, inputSHA := read("public-result.json", 32<<20)
	previewRaw, previewSHA := read("public-career-preview-package.json", 32<<20)
	markerRaw, _ := read("SET51-PACKAGE-OWNED.json", 4096)
	var marker struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		InputSHA   string `json:"input_sha256"`
		PreviewSHA string `json:"preview_sha256"`
	}
	if json.Unmarshal(markerRaw, &marker) != nil || marker.Version != 1 || marker.Scope != "Set51 completed normal package fixtures" || marker.InputSHA != inputSHA || marker.PreviewSHA != previewSHA {
		t.Fatal("exact completed package declaration required")
	}
	var inherited struct {
		Status           string `json:"status"`
		FixtureOnly      bool   `json:"fixture_only"`
		PublicChainMined bool   `json:"public_chain_mined"`
		GUIExercised     bool   `json:"gui_exercised"`
		Careers          struct {
			Activation careerstate.Activation            `json:"activation"`
			Original   map[string]pilot.ManifestSnapshot `json:"original_baseline"`
			Baseline   map[string]pilot.ManifestSnapshot `json:"baseline"`
		} `json:"careers"`
		Stable []struct {
			Fixture   string         `json:"fixture"`
			Inventory gymstable.Page `json:"inventory"`
		} `json:"stable"`
	}
	var preview struct {
		Status      string                 `json:"status"`
		NormalPoW   bool                   `json:"normal_pow"`
		TaggedClock bool                   `json:"tagged_clock"`
		InputSHA    string                 `json:"input_sha256"`
		Activation  careerstate.Activation `json:"activation"`
	}
	if json.Unmarshal(raw, &inherited) != nil || inherited.Status != "passed" || !inherited.FixtureOnly || inherited.PublicChainMined || inherited.GUIExercised || len(inherited.Stable) != 2 || len(inherited.Careers.Original) != 2 || len(inherited.Careers.Baseline) != 2 || inherited.Careers.Activation.ActivationID == "" || json.Unmarshal(previewRaw, &preview) != nil || preview.Status != "passed" || !preview.NormalPoW || preview.TaggedClock || preview.InputSHA != inputSHA || preview.Activation != inherited.Careers.Activation {
		t.Fatal("unchanged inherited ten-family package evidence required")
	}
	items := map[string]gymstable.Item{}
	statuses := map[string]gymstable.Status{}
	for _, row := range inherited.Stable {
		if (row.Fixture != "zero256" && row.Fixture != "incrementing256") || len(row.Inventory.Items) == 0 || items[row.Fixture].Assignment.ProspectID != "" {
			t.Fatal("two distinct original fixture inventories required")
		}
		items[row.Fixture], statuses[row.Fixture] = row.Inventory.Items[0], row.Inventory.Status
	}
	data := filepath.Join(root, "data")
	if real, e := filepath.EvalSymlinks(data); e != nil || real != data {
		t.Fatal("canonical disposable journals required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p := &historyPackageAcceptance{&careerAcceptance{t: t, ctx: ctx, helper: helper, data: data, node: node, activation: inherited.Careers.Activation, original: inherited.Careers.Original, baseline: inherited.Careers.Baseline}}
	defer func() {
		if p.b != nil {
			_ = p.b.Close()
			p.processes.checkClosed(t)
		}
	}()
	rows, detached, cleanups := []map[string]any{}, []map[string]any{}, []map[string]any{}
	p.open()
	oldView, oldJob := "", ""
	for n, fixture := range []string{"zero256", "incrementing256", "zero256"} {
		phase := "initial"
		if n == 2 {
			cleanups = append(cleanups, p.close())
			p.open()
			phase = "process_restart"
		}
		account := p.login(fixture)
		if account.View == nil || !account.Account.Held {
			t.Fatal("original held normal fixture required")
		}
		view, item := *account.View, items[fixture]
		resultBefore := newResultAcceptance(t, ctx, p.b).captureSnapshot()
		if s := resultBefore.Snapshot; s == nil || s.State != "ready" || s.View == nil || *s.View != view || s.StoreID != p.activation.ResultStoreID || s.CreatedAtMS != p.activation.ResultCreatedAtMS || s.SourceCreatedAtMS != p.activation.SourceCreatedAtMS {
			t.Fatal("original Result authority", resultBefore.Error)
		}
		stable := p.b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
		if !stable.OK || stable.Snapshot == nil || stable.Snapshot.View == nil || *stable.Snapshot.View != view || stable.Snapshot.Status == nil || stable.Snapshot.Status.StoreID != statuses[fixture].StoreID || stable.Snapshot.Status.Pin != statuses[fixture].Pin {
			t.Fatal("original Stable authority", stable.Error)
		}
		var stale *pilot.CareerHistoryResponse
		if oldView != "" {
			r := p.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: oldView, JobID: oldJob})
			if r.OK || r.Job != nil || r.Error != "stale_view" {
				t.Fatal("old original View", r.Error)
			}
			stale = &r
		}
		snap, ack, terminal := p.capture(view, item.Assignment.ProspectID)
		if terminal.Job.Capture.Assignment != item.Assignment || terminal.Job.Capture.Status.Pin != statuses[fixture].Pin {
			t.Fatal("history changed original source assignment")
		}
		first := p.emptyPage(view, terminal.Job.JobID)
		repeat := p.emptyPage(view, terminal.Job.JobID)
		end := p.emptyPage(view, terminal.Job.JobID)
		if !reflect.DeepEqual(first.Page, repeat.Page) || !reflect.DeepEqual(first.Page, end.Page) {
			t.Fatal("empty page changed across repeated exact end reads")
		}
		var emptyLink pilot.ResultResponse
		linkDeadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(linkDeadline) {
			emptyLink = p.b.CareerHistoryResult(ctx, view.ViewID, terminal.Job.JobID, 1)
			if emptyLink.Error != "busy" {
				break
			}
			p.pause()
		}
		if emptyLink.OK || emptyLink.Error != "result_not_found" || emptyLink.Record != nil || emptyLink.Job != nil || emptyLink.Status != nil || emptyLink.Snapshot != nil {
			t.Fatal("empty history supplied a Result link", emptyLink.Error)
		}
		if _, e := p.b.Request(ctx, "stop", 0); e != nil {
			t.Fatal(e)
		}
		stoppedJob := p.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: view.ViewID, JobID: terminal.Job.JobID})
		stoppedPage := p.history(pilot.CareerHistoryRequest{Command: "career_history_page", ViewID: view.ViewID, JobID: terminal.Job.JobID, Limit: 8})
		for _, r := range []pilot.CareerHistoryResponse{stoppedJob, stoppedPage} {
			if r.OK || r.Job != nil || r.Page != nil || r.Error != "history_not_found" {
				t.Fatal("Stop retained old pageable owner", r.Error)
			}
		}
		freshSnap, freshACK, freshJob := p.capture(view, item.Assignment.ProspectID)
		freshPage := p.emptyPage(view, freshJob.Job.JobID)
		if freshJob.Job.JobID == terminal.Job.JobID || freshJob.Job.Capture.Assignment != item.Assignment || freshJob.Job.Capture.CareerDigest == "" {
			t.Fatal("fresh post-Stop empty capture")
		}
		resultAfter := newResultAcceptance(t, ctx, p.b).captureSnapshot()
		if !reflect.DeepEqual(resultBefore.Snapshot, resultAfter.Snapshot) {
			t.Fatal("history changed Result journal metadata")
		}
		after := p.manifest()
		p.unchanged(account, after)
		p.unchanged(p.original[fixture], after)
		p.processes.capture(t, fixture, "career-history-"+phase, view.AccountID, view.AuthorityOrigin, false)
		rows = append(rows, map[string]any{"fixture": fixture, "phase": phase, "view": view, "held": account.Account.Held, "prospect_id": item.Assignment.ProspectID, "account_before": account, "account_after": after, "stable_snapshot": stable, "result_before": resultBefore, "result_after": resultAfter, "snapshot": snap, "ack": ack, "terminal": terminal, "pages": []pilot.CareerHistoryResponse{first, repeat, end}, "empty_result_link": emptyLink, "stop_job_refusal": stoppedJob, "stop_page_refusal": stoppedPage, "fresh_snapshot": freshSnap, "fresh_ack": freshACK, "fresh_terminal": freshJob, "fresh_page": freshPage, "old_view_refusal": stale})
		oldView, oldJob = view.ViewID, freshJob.Job.JobID
		if _, e := p.b.Request(ctx, "logout", 0); e != nil {
			t.Fatal(e)
		}
		p.waitAccount("signed_out")
		r := p.history(pilot.CareerHistoryRequest{Command: "career_history_job", ViewID: view.ViewID, JobID: freshJob.Job.JobID})
		if r.OK || r.Job != nil || r.Error != "stale_view" {
			t.Fatal("detached original View", r.Error)
		}
		detached = append(detached, map[string]any{"fixture": fixture, "phase": phase, "view": view, "job_id": freshJob.Job.JobID, "refusal": r})
	}
	cleanups = append(cleanups, p.close())
	if now, _ := os.ReadFile(path); !reflect.DeepEqual(now, raw) {
		t.Fatal("inherited evidence changed")
	}
	if now, _ := os.ReadFile(filepath.Join(root, "public-career-preview-package.json")); !reflect.DeepEqual(now, previewRaw) {
		t.Fatal("inherited preview evidence changed")
	}
	out := map[string]any{"version": 1, "status": "passed", "normal_pow": true, "normal_clock": true, "tagged_clock": false, "gui_exercised": false, "public_chain_mined": false, "additional_mining": false, "award_executed": false, "input_sha256": inputSHA, "preview_sha256": previewSHA, "activation": p.activation, "rows": rows, "detached": detached, "cleanup": cleanups, "startup_ms": p.startups, "startup_admission_budget_ms": 25000, "native_startup_bound_ms": 30000, "history_metadata_budget_ms": 500, "source_work_budget_ms": 30000, "capture_jobs": 6, "proof_call_count_measured": false, "scope": "Actual retained normal bundle and original held fixtures; zero-finalized-history capture, repeated exact empty/end pages, Stop/new capture, logout and process restart. Existing ten-family coverage and isolated fixture mining are separate and unchanged. No added mining, awards, chain action, clock override, GUI or production claim. Functional busy waits are not metadata latency acceptance."}
	encoded, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(root, "public-career-history-package.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(append(encoded, '\n')); e != nil {
		_ = f.Close()
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
