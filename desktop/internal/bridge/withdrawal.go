// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"reflect"
	"strings"
	"time"
)

type withdrawalExpectation struct {
	request              pilot.WithdrawalRequest
	binding              pilot.WithdrawalBinding
	review               *pilot.WithdrawalReview
	action               *pilot.WithdrawalAction
	dispatched, rejected bool
	terminal             string
}

func withdrawalMutating(command string) bool {
	return oneOf(command, "withdrawal_prepare", "withdrawal_review", "withdrawal_approve", "withdrawal_abandon", "withdrawal_observe")
}
func withdrawalSameIntent(a, b pilot.WithdrawalRequest) bool {
	a.ID, b.ID = 1, 1
	return reflect.DeepEqual(a, b)
}
func (b *PoolBridge) RememberWithdrawal(r pilot.WithdrawalRequest) error {
	r.ID, r.Version = 1, 1
	if _, e := pilot.MarshalWithdrawalRequest(r); e != nil || !oneOf(r.Command, "withdrawal_prepare", "withdrawal_review", "withdrawal_abandon", "withdrawal_observe") {
		return ErrUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.withdrawalBinding == nil || r.ViewID != b.withdrawalBinding.View.ViewID {
		return ErrUnavailable
	}
	if prior := b.withdrawalExpected; prior != nil && !prior.rejected && prior.terminal == "" {
		return ErrBusy
	}
	if r.Command != "withdrawal_prepare" {
		o := b.withdrawalOperation
		if o == nil || o.OperationID != r.OperationID {
			return ErrUnavailable
		}
		if r.DraftDigest != "" && (o.Draft == nil || o.Draft.DraftDigest != r.DraftDigest) {
			return ErrUnavailable
		}
		if r.TxID != "" && o.TxID != r.TxID {
			return ErrUnavailable
		}
	}
	if r.Intent != nil {
		intent := *r.Intent
		r.Intent = &intent
	}
	b.withdrawalExpected = &withdrawalExpectation{request: r, binding: *b.withdrawalBinding}
	return nil
}
func (b *PoolBridge) WithdrawalRequest(ctx context.Context, r pilot.WithdrawalRequest) pilot.WithdrawalResponse {
	r.ID, r.Version = 1, 1
	if ctx == nil {
		return pilot.WithdrawalFailure(1, "invalid_request")
	}
	if _, e := pilot.MarshalWithdrawalRequest(r); e != nil {
		return pilot.WithdrawalFailure(1, "invalid_request")
	}
	if r.Command == "withdrawal_snapshot" {
		b.mu.Lock()
		ready := b.withdrawalPool != nil
		b.mu.Unlock()
		if !ready {
			if _, e := b.PoolRequest(ctx, pilot.PoolRequest{Command: "snapshot"}); e != nil {
				return pilot.WithdrawalFailure(1, "withdrawal_unavailable")
			}
		}
	}
	b.mu.Lock()
	expected := b.withdrawalExpected
	b.mu.Unlock()
	id, e := b.begin(ctx)
	if e != nil {
		b.mu.Lock()
		if withdrawalMutating(r.Command) && expected != nil && b.withdrawalExpected == expected && !expected.dispatched && withdrawalSameIntent(expected.request, r) {
			expected.rejected = true
		}
		b.mu.Unlock()
		code := "withdrawal_unavailable"
		if errors.Is(e, ErrBusy) {
			code = "busy"
		}
		return pilot.WithdrawalFailure(1, code)
	}
	defer func() { <-b.gate }()
	r.ID = id
	b.mu.Lock()
	valid := b.withdrawalRequestBoundLocked(r)
	expected = b.withdrawalExpected
	if valid && withdrawalMutating(r.Command) && expected != nil {
		expected.dispatched = true
	}
	b.mu.Unlock()
	if !valid {
		return pilot.WithdrawalFailure(id, "stale_view")
	}
	raw, e := pilot.MarshalWithdrawalRequest(r)
	if e != nil {
		return pilot.WithdrawalFailure(id, "invalid_request")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, e := b.exchangeExpected(work, &stableExpected{id: id, withdrawal: true}, raw)
	if e != nil || f.withdrawal == nil {
		code := "withdrawal_unavailable"
		if withdrawalMutating(r.Command) {
			code = "outcome_unknown"
		}
		return pilot.WithdrawalFailure(id, code)
	}
	out := *f.withdrawal
	if !out.OK {
		b.mu.Lock()
		if expected != nil && b.withdrawalExpected == expected && expected.terminal == "" && (withdrawalMutating(r.Command) && out.Error != "outcome_unknown" || r.Command == "withdrawal_action" && out.Error == "withdrawal_not_found") {
			expected.rejected = true
		}
		b.mu.Unlock()
		return out
	}
	if !b.correlateWithdrawal(r, out) {
		b.fail(ErrUnavailable)
		code := "withdrawal_unavailable"
		if withdrawalMutating(r.Command) {
			code = "outcome_unknown"
		}
		return pilot.WithdrawalFailure(id, code)
	}
	return out
}
func (b *StableBridge) withdrawalRequestBoundLocked(r pilot.WithdrawalRequest) bool {
	if b.failure != nil || !b.poolMode {
		return false
	}
	if r.Command == "withdrawal_snapshot" {
		return true
	}
	if b.withdrawalBinding == nil || b.withdrawalBinding.View.ViewID != r.ViewID {
		return false
	}
	if r.Command == "withdrawal_operation" {
		return true
	}
	x := b.withdrawalExpected
	if x == nil || x.rejected || x.binding != *b.withdrawalBinding || x.request.OperationID != r.OperationID || x.request.ActionID != r.ActionID {
		return false
	}
	if r.Command == "withdrawal_action" || r.Command == "withdrawal_stop" {
		return true
	}
	if r.Command == "withdrawal_approve" {
		v := x.review
		return v != nil && v.ReviewID == r.ReviewID && v.Draft.DraftDigest == r.DraftDigest && v.TxID == r.TxID && time.Now().UnixMilli() < v.ExpiresAtMS
	}
	return withdrawalSameIntent(x.request, r)
}
func (b *StableBridge) withdrawalOperationBoundLocked(o pilot.WithdrawalOperation) bool {
	if b.withdrawalBinding == nil || o.JournalID != b.withdrawalBinding.JournalID || o.WithdrawalBindingID != b.withdrawalBinding.WithdrawalBindingID || pilot.ValidateWithdrawalOperation(o) != nil {
		return false
	}
	if old := b.withdrawalOperation; old != nil && old.OperationID == o.OperationID {
		if o.Sequence != old.Sequence || o.Revision < old.Revision || o.CreatedAtMS != old.CreatedAtMS || o.UpdatedAtMS < old.UpdatedAtMS || o.Intent != old.Intent || o.IntentDigest != old.IntentDigest || old.Draft != nil && !reflect.DeepEqual(old.Draft, o.Draft) || old.TxID != "" && (old.TxID != o.TxID || old.SignedSHA256 != o.SignedSHA256) {
			return false
		}
		if o.Revision == old.Revision && !reflect.DeepEqual(*old, o) {
			return false
		}
		rank := map[string]int{"intent": 0, "prepared": 1, "signed": 2, "dispatching": 3, "submitted": 4, "abandoned": 4}
		if rank[o.Stage] < rank[old.Stage] || oneOf(old.Stage, "submitted", "abandoned") && old.Stage != o.Stage {
			return false
		}
	}
	copy := o
	if o.Draft != nil {
		d := *o.Draft
		copy.Draft = &d
	}
	if o.Dispatch != nil {
		d := *o.Dispatch
		copy.Dispatch = &d
	}
	if o.Observation != nil {
		d := *o.Observation
		copy.Observation = &d
	}
	b.withdrawalOperation = &copy
	return true
}
func (b *StableBridge) correlateWithdrawal(r pilot.WithdrawalRequest, out pilot.WithdrawalResponse) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || out.ID != r.ID || !out.OK {
		return false
	}
	if s := out.Snapshot; s != nil {
		if r.Command != "withdrawal_snapshot" || pilot.ValidateWithdrawalSnapshot(*s) != nil {
			return false
		}
		if s.Binding == nil {
			return true
		}
		if b.withdrawalPool == nil {
			return false
		}
		expected, e := pilot.WithdrawalBindingForSnapshot(*b.withdrawalPool, s.Binding.JournalID)
		if e != nil || expected != *s.Binding || b.withdrawalBinding != nil && expected != *b.withdrawalBinding {
			return false
		}
		b.withdrawalBinding = &expected
		return true
	}
	if o := out.Operation; o != nil {
		return r.Command == "withdrawal_operation" && o.OperationID == r.OperationID && b.withdrawalOperationBoundLocked(*o)
	}
	x := b.withdrawalExpected
	if x == nil || x.rejected || b.withdrawalBinding == nil || x.binding != *b.withdrawalBinding {
		return false
	}
	if v := out.Review; v != nil {
		if r.Command != "withdrawal_review" || v.Binding != x.binding || v.OperationID != r.OperationID || v.ActionID != r.ActionID || v.ReviewID != r.ReviewID || v.Kind != r.ReviewKind || v.Draft.DraftDigest != r.DraftDigest || v.TxID != r.TxID || pilot.ValidateWithdrawalReview(*v) != nil || x.review != nil && *x.review != *v {
			return false
		}
		copy := *v
		x.review = &copy
		return true
	}
	if a := out.Action; a != nil {
		if a.Binding != x.binding || a.OperationID != r.OperationID || a.ActionID != r.ActionID || pilot.ValidateWithdrawalAction(*a) != nil {
			return false
		}
		kind := strings.TrimPrefix(x.request.Command, "withdrawal_")
		if kind == "review" {
			kind = x.request.ReviewKind
		}
		if a.Kind != kind {
			return false
		}
		if x.review != nil && a.ReviewID != x.review.ReviewID || x.review == nil && x.request.ReviewID != a.ReviewID {
			return false
		}
		digest := historyDigest(*a)
		if x.terminal != "" && x.terminal != digest {
			return false
		}
		if a.Operation != nil {
			if x.request.Intent != nil && a.Operation.Intent != *x.request.Intent || !b.withdrawalOperationBoundLocked(*a.Operation) {
				return false
			}
		}
		copy := *a
		if a.Operation != nil {
			operation := *b.withdrawalOperation
			copy.Operation = &operation
		}
		x.action = &copy
		if a.State != "running" && a.State != "review" {
			x.terminal = digest
		}
		return true
	}
	return false
}
