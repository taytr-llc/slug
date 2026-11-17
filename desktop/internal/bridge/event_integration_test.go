//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymevent"
)

// This extends the exact normal packaged-helper flow. It uses real clocks,
// fresh public-fixture gyms and their actual mined records. A closed-cutoff
// success cannot be demonstrated before the next eligible real Saturday.
type eventAcceptance struct {
	t               *testing.T
	ctx             context.Context
	b               *StableBridge
	authority       *eventAuthority
	evidence        []map[string]any
	restartEvidence map[string]any
}

func newEventAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *eventAcceptance {
	return &eventAcceptance{t: t, ctx: ctx, b: b, evidence: []map[string]any{}}
}
func (p *eventAcceptance) reply(q pilot.EventRequest) pilot.EventResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.EventRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		select {
		case <-p.ctx.Done():
			p.t.Fatal(p.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	p.t.Fatal("event request remained busy", q.Command)
	return pilot.EventResponse{}
}
func (p *eventAcceptance) request(q pilot.EventRequest) pilot.EventResponse {
	p.t.Helper()
	r := p.reply(q)
	if !r.OK {
		p.t.Fatal(q.Command, r.Error)
	}
	return r
}
func (p *eventAcceptance) refuseCapture(view, saturday, code string) pilot.EventJob {
	p.t.Helper()
	r := p.request(pilot.EventRequest{Command: "event_capture", ViewID: view, Saturday: saturday})
	if r.Job == nil {
		p.t.Fatal("missing capture job")
	}
	j := *r.Job
	deadline := time.Now().Add(45 * time.Second)
	for j.State == "queued" || j.State == "running" {
		if time.Now().After(deadline) {
			p.t.Fatal("refusal job did not finish")
		}
		time.Sleep(50 * time.Millisecond)
		r = p.request(pilot.EventRequest{Command: "event_job", ViewID: view, JobID: j.JobID})
		if r.Job == nil {
			p.t.Fatal("missing refusal job result")
		}
		j = *r.Job
	}
	if j.State != "failed" || j.Error != code || j.Result != nil {
		p.t.Fatal("incorrect real-clock refusal", j.State, j.Error, code)
	}
	return j
}
func (p *eventAcceptance) observe(fixture, phase string, s pilot.StableSnapshot) {
	p.t.Helper()
	r := p.request(pilot.EventRequest{Command: "event_snapshot"})
	if r.Snapshot == nil || r.Snapshot.View == nil || s.View == nil || *r.Snapshot.View != *s.View {
		p.t.Fatal("event attach lost original View")
	}
	snap := *r.Snapshot
	row := map[string]any{"fixture": fixture, "phase": phase, "snapshot": snap, "real_clock": true, "closed_capture_executed": false}
	if snap.State == "setup_incomplete" {
		if fixture != "zero256" || p.authority != nil || snap.Pin != nil || snap.StoreID != "" {
			p.t.Fatal("unexpected missing participant or coordinator rollback")
		}
		p.evidence = append(p.evidence, row)
		return
	}
	if snap.Pin == nil || snap.CoordinatorCreatedAtMS <= 0 || !oneOf(snap.State, "ready", "busy") {
		p.t.Fatal("event coordinator not ready", snap.State)
	}
	a := eventAuthority{pin: *snap.Pin, storeID: snap.StoreID, createdMS: snap.CoordinatorCreatedAtMS}
	if p.authority != nil && (p.authority.storeID != a.storeID || p.authority.createdMS != a.createdMS || !equalEventPin(p.authority.pin, a.pin)) {
		p.t.Fatal("coordinator changed after account transition")
	}
	p.authority = &a
	view := s.View.ViewID
	entry := newEntryAcceptance(p.t, p.ctx, p.b)
	up := entry.request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil {
		p.t.Fatal("missing authority Saturday")
	}
	status := p.request(pilot.EventRequest{Command: "event_status", ViewID: view, Saturday: up.Event.Saturday}).Status
	if status == nil || status.Exists || status.Phase != "before_cutoff" || status.ObservedAtMS >= status.Event.CutoffMS || status.SeedAuthority != "unavailable" {
		p.t.Fatal("normal package fabricated a completed cutoff")
	}
	future := p.refuseCapture(view, up.Event.Saturday, "event_not_due")
	day, err := time.Parse("2006-01-02", up.Event.Saturday)
	if err != nil {
		p.t.Fatal(err)
	}
	previous := day.AddDate(0, 0, -7).Format("2006-01-02")
	old, err := gymevent.EventForDate(a.pin, previous)
	if err != nil || old.CutoffMS > a.createdMS {
		p.t.Fatal("expected previous real lock before fresh installation", err)
	}
	late := p.refuseCapture(view, previous, "event_created_too_late")
	history := p.request(pilot.EventRequest{Command: "event_history", ViewID: view, Limit: 8}).History
	if history == nil || len(history.Events) != 0 || history.HasMore {
		p.t.Fatal("refused captures created historical events")
	}
	missing := p.reply(pilot.EventRequest{Command: "event_record", ViewID: view, Saturday: up.Event.Saturday})
	if missing.OK || missing.Error != "event_not_found" {
		p.t.Fatal("missing event supplied a record", missing.Error)
	}
	row["status"], row["future_refusal"], row["historical_refusal"], row["history"] = status, future, late, history
	row["record_refusal"] = missing.Error
	p.evidence = append(p.evidence, row)
}
func (p *eventAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.EventRequest(p.ctx, pilot.EventRequest{Command: "event_snapshot"})
	if r.OK && (r.View != nil || r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
		p.t.Fatal("signed-out event disclosed account-bound presentation")
	}
}
func (p *eventAcceptance) checkRestart(b *StableBridge, s pilot.StableSnapshot) {
	p.t.Helper()
	if p.authority == nil {
		p.t.Fatal("missing original shared coordinator")
	}
	restarted := newEventAcceptance(p.t, p.ctx, b)
	restarted.authority = p.authority
	restarted.observe("zero256", "process_restart", s)
	p.restartEvidence = map[string]any{"status": "passed", "same_pin_and_creation_time": true, "no_automatic_capture": true, "real_clock_closed_coverage": false, "evidence": restarted.evidence}
}
