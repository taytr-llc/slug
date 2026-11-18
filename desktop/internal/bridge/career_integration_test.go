//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Career package coverage follows the unchanged Result fixture flow. It upgrades
// the completed two-fixture journals prospectively, without extra mining, Run,
// publication or backdated clock. No real-calendar finalized award is invented.
type careerAcceptance struct {
	t                  *testing.T
	ctx                context.Context
	b                  *StableBridge
	helper, data, node string
	processes          *manifestProcessInventory
	prior              *resultAuthority
	activation         careerstate.Activation
	original           map[string]pilot.ManifestSnapshot
	baseline           map[string]pilot.ManifestSnapshot
	evidence           []map[string]any
	detached           []map[string]any
	startups           []int64
	lastJobID          string
}

func (p *careerAcceptance) pause() {
	p.t.Helper()
	select {
	case <-p.ctx.Done():
		p.t.Fatal(p.ctx.Err())
	case <-time.After(100 * time.Millisecond):
	}
}
func (p *careerAcceptance) manifest() pilot.ManifestSnapshot {
	p.t.Helper()
	r, e := p.b.Request(p.ctx, "snapshot", 0)
	if e != nil || r.Snapshot == nil {
		p.t.Fatal("career package account", e)
	}
	return *r.Snapshot
}
func (p *careerAcceptance) waitAccount(state string) pilot.ManifestSnapshot {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		s := p.manifest()
		if s.Account.State == "failed" || s.State == "failed" {
			p.t.Fatal("career package account failed", s.Account.LastError, s.LastError)
		}
		if s.Account.State == state && (state != "signed_in" || s.View != nil) {
			return s
		}
		p.pause()
	}
	p.t.Fatal("career package account timeout", state)
	return pilot.ManifestSnapshot{}
}
func (p *careerAcceptance) open() {
	p.t.Helper()
	start := time.Now()
	var e error
	p.b, e = LaunchCareers(p.helper, p.data, p.node)
	if e != nil {
		p.t.Fatal("career package launch", e)
	}
	p.processes = newManifestProcessInventory(p.data, p.node, p.b.cmd.Process.Pid)
	p.processes.nodes = []manifestObservedProcess{}
	s := p.waitAccount("signed_out")
	elapsed := time.Since(start)
	if elapsed > 30*time.Second || s.View != nil {
		p.t.Fatal("career parent startup/identity boundary", elapsed)
	}
	p.startups = append(p.startups, elapsed.Milliseconds())
}
func (p *careerAcceptance) close() map[string]any {
	p.t.Helper()
	if e := p.b.Close(); e != nil {
		p.t.Fatal("career package helper close", e)
	}
	p.processes.checkClosed(p.t)
	if p.b.cmd.ProcessState == nil || !p.b.cmd.ProcessState.Exited() || p.b.cmd.ProcessState.ExitCode() != 0 {
		p.t.Fatal("career helper unclean exit")
	}
	out := p.processes.evidence()
	out["process_observation_scope"] = "Career phase only: explicit activation, reads, identity changes and restart; no added miner or node owner. Inherited fixture mining is captured separately."
	p.b = nil
	p.processes = nil
	return out
}
func (p *careerAcceptance) login(fixture string) pilot.ManifestSnapshot {
	p.t.Helper()
	if _, e := p.b.RequestFixture(p.ctx, fixture); e != nil {
		p.t.Fatal(e)
	}
	s := p.waitAccount("signed_in")
	if s.State != "stopped" || s.WalletState != "closed" {
		p.t.Fatal("career login started miner/wallet")
	}
	p.unchanged(p.original[fixture], s)
	if old, ok := p.baseline[fixture]; ok {
		p.unchanged(old, s)
	} else {
		p.baseline[fixture] = s
	}
	return s
}
func (p *careerAcceptance) unchanged(a, b pilot.ManifestSnapshot) {
	p.t.Helper()
	if a.Attempts != b.Attempts || a.AcceptedCount != b.AcceptedCount || a.Pending != b.Pending || a.PayoutAddress != b.PayoutAddress || a.GymID != b.GymID || a.SpendableAtoms != b.SpendableAtoms || a.ImmatureAtoms != b.ImmatureAtoms || b.State != "stopped" || b.WalletState != "closed" {
		p.t.Fatal("career read phase changed mining/wallet")
	}
}
func (p *careerAcceptance) reply(q pilot.CareerRequest) pilot.CareerResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.CareerRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		p.pause()
	}
	p.t.Fatal("career metadata remained busy", q.Command)
	return pilot.CareerResponse{}
}

