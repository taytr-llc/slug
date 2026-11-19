// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"time"
)

type payoutExpectation struct {
	job      pilot.PoolPayoutJob
	terminal string
	rejected bool
	// Set before attempting the first pipe write. A later local refusal cannot
	// resolve that earlier transport uncertainty.
	dispatched bool
}

func payoutFailure(id uint64, code string) pilot.PoolPayoutResponse {
	if id == 0 {
		id = 1
	}
	return pilot.PoolPayoutResponse{ID: id, Version: 1, Error: code}
}
func (b *PoolBridge) RememberPoolPayout(view, id string) error {
	req := pilot.PoolPayoutRequest{ID: 1, Version: 1, Command: "pool_payout_begin", ViewID: view, JobID: id}
	if _, e := pilot.MarshalPoolPayoutRequest(req); e != nil {
		return e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || !b.poolMode || b.payoutBinding == nil || b.payoutBinding.View.ViewID != view {
		return ErrUnavailable
	}
	if b.payoutKnownLocked(id) != nil {
		return ErrUnavailable
	}
	if e := b.payoutAttempt; e != nil && !e.rejected && e.terminal == "" {
		return ErrBusy
	}
	b.payoutAttempt = &payoutExpectation{job: pilot.PoolPayoutJob{Version: 1, JobID: id, State: "running", Binding: *b.payoutBinding}}
	return nil
}
func (b *StableBridge) payoutKnownLocked(id string) *payoutExpectation {
	for _, e := range []*payoutExpectation{b.payoutAttempt, b.payoutCapture} {
		if e != nil && !e.rejected && e.job.JobID == id {
			return e
		}
	}
	return nil
}
func (b *StableBridge) payoutSelectedLocked(r pilot.PoolPayoutRequest) bool {
	p := b.payoutRows
	if p == nil || p.JobID != r.JobID || b.payoutCapture == nil || b.payoutCapture.job.Capture == nil || p.Capture != *b.payoutCapture.job.Capture {
		return false
	}
	for _, item := range p.Items {
		if item.TxID == r.TxID && item.OutputIndex == r.OutputIndex {
			return true
		}
	}
	return false
}
func (b *PoolBridge) PoolPayoutRequest(ctx context.Context, r pilot.PoolPayoutRequest) pilot.PoolPayoutResponse {
	pre := r
	pre.ID, pre.Version = 1, 1
	if ctx == nil {
		return payoutFailure(1, "invalid_request")
	}
	if _, e := pilot.MarshalPoolPayoutRequest(pre); e != nil {
		return payoutFailure(1, "invalid_request")
	}
	if !b.poolMode {
		return payoutFailure(1, "payout_unavailable")
	}
	if r.Command == "pool_payout_snapshot" {
		b.mu.Lock()
		ready := b.payoutBinding != nil
		b.mu.Unlock()
		if !ready {
			if _, e := b.PoolRequest(ctx, pilot.PoolRequest{Command: "snapshot"}); e != nil {
				return payoutFailure(1, "payout_unavailable")
			}
		}
	}
	b.mu.Lock()
	beforeDispatch := b.payoutKnownLocked(r.JobID)
	b.mu.Unlock()
	id, e := b.begin(ctx)
	if e != nil {
		out := payoutFailure(1, "payout_unavailable")
		if errors.Is(e, ErrBusy) {
			out = payoutFailure(1, "busy")
		}
		b.rejectUnsentPoolPayoutAttempt(pre, beforeDispatch)
		return out
	}
	defer func() { <-b.gate }()
	r.ID, r.Version = id, 1
	b.mu.Lock()
	valid := b.payoutRequestBoundLocked(r)
	expected := b.payoutKnownLocked(r.JobID)
	if valid && expected != nil && r.Command == "pool_payout_begin" {
		// Reserve dispatch atomically with validation. A concurrent same-ID
		// request refused by the gate must not retire this in-flight action.
		expected.dispatched = true
	}
	b.mu.Unlock()
	if !valid {
		return payoutFailure(id, "stale_view")
	}
	raw, e := pilot.MarshalPoolPayoutRequest(r)
	if e != nil {
		return payoutFailure(id, "invalid_request")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, e := b.exchangeExpected(work, &stableExpected{id: id, payout: true}, raw)
	if e != nil || f.payout == nil {
		if r.Command == "pool_payout_begin" {
			return payoutFailure(id, "outcome_unknown")
		}
		return payoutFailure(id, "payout_unavailable")
	}
	out := *f.payout
	if !out.OK {
		b.rejectPoolPayoutAttempt(r, out, expected)
		return out
	}
	if !b.correlatePoolPayout(r, out) {
		b.fail(ErrUnavailable)
		if r.Command == "pool_payout_begin" {
			return payoutFailure(id, "outcome_unknown")
		}
		return payoutFailure(id, "payout_unavailable")
	}
	return out
}

func (b *StableBridge) unresolvedPayoutAttemptLocked(r pilot.PoolPayoutRequest, expected *payoutExpectation) bool {
	return expected != nil && expected == b.payoutAttempt && expected != b.payoutCapture && expected.terminal == "" && expected.job.JobID == r.JobID && b.payoutBinding != nil && expected.job.Binding == *b.payoutBinding && r.ViewID == b.payoutBinding.View.ViewID
}

func (b *StableBridge) rejectUnsentPoolPayoutAttempt(r pilot.PoolPayoutRequest, expected *payoutExpectation) {
	if r.Command != "pool_payout_begin" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unresolvedPayoutAttemptLocked(r, expected) && !expected.dispatched {
		expected.rejected = true
	}
}

// A definite refusal or exact recovery miss retires only the still unresolved
// action. It neither cancels a task nor discards an older captured history.
func (b *StableBridge) rejectPoolPayoutAttempt(r pilot.PoolPayoutRequest, out pilot.PoolPayoutResponse, expected *payoutExpectation) {
	if out.OK || out.ID != r.ID || !(r.Command == "pool_payout_begin" && out.Error != "outcome_unknown" || r.Command == "pool_payout_job" && out.Error == "payout_not_found") {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unresolvedPayoutAttemptLocked(r, expected) {
		expected.rejected = true
	}
}
func (b *StableBridge) payoutRequestBoundLocked(r pilot.PoolPayoutRequest) bool {
	if b.failure != nil || !b.poolMode {
		return false
	}
	if r.Command == "pool_payout_snapshot" {
		return true
	}
	e := b.payoutKnownLocked(r.JobID)
	if e == nil || b.payoutBinding == nil || e.job.Binding != *b.payoutBinding || r.ViewID != b.payoutBinding.View.ViewID {
		return false
	}
	switch r.Command {
	case "pool_payout_begin":
		return e == b.payoutAttempt
	case "pool_payout_page":
		return e == b.payoutCapture && e.job.State == "done"
	case "pool_payout_wallet":
		return e == b.payoutCapture && e.job.State == "done" && b.payoutSelectedLocked(r)
	}
	return true
}
func (b *StableBridge) correlatePoolPayout(r pilot.PoolPayoutRequest, out pilot.PoolPayoutResponse) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || out.ID != r.ID || !out.OK {
		return false
	}
	if s := out.Snapshot; s != nil {
		if r.Command != "pool_payout_snapshot" || pilot.ValidatePoolPayoutSnapshot(*s) != nil {
			return false
		}
		if s.Binding == nil {
			return true
		}
		if b.payoutBinding == nil || *s.Binding != *b.payoutBinding {
			return false
		}
		for _, id := range []string{s.ActiveJobID, s.LastJobID, s.CaptureJobID} {
			if id != "" {
				e := b.payoutKnownLocked(id)
				if e == nil || e.job.Binding != *s.Binding {
					return false
				}
			}
		}
		return true
	}
	e := b.payoutKnownLocked(r.JobID)
	if e == nil || b.payoutBinding == nil || e.job.Binding != *b.payoutBinding {
		return false
	}
	if j := out.Job; j != nil {
		if !oneOf(r.Command, "pool_payout_begin", "pool_payout_job", "pool_payout_cancel") || j.JobID != r.JobID || j.Binding != e.job.Binding || pilot.ValidatePoolPayoutJob(*j) != nil {
			return false
		}
		if e.terminal != "" && e.terminal != historyDigest(*j) {
			return false
		}
		if j.State != "running" {
			e.terminal = historyDigest(*j)
		}
		e.job = *j
		if j.Capture != nil {
			v := *j.Capture
			e.job.Capture = &v
		}
		if j.State == "done" {
			if b.payoutCapture != e {
				b.payoutRows = nil
			}
			b.payoutCapture = e
		}
		return true
	}
	if e != b.payoutCapture || e.job.Capture == nil {
		return false
	}
	if p := out.Page; p != nil {
		if r.Command != "pool_payout_page" || p.JobID != r.JobID || p.Binding != e.job.Binding || p.Capture != *e.job.Capture || p.After != r.After || p.Limit != r.Limit || pilot.ValidatePoolPayoutPage(*p) != nil {
			return false
		}
		copy := *p
		copy.Items = append(p.Items[:0:0], p.Items...)
		b.payoutRows = &copy
		return true
	}
	if o := out.Observation; o != nil {
		if r.Command != "pool_payout_wallet" || o.JobID != r.JobID || o.Binding != e.job.Binding || o.Capture != *e.job.Capture || o.Payment.TxID != r.TxID || o.Payment.OutputIndex != r.OutputIndex || pilot.ValidatePoolPayoutObservation(*o) != nil || !b.payoutSelectedLocked(r) {
			return false
		}
		for _, p := range b.payoutRows.Items {
			if p.TxID == r.TxID && p.OutputIndex == r.OutputIndex {
				return p == o.Payment
			}
		}
	}
	return false
}
