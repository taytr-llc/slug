// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/walletwithdraw"
	"reflect"
	"time"
)

func (c *PoolChild) beginWithdrawal(ctx context.Context, r WithdrawalRequest) (WithdrawalAction, error) {
	if !c.stopMu.TryLock() {
		return WithdrawalAction{}, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	cap := gymaccount.Withdrawal
	kind := ""
	switch r.Command {
	case "withdrawal_prepare":
		kind = "prepare"
	case "withdrawal_approve":
		kind = "send"
	case "withdrawal_abandon":
		kind = "abandon"
	case "withdrawal_observe":
		kind = "observe"
		cap = gymaccount.View
	default:
		return WithdrawalAction{}, gymstable.ErrInvalid
	}
	taskctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	t := &withdrawalTask{request: r, ctx: taskctx, cancel: cancel, done: make(chan struct{})}
	var out WithdrawalAction
	admitted := false
	e := c.withdrawalFence(ctx, r.ViewID, cap, nil)(func(check func() error) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if current := c.withdrawalTask; current != nil {
			if current.action.ActionID == r.ActionID && withdrawalSameRequest(current.request, r) {
				out = cloneWithdrawalAction(current.action)
				return nil
			}
			return gymstable.ErrBusy
		}
		if prior := c.withdrawalLast; prior != nil && prior.ActionID == r.ActionID && prior.State != "review" {
			if c.withdrawalLastRequest == nil || !withdrawalSameRequest(*c.withdrawalLastRequest, r) {
				return walletwithdraw.ErrConflict
			}
			out = cloneWithdrawalAction(*prior)
			return nil
		}
		if c.backup != nil || c.backupCloseErr != nil || c.withdrawalScratch != 0 {
			return gymstable.ErrBusy
		}
		if r.Command == "withdrawal_approve" {
			review := c.withdrawalReviewValue
			if review == nil || review.OperationID != r.OperationID || review.ActionID != r.ActionID || review.ReviewID != r.ReviewID || review.Draft.DraftDigest != r.DraftDigest || review.TxID != r.TxID || review.Binding.View.ViewID != r.ViewID || time.Now().UnixMilli() < review.ReviewedAtMS || time.Now().UnixMilli() >= review.ExpiresAtMS {
				return gymaccount.ErrUnauthorized
			}
			copy := *review
			t.review = &copy
			kind = review.Kind
		} else if c.withdrawalReviewValue != nil {
			return gymstable.ErrBusy
		}
		t.action = WithdrawalAction{Version: 1, Binding: c.withdrawalBinding(), OperationID: r.OperationID, ActionID: r.ActionID, Kind: kind, State: "running", ReviewID: r.ReviewID}
		c.withdrawalTask = t
		c.withdrawalScratch = WithdrawalWorkBytes
		c.withdrawalLast = nil
		c.withdrawalLastRequest = nil
		c.withdrawalReviewValue = nil
		out = cloneWithdrawalAction(t.action)
		admitted = true
		return nil
	})
	if !admitted {
		cancel()
		return out, e
	}
	if e != nil {
		// Admission is local metadata only. A malformed/failed outer capability
		// never authorizes a worker, even if it invoked the callback once.
		cancel()
		c.mu.Lock()
		if c.withdrawalTask == t {
			failed := cloneWithdrawalAction(t.action)
			failed.State, failed.Error = "outcome_unknown", "outcome_unknown"
			c.withdrawalLast = &failed
			req := ownedWithdrawalRequest(r)
			c.withdrawalLastRequest = &req
			c.withdrawalTask, c.withdrawalScratch = nil, 0
		}
		close(t.done)
		c.mu.Unlock()
		return WithdrawalAction{}, errors.Join(walletwithdraw.ErrOutcomeUnknown, e)
	}
	go c.runWithdrawal(t)
	return out, nil
}
func (c *PoolChild) runWithdrawal(t *withdrawalTask) {
	var o walletwithdraw.Operation
	owner, e := c.withdrawal.Journal.TryAcquire(t.ctx)
	if e == nil {
		switch t.action.Kind {
		case "prepare":
			o, e = c.prepareWithdrawal(t, owner)
		case "send", "retry":
			o, e = c.sendWithdrawal(t, owner)
		case "abandon":
			o, e = c.abandonWithdrawal(t, owner)
		case "observe":
			o, e = c.observeWithdrawal(t, owner)
		}
		if latest, x := owner.Lookup(t.action.OperationID); x == nil {
			o = latest
		}
		e = errors.Join(e, owner.Close())
	}
	var projected *WithdrawalOperation
	if o.OperationID != "" {
		p, x := withdrawalProjection(t.action.Binding, o)
		if x != nil {
			e = errors.Join(e, x)
		} else {
			projected = &p
		}
	}
	final := cloneWithdrawalAction(t.action)
	final.Operation = projected
	final.State = "done"
	c.mu.Lock()
	stopped := t.stopped
	c.mu.Unlock()
	if e != nil {
		final.State, final.Error = "failed", withdrawalError(e)
		if final.Error == "outcome_unknown" {
			final.State = "outcome_unknown"
		} else if stopped {
			final.State, final.Error = "stopped", ""
		}
	}
	if stopped && e == nil {
		final.State = "stopped"
	}
	publish, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	_ = c.withdrawalFence(publish, t.action.Binding.View.ViewID, gymaccount.View, t)(func(check func() error) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		copy := cloneWithdrawalAction(final)
		c.withdrawalLast = &copy
		req := t.request
		c.withdrawalLastRequest = &req
		return nil
	})
	cancel()
	t.cancel()
	c.mu.Lock()
	if c.withdrawalTask == t {
		c.withdrawalTask = nil
		c.withdrawalScratch = 0
	}
	close(t.done)
	c.mu.Unlock()
}
func (c *PoolChild) prepareWithdrawal(t *withdrawalTask, owner *walletwithdraw.Owner) (o walletwithdraw.Operation, e error) {
	request := walletwithdraw.Request{Version: 1, OperationID: t.request.OperationID, Recipient: t.request.Intent.Destination, AmountAtoms: t.request.Intent.AmountAtoms}
	p, e := owner.StageIntent(request)
	if e != nil {
		return o, e
	}
	o, e = c.publishWithdrawal(t.ctx, p, gymaccount.Withdrawal, t)
	if e != nil {
		return o, e
	}
	if o.Stage != walletwithdraw.StageIntent {
		return c.withdrawalRecover(t.ctx, owner, o)
	}
	// The durable intent precedes all wallet selection and change allocation.
	d, e := c.withdrawalIO.Prepare(t.ctx, request)
	if e != nil {
		return o, e
	}
	p, e = owner.StagePrepared(o.OperationID, d)
	if e != nil {
		return o, e
	}
	return c.publishWithdrawal(t.ctx, p, gymaccount.Withdrawal, t)
}
func (c *PoolChild) sendWithdrawal(t *withdrawalTask, owner *walletwithdraw.Owner) (o walletwithdraw.Operation, e error) {
	o, e = owner.Lookup(t.action.OperationID)
	if e != nil {
		return o, e
	}
	if t.review == nil || o.Draft == nil || o.DraftDigest != t.review.Draft.DraftDigest {
		return o, walletwithdraw.ErrConflict
	}
	if t.action.Kind == "send" {
		if o.Stage != walletwithdraw.StagePrepared {
			return o, walletwithdraw.ErrConflict
		}
		if e = c.withdrawalCheck(t.ctx, t.request.ViewID, gymaccount.Withdrawal); e != nil {
			return o, e
		}
		if e = c.withdrawalIO.Inspect(t.ctx, *o.Draft); e != nil {
			return o, e
		}
		raw, x := c.withdrawalIO.Sign(t.ctx, *o.Draft)
		if x != nil {
			return o, x
		}
		p, x := owner.StageSigned(o.OperationID, raw, withdrawalAuthorization(t))
		clear(raw)
		if x != nil {
			return o, x
		}
		o, e = c.publishWithdrawal(t.ctx, p, gymaccount.Withdrawal, t)
		if e != nil {
			return o, e
		}
	} else if o.Signed == nil || o.Signed.TxID != t.review.TxID || !stableOne(string(o.Stage), "signed", "dispatching") {
		return o, walletwithdraw.ErrConflict
	}
	rec, e := owner.Reconciliation(o.OperationID)
	if e != nil {
		return o, e
	}
	if e = c.withdrawalIO.Reconcile(t.ctx, rec); e != nil {
		return o, e
	}
	p, e := owner.StageDispatch(o.OperationID, withdrawalAuthorization(t))
	if e != nil {
		return o, e
	}
	o, e = c.publishWithdrawal(t.ctx, p, gymaccount.Withdrawal, t)
	if e != nil {
		return o, e
	}
	ticket, e := owner.TakeDelivery(o.OperationID)
	if e != nil {
		return o, e
	}
	transport, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	defer cancel()
	result, code, deliveryErr := c.withdrawalIO.Deliver(transport, ticket)
	if deliveryErr != nil {
		result, code = walletwithdraw.DeliveryUnknown, "node_unavailable"
	}
	p, e = owner.StageDeliveryResult(o.OperationID, o.Dispatch.Attempt, result, code)
	if e != nil {
		return o, e
	}
	o, e = c.publishWithdrawal(t.ctx, p, gymaccount.View, t)
	if e != nil {
		return o, e
	}
	// Only exact observation, never a send acknowledgement, ends unresolved state.
	observation, x := c.withdrawalIO.Observe(transport, owner, o.OperationID)
	if x != nil {
		observation = walletwithdraw.ObservationInput{State: walletwithdraw.ObservationUnknown, Error: "node_unavailable"}
	}
	p, e = owner.StageObservation(o.OperationID, observation)
	if e != nil {
		return o, e
	}
	return c.publishWithdrawal(t.ctx, p, gymaccount.View, t)
}
func withdrawalAuthorization(t *withdrawalTask) walletwithdraw.AuthorizationInput {
	return walletwithdraw.AuthorizationInput{ActionID: t.action.ActionID, ReviewID: t.action.ReviewID, ViewID: t.action.Binding.View.ViewID, Generation: t.action.Binding.View.Generation}
}
func (c *PoolChild) abandonWithdrawal(t *withdrawalTask, owner *walletwithdraw.Owner) (o walletwithdraw.Operation, e error) {
	o, e = owner.Lookup(t.action.OperationID)
	if e != nil {
		return o, e
	}
	o, e = c.withdrawalRecover(t.ctx, owner, o)
	if e != nil {
		return o, e
	}
	if o.DraftDigest != t.request.DraftDigest || o.Signed != nil {
		return o, walletwithdraw.ErrConflict
	}
	if o.Draft == nil {
		if adapter, ok := c.withdrawalIO.(interface {
			AbandonUnprepared(context.Context, string) error
		}); ok {
			if e := adapter.AbandonUnprepared(t.ctx, o.OperationID); e != nil {
				return o, e
			}
		}
	}
	p, e := owner.StageAbandon(o.OperationID)
	if e != nil {
		return o, e
	}
	o, e = c.publishWithdrawal(t.ctx, p, gymaccount.Withdrawal, t)
	if e != nil {
		return o, e
	}
	if o.Draft != nil {
		rec, x := owner.Reconciliation(o.OperationID)
		if x != nil {
			return o, x
		}
		e = c.withdrawalIO.Reconcile(t.ctx, rec)
	}
	return o, e
}
func (c *PoolChild) observeWithdrawal(t *withdrawalTask, owner *walletwithdraw.Owner) (o walletwithdraw.Operation, e error) {
	o, e = owner.Lookup(t.action.OperationID)
	if e != nil {
		return o, e
	}
	if o.Signed == nil || o.Signed.TxID != t.request.TxID {
		return o, walletwithdraw.ErrConflict
	}
	o, e = c.withdrawalRecover(t.ctx, owner, o)
	if e != nil {
		return o, e
	}
	ctx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	defer cancel()
	input, x := c.withdrawalIO.Observe(ctx, owner, o.OperationID)
	if x != nil {
		input = walletwithdraw.ObservationInput{State: walletwithdraw.ObservationUnknown, Error: "node_unavailable"}
	}
	p, e := owner.StageObservation(o.OperationID, input)
	if e != nil {
		return o, e
	}
	return c.publishWithdrawal(t.ctx, p, gymaccount.View, t)
}

// Caller holds stopMu. Join outside every account/cache lock before releasing
// the wallet, journal, scratch accounting or a subsequent child generation.
func (c *PoolChild) cancelWithdrawal() {
	c.mu.Lock()
	t := c.withdrawalTask
	if t != nil {
		t.stopped = true
		t.cancel()
	}
	if c.withdrawalLast != nil && c.withdrawalLast.State == "review" {
		c.withdrawalLast.State = "stopped"
	}
	c.withdrawalReviewValue = nil
	c.mu.Unlock()
	if t != nil {
		<-t.done
	}
}

func withdrawalSameRequest(a, b WithdrawalRequest) bool {
	a.ID, b.ID = 1, 1
	return reflect.DeepEqual(a, b)
}