// The proof-backed source remains an ordinary Stable job with its existing
// thirty-second work budget. The Career call reads that exact completed job's
// retained composite under the separate metadata budget.
func (p *careerAcceptance) page(q pilot.CareerRequest, view pilot.ManifestViewPin) (pilot.CareerResponse, pilot.StableJob) {
	p.t.Helper()
	command, kind := "stable_inventory", "inventory"
	if q.Command == "career_roster" {
		command, kind = "stable_roster", "roster"
	}
	request := pilot.StableRequest{Command: command, ViewID: view.ViewID, After: q.After, FirstSlot: q.FirstSlot, Limit: q.Limit}
	deadline := time.Now().Add(45 * time.Second)
	var j pilot.StableJob
	for time.Now().Before(deadline) {
		r := p.b.StableRequest(p.ctx, request)
		if r.OK && r.Job != nil {
			j = *r.Job
			break
		}
		if r.Error != "busy" {
			p.t.Fatal("career source job admission", command, r.Error)
		}
		p.pause()
	}
	if j.JobID == "" || j.State != "queued" || j.View != view || j.Kind != kind {
		p.t.Fatal("career source job ACK", command, j.State)
	}
	id, store := j.JobID, j.StoreID
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			p.t.Fatal("career source job timeout", command)
		}
		p.pause()
		r := p.b.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_job", ViewID: view.ViewID, JobID: id})
		if !r.OK || r.Job == nil || r.Job.JobID != id || r.Job.View != view || r.Job.Kind != kind || r.Job.StoreID != store {
			p.t.Fatal("career source job correlation", r.Error)
		}
		j = *r.Job
	}
	if j.State != "done" || j.Error != "" {
		p.t.Fatal("career source job failed", kind, j.State, j.Error)
	}
	q.ViewID, q.JobID = view.ViewID, id
	if kind == "inventory" {
		if j.Page == nil || j.Roster != nil || j.Page.After != q.After {
			p.t.Fatal("career inventory source tuple")
		}
		q.After = j.Page.After
	} else if j.Roster == nil || j.Page != nil || j.Roster.FirstSlot != q.FirstSlot {
		p.t.Fatal("career roster source tuple")
	}
	r := p.reply(q)
	if !r.OK || r.JobID != id || r.View == nil || *r.View != view || r.StoreID != store {
		p.t.Fatal("career completed source job binding", r.Error)
	}
	if kind == "inventory" {
		if r.Page == nil || !reflect.DeepEqual(r.Page.Source, *j.Page) {
			p.t.Fatal("career inventory source changed")
		}
	} else if r.Roster == nil || !reflect.DeepEqual(r.Roster.Source, *j.Roster) {
		p.t.Fatal("career roster source changed")
	}
	p.lastJobID = id
	return r, j
}
func (p *careerAcceptance) observe(fixture, phase string, s pilot.ManifestSnapshot, oldView string) {
	p.t.Helper()
	r := newResultAcceptance(p.t, p.ctx, p.b).captureSnapshot()
	snap := r.Snapshot
	if snap == nil || snap.State != "ready" || snap.Pin == nil || snap.View == nil || *snap.View != *s.View {
		p.t.Fatal("career source not ready", r.Error)
	}
	if !reflect.DeepEqual(*snap.Pin, p.prior.pin) || snap.StoreID != p.prior.storeID || snap.CreatedAtMS != p.prior.createdMS || snap.SourceCreatedAtMS != p.prior.sourceCreatedMS {
		p.t.Fatal("career activation changed publication authority")
	}
	stable := p.b.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !stable.OK || stable.Snapshot == nil || stable.Snapshot.Status == nil || stable.Snapshot.View == nil || *stable.Snapshot.View != *s.View {
		p.t.Fatal("career stable binding", stable.Error)
	}
	inv, invJob := p.page(pilot.CareerRequest{Command: "career_inventory", Limit: 8}, *s.View)
	roster, rosterJob := p.page(pilot.CareerRequest{Command: "career_roster", FirstSlot: 1, Limit: 8}, *s.View)
	if !inv.OK || inv.Page == nil || !roster.OK || roster.Roster == nil {
		p.t.Fatal("normal career current pages", inv.Error, roster.Error)
	}
	validate := func(a careerstate.Activation, source []gymstable.Item, items []gymstable.CareerItem) {
		if p.activation.ActivationID == "" {
			p.activation = a
		}
		if a != p.activation || a.ResultStoreID != snap.StoreID || a.ResultCreatedAtMS != snap.CreatedAtMS || a.SourceCreatedAtMS != snap.SourceCreatedAtMS || len(items) == 0 || len(items) != len(source) {
			p.t.Fatal("career activation/source identity")
		}
		for i, x := range items {
			if x.ProspectID != source[i].Assignment.ProspectID || x.Totals != (fightercareer.Totals{}) || x.Condition.ProXP != 0 || source[i].Condition.ProXP != 0 {
				p.t.Fatal("normal calendar invented an award")
			}
		}
	}
	validate(inv.Page.Activation, inv.Page.Source.Items, inv.Page.Items)
	oldItems := make([]gymstable.Item, len(roster.Roster.Source.Items))
	for i, x := range roster.Roster.Source.Items {
		oldItems[i] = x.Item
	}
	validate(roster.Roster.Activation, oldItems, roster.Roster.Items)
	if inv.View == nil || roster.View == nil || *inv.View != *s.View || *roster.View != *s.View || inv.StoreID != stable.Snapshot.Status.StoreID || roster.StoreID != inv.StoreID || inv.Page.Source.Status.RosterRevision != stable.Snapshot.Status.RosterRevision || roster.Roster.Source.Status.Slots != stable.Snapshot.Status.Slots {
		p.t.Fatal("career page lost current original view/roster")
	}
	bad := p.b.CareerRequest(p.ctx, pilot.CareerRequest{Command: "career_inventory", ViewID: s.View.ViewID, JobID: rosterJob.JobID, Limit: 9})
	if bad.OK || bad.Error != "invalid_request" || bad.JobID != "" || bad.Page != nil || bad.Roster != nil {
		p.t.Fatal("oversized career page accepted")
	}
	stale := ""
	if oldView != "" {
		z := p.b.CareerRequest(p.ctx, pilot.CareerRequest{Command: "career_inventory", ViewID: oldView, JobID: rosterJob.JobID, Limit: 8})
		if z.OK || z.Error != "stale_view" || z.JobID != "" || z.Page != nil || z.Roster != nil {
			p.t.Fatal("old career View disclosed", z.Error)
		}
		stale = z.Error
	}
	after := newResultAcceptance(p.t, p.ctx, p.b).captureSnapshot().Snapshot
	if !reflect.DeepEqual(after, snap) {
		p.t.Fatal("career read/refusal created Result work or nodes")
	}
	current := p.manifest()
	p.unchanged(s, current)
	p.unchanged(p.baseline[fixture], current)
	p.unchanged(p.original[fixture], current)
	p.processes.capture(p.t, fixture, "career-"+phase, current.View.AccountID, current.View.AuthorityOrigin, false)
	p.evidence = append(p.evidence, map[string]any{"fixture": fixture, "phase": phase, "real_clock": true, "award_executed": false, "additional_mining": false, "result_snapshot": snap, "stable_snapshot": stable.Snapshot, "account_before": s, "account_after": current, "inventory": inv, "roster": roster, "inventory_source_job": invJob, "roster_source_job": rosterJob, "invalid_limit_refusal": bad.Error, "old_view_id": oldView, "stale_view_refusal": stale, "after_refusals": after})
}
func (p *careerAcceptance) logout(fixture string, s pilot.ManifestSnapshot) {
	p.t.Helper()
	if _, e := p.b.Request(p.ctx, "logout", 0); e != nil {
		p.t.Fatal(e)
	}
	p.waitAccount("signed_out")
	r := p.b.CareerRequest(p.ctx, pilot.CareerRequest{Command: "career_inventory", ViewID: s.View.ViewID, JobID: p.lastJobID, Limit: 8})
	if r.OK || r.JobID != "" || r.View != nil || r.Page != nil || r.Roster != nil || !oneOf(r.Error, "stale_view", "career_inactive", "unauthorized") {
		p.t.Fatal("detached career disclosure", r.Error)
	}
	p.detached = append(p.detached, map[string]any{"fixture": fixture, "view_id": s.View.ViewID, "refusal": r})
}
func runCareerPackageAcceptance(t *testing.T, ctx context.Context, helper, data, node string, prior *resultAcceptance, original map[string]pilot.ManifestSnapshot) map[string]any {
	t.Helper()
	if prior == nil || prior.authority == nil || len(prior.evidence) != 6 || prior.restartEvidence == nil {
		t.Fatal("complete original Result phases required before career activation")
	}
	if len(original) != 2 {
		t.Fatal("original pre-activation fixture counters missing")
	}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s, ok := original[fixture]
		if !ok || s.View == nil || s.Account.State != "signed_in" || s.State != "stopped" || s.WalletState != "closed" {
			t.Fatal("invalid pre-activation counter observation", fixture)
		}
	}
	// The preceding controlled fixture flow dispatched no Result jobs. Four ready
	// observations each refused all missing-source actions before any preparation.
	for _, row := range append(append([]map[string]any{}, prior.evidence...), prior.restartEvidence["evidence"].([]map[string]any)...) {
		if row["publication_executed"] != false || row["proof_export_executed"] != false {
			t.Fatal("normal result flow finalized before activation")
		}
		if row["snapshot"].(*pilot.ResultSnapshot).State == "ready" {
			for _, key := range []string{"status_refusal", "record_refusal", "publish_refusal", "reconcile_refusal", "attest_refusal", "export_refusal"} {
				if row[key] != "result_source_incomplete" {
					t.Fatal("nonempty result source before activation", key)
				}
			}
		}
	}
	p := &careerAcceptance{t: t, ctx: ctx, helper: helper, data: data, node: node, prior: prior.authority, original: original, baseline: map[string]pilot.ManifestSnapshot{}, evidence: []map[string]any{}, detached: []map[string]any{}}
	defer func() {
		if p.b != nil {
			_ = p.b.Close()
			if p.processes != nil {
				p.processes.checkClosed(t)
			}
		}
	}()
	p.open()
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := p.login(fixture)
		p.observe(fixture, "initial", s, "")
		p.logout(fixture, s)
		reopened := p.login(fixture)
		p.observe(fixture, "reopened", reopened, s.View.ViewID)
		if _, e := p.b.Request(ctx, "reset_fixture", 0); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(45 * time.Second)
		var held pilot.ManifestSnapshot
		for time.Now().Before(deadline) {
			held = p.waitAccount("signed_in")
			if held.Account.Held && held.View.ViewID != reopened.View.ViewID && held.View.Generation > reopened.View.Generation {
				break
			}
			p.pause()
		}
		if !held.Account.Held || held.View.ViewID == reopened.View.ViewID {
			t.Fatal("career reset did not revoke original View")
		}
		p.observe(fixture, "held", held, reopened.View.ViewID)
		p.logout(fixture, held)
	}
	cleanup := p.close()
	rows := p.evidence
	p.evidence = nil
	p.open()
	s := p.login("zero256")
	p.observe("zero256", "process_restart", s, "")
	restartRows := p.evidence
	restartCleanup := p.close()
	// Old Result constructors must refuse the v9 parent, rather than re-open it
	// without the durable career guards. The child is joined before return.
	oldctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(oldctx, helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--candidate-results", "--data-dir", data, "--node-binary", node)
	e := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || oldctx.Err() != nil || cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatal("old result route did not fail closed on career parent", e)
	}
	return map[string]any{"scope": "prospective_candidate_career", "activation_mode": "explicit_upgrade_after_result_fixture_flow", "real_clock_closed_coverage": false, "award_executed": false, "additional_mining": false, "activation": p.activation, "rows": rows, "detached": p.detached, "original_baseline": original, "baseline": p.baseline, "cleanup": cleanup, "startup_ms": p.startups, "startup_admission_budget_ms": 25000, "native_startup_bound_ms": 30000, "metadata_budget_ms": 500, "legacy_result_route_refused": true, "legacy_result_exit_code": cmd.ProcessState.ExitCode(), "restart": map[string]any{"status": "passed", "same_activation": true, "no_automatic_awards": true, "no_automatic_nodes": true, "real_clock_closed_coverage": false, "evidence": restartRows, "cleanup": restartCleanup}, "limits": "Inherited fixture flow mines isolated normal-PoW candidates; the additional Career phase does not mine or publish. No real-calendar closed result, positive award, GUI or production authority is claimed."}
}
