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

// This separate normal-calendar phase follows the unchanged nine-family package
// producer. It opens only the runner-declared completed disposable fixture tree.
// The inherited flow mines; this additional practice phase never starts mining.
func TestCareerPreviewPackagedHelperNormalPoW(t *testing.T) {
	path, helper, node := os.Getenv("SLUG_CAREER_PREVIEW_PACKAGE_RESULT"), os.Getenv("SLUG_STABLE_HELPER"), os.Getenv("SLUG_STABLE_NODE")
	if path == "" || helper == "" || node == "" {
		t.Skip("explicit completed package evidence and frozen bundle required")
	}
	canonical, e := filepath.EvalSymlinks(path)
	if e != nil || !filepath.IsAbs(path) || canonical != path || filepath.Base(path) != "public-result.json" {
		t.Fatal("canonical owned package result required")
	}
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 32<<20 {
		t.Fatal("bounded completed package evidence required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	inputSHA := hex.EncodeToString(digest[:])
	root := filepath.Dir(path)
	var marker struct {
		Version  uint32 `json:"version"`
		Scope    string `json:"scope"`
		InputSHA string `json:"input_sha256"`
	}
	m, e := os.ReadFile(filepath.Join(root, "SET50-PACKAGE-OWNED.json"))
	if e != nil || json.Unmarshal(m, &marker) != nil || marker.Version != 1 || marker.Scope != "Set50 completed normal package fixtures" || marker.InputSHA != inputSHA {
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
	if json.Unmarshal(raw, &inherited) != nil || inherited.Status != "passed" || !inherited.FixtureOnly || inherited.PublicChainMined || inherited.GUIExercised || len(inherited.Stable) != 2 || len(inherited.Careers.Original) != 2 || len(inherited.Careers.Baseline) != 2 || inherited.Careers.Activation.ActivationID == "" {
		t.Fatal("complete inherited nine-family package evidence required")
	}
	pairs := map[string][2]gymstable.Item{}
	stableSources := map[string]gymstable.Status{}
	for _, row := range inherited.Stable {
		if row.Fixture != "zero256" && row.Fixture != "incrementing256" {
			t.Fatal("unknown published fixture")
		}
		if _, ok := pairs[row.Fixture]; ok {
			t.Fatal("duplicate fixture")
		}
		var pair [2]gymstable.Item
		for i, a := range row.Inventory.Items {
			for _, b := range row.Inventory.Items[i+1:] {
				if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
					pair = [2]gymstable.Item{a, b}
					break
				}
			}
			if pair[0].Assignment.ProspectID != "" {
				break
			}
		}
		if pair[0].Assignment.ProspectID == "" {
			t.Fatal("genuine inherited same-class pair missing")
		}
		if pair[0].Assignment.ProspectID > pair[1].Assignment.ProspectID {
			pair[0], pair[1] = pair[1], pair[0]
		}
		pairs[row.Fixture] = pair
		stableSources[row.Fixture] = row.Inventory.Status
	}
	data := filepath.Join(root, "data")
	if real, err := filepath.EvalSymlinks(data); err != nil || real != data {
		t.Fatal("canonical owned package journals required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p := &careerAcceptance{t: t, ctx: ctx, helper: helper, data: data, node: node, original: inherited.Careers.Original, baseline: inherited.Careers.Baseline}
	defer func() {
		if p.b != nil {
			_ = p.b.Close()
			p.processes.checkClosed(t)
		}
	}()
	rows, detached, cleanups := []map[string]any{}, []map[string]any{}, []map[string]any{}
	p.open()
	oldView, oldJob := "", ""
	for n, fixture := range []string{"zero256", "zero256", "incrementing256", "incrementing256", "zero256"} {
		phase := "initial"
		if n == 1 || n == 3 {
			phase = "reopened"
		}
		if n == 4 {
			cleanups = append(cleanups, p.close())
			p.open()
			phase = "process_restart"
		}
		account := p.login(fixture)
		if account.View == nil || !account.Account.Held {
			t.Fatal("retained held fixture/current View required")
		}
		view, pair := *account.View, pairs[fixture]
		// Manifest login does not establish the native Stable source binding.
		// Discover both authorities through their existing typed snapshots first.
		resultBefore := newResultAcceptance(t, ctx, p.b).captureSnapshot()
		result := resultBefore.Snapshot
		activation := inherited.Careers.Activation
		if result == nil || result.State != "ready" || result.View == nil || *result.View != view || result.StoreID != activation.ResultStoreID || result.CreatedAtMS != activation.ResultCreatedAtMS || result.SourceCreatedAtMS != activation.SourceCreatedAtMS {
			t.Fatal("original result authority required before career source job", resultBefore.Error)
		}
		stable := p.b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
		expected := stableSources[fixture]
		if !stable.OK || stable.Snapshot == nil || stable.Snapshot.View == nil || *stable.Snapshot.View != view || stable.Snapshot.Status == nil || stable.Snapshot.Status.StoreID != expected.StoreID || stable.Snapshot.Status.Pin != expected.Pin {
			t.Fatal("original stable authority required before career source job", stable.Error)
		}
		before, sourceJob := p.page(pilot.CareerRequest{Command: "career_inventory", Limit: 8}, view)
		if before.Page == nil || before.Page.Activation != inherited.Careers.Activation {
			t.Fatal("unchanged activation required")
		}
		found := [2]int{}
		for _, item := range before.Page.Items {
			if item.Totals != (fightercareer.Totals{}) || item.Condition.ProXP != 0 {
				t.Fatal("normal package invented career awards")
			}
			for i, f := range pair {
				if item.ProspectID == f.Assignment.ProspectID {
					found[i]++
				}
			}
		}
		if found != [2]int{1, 1} {
			t.Fatal("inherited same-class pair absent from exact current page")
		}
		var snap pilot.CareerPreviewResponse
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			snap = p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_snapshot"})
			if snap.Error != "busy" {
				break
			}
			p.pause()
		}
		if !snap.OK || snap.Snapshot == nil || snap.Snapshot.State != "idle" || snap.Snapshot.View == nil || *snap.Snapshot.View != view || snap.Snapshot.Activation == nil || *snap.Snapshot.Activation != inherited.Careers.Activation || snap.Snapshot.ActiveJobID != "" || snap.Snapshot.QueuedJobID != "" || snap.Snapshot.LastJobID != "" {
			t.Fatal("fresh login/restart retained or invented practice", snap.Error)
		}
		var stale *pilot.CareerPreviewResponse
		if oldView != "" {
			refusal := p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: oldView, JobID: oldJob})
			stale = &refusal
			if stale.OK || stale.Job != nil || stale.Snapshot != nil || stale.Error != "stale_view" {
				t.Fatal("old session practice disclosure", stale.Error)
			}
		}
		var ack pilot.CareerPreviewResponse
		deadline = time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			ack = p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_begin", ViewID: view.ViewID, FighterAID: pair[1].Assignment.ProspectID, FighterBID: pair[0].Assignment.ProspectID})
			if ack.Error != "busy" {
				break
			}
			p.pause()
		}
		if !ack.OK || ack.Job == nil || ack.Job.State != "queued" || ack.Job.Result != nil || ack.Job.View != view || ack.Job.FighterAID != pair[0].Assignment.ProspectID || ack.Job.FighterBID != pair[1].Assignment.ProspectID {
			t.Fatal("exact explicit queued practice ACK", ack.Error)
		}
		job := *ack.Job
		deadline = time.Now().Add(45 * time.Second)
		for job.State == "queued" || job.State == "running" {
			if time.Now().After(deadline) {
				t.Fatal("practice terminal timeout")
			}
			p.pause()
			r := p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: view.ViewID, JobID: job.JobID})
			if !r.OK || r.Job == nil || r.Job.JobID != ack.Job.JobID || r.Job.View != view {
				t.Fatal("practice original job correlation", r.Error)
			}
			job = *r.Job
		}
		if job.State != "done" || job.Result == nil || job.Error != "" || job.Result.Pair.Activation != inherited.Careers.Activation || job.Result.Pair.Status.StoreID != before.StoreID {
			t.Fatal("normal practice completion", job.State, job.Error)
		}
		for i, f := range job.Result.Pair.Fighters {
			if f.Assignment != pair[i].Assignment || f.Manifest != pair[i].Manifest || f.Totals != (fightercareer.Totals{}) || f.Condition.ProXP != 0 || f.Condition.BornAtMS != pair[i].Assignment.BornAtMS || f.Condition.AsOfMS != job.Result.Pair.AsOfMS {
				t.Fatal("practice changed original fighter/XP")
			}
		}
		portraits := []CareerPreviewPortraitResponse{}
		for _, f := range pair {
			first := p.b.CareerFightPreviewPortrait(ctx, view.ViewID, job.JobID, f.Assignment.ProspectID)
			cached := p.b.CareerFightPreviewPortrait(ctx, view.ViewID, job.JobID, f.Assignment.ProspectID)
			if !first.OK || first.Portrait == nil || !reflect.DeepEqual(first, cached) || first.Portrait.View != view || first.Portrait.ManifestDigest != f.Assignment.ManifestDigest || first.Portrait.PortraitDigest != f.Assignment.PortraitDigest || first.Portrait.ResultDigest != job.Result.EngineResultDigest {
				t.Fatal("fresh typed portrait/cache authorization", first.Error, cached.Error)
			}
			portraits = append(portraits, first)
		}
		if _, e := p.b.Request(ctx, "stop", 0); e != nil {
			t.Fatal(e)
		}
		stop := p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: view.ViewID, JobID: job.JobID})
		if !stop.OK || stop.Job == nil || !reflect.DeepEqual(*stop.Job, job) {
			t.Fatal("Stop changed completed unrecorded practice", stop.Error)
		}
		after, afterJob := p.page(pilot.CareerRequest{Command: "career_inventory", Limit: 8}, view)
		// The real clock may advance conditions. Immutable bindings and zero totals
		// must remain exact while both observations retain their own as-of time.
		if after.Page == nil || after.Page.Activation != before.Page.Activation || len(after.Page.Source.Items) != len(before.Page.Source.Items) {
			t.Fatal("practice changed inventory")
		}
		if len(after.Page.Items) != len(before.Page.Items) {
			t.Fatal("practice changed career inventory")
		}
		for i, x := range after.Page.Items {
			a, b := before.Page.Source.Items[i], after.Page.Source.Items[i]
			if x.ProspectID != before.Page.Items[i].ProspectID || x.Totals != before.Page.Items[i].Totals || x.Condition.ProXP != 0 || a.Assignment != b.Assignment || a.Manifest != b.Manifest {
				t.Fatal("practice changed awards or immutable assignments")
			}
		}
		bs, as := before.Page.Source.Status, after.Page.Source.Status
		bs.LastAsOfMS, as.LastAsOfMS = 0, 0
		if bs != as {
			t.Fatal("practice changed stable/roster state")
		}
		resultAfter := newResultAcceptance(t, ctx, p.b).captureSnapshot()
		if !reflect.DeepEqual(resultBefore.Snapshot, resultAfter.Snapshot) {
			t.Fatal("practice changed publication state")
		}
		accountAfter := p.manifest()
		p.unchanged(account, accountAfter)
		p.unchanged(p.original[fixture], accountAfter)
		p.processes.capture(t, fixture, "career-preview-"+phase, view.AccountID, view.AuthorityOrigin, false)
		rows = append(rows, map[string]any{"fixture": fixture, "phase": phase, "view": view, "held": account.Account.Held, "account_before": account, "account_after": accountAfter, "page_before": before, "source_job_before": sourceJob, "page_after": after, "source_job_after": afterJob, "snapshot": snap, "ack": ack, "job": job, "portraits": portraits, "stop_replay": stop, "old_view_refusal": stale, "result_before": resultBefore, "result_after": resultAfter})
		oldView, oldJob = view.ViewID, job.JobID
		if _, e := p.b.Request(ctx, "logout", 0); e != nil {
			t.Fatal(e)
		}
		p.waitAccount("signed_out")
		refused := p.b.CareerPreviewRequest(ctx, pilot.CareerPreviewRequest{Command: "career_preview_job", ViewID: view.ViewID, JobID: job.JobID})
		portrait := p.b.CareerFightPreviewPortrait(ctx, view.ViewID, job.JobID, job.FighterAID)
		if refused.OK || refused.Job != nil || refused.Snapshot != nil || refused.Error != "stale_view" || portrait.OK || portrait.Portrait != nil || portrait.Error != "stale_view" {
			t.Fatal("detached original session disclosed practice", refused.Error, portrait.Error)
		}
		detached = append(detached, map[string]any{"fixture": fixture, "phase": phase, "view": view, "job_id": job.JobID, "refusal": refused, "portrait_refusal": portrait})
	}
	cleanups = append(cleanups, p.close())
	if current, e := os.ReadFile(path); e != nil || !reflect.DeepEqual(current, raw) {
		t.Fatal("inherited evidence bytes changed")
	}
	out := map[string]any{"version": 1, "status": "passed", "normal_pow": true, "tagged_clock": false, "gui_exercised": false, "public_chain_mined": false, "additional_mining": false, "award_executed": false, "input_sha256": inputSHA, "activation": inherited.Careers.Activation, "rows": rows, "detached": detached, "cleanup": cleanups, "startup_ms": p.startups, "startup_admission_budget_ms": 25000, "native_startup_bound_ms": 30000, "preview_metadata_budget_ms": 250, "career_metadata_budget_ms": 500, "source_work_budget_ms": 30000, "scope": "Actual retained normal bundle, original fixture login and private pipe, prospective Career activation, same-gym/class XP0 practice, typed portraits and cache-hit authorization, Stop, logout/reopen/restart. Inherited isolated fixture mining is separate; this phase adds no mining, awards, result publication, GUI or production claim. Functional busy waits are not metadata performance acceptance."}
	encoded, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(root, "public-career-preview-package.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
