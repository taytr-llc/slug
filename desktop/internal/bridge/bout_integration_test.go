//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
)

// Normal-clock package acceptance cannot manufacture a closed, finalized source.
// Successful saved transcripts are covered separately by the explicit normal-PoW
// scoped-clock source pipeline. This acceptance cannot invent a booked BoutID.
type boutAcceptance struct {
	t               *testing.T
	ctx             context.Context
	b               *StableBridge
	authority       *boutAuthority
	evidence        []map[string]any
	restartEvidence map[string]any
}

func newBoutAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *boutAcceptance {
	return &boutAcceptance{t: t, ctx: ctx, b: b, evidence: []map[string]any{}}
}
func (p *boutAcceptance) reply(q pilot.BoutRequest) pilot.BoutResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.BoutRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		select {
		case <-p.ctx.Done():
			p.t.Fatal(p.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	p.t.Fatal("bout request remained busy", q.Command)
	return pilot.BoutResponse{}
}
func (p *boutAcceptance) request(q pilot.BoutRequest) pilot.BoutResponse {
	p.t.Helper()
	r := p.reply(q)
	if !r.OK {
		p.t.Fatal(q.Command, r.Error)
	}
	return r
}
func (p *boutAcceptance) refusedRun(view, date, bid string) pilot.BoutJob {
	p.t.Helper()
	r := p.request(pilot.BoutRequest{Command: "bout_run", ViewID: view, Saturday: date, BoutID: bid})
	if r.Job == nil || r.Job.State != "queued" {
		p.t.Fatal("missing queued bout Run ACK")
	}
	j := *r.Job
	deadline := time.Now().Add(45 * time.Second)
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			p.t.Fatal("bout refusal timeout")
		}
		time.Sleep(50 * time.Millisecond)
		r = p.request(pilot.BoutRequest{Command: "bout_job", ViewID: view, JobID: j.JobID})
		if r.Job == nil {
			p.t.Fatal("missing card Job")
		}
		j = *r.Job
	}
	if j.State != "failed" || j.Error != "bout_source_incomplete" || j.Result != nil {
		p.t.Fatal("bout refusal", j.State, j.Error)
	}
	return j
}
func (p *boutAcceptance) observe(fixture, phase string, s pilot.StableSnapshot) {
	p.t.Helper()
	snap := p.request(pilot.BoutRequest{Command: "bout_snapshot"}).Snapshot
	if snap == nil || snap.View == nil || s.View == nil || *snap.View != *s.View {
		p.t.Fatal("bout lost original view")
	}
	row := map[string]any{"fixture": fixture, "phase": phase, "snapshot": snap, "real_clock": true, "run_succeeded": false, "closed_capture_executed": false, "publication_executed": false}
	if snap.State == "setup_incomplete" {
		if fixture != "zero256" || p.authority != nil || snap.Pin != nil || snap.StoreID != "" || snap.CreatedAtMS != 0 || snap.SourceCreatedAtMS != 0 {
			p.t.Fatal("bout setup rollback")
		}
		p.evidence = append(p.evidence, row)
		return
	}
	if snap.Pin == nil || !oneOf(snap.State, "ready", "busy") || snap.SourceCreatedAtMS <= 0 || snap.CreatedAtMS < snap.SourceCreatedAtMS {
		p.t.Fatal("bout not ready", snap.State)
	}
	a := boutAuthority{pin: *snap.Pin, storeID: snap.StoreID, createdMS: snap.CreatedAtMS, sourceCreatedMS: snap.SourceCreatedAtMS}
	if p.authority != nil {
		old, _ := gymbout.MarshalPin(p.authority.pin)
		current, _ := gymbout.MarshalPin(a.pin)
		if string(old) != string(current) || p.authority.storeID != a.storeID || p.authority.createdMS != a.createdMS || p.authority.sourceCreatedMS != a.sourceCreatedMS {
			p.t.Fatal("bout authority changed")
		}
	}
	p.authority = &a
	view := s.View.ViewID
	up := newEntryAcceptance(p.t, p.ctx, p.b).request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil {
		p.t.Fatal("missing upcoming Saturday")
	}
	status := p.request(pilot.BoutRequest{Command: "bout_card_status", ViewID: view, Saturday: up.Event.Saturday}).CardStatus
	if status == nil || status.Source.CardExists || status.Source.Usable || len(status.Bouts) != 0 {
		p.t.Fatal("package fabricated booked bouts")
	}
	bid := strings.Repeat("a", 64)
	future := p.refusedRun(view, up.Event.Saturday, bid)
	missingStatus := p.reply(pilot.BoutRequest{Command: "bout_status", ViewID: view, Saturday: up.Event.Saturday, BoutID: bid})
	missing := p.reply(pilot.BoutRequest{Command: "bout_record", ViewID: view, Saturday: up.Event.Saturday, BoutID: bid})
	if missingStatus.OK || missingStatus.Error != "bout_not_found" || missing.OK || missing.Error != "bout_not_found" {
		p.t.Fatal("missing booked transcript", missingStatus.Error, missing.Error)
	}
	portrait := p.b.BoutPortrait(p.ctx, view, up.Event.Saturday, bid, strings.Repeat("b", 64))
	if portrait.OK || portrait.Error != "bout_not_found" || portrait.Portrait != nil || portrait.Status != nil {
		p.t.Fatal("invented portrait authority", portrait.Error)
	}
	row["card_status"], row["run_refusal"], row["selected_status_refusal"], row["record_refusal"], row["portrait_refusal"] = status, future, missingStatus.Error, missing.Error, portrait.Error
	p.evidence = append(p.evidence, row)
}
func (p *boutAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.BoutRequest(p.ctx, pilot.BoutRequest{Command: "bout_snapshot"})
	if r.OK && (r.View != nil || r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
		p.t.Fatal("detached card leaked account presentation")
	}
}
func (p *boutAcceptance) checkRestart(b *StableBridge, s pilot.StableSnapshot) {
	p.t.Helper()
	if p.authority == nil {
		p.t.Fatal("missing card authority")
	}
	next := newBoutAcceptance(p.t, p.ctx, b)
	next.authority = p.authority
	next.observe("zero256", "process_restart", s)
	p.restartEvidence = map[string]any{"status": "passed", "same_pin_and_creation_time": true, "no_automatic_run": true, "real_clock_closed_coverage": false, "evidence": next.evidence}
}
