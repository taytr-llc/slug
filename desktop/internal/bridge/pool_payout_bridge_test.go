// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/poolsettle"
	"strings"
	"sync"
	"testing"
	"time"
)

func payoutBridgeFixture(t *testing.T) (*PoolBridge, pilot.PoolPayoutJob, poolsettle.PayoutItem) {
	t.Helper()
	b, e := pilot.PoolPayoutBindingForSnapshot(poolBridgeSnapshot(t))
	if e != nil {
		t.Fatal(e)
	}
	bridge := &PoolBridge{&StableBridge{poolMode: true, payoutBinding: &b}}
	id := strings.Repeat("a", 64)
	if e = bridge.RememberPoolPayout(b.View.ViewID, id); e != nil {
		t.Fatal(e)
	}
	ms := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	j := pilot.PoolPayoutJob{Version: 1, JobID: id, State: "done", Binding: b, Capture: &pilot.PoolPayoutCapture{Version: 1, Count: 1, PaidAtoms: poolsettle.MinimumAtoms, SettlementObservedAtMS: ms + 1, CapturedAtMS: ms + 2}}
	item := poolsettle.PayoutItem{Day: "2025-01-01", CreatedAtMS: ms, TxID: strings.Repeat("f", 64), OutputIndex: 1, Atoms: poolsettle.MinimumAtoms, RecordedState: "confirmed"}
	return bridge, j, item
}
func TestPoolPayoutBridgeCapturedPaymentAndTerminalBinding(t *testing.T) {
	b, j, item := payoutBridgeFixture(t)
	req := pilot.PoolPayoutRequest{ID: 1, Version: 1, Command: "pool_payout_job", ViewID: j.Binding.View.ViewID, JobID: j.JobID}
	if !b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &j}) {
		t.Fatal("known capture refused")
	}
	bad := j
	meta := *j.Capture
	meta.CapturedAtMS++
	bad.Capture = &meta
	if b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &bad}) {
		t.Fatal("terminal metadata changed")
	}
	p := pilot.PoolPayoutPage{Version: 1, Binding: j.Binding, JobID: j.JobID, Capture: *j.Capture, Limit: 8, Next: 1, Items: []poolsettle.PayoutItem{item}}
	req.Command, req.Limit = "pool_payout_page", 8
	if !b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Page: &p}) {
		t.Fatal("own page refused")
	}
	p.Items[0].Atoms++
	if b.payoutRows.Items[0] != item {
		t.Fatal("retained page aliases response")
	}
	p.Items[0] = item
	req.Command, req.Limit, req.TxID, req.OutputIndex = "pool_payout_wallet", 0, item.TxID, item.OutputIndex
	if !b.payoutRequestBoundLocked(req) {
		t.Fatal("selected row not authorized")
	}
	foreign := req
	foreign.TxID = strings.Repeat("e", 64)
	if b.payoutRequestBoundLocked(foreign) {
		t.Fatal("arbitrary transaction proxy")
	}
	obs := pilot.PoolPayoutObservation{Version: 1, Binding: j.Binding, JobID: j.JobID, Capture: *j.Capture, Payment: item, ObservedAtMS: j.Capture.CapturedAtMS + 1, State: "not_observed", WalletSynced: true, WalletHeight: 10, WalletTip: strings.Repeat("b", 64)}
	if !b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Observation: &obs}) {
		t.Fatal("dated own observation refused")
	}
	obs.Binding.RecipientScript = "0014" + strings.Repeat("1", 40)
	if b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Observation: &obs}) {
		t.Fatal("recipient substitution accepted")
	}
	newer := strings.Repeat("c", 64)
	if e := b.RememberPoolPayout(j.Binding.View.ViewID, newer); e != nil {
		t.Fatal(e)
	}
	failed := pilot.PoolPayoutJob{Version: 1, JobID: newer, State: "failed", Binding: j.Binding, Error: "busy"}
	req = pilot.PoolPayoutRequest{ID: 2, Version: 1, Command: "pool_payout_job", ViewID: j.Binding.View.ViewID, JobID: newer}
	if !b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 2, Version: 1, OK: true, Job: &failed}) || b.payoutCapture.job.JobID != j.JobID {
		t.Fatal("failed refresh replaced old capture")
	}
	snap := pilot.PoolPayoutSnapshot{Version: 1, State: "idle", Binding: &j.Binding, Activity: "none", LastJobID: strings.Repeat("d", 64), CaptureJobID: j.JobID}
	req.Command = "pool_payout_snapshot"
	if b.correlatePoolPayout(req, pilot.PoolPayoutResponse{ID: 2, Version: 1, OK: true, Snapshot: &snap}) {
		t.Fatal("unknown latest task accepted")
	}
	b.clearStableView()
	if b.payoutBinding != nil || b.payoutCapture != nil || b.payoutAttempt != nil || b.payoutRows != nil {
		t.Fatal("old view retained correlation")
	}
}
func TestPoolPayoutWireClosedFramesAndNoFalseEmpty(t *testing.T) {
	_, j, item := payoutBridgeFixture(t)
	p := pilot.PoolPayoutPage{Version: 1, Binding: j.Binding, JobID: j.JobID, Capture: *j.Capture, Limit: 8, Next: 1, Items: []poolsettle.PayoutItem{item}}
	raw, e := pilot.MarshalPoolPayoutResponse(pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Page: &p})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pilot.ParsePoolPayoutResponse(raw); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"id":1`), 1), bytes.Replace(raw, []byte(`"count":1`), []byte(`"count":1e0`), 1), bytes.Replace(raw, []byte(`"next":1`), []byte(`"next":0`), 1), bytes.Replace(raw, []byte(`"recorded_state":"confirmed"`), []byte(`"recorded_state":"confirmed","private":"x"`), 1), append(append([]byte{}, raw...), []byte(`{}`)...)} {
		if _, e := pilot.ParsePoolPayoutResponse(bad); e == nil {
			t.Fatal("malformed closed response accepted")
		}
	}
	blank := pilot.BlankPoolPayoutSnapshot()
	if pilot.ValidatePoolPayoutSnapshot(blank) != nil {
		t.Fatal("blank unavailable rejected")
	}
	blank.State = "idle"
	blank.Error = ""
	if pilot.ValidatePoolPayoutSnapshot(blank) == nil {
		t.Fatal("unavailable relabeled verified empty")
	}
}

func TestPoolPayoutBridgeExactRecoveryMissAllowsNewAction(t *testing.T) {
	b, old, _ := payoutBridgeFixture(t)
	r := pilot.PoolPayoutRequest{ID: 1, Version: 1, Command: "pool_payout_job", ViewID: old.Binding.View.ViewID, JobID: old.JobID}
	if !b.correlatePoolPayout(r, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &old}) {
		t.Fatal("old capture refused")
	}
	newID := strings.Repeat("b", 64)
	if e := b.RememberPoolPayout(r.ViewID, newID); e != nil {
		t.Fatal(e)
	}
	expected := b.payoutAttempt
	r.JobID = newID
	miss := pilot.PoolPayoutResponse{ID: 1, Version: 1, Error: "payout_not_found"}
	wrong := r
	wrong.JobID = old.JobID
	b.rejectPoolPayoutAttempt(wrong, miss, expected)
	if expected.rejected {
		t.Fatal("foreign lookup retired the new action")
	}
	b.rejectPoolPayoutAttempt(r, pilot.PoolPayoutResponse{ID: 1, Version: 1, Error: "payout_unavailable"}, expected)
	if expected.rejected {
		t.Fatal("uncertain lookup retired the action")
	}
	b.rejectPoolPayoutAttempt(r, miss, expected)
	if !expected.rejected || b.payoutCapture.job.JobID != old.JobID || b.payoutCapture.terminal == "" {
		t.Fatal("exact recovery miss lost the prior capture")
	}
	if e := b.RememberPoolPayout(r.ViewID, strings.Repeat("c", 64)); e != nil {
		t.Fatalf("new explicit action remained blocked after definite miss: %v", e)
	}
	b.rejectPoolPayoutAttempt(r, miss, expected)
	if b.payoutAttempt.rejected || b.payoutCapture.job.JobID != old.JobID {
		t.Fatal("delayed old miss changed the new action")
	}
}

func TestPoolPayoutBridgeLocalBusyRetiresOnlyNeverDispatchedAction(t *testing.T) {
	for _, dispatched := range []bool{false, true} {
		t.Run(map[bool]string{false: "never-sent", true: "earlier-send-unknown"}[dispatched], func(t *testing.T) {
			b, old, _ := payoutBridgeFixture(t)
			r := pilot.PoolPayoutRequest{ID: 1, Version: 1, Command: "pool_payout_job", ViewID: old.Binding.View.ViewID, JobID: old.JobID}
			if !b.correlatePoolPayout(r, pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &old}) {
				t.Fatal("old capture refused")
			}
			id := strings.Repeat("b", 64)
			if e := b.RememberPoolPayout(r.ViewID, id); e != nil {
				t.Fatal(e)
			}
			expected := b.payoutAttempt
			expected.dispatched = dispatched
			b.gate = make(chan struct{}, 1)
			b.gate <- struct{}{} // Another read already owns dispatch.
			out := b.PoolPayoutRequest(context.Background(), pilot.PoolPayoutRequest{Command: "pool_payout_begin", ViewID: r.ViewID, JobID: id})
			<-b.gate
			if out.OK || out.Error != "busy" || expected.rejected == dispatched || b.payoutCapture.job.JobID != old.JobID {
				t.Fatal("local busy resolved the wrong action or lost dated capture")
			}
			e := b.RememberPoolPayout(r.ViewID, strings.Repeat("c", 64))
			if dispatched && !errors.Is(e, ErrBusy) || !dispatched && e != nil {
				t.Fatal("incorrect new-action admission after local busy", e)
			}
			if dispatched {
				r.JobID = id
				if !b.payoutRequestBoundLocked(r) {
					t.Fatal("earlier uncertain action lost exact recovery")
				}
			}
		})
	}
}

type payoutBlockedWrite struct{ entered, release chan struct{} }

func (w payoutBlockedWrite) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}
func (w payoutBlockedWrite) Close() error { return nil }

func TestPoolPayoutBridgeConcurrentBeginKeepsReservedDispatch(t *testing.T) {
	b, j, _ := payoutBridgeFixture(t)
	b.gate = make(chan struct{}, 1)
	b.failed = make(chan struct{})
	b.responses = make(chan stableFrame, 1)
	w := payoutBlockedWrite{make(chan struct{}), make(chan struct{})}
	b.input = w
	j.State, j.Capture = "running", nil
	var once sync.Once
	finish := func() {
		once.Do(func() {
			b.responses <- stableFrame{payout: &pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &j}}
			close(w.release)
		})
	}
	defer finish()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan pilot.PoolPayoutResponse, 1)
	r := pilot.PoolPayoutRequest{Command: "pool_payout_begin", ViewID: j.Binding.View.ViewID, JobID: j.JobID}
	go func() { done <- b.PoolPayoutRequest(ctx, r) }()
	select {
	case <-w.entered:
	case <-ctx.Done():
		t.Fatal("first dispatch did not enter")
	}
	out := b.PoolPayoutRequest(ctx, r)
	if out.OK || out.Error != "busy" {
		t.Fatal("concurrent dispatch not refused")
	}
	b.mu.Lock()
	retained := b.payoutAttempt != nil && !b.payoutAttempt.rejected && b.payoutAttempt.dispatched
	b.mu.Unlock()
	if !retained {
		t.Fatal("concurrent local busy retired the reserved write")
	}
	finish()
	select {
	case out = <-done:
		if !out.OK || out.Job == nil || out.Job.JobID != j.JobID {
			t.Fatal("original dispatch lost its correlation")
		}
	case <-ctx.Done():
		t.Fatal("original dispatch did not finish")
	}
}
