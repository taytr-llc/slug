//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

// Optional set42 coverage is enabled only by SLUG_PREVIEW_ACCEPTANCE=1 in the
// existing genuine-work helper flow. It never edits receipt/class/birth inputs.
// No native acceptance assertion substitutes for independent engine calibration.
type previewAcceptance struct {
	t            *testing.T
	ctx          context.Context
	bridge       *StableBridge
	pairs        map[string][2]gymstable.Item
	latest       map[string]pilot.PreviewJob
	evidence     []map[string]any
	restartCheck func(*StableBridge, pilot.StableSnapshot) any
}

func newPreviewAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *previewAcceptance {
	return &previewAcceptance{t: t, ctx: ctx, bridge: b, pairs: map[string][2]gymstable.Item{}, latest: map[string]pilot.PreviewJob{}, evidence: []map[string]any{}}
}
func (p *previewAcceptance) pause() {
	p.t.Helper()
	select {
	case <-p.ctx.Done():
		p.t.Fatal(p.ctx.Err())
	case <-time.After(100 * time.Millisecond):
	}
}
func (p *previewAcceptance) manifest() pilot.ManifestSnapshot {
	p.t.Helper()
	r, e := p.bridge.Request(p.ctx, "snapshot", 0)
	if e != nil || r.Snapshot == nil {
		p.t.Fatal("preview manifest snapshot", e)
	}
	return *r.Snapshot
}
func (p *previewAcceptance) stable() pilot.StableSnapshot {
	p.t.Helper()
	r := p.bridge.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !r.OK || r.Snapshot == nil || r.Snapshot.View == nil || r.Snapshot.Status == nil {
		p.t.Fatal("preview stable snapshot", r.Error)
	}
	return *r.Snapshot
}
func (p *previewAcceptance) preview() pilot.PreviewSnapshot {
	p.t.Helper()
	r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_snapshot"})
	if !r.OK || r.Snapshot == nil || r.Snapshot.View == nil {
		p.t.Fatal("preview snapshot", r.Error)
	}
	return *r.Snapshot
}
func (p *previewAcceptance) selectOne(view string, item gymstable.Item) gymstable.Page {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var job *pilot.StableJob
	for time.Now().Before(deadline) {
		if job == nil {
			r := p.bridge.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_inventory", ViewID: view, After: item.Assignment.Sequence - 1, Limit: 1})
			if !r.OK {
				if r.Error == "busy" {
					p.pause()
					continue
				}
				p.t.Fatal("select preview member", r.Error)
			}
			job = r.Job
		} else if job.State == "queued" || job.State == "running" {
			p.pause()
			r := p.bridge.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_job", ViewID: view, JobID: job.JobID})
			if !r.OK {
				p.t.Fatal("preview member page poll", r.Error)
			}
			job = r.Job
		} else {
			break
		}
	}
	if job == nil || job.State != "done" || job.Page == nil || len(job.Page.Items) != 1 || job.Page.Items[0].Assignment != item.Assignment {
		p.t.Fatal("preview page selection did not preserve exact assignment")
	}
	return *job.Page
}
func (p *previewAcceptance) run(fixture, phase string, page gymstable.Page) {
	p.t.Helper()
	p.t.Log("unrecorded preview", fixture, phase)
	pair, found := p.pairs[fixture]
	if !found {
		for i, a := range page.Items {
			for _, b := range page.Items[i+1:] {
				if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
					pair = [2]gymstable.Item{a, b}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			p.t.Fatal("six genuine candidates did not contain a same-class pair")
		}
		if pair[1].Assignment.ProspectID < pair[0].Assignment.ProspectID {
			pair[0], pair[1] = pair[1], pair[0]
		}
		p.pairs[fixture] = pair
	}
	stable := p.stable()
	view := stable.View.ViewID
	selectedA := p.selectOne(view, pair[0])
	selectedB := p.selectOne(view, pair[1])
	// A is deliberately no longer in the old selected-page appearance cache.
	if _, err := p.bridge.Request(p.ctx, "manifest_portrait", pair[0].Assignment.Sequence); err == nil {
		p.t.Fatal("cross-page control unexpectedly retained the first old-cache portrait")
	}
	before := p.stable()
	walletBefore := p.manifest()
	snap := p.preview()
	if snap.View.ViewID != view || snap.StoreID != before.Status.StoreID || walletBefore.State != "stopped" {
		p.t.Fatal("preview not attached to stopped current view")
	}
	var job *pilot.PreviewJob
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// Reverse the selection order to exercise typed native canonicalization.
		r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_begin", ViewID: view, FighterAID: pair[1].Assignment.ProspectID, FighterBID: pair[0].Assignment.ProspectID})
		if r.OK {
			job = r.Job
			break
		}
		if r.Error != "busy" {
			p.t.Fatal("preview begin", r.Error)
		}
		p.pause()
	}
	if job == nil || job.State != "queued" || job.Result != nil || job.FighterAID != pair[0].Assignment.ProspectID || job.FighterBID != pair[1].Assignment.ProspectID {
		p.t.Fatal("preview ACK was not an exact queued pair")
	}
	ack := *job
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) && (job.State == "queued" || job.State == "running") {
		p.pause()
		r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: view, JobID: job.JobID})
		if !r.OK || r.Job == nil {
			p.t.Fatal("preview poll", r.Error)
		}
		job = r.Job
	}
	if job.State != "done" || job.Result == nil {
		p.t.Fatal("preview did not complete", job.State, job.Error)
	}
	result := job.Result
	if result.Pair.AsOfMS != result.Pair.Status.LastAsOfMS || result.Pair.Status.StoreID != before.Status.StoreID {
		p.t.Fatal("preview pair observation binding")
	}
	for i, item := range result.Pair.Fighters {
		if item.Assignment != pair[i].Assignment || item.Manifest != pair[i].Manifest || item.Condition.ProXP != 0 || item.Condition.AsOfMS != result.Pair.AsOfMS || item.Condition.BornAtMS != pair[i].Assignment.BornAtMS {
			p.t.Fatal("preview changed immutable member or conditions")
		}
	}
	for range 3 {
		r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: view, JobID: job.JobID})
		if !r.OK || r.Job == nil || !reflect.DeepEqual(*r.Job, *job) {
			p.t.Fatal("cached replay changed completed result", r.Error)
		}
	}
	portraits := [2]PreviewPortrait{}
	for i, item := range pair {
		r := p.bridge.FightPreviewPortrait(p.ctx, view, job.JobID, item.Assignment.ProspectID)
		if !r.OK || r.Portrait == nil || r.Portrait.ProspectID != item.Assignment.ProspectID || r.Portrait.ResultDigest != result.EngineResultDigest || r.Portrait.ManifestDigest != item.Assignment.ManifestDigest || r.Portrait.PortraitDigest != item.Assignment.PortraitDigest {
			p.t.Fatal("native cross-page preview portrait", r.Error)
		}
		portraits[i] = *r.Portrait
		again := p.bridge.FightPreviewPortrait(p.ctx, view, job.JobID, item.Assignment.ProspectID)
		if !again.OK || again.Portrait == nil || *again.Portrait != portraits[i] {
			p.t.Fatal("fresh-authorized portrait cache hit", again.Error)
		}
	}
	// Preview may observe a later clock, but no inventory/roster/history state.
	after := p.stable()
	walletAfter := p.manifest()
	left, right := *before.Status, *after.Status
	left.LastAsOfMS, right.LastAsOfMS = 0, 0
	if left != right || walletBefore.Attempts != walletAfter.Attempts || walletBefore.AcceptedCount != walletAfter.AcceptedCount ||
		walletBefore.PayoutAddress != walletAfter.PayoutAddress || walletBefore.GymID != walletAfter.GymID || walletBefore.GymPublicKey != walletAfter.GymPublicKey ||
		walletBefore.SpendableAtoms != walletAfter.SpendableAtoms || walletBefore.ImmatureAtoms != walletAfter.ImmatureAtoms || walletAfter.State != "stopped" {
		p.t.Fatal("preview mutated stable, mining or wallet state")
	}
	if cached, err := p.bridge.Request(p.ctx, "manifest_receipt", pair[1].Assignment.Sequence); err != nil || cached.ManifestReceipt == nil || cached.ManifestReceipt.Record.Verified.ProspectID != pair[1].Assignment.ProspectID {
		p.t.Fatal("preview evicted protected selected-page evidence", err)
	}
	p.latest[fixture] = *job
	p.evidence = append(p.evidence, map[string]any{"fixture": fixture, "phase": phase, "selected_pages": [2]gymstable.Page{selectedA, selectedB}, "ack": ack, "job": *job, "portraits": portraits, "before": before, "after": after, "wallet_before": walletBefore, "wallet_after": walletAfter, "replay_identical": true, "cross_page_native_portraits": true, "protected_page_retained": true, "unrecorded": true})
}
func (p *previewAcceptance) refuseDetached(fixture string) {
	p.t.Helper()
	old := p.latest[fixture]
	r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: old.View.ViewID, JobID: old.JobID})
	if r.OK || r.Job != nil {
		p.t.Fatal("detached preview disclosed a result")
	}
	portrait := p.bridge.FightPreviewPortrait(p.ctx, old.View.ViewID, old.JobID, old.FighterAID)
	if portrait.OK || portrait.Portrait != nil {
		p.t.Fatal("detached preview disclosed a cached portrait")
	}
}
func (p *previewAcceptance) emptyReopen(fixture, view string) {
	p.t.Helper()
	old := p.latest[fixture]
	s := p.preview()
	if s.View.ViewID != view || s.State != "idle" || s.ActiveJobID != "" || s.QueuedJobID != "" || s.LastJobID != "" {
		p.t.Fatal("reopened view retained a preview cache")
	}
	for _, v := range []string{old.View.ViewID, view} {
		r := p.bridge.PreviewRequest(p.ctx, pilot.PreviewRequest{Command: "preview_job", ViewID: v, JobID: old.JobID})
		if r.OK || r.Job != nil {
			p.t.Fatal("old preview resurrected under a new view")
		}
		portrait := p.bridge.FightPreviewPortrait(p.ctx, v, old.JobID, old.FighterAID)
		if portrait.OK || portrait.Portrait != nil {
			p.t.Fatal("old preview portrait resurrected")
		}
	}
}
func (p *previewAcceptance) checkProcessRestart(helper, data, node string) map[string]any {
	p.t.Helper()
	launch := LaunchStable
	if p.bridge.entriesEnabled {
		launch = LaunchEntries
	}
	if p.bridge.eventsEnabled {
		launch = LaunchEvents
	}
	if p.bridge.seedsEnabled {
		launch = LaunchSeeds
	}
	if p.bridge.cardsEnabled {
		launch = LaunchCards
	}
	b, err := launch(helper, data, node)
	if err != nil {
		p.t.Fatal("restart preview helper", err)
	}
	inventory := newManifestProcessInventory(data, node, b.cmd.Process.Pid)
	closed := false
	defer func() {
		if err := b.Close(); err != nil {
			p.t.Error(err)
		}
		if !closed {
			inventory.checkClosed(p.t)
		}
	}()
	restarted := newPreviewAcceptance(p.t, p.ctx, b)
	restarted.latest = p.latest
	if _, err = b.RequestFixture(p.ctx, "zero256"); err != nil {
		p.t.Fatal("restart fixture login", err)
	}
	deadline := time.Now().Add(150 * time.Second)
	var manifest pilot.ManifestSnapshot
	var stable pilot.StableSnapshot
	for time.Now().Before(deadline) {
		manifest = restarted.manifest()
		if manifest.Account.State == "failed" {
			p.t.Fatal("restarted fixture failed", manifest.Account.LastError)
		}
		if manifest.Account.State == "signed_in" {
			r := b.StableRequest(p.ctx, pilot.StableRequest{Command: "stable_snapshot"})
			if r.OK && r.Snapshot != nil && r.Snapshot.View != nil && r.Snapshot.Status != nil && r.Snapshot.Status.CaughtUp && r.Snapshot.ActiveJobID == "" {
				stable = *r.Snapshot
				break
			}
		}
		restarted.pause()
	}
	if stable.View == nil || stable.Status == nil {
		p.t.Fatal("helper restart did not attach stable")
	}
	old := p.latest["zero256"]
	if stable.Status.StoreID != old.StoreID || stable.Status.RosterRevision != old.Result.Pair.Status.RosterRevision || stable.Status.Slots != old.Result.Pair.Status.Slots {
		p.t.Fatal("restart changed retained roster identity")
	}
	restarted.emptyReopen("zero256", stable.View.ViewID)
	var entryRestart any
	if p.restartCheck != nil {
		entryRestart = p.restartCheck(b, stable)
	}
	inventory.capture(p.t, "zero256", "preview-restart", manifest.Account.ID, stable.View.AuthorityOrigin, false)
	if err = b.Close(); err != nil {
		p.t.Fatal("restarted helper Close", err)
	}
	closed = true
	inventory.checkClosed(p.t)
	if b.cmd.ProcessState == nil || !b.cmd.ProcessState.Exited() || b.cmd.ProcessState.ExitCode() != 0 {
		p.t.Fatal("restarted helper exit")
	}
	return map[string]any{"status": "passed", "helper_pid": b.cmd.Process.Pid, "helper_exit": b.cmd.ProcessState.ExitCode(), "cleanup": inventory.evidence(), "preview_cache_absent": true, "retained_stable": stable, "entry_restart": entryRestart}
}
