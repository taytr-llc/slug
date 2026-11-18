//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymresult"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The real-calendar package has no saved booked bout. These are exact current
// source-prerequisite refusals, not a publication/closed-event/finality claim.
// Actual publication and exported proofs have a separate scoped-clock pipeline.
type resultAcceptance struct {
	t               *testing.T
	ctx             context.Context
	b               *StableBridge
	authority       *resultAuthority
	evidence        []map[string]any
	restartEvidence map[string]any
}

func newResultAcceptance(t *testing.T, ctx context.Context, b *StableBridge) *resultAcceptance {
	return &resultAcceptance{t: t, ctx: ctx, b: b, evidence: []map[string]any{}}
}
func (p *resultAcceptance) reply(q pilot.ResultRequest) pilot.ResultResponse {
	p.t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		r := p.b.ResultRequest(p.ctx, q)
		if r.Error != "busy" {
			return r
		}
		select {
		case <-p.ctx.Done():
			p.t.Fatal(p.ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	p.t.Fatal("result request remained busy", q.Command)
	return pilot.ResultResponse{}
}

// A prior family's terminal can be visible before its shared lane drains.
// Wait only for that definite busy metadata; never repeat an unknown action.
func (p *resultAcceptance) captureSnapshot() pilot.ResultResponse {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(p.ctx, 45*time.Second)
	defer cancel()
	for {
		r := p.b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_snapshot"})
		if r.OK && r.Snapshot != nil {
			s := r.Snapshot
			if s.NodeState != "offline" || s.NodeOwner != "none" || s.ActiveJobID != "" || s.QueuedJobID != "" || s.LastJobID != "" {
				p.t.Fatal("passive result capture created nodes or jobs")
			}
			switch s.State {
			case "ready", "setup_incomplete":
				return r
			case "busy":
			default:
				p.t.Fatal("unexpected result capture state", s.State, s.Error)
			}
		} else if r.Error != "busy" {
			p.t.Fatal("result capture refused", r.Error)
		}
		select {
		case <-ctx.Done():
			p.t.Fatal("result capture remained busy", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (p *resultAcceptance) observe(fixture, phase string, s pilot.StableSnapshot) {
	p.t.Helper()
	r := p.captureSnapshot()
	snap := r.Snapshot
	if !r.OK || snap == nil || snap.View == nil || s.View == nil || *snap.View != *s.View {
		p.t.Fatal("result lost original view", r.Error)
	}
	row := map[string]any{"fixture": fixture, "phase": phase, "snapshot": snap, "real_clock": true, "publication_executed": false, "closed_capture_executed": false, "run_executed": false, "proof_export_executed": false}
	if snap.NodeState != "offline" || snap.NodeOwner != "none" || snap.ActiveJobID != "" || snap.QueuedJobID != "" || snap.LastJobID != "" {
		p.t.Fatal("passive package result created node/job")
	}
	if snap.State == "setup_incomplete" {
		if fixture != "zero256" || p.authority != nil || snap.Pin != nil || snap.StoreID != "" || snap.CreatedAtMS != 0 || snap.SourceCreatedAtMS != 0 || snap.PublisherAccountID != "" {
			p.t.Fatal("result setup rollback")
		}
		p.evidence = append(p.evidence, row)
		return
	}
	if snap.Pin == nil || snap.State != "ready" || snap.SourceCreatedAtMS <= 0 || snap.CreatedAtMS < snap.SourceCreatedAtMS {
		p.t.Fatal("result not ready", snap.State)
	}
	a := resultAuthority{pin: *snap.Pin, storeID: snap.StoreID, createdMS: snap.CreatedAtMS, sourceCreatedMS: snap.SourceCreatedAtMS}
	if p.authority != nil {
		old, _ := gymresult.MarshalPin(p.authority.pin)
		current, _ := gymresult.MarshalPin(a.pin)
		if string(old) != string(current) || p.authority.storeID != a.storeID || p.authority.createdMS != a.createdMS || p.authority.sourceCreatedMS != a.sourceCreatedMS {
			p.t.Fatal("result authority changed")
		}
	}
	p.authority = &a
	view := s.View.ViewID
	up := newEntryAcceptance(p.t, p.ctx, p.b).request(pilot.EntryRequest{Command: "entry_upcoming", ViewID: view}).Status
	if up == nil {
		p.t.Fatal("upcoming Saturday")
	}
	bid := strings.Repeat("a", 64)
	row["saturday"], row["probe_bout_id"] = up.Event.Saturday, bid
	for _, cmd := range []string{"result_status", "result_record", "result_publish", "result_reconcile", "result_attest"} {
		reply := p.reply(pilot.ResultRequest{Command: cmd, ViewID: view, Saturday: up.Event.Saturday, BoutID: bid})
		if reply.OK || reply.Error != "result_source_incomplete" || reply.Job != nil || reply.Status != nil || reply.Record != nil {
			p.t.Fatal("missing saved source must refuse before work", cmd, reply.Error)
		}
		row[strings.TrimPrefix(cmd, "result_")+"_refusal"] = reply.Error
	}
	path := filepath.Join(p.t.TempDir(), "candidate-proof.json")
	export := p.b.ExportResultProof(p.ctx, view, up.Event.Saturday, bid, path)
	if export.OK || export.Error != "result_source_incomplete" || export.Job != nil {
		p.t.Fatal("absent source proof export", export.Error)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		p.t.Fatal("refused proof created destination", e)
	}
	row["export_refusal"] = export.Error
	after := p.reply(pilot.ResultRequest{Command: "result_snapshot"}).Snapshot
	if after == nil || after.View == nil || *after.View != *snap.View || after.NodeState != "offline" || after.NodeOwner != "none" || after.ActiveJobID != "" || after.QueuedJobID != "" || after.LastJobID != "" {
		p.t.Fatal("source refusal started nodes or retained work")
	}
	row["after_refusals"] = after
	p.evidence = append(p.evidence, row)
}
func (p *resultAcceptance) refuseDetached() {
	p.t.Helper()
	r := p.b.ResultRequest(p.ctx, pilot.ResultRequest{Command: "result_snapshot"})
	if r.OK && (r.View != nil || r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
		p.t.Fatal("detached result leaked authority")
	}
}
func (p *resultAcceptance) checkRestart(b *StableBridge, s pilot.StableSnapshot) {
	p.t.Helper()
	if p.authority == nil {
		p.t.Fatal("missing result authority")
	}
	next := newResultAcceptance(p.t, p.ctx, b)
	next.authority = p.authority
	next.observe("zero256", "process_restart", s)
	p.restartEvidence = map[string]any{"status": "passed", "same_pin_and_creation_time": true, "no_automatic_publication": true, "no_automatic_nodes": true, "real_clock_closed_coverage": false, "evidence": next.evidence}
}
