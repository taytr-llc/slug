//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymseed"
)

// Real-clock package acceptance cannot manufacture an elapsed Saturday cutoff.
// Actual closed publication/finality/reorg is separately exercised with normal
// PoW and the explicit non-release test clock.
type seedAcceptance struct {
	t               *testing.T
	ctx             context.Context
	b               *StableBridge
	authority       *seedAuthority
	evidence        []map[string]any
	restartEvidence map[string]any
}

func newSeedAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *seedAcceptance {
	return &seedAcceptance{t: t, ctx: ctx, b: b, evidence: []map[string]any{}}
}
func (p *seedAcceptance) reply(q pilot.SeedRequest) pilot.SeedResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.SeedRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		select {
		case <-p.ctx.Done():
			p.t.Fatal(p.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	p.t.Fatal("seed request remained busy", q.Command)
	return pilot.SeedResponse{}
}
func (p *seedAcceptance) request(q pilot.SeedRequest) pilot.SeedResponse {
	p.t.Helper()
	r := p.reply(q)
	if !r.OK {
		p.t.Fatal(q.Command, r.Error)
	}
	return r
}
func (p *seedAcceptance) refusedJob(command, view, date, code string) pilot.SeedJob {
	p.t.Helper()
	r := p.request(pilot.SeedRequest{Command: command, ViewID: view, Saturday: date})
	if r.Job == nil {
		p.t.Fatal("missing seed Begin ACK")
	}
	j := *r.Job
	deadline := time.Now().Add(45 * time.Second)
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			p.t.Fatal("seed refusal timeout")
		}
		time.Sleep(50 * time.Millisecond)
		r = p.request(pilot.SeedRequest{Command: "seed_job", ViewID: view, JobID: j.JobID})
		if r.Job == nil {
			p.t.Fatal("missing seed Job")
		}
		j = *r.Job
	}
	if j.State != "failed" || j.Error != code || j.Result != nil {
		p.t.Fatal("seed refusal", command, j.State, j.Error, code)
	}
	return j
}
func (p *seedAcceptance) observe(fixture, phase string, s pilot.StableSnapshot) {
	p.t.Helper()
	r := p.request(pilot.SeedRequest{Command: "seed_snapshot"})
	snap := r.Snapshot
	if snap == nil || snap.View == nil || s.View == nil || *snap.View != *s.View {
		p.t.Fatal("seed lost original view")
	}
	row := map[string]any{"fixture": fixture, "phase": phase, "snapshot": snap, "real_clock": true, "publication_executed": false, "closed_capture_executed": false}
	if snap.State == "setup_incomplete" {
		if fixture != "zero256" || p.authority != nil || snap.Pin != nil || snap.StoreID != "" {
			p.t.Fatal("seed setup rollback")
		}
		p.evidence = append(p.evidence, row)
		return
	}
	if snap.Pin == nil || snap.CreatedAtMS <= 0 || !oneOf(snap.State, "ready", "busy") {
		p.t.Fatal("seed not ready", snap.State)
	}
	a := seedAuthority{pin: *snap.Pin, storeID: snap.StoreID, createdMS: snap.CreatedAtMS}
	if p.authority != nil {
		old, _ := gymseed.MarshalPin(p.authority.pin)
		current, _ := gymseed.MarshalPin(a.pin)
		if p.authority.storeID != a.storeID || p.authority.createdMS != a.createdMS || string(old) != string(current) {
			p.t.Fatal("seed authority changed")
		}
	}
	p.authority = &a
	view := s.View.ViewID
	entry := newEntryAcceptance(p.t, p.ctx, p.b)
	up := entry.request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil {
		p.t.Fatal("missing upcoming Saturday")
	}
	status := p.request(pilot.SeedRequest{Command: "seed_status", ViewID: view, Saturday: up.Event.Saturday}).Status
	if status == nil || status.Exists || status.Phase != "absent" || status.ChainObservedAtMS != 0 || status.CurrentFinality || status.CheckpointPresent {
		p.t.Fatal("package fabricated seed evidence")
	}
	future := p.refusedJob("seed_prepare", view, up.Event.Saturday, "seed_source_incomplete")
	date, e := time.Parse("2006-01-02", up.Event.Saturday)
	if e != nil {
		p.t.Fatal(e)
	}
	late := p.refusedJob("seed_prepare", view, date.AddDate(0, 0, -7).Format("2006-01-02"), "seed_created_too_late")
	history := p.request(pilot.SeedRequest{Command: "seed_history", ViewID: view, Limit: 8}).History
	if history == nil || len(history.Events) != 0 || history.HasMore {
		p.t.Fatal("refused seed action allocated record")
	}
	missing := p.reply(pilot.SeedRequest{Command: "seed_record", ViewID: view, Saturday: up.Event.Saturday})
	if missing.OK || missing.Error != "seed_not_found" {
		p.t.Fatal("missing seed record", missing.Error)
	}
	row["status"], row["future_refusal"], row["historical_refusal"], row["history"], row["record_refusal"] = status, future, late, history, missing.Error
	if fixture == "incrementing256" || phase == "held" || phase == "process_restart" {
		row["publish_refusal"] = p.refusedJob("seed_publish", view, up.Event.Saturday, "forbidden")
		row["attest_refusal"] = p.refusedJob("seed_attest", view, up.Event.Saturday, "forbidden")
	}
	p.evidence = append(p.evidence, row)
}
func (p *seedAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.SeedRequest(p.ctx, pilot.SeedRequest{Command: "seed_snapshot"})
	if r.OK && (r.View != nil || r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
		p.t.Fatal("detached seed leaked account presentation")
	}
}
func (p *seedAcceptance) checkRestart(b *StableBridge, s pilot.StableSnapshot) {
	p.t.Helper()
	if p.authority == nil {
		p.t.Fatal("missing seed authority")
	}
	next := newSeedAcceptance(p.t, p.ctx, b)
	next.authority = p.authority
	next.observe("zero256", "process_restart", s)
	p.restartEvidence = map[string]any{"status": "passed", "same_pin_and_creation_time": true, "no_automatic_publication": true, "real_clock_closed_coverage": false, "evidence": next.evidence}
}
