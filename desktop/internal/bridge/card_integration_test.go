//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymcard"
)

// Normal-clock package acceptance cannot manufacture a closed, finalized source.
// Successful booking is covered separately by the explicit normal-PoW test-clock
// pipeline and the final-source retained-journal native replay.
type cardAcceptance struct {
	t               *testing.T
	ctx             context.Context
	b               *StableBridge
	authority       *cardAuthority
	evidence        []map[string]any
	restartEvidence map[string]any
}

func newCardAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *cardAcceptance {
	return &cardAcceptance{t: t, ctx: ctx, b: b, evidence: []map[string]any{}}
}
func (p *cardAcceptance) reply(q pilot.CardRequest) pilot.CardResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.CardRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		select {
		case <-p.ctx.Done():
			p.t.Fatal(p.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	p.t.Fatal("card request remained busy", q.Command)
	return pilot.CardResponse{}
}
func (p *cardAcceptance) request(q pilot.CardRequest) pilot.CardResponse {
	p.t.Helper()
	r := p.reply(q)
	if !r.OK {
		p.t.Fatal(q.Command, r.Error)
	}
	return r
}
func (p *cardAcceptance) refusedBuild(view, date string) pilot.CardJob {
	p.t.Helper()
	r := p.request(pilot.CardRequest{Command: "card_build", ViewID: view, Saturday: date})
	if r.Job == nil || r.Job.State != "queued" {
		p.t.Fatal("missing queued card Build ACK")
	}
	j := *r.Job
	deadline := time.Now().Add(45 * time.Second)
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			p.t.Fatal("card refusal timeout")
		}
		time.Sleep(50 * time.Millisecond)
		r = p.request(pilot.CardRequest{Command: "card_job", ViewID: view, JobID: j.JobID})
		if r.Job == nil {
			p.t.Fatal("missing card Job")
		}
		j = *r.Job
	}
	if j.State != "failed" || j.Error != "card_source_incomplete" || j.Result != nil {
		p.t.Fatal("card refusal", j.State, j.Error)
	}
	return j
}
func (p *cardAcceptance) observe(fixture, phase string, s pilot.StableSnapshot) {
	p.t.Helper()
	snap := p.request(pilot.CardRequest{Command: "card_snapshot"}).Snapshot
	if snap == nil || snap.View == nil || s.View == nil || *snap.View != *s.View {
		p.t.Fatal("card lost original view")
	}
	row := map[string]any{"fixture": fixture, "phase": phase, "snapshot": snap, "real_clock": true, "build_succeeded": false, "closed_capture_executed": false, "publication_executed": false}
	if snap.State == "setup_incomplete" {
		if fixture != "zero256" || p.authority != nil || snap.Pin != nil || snap.StoreID != "" || snap.CreatedAtMS != 0 || snap.SourceCreatedAtMS != 0 {
			p.t.Fatal("card setup rollback")
		}
		p.evidence = append(p.evidence, row)
		return
	}
	if snap.Pin == nil || !oneOf(snap.State, "ready", "busy") || snap.SourceCreatedAtMS <= 0 || snap.CreatedAtMS < snap.SourceCreatedAtMS {
		p.t.Fatal("card not ready", snap.State)
	}
	a := cardAuthority{pin: *snap.Pin, storeID: snap.StoreID, createdMS: snap.CreatedAtMS, sourceCreatedMS: snap.SourceCreatedAtMS}
	if p.authority != nil {
		old, _ := gymcard.MarshalPin(p.authority.pin)
		current, _ := gymcard.MarshalPin(a.pin)
		if string(old) != string(current) || p.authority.storeID != a.storeID || p.authority.createdMS != a.createdMS || p.authority.sourceCreatedMS != a.sourceCreatedMS {
			p.t.Fatal("card authority changed")
		}
	}
	p.authority = &a
	view := s.View.ViewID
	up := newEntryAcceptance(p.t, p.ctx, p.b).request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil {
		p.t.Fatal("missing upcoming Saturday")
	}
	status := p.request(pilot.CardRequest{Command: "card_status", ViewID: view, Saturday: up.Event.Saturday}).Status
	if status == nil || status.Exists || status.Usable || status.Counts != (gymcard.Counts{}) || status.BoutCount != 0 {
		p.t.Fatal("package fabricated card")
	}
	future := p.refusedBuild(view, up.Event.Saturday)
	missing := p.reply(pilot.CardRequest{Command: "card_record", ViewID: view, Saturday: up.Event.Saturday})
	if missing.OK || missing.Error != "card_not_found" {
		p.t.Fatal("missing card record", missing.Error)
	}
	history := p.request(pilot.CardRequest{Command: "card_history", ViewID: view, Limit: 8}).History
	if history == nil || len(history.Cards) != 0 || history.HasMore || history.NextCardID != "" {
		p.t.Fatal("refused card Build allocated record")
	}
	row["status"], row["future_refusal"], row["record_refusal"], row["history"] = status, future, missing.Error, history
	p.evidence = append(p.evidence, row)
}
func (p *cardAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.CardRequest(p.ctx, pilot.CardRequest{Command: "card_snapshot"})
	if r.OK && (r.View != nil || r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
		p.t.Fatal("detached card leaked account presentation")
	}
}
func (p *cardAcceptance) checkRestart(b *StableBridge, s pilot.StableSnapshot) {
	p.t.Helper()
	if p.authority == nil {
		p.t.Fatal("missing card authority")
	}
	next := newCardAcceptance(p.t, p.ctx, b)
	next.authority = p.authority
	next.observe("zero256", "process_restart", s)
	p.restartEvidence = map[string]any{"status": "passed", "same_pin_and_creation_time": true, "no_automatic_booking": true, "real_clock_closed_coverage": false, "evidence": next.evidence}
}
