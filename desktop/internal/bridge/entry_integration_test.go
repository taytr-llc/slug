//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

// Entry acceptance extends the genuine-work, two-fixture packaged flow. It
// never edits classes, births, clocks, receipts or the trusted source store.
type entryAcceptance struct {
	t        *testing.T
	ctx      context.Context
	b        *StableBridge
	saved    map[string]gymentry.Receipt
	pending  map[string]gymentry.Intent
	evidence []map[string]any
}

func newEntryAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *entryAcceptance {
	return &entryAcceptance{t: t, ctx: ctx, b: b, saved: map[string]gymentry.Receipt{}, pending: map[string]gymentry.Intent{}, evidence: []map[string]any{}}
}
func (p *entryAcceptance) pause() {
	p.t.Helper()
	select {
	case <-p.ctx.Done():
		p.t.Fatal(p.ctx.Err())
	case <-time.After(100 * time.Millisecond):
	}
}
func (p *entryAcceptance) request(q pilot.EntryRequest) pilot.EntryResponse {
	p.t.Helper()
	r := p.reply(q)
	if !r.OK {
		p.t.Fatal(q.Command, r.Error)
	}
	return r
}
func (p *entryAcceptance) reply(q pilot.EntryRequest) pilot.EntryResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.EntryRequest(p.ctx, q)
		if r.OK || r.Error != "busy" {
			return r
		}
		p.pause()
	}
	p.t.Fatal("entry remained busy", q.Command)
	return pilot.EntryResponse{}
}
func (p *entryAcceptance) job(q pilot.EntryRequest) pilot.EntryJob {
	p.t.Helper()
	r := p.request(q)
	if r.Job == nil {
		p.t.Fatal("entry job missing", q.Command)
	}
	j := *r.Job
	deadline := time.Now().Add(60 * time.Second)
	for j.State == "queued" || j.State == "running" {
		if !time.Now().Before(deadline) {
			p.t.Fatal("entry job timeout", q.Command)
		}
		p.pause()
		r = p.request(pilot.EntryRequest{Command: "entry_job", ViewID: q.ViewID, JobID: j.JobID})
		if r.Job == nil {
			p.t.Fatal("entry job poll missing")
		}
		j = *r.Job
	}
	if j.State != "complete" {
		p.t.Fatal("entry job failed", j.Kind, j.State, j.Error)
	}
	return j
}
func (p *entryAcceptance) attach(s pilot.StableSnapshot) string {
	p.t.Helper()
	if s.View == nil || s.Status == nil {
		p.t.Fatal("entry missing stable identity")
	}
	r := p.request(pilot.EntryRequest{Command: "entry_snapshot"})
	if r.Snapshot == nil || r.Snapshot.View == nil || *r.Snapshot.View != *s.View || r.Snapshot.State == "unavailable" {
		p.t.Fatal("entry did not attach")
	}
	return s.View.ViewID
}
func (p *entryAcceptance) prepare(view string, q gymentry.Request) gymentry.Intent {
	p.t.Helper()
	j := p.job(pilot.EntryRequest{Command: "entry_prepare", ViewID: view, Request: &q})
	if j.Intent == nil || !reflect.DeepEqual(j.Intent.Request, q) {
		p.t.Fatal("prepared selection changed")
	}
	return *j.Intent
}
func (p *entryAcceptance) apply(view string, i gymentry.Intent) gymentry.MutationResult {
	p.t.Helper()
	j := p.job(pilot.EntryRequest{Command: "entry_apply", ViewID: view, OperationID: i.OperationID})
	if j.Mutation == nil || j.Mutation.Receipt.OperationID != i.OperationID {
		p.t.Fatal("entry mutation missing")
	}
	return *j.Mutation
}
func (p *entryAcceptance) initial(fixture string, s pilot.StableSnapshot, item gymstable.Item) {
	p.t.Helper()
	view := p.attach(s)
	up := p.request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil || up.Exists || up.Event.CutoffMS <= up.ObservedAtMS {
		p.t.Fatal("invalid fresh upcoming event")
	}
	q := gymentry.Request{Version: 1, EventID: up.Event.EventID, Saturday: up.Event.Saturday, Action: "replace", ExpectedRosterRevision: s.Status.RosterRevision, ProspectIDs: []string{item.Assignment.ProspectID}}
	i := p.prepare(view, q)
	first := p.apply(view, i)
	if first.Replayed || first.Receipt.Revision != 1 || len(first.Receipt.Members) != 1 || first.Receipt.Members[0].Assignment != item.Assignment {
		p.t.Fatal("initial entry not exact trusted roster member")
	}
	replay := p.apply(view, i)
	if !replay.Replayed || !reflect.DeepEqual(replay.Receipt, first.Receipt) {
		p.t.Fatal("entry replay changed receipt")
	}
	q.ExpectedRevision = 1
	repeat := p.apply(view, p.prepare(view, q))
	if repeat.Replayed || repeat.Receipt.Revision != 2 {
		p.t.Fatal("new identical save did not increment entry revision")
	}
	withdraw := q
	withdraw.Action = "withdraw"
	withdraw.ExpectedRevision = 2
	withdraw.ExpectedRosterRevision = 0
	withdraw.ProspectIDs = []string{}
	empty := p.apply(view, p.prepare(view, withdraw))
	if empty.Receipt.Revision != 3 || len(empty.Receipt.Members) != 0 {
		p.t.Fatal("withdraw not durable empty revision")
	}
	q.ExpectedRevision = 3
	saved := p.apply(view, p.prepare(view, q))
	if saved.Receipt.Revision != 4 {
		p.t.Fatal("save after withdrawal failed")
	}
	p.saved[fixture] = saved.Receipt
	check := p.job(pilot.EntryRequest{Command: "entry_check", ViewID: view, Saturday: q.Saturday}).Check
	if check == nil || len(check.Members) != 1 || check.Members[0].Eligibility != "eligible" || check.Members[0].Item.Assignment != item.Assignment || check.Status.Revision != 4 {
		p.t.Fatal("checked entry lost exact source/eligibility")
	}
	q.ExpectedRevision = 4
	p.pending[fixture] = p.prepare(view, q)
	p.evidence = append(p.evidence, map[string]any{"fixture": fixture, "phase": "initial", "upcoming": up, "first": first, "replay": replay, "identical_new_save": repeat, "withdraw": empty, "saved": saved, "check": check, "pending": p.pending[fixture]})
}
func (p *entryAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.EntryRequest(p.ctx, pilot.EntryRequest{Command: "entry_snapshot"})
	if r.OK && r.View != nil {
		p.t.Fatal("signed-out entry disclosed")
	}
}
func (p *entryAcceptance) reopened(fixture string, s pilot.StableSnapshot) {
	p.t.Helper()
	view := p.attach(s)
	saved := p.saved[fixture]
	intent := p.pending[fixture]
	status := p.request(pilot.EntryRequest{Command: "entry_status", ViewID: view, Saturday: saved.Event.Saturday}).Status
	if status == nil || status.Revision != saved.Revision || status.EntryID != saved.EntryID {
		p.t.Fatal("reopen changed saved entry")
	}
	old := p.request(pilot.EntryRequest{Command: "entry_operation", ViewID: view, OperationID: saved.OperationID})
	if old.Operation == nil || old.Operation.Operation == nil || !reflect.DeepEqual(old.Operation.Operation.Receipt, &saved) {
		p.t.Fatal("historical entry receipt changed")
	}
	pending := p.request(pilot.EntryRequest{Command: "entry_pending", ViewID: view}).Operation
	if pending == nil || pending.Operation == nil || !reflect.DeepEqual(pending.Operation.Intent, intent) {
		p.t.Fatal("pending entry lost on reopen")
	}
	refused := p.b.EntryRequest(p.ctx, pilot.EntryRequest{Command: "entry_apply", ViewID: view, OperationID: intent.OperationID})
	if refused.OK || refused.Error != "stale_view" {
		p.t.Fatal("old entry author borrowed new login", refused.Error)
	}
	// A synced Abandon can outlive the short metadata acknowledgement window.
	// Reconcile its exact operation; never resend a possibly applied mutation.
	abandoned := p.reply(pilot.EntryRequest{Command: "entry_abandon", ViewID: view, OperationID: intent.OperationID})
	ackUnknown := !abandoned.OK && abandoned.Error == "outcome_unknown"
	if ackUnknown {
		p.t.Log("reconciling uncertain entry Abandon", fixture)
		abandoned = p.request(pilot.EntryRequest{Command: "entry_operation", ViewID: view, OperationID: intent.OperationID})
	}
	if !abandoned.OK {
		p.t.Fatal("entry abandonment refused", abandoned.Error)
	}
	if abandoned.Operation == nil || abandoned.Operation.Operation == nil || abandoned.Operation.Operation.State != "abandoned" {
		p.t.Fatal("pending entry abandon failed")
	}
	cleared := p.request(pilot.EntryRequest{Command: "entry_pending", ViewID: view}).Operation
	if cleared == nil || cleared.Operation != nil {
		p.t.Fatal("abandon retained pending intent")
	}
	p.evidence = append(p.evidence, map[string]any{"fixture": fixture, "phase": "reopened", "status": status, "historical": old.Operation, "abandoned": abandoned.Operation, "abandon_ack_unknown_reconciled": ackUnknown, "old_author_refused": true})
}
func (p *entryAcceptance) held(fixture string, s pilot.StableSnapshot, item gymstable.Item) {
	p.t.Helper()
	view := p.attach(s)
	old := p.saved[fixture]
	status := p.request(pilot.EntryRequest{Command: "entry_status", ViewID: view, Saturday: old.Event.Saturday}).Status
	if status == nil || !status.RosterChanged || status.Revision != old.Revision {
		p.t.Fatal("roster change silently changed entry")
	}
	q := gymentry.Request{Version: 1, EventID: old.Event.EventID, Saturday: old.Event.Saturday, Action: "replace", ExpectedRevision: old.Revision, ExpectedRosterRevision: s.Status.RosterRevision, ProspectIDs: []string{item.Assignment.ProspectID}}
	changed := p.apply(view, p.prepare(view, q))
	if changed.Receipt.Revision != old.Revision+1 || changed.Receipt.RosterRevision != s.Status.RosterRevision {
		p.t.Fatal("held entry management failed")
	}
	p.saved[fixture] = changed.Receipt
	check := p.job(pilot.EntryRequest{Command: "entry_check", ViewID: view, Saturday: q.Saturday}).Check
	if check == nil || len(check.Members) != 1 || check.Members[0].Eligibility != "eligible" {
		p.t.Fatal("held entry check failed")
	}
	history := p.request(pilot.EntryRequest{Command: "entry_history", ViewID: view, Limit: 8}).History
	if history == nil || len(history.Events) != 1 || history.Events[0].Revision != changed.Receipt.Revision {
		p.t.Fatal("entry history failed")
	}
	p.evidence = append(p.evidence, map[string]any{"fixture": fixture, "phase": "held", "before": status, "mutation": changed, "check": check, "history": history})
}
func (p *entryAcceptance) checkProcessRestart(b *StableBridge, s pilot.StableSnapshot) any {
	p.t.Helper()
	r := newEntryAcceptance(p.t, p.ctx, b)
	view := r.attach(s)
	saved := p.saved["zero256"]
	got := r.request(pilot.EntryRequest{Command: "entry_operation", ViewID: view, OperationID: saved.OperationID})
	if got.Operation == nil || got.Operation.Operation == nil || !reflect.DeepEqual(got.Operation.Operation.Receipt, &saved) {
		p.t.Fatal("process restart changed entry receipt")
	}
	status := r.request(pilot.EntryRequest{Command: "entry_status", ViewID: view, Saturday: saved.Event.Saturday})
	if status.Status == nil || status.Status.Revision != saved.Revision || status.Status.EntryID != saved.EntryID {
		p.t.Fatal("process restart entry status")
	}
	return map[string]any{"status": "passed", "receipt": got.Operation, "event": status.Status, "no_automatic_apply": true}
}
