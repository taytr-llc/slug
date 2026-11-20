// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"time"
)

type activityExpectation struct {
	job      pilot.WalletActivityJob
	terminal string
	rejected bool
	// Set before attempting the first pipe write. A later local refusal cannot
	// resolve that earlier transport uncertainty.
	dispatched bool
}

func activityFailure(id uint64, code string) pilot.WalletActivityResponse {
	if id == 0 {
		id = 1
	}
	return pilot.WalletActivityResponse{ID: id, Version: 1, Error: code}
}
func (b *PoolBridge) RememberWalletActivity(view, id string) error {
	req := pilot.WalletActivityRequest{ID: 1, Version: 1, Command: "wallet_activity_begin", ViewID: view, JobID: id}
	if _, e := pilot.MarshalWalletActivityRequest(req); e != nil {
		return e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || !b.poolMode || b.activityBinding == nil || b.activityBinding.View.ViewID != view {
		return ErrUnavailable
	}
	if b.activityKnownLocked(id) != nil {
		return ErrUnavailable
	}
	if e := b.activityAttempt; e != nil && !e.rejected && e.terminal == "" {
		return ErrBusy
	}
	b.activityAttempt = &activityExpectation{job: pilot.WalletActivityJob{Version: 1, JobID: id, State: "running", Binding: *b.activityBinding}}
	return nil
}
func (b *StableBridge) activityKnownLocked(id string) *activityExpectation {
	for _, e := range []*activityExpectation{b.activityAttempt, b.activityCapture} {
		if e != nil && !e.rejected && e.job.JobID == id {
			return e
		}
	}
	return nil
}
func (b *StableBridge) activitySelectedLocked(r pilot.WalletActivityRequest) bool {
	p := b.activityRows
	if p == nil || p.JobID != r.JobID || b.activityCapture == nil || b.activityCapture.job.Capture == nil || p.Capture != *b.activityCapture.job.Capture {
		return false
	}
	for _, item := range p.Items {
		if item.TxID == r.TxID {
			return true
		}
	}
	return false
}
func (b *PoolBridge) WalletActivityRequest(ctx context.Context, r pilot.WalletActivityRequest) pilot.WalletActivityResponse {
	pre := r
	pre.ID, pre.Version = 1, 1
	if ctx == nil {
		return activityFailure(1, "invalid_request")
	}
	if _, e := pilot.MarshalWalletActivityRequest(pre); e != nil {
		return activityFailure(1, "invalid_request")
	}
	if !b.poolMode {
		return activityFailure(1, "activity_unavailable")
	}
	if r.Command == "wallet_activity_snapshot" {
		b.mu.Lock()
		ready := b.activityBinding != nil
		b.mu.Unlock()
		if !ready {
			if _, e := b.PoolRequest(ctx, pilot.PoolRequest{Command: "snapshot"}); e != nil {
				return activityFailure(1, "activity_unavailable")
			}
		}
	}
	b.mu.Lock()
	beforeDispatch := b.activityKnownLocked(r.JobID)
	b.mu.Unlock()
	id, e := b.begin(ctx)
	if e != nil {
		out := activityFailure(1, "activity_unavailable")
		if errors.Is(e, ErrBusy) {
			out = activityFailure(1, "busy")
		}
		b.rejectUnsentWalletActivityAttempt(pre, beforeDispatch)
		return out
	}
	defer func() { <-b.gate }()
	r.ID, r.Version = id, 1
	b.mu.Lock()
	valid := b.activityRequestBoundLocked(r)
	expected := b.activityKnownLocked(r.JobID)
	if valid && expected != nil && r.Command == "wallet_activity_begin" {
		// Reserve dispatch atomically with validation. A concurrent same-ID
		// request refused by the gate must not retire this in-flight action.
		expected.dispatched = true
	}
	b.mu.Unlock()
	if !valid {
		return activityFailure(id, "stale_view")
	}
	raw, e := pilot.MarshalWalletActivityRequest(r)
	if e != nil {
		return activityFailure(id, "invalid_request")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, e := b.exchangeExpected(work, &stableExpected{id: id, activity: true}, raw)
	if e != nil || f.activity == nil {
		if r.Command == "wallet_activity_begin" {
			return activityFailure(id, "outcome_unknown")
		}
		return activityFailure(id, "activity_unavailable")
	}
	out := *f.activity
	if !out.OK {
		b.rejectWalletActivityAttempt(r, out, expected)
		return out
	}
	if !b.correlateWalletActivity(r, out) {
		b.fail(ErrUnavailable)
		if r.Command == "wallet_activity_begin" {
			return activityFailure(id, "outcome_unknown")
		}
		return activityFailure(id, "activity_unavailable")
	}
	return out
}

func (b *StableBridge) unresolvedActivityAttemptLocked(r pilot.WalletActivityRequest, expected *activityExpectation) bool {
	return expected != nil && expected == b.activityAttempt && expected != b.activityCapture && expected.terminal == "" && expected.job.JobID == r.JobID && b.activityBinding != nil && expected.job.Binding == *b.activityBinding && r.ViewID == b.activityBinding.View.ViewID
}

func (b *StableBridge) rejectUnsentWalletActivityAttempt(r pilot.WalletActivityRequest, expected *activityExpectation) {
	if r.Command != "wallet_activity_begin" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unresolvedActivityAttemptLocked(r, expected) && !expected.dispatched {
		expected.rejected = true
	}
}

// A definite refusal or exact recovery miss retires only the still unresolved
// action. It neither cancels a task nor discards an older captured history.
func (b *StableBridge) rejectWalletActivityAttempt(r pilot.WalletActivityRequest, out pilot.WalletActivityResponse, expected *activityExpectation) {
	if out.OK || out.ID != r.ID || !(r.Command == "wallet_activity_begin" && out.Error != "outcome_unknown" || r.Command == "wallet_activity_job" && out.Error == "activity_not_found") {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unresolvedActivityAttemptLocked(r, expected) {
		expected.rejected = true
	}
}
func (b *StableBridge) activityRequestBoundLocked(r pilot.WalletActivityRequest) bool {
	if b.failure != nil || !b.poolMode {
		return false
	}
	if r.Command == "wallet_activity_snapshot" {
		return true
	}
	e := b.activityKnownLocked(r.JobID)
	if e == nil || b.activityBinding == nil || e.job.Binding != *b.activityBinding || r.ViewID != b.activityBinding.View.ViewID {
		return false
	}
	switch r.Command {
	case "wallet_activity_begin":
		return e == b.activityAttempt
	case "wallet_activity_page":
		return e == b.activityCapture && e.job.State == "done"
	case "wallet_activity_detail":
		return e == b.activityCapture && e.job.State == "done" && b.activitySelectedLocked(r)
	}
	return true
}
func (b *StableBridge) correlateWalletActivity(r pilot.WalletActivityRequest, out pilot.WalletActivityResponse) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || out.ID != r.ID || !out.OK {
		return false
	}
	if s := out.Snapshot; s != nil {
		if r.Command != "wallet_activity_snapshot" || pilot.ValidateWalletActivitySnapshot(*s) != nil {
			return false
		}
		if s.Binding == nil {
			return true
		}
		if b.activityBinding == nil || *s.Binding != *b.activityBinding {
			return false
		}
		for _, id := range []string{s.ActiveJobID, s.LastJobID, s.CaptureJobID} {
			if id != "" {
				e := b.activityKnownLocked(id)
				if e == nil || e.job.Binding != *s.Binding {
					return false
				}
			}
		}
		return true
	}
	e := b.activityKnownLocked(r.JobID)
	if e == nil || b.activityBinding == nil || e.job.Binding != *b.activityBinding {
		return false
	}
	if j := out.Job; j != nil {
		if !oneOf(r.Command, "wallet_activity_begin", "wallet_activity_job", "wallet_activity_cancel") || j.JobID != r.JobID || j.Binding != e.job.Binding || pilot.ValidateWalletActivityJob(*j) != nil {
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
			if b.activityCapture != e {
				b.activityRows = nil
			}
			b.activityCapture = e
		}
		return true
	}
	if e != b.activityCapture || e.job.Capture == nil {
		return false
	}
	if p := out.Page; p != nil {
		if r.Command != "wallet_activity_page" || p.JobID != r.JobID || p.Binding != e.job.Binding || p.Capture != *e.job.Capture || p.After != r.After || p.Limit != r.Limit || pilot.ValidateWalletActivityPage(*p) != nil {
			return false
		}
		if !activityNeighborPages(b.activityRows, p) {
			return false
		}
		owned := pilot.CloneWalletActivityPage(*p)
		b.activityRows = &owned
		return true
	}
	if d := out.Detail; d != nil {
		if r.Command != "wallet_activity_detail" || d.JobID != r.JobID || d.Binding != e.job.Binding || d.Capture != *e.job.Capture || d.Item.TxID != r.TxID || pilot.ValidateWalletActivityDetail(*d) != nil || !b.activitySelectedLocked(r) {
			return false
		}
		for _, item := range b.activityRows.Items {
			if item.TxID == r.TxID {
				return pilot.WalletActivityItemEqual(item, d.Item)
			}
		}
	}
	return false
}

// Only one eight-row selection is retained. Overlapping rows must be identical;
// adjacent boundaries preserve the immutable capture order before replacement.
func activityNeighborPages(previous, next *pilot.WalletActivityPage) bool {
	if previous == nil || previous.JobID != next.JobID || previous.Capture != next.Capture {
		return true
	}
	for i, a := range previous.Items {
		index := previous.After + uint32(i)
		if index >= next.After && index < next.Next && !pilot.WalletActivityItemEqual(a, next.Items[index-next.After]) {
			return false
		}
	}
	if len(previous.Items) > 0 && len(next.Items) > 0 {
		if previous.Next == next.After && !pilot.WalletActivityItemLess(previous.Items[len(previous.Items)-1], next.Items[0]) {
			return false
		}
		if next.Next == previous.After && !pilot.WalletActivityItemLess(next.Items[len(next.Items)-1], previous.Items[0]) {
			return false
		}
	}
	return true
}
