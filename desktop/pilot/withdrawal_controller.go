// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/wallet"
	"sync"
	"time"
)

var errWithdrawalFresh = errors.New("withdrawal requires fresh authentication")

type withdrawalTask struct {
	request WithdrawalRequest
	action  WithdrawalAction
	review  *WithdrawalReview
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool
}

func withdrawalError(e error) string {
	switch {
	case errors.Is(e, errWithdrawalFresh):
		return "reauthentication_required"
	case errors.Is(e, walletwithdraw.ErrOutcomeUnknown), errors.Is(e, walletwithdraw.ErrPoisoned), errors.Is(e, wallet.ErrWithdrawalOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, gymaccount.ErrHeld):
		return "held"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired), errors.Is(e, walletwithdraw.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(e, walletwithdraw.ErrNotFound), errors.Is(e, wallet.ErrWithdrawalNotFound):
		return "withdrawal_not_found"
	case errors.Is(e, walletwithdraw.ErrBusy), errors.Is(e, wallet.ErrWithdrawalBusy), errors.Is(e, gymstable.ErrBusy):
		return "busy"
	case errors.Is(e, walletwithdraw.ErrCapacity), errors.Is(e, wallet.ErrWithdrawalCapacity):
		return "capacity"
	case errors.Is(e, walletwithdraw.ErrConflict), errors.Is(e, wallet.ErrWithdrawalConflict):
		return "operation_conflict"
	case errors.Is(e, gymstable.ErrInvalid), errors.Is(e, walletwithdraw.ErrInvalid), errors.Is(e, wallet.ErrWithdrawalInvalid):
		return "invalid_request"
	}
	return "withdrawal_unavailable"
}
func (c *PoolChild) withdrawalBinding() WithdrawalBinding {
	return WithdrawalBinding{View: c.stable.viewPin(), SourceBindingID: c.resources.Recipient.SourceBindingID, JournalID: c.withdrawal.Scope.Identity.JournalID, WithdrawalBindingID: c.withdrawal.Scope.BindingID}
}
func (c *PoolChild) withdrawalFence(ctx context.Context, v string, cap gymaccount.Capability, task *withdrawalTask) walletwithdraw.Fence {
	return func(fn func(func() error) error) error {
		if ctx == nil || c.account.Lease == nil || fn == nil {
			return gymaccount.ErrUnauthorized
		}
		if cap == gymaccount.Withdrawal {
			now := time.Now().UnixMilli()
			if c.account.AuthenticatedAtMS <= 0 || now < c.account.AuthenticatedAtMS || now >= c.account.AuthenticatedAtMS+300000 || now >= c.account.SessionExpiresAtMS {
				return errWithdrawalFresh
			}
		}
		var gate sync.Mutex
		calls := 0
		sealed := false
		var inner error
		outer := c.account.Lease(ctx, cap, func(a gymaccount.Account, recheck func() error) error {
			gate.Lock()
			defer gate.Unlock()
			calls++
			if sealed || calls != 1 || recheck == nil {
				inner = gymaccount.ErrUnauthorized
				return inner
			}
			check := func() error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if c.ctx.Err() != nil {
					return c.ctx.Err()
				}
				if e := recheck(); e != nil {
					return e
				}
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.closed || !c.prepared || c.withdrawal == nil || c.stable == nil || c.account.Manifest.ViewID != v || a.ID != c.AccountID() || a.Generation != c.account.Manifest.Generation || task != nil && c.withdrawalTask != task {
					return gymaccount.ErrUnauthorized
				}
				if cap == gymaccount.Withdrawal {
					now := time.Now().UnixMilli()
					if c.account.AuthenticatedAtMS <= 0 || now < c.account.AuthenticatedAtMS || now >= c.account.AuthenticatedAtMS+300000 || now >= c.account.SessionExpiresAtMS {
						return gymaccount.ErrUnauthorized
					}
					if task != nil && task.review != nil && (now < task.review.ReviewedAtMS || now >= task.review.ExpiresAtMS) {
						return gymaccount.ErrUnauthorized
					}
				}
				return nil
			}
			if inner = check(); inner == nil {
				inner = fn(check)
			}
			if inner == nil {
				inner = check()
			}
			return inner
		})
		gate.Lock()
		defer gate.Unlock()
		sealed = true
		if calls != 1 {
			return errors.Join(gymaccount.ErrUnauthorized, inner, outer)
		}
		return errors.Join(inner, outer)
	}
}
func (c *PoolChild) withdrawalCheck(ctx context.Context, v string, cap gymaccount.Capability) error {
	return c.withdrawalFence(ctx, v, cap, nil)(func(check func() error) error { return check() })
}
func (c *PoolChild) HandleWithdrawal(ctx context.Context, r WithdrawalRequest) WithdrawalResponse {
	if ctx == nil || !withdrawalRequestValid(r) {
		return WithdrawalFailure(r.ID, "invalid_request")
	}
	r = ownedWithdrawalRequest(r)
	c.mu.Lock()
	ready := c.prepared && !c.closed && c.withdrawal != nil
	v := c.account.Manifest.ViewID
	c.mu.Unlock()
	if !ready && r.Command == "withdrawal_snapshot" {
		s := BlankWithdrawalSnapshot()
		return WithdrawalResponse{ID: r.ID, Version: 1, OK: true, Snapshot: &s}
	}
	if !ready {
		return WithdrawalFailure(r.ID, "withdrawal_unavailable")
	}
	if r.Command != "withdrawal_snapshot" && r.ViewID != v {
		return WithdrawalFailure(r.ID, "stale_view")
	}
	out := WithdrawalResponse{ID: r.ID, Version: 1, OK: true}
	var e error
	switch r.Command {
	case "withdrawal_snapshot":
		var s WithdrawalSnapshot
		s, e = c.withdrawalSnapshot(ctx, v)
		out.Snapshot = &s
	case "withdrawal_operation":
		var o WithdrawalOperation
		o, e = c.withdrawalOperation(ctx, v, r.OperationID)
		out.Operation = &o
	case "withdrawal_action", "withdrawal_stop":
		var a WithdrawalAction
		a, e = c.withdrawalAction(ctx, r, r.Command == "withdrawal_stop")
		out.Action = &a
	case "withdrawal_review":
		var review WithdrawalReview
		review, e = c.withdrawalReview(ctx, r)
		out.Review = &review
	default:
		var a WithdrawalAction
		a, e = c.beginWithdrawal(ctx, r)
		out.Action = &a
	}
	if e != nil {
		return WithdrawalFailure(r.ID, withdrawalError(e))
	}
	if !withdrawalResponseValid(out) {
		return WithdrawalFailure(r.ID, "withdrawal_unavailable")
	}
	return out
}
func (c *PoolChild) withdrawalSnapshot(ctx context.Context, v string) (WithdrawalSnapshot, error) {
	if e := c.withdrawalCheck(ctx, v, gymaccount.View); e != nil {
		return WithdrawalSnapshot{}, e
	}
	owner, e := c.withdrawal.Journal.TryAcquire(ctx)
	if e != nil {
		return WithdrawalSnapshot{}, e
	}
	snap, e := owner.Snapshot()
	e = errors.Join(e, owner.Close())
	if e != nil {
		return WithdrawalSnapshot{}, e
	}
	can := c.withdrawalCheck(ctx, v, gymaccount.Withdrawal) == nil
	var out WithdrawalSnapshot
	e = c.withdrawalFence(ctx, v, gymaccount.View, nil)(func(check func() error) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		b := c.withdrawalBinding()
		out = WithdrawalSnapshot{Version: 1, State: "idle", Binding: &b, UnresolvedOperationID: snap.UnresolvedOperationID, LatestOperationID: snap.LatestOperationID, CanPrepare: can && snap.UnresolvedOperationID == "", CanReview: can && snap.UnresolvedOperationID != ""}
		if a := c.withdrawalLast; a != nil {
			out.LastActionID = a.ActionID
		}
		if t := c.withdrawalTask; t != nil {
			out.State, out.ActiveActionID, out.CanPrepare = "busy", t.action.ActionID, false
		}
		return nil
	})
	return out, e
}
func (c *PoolChild) withdrawalRecover(ctx context.Context, owner *walletwithdraw.Owner, o walletwithdraw.Operation) (walletwithdraw.Operation, error) {
	if o.Stage == walletwithdraw.StageIntent {
		row, e := c.withdrawalIO.WalletRecord(ctx, o.OperationID)
		if errors.Is(e, wallet.ErrWithdrawalNotFound) {
			return o, nil
		}
		if e != nil {
			return o, e
		}
		d, e := withdrawalWalletDraft(row.Draft)
		if e != nil {
			return o, e
		}
		p, e := owner.StagePrepared(o.OperationID, d)
		if e != nil {
			return o, e
		}
		o, e = c.publishWithdrawal(ctx, p, gymaccount.View, nil)
		if e != nil {
			return o, e
		}
	}
	if o.Signed != nil || o.Stage == walletwithdraw.StageAbandoned && o.Draft != nil {
		rec, e := owner.Reconciliation(o.OperationID)
		if e != nil {
			return o, e
		}
		if e = c.withdrawalIO.Reconcile(ctx, rec); e != nil {
			return o, e
		}
	}
	return o, nil
}
func (c *PoolChild) withdrawalOperation(ctx context.Context, v, id string) (out WithdrawalOperation, e error) {
	if e := c.withdrawalCheck(ctx, v, gymaccount.View); e != nil {
		return WithdrawalOperation{}, e
	}
	// Recovery is journal/wallet work and must never run under the account lease.
	if !c.stopMu.TryLock() {
		return WithdrawalOperation{}, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	work, release, e := c.reserveWithdrawalRead(ctx)
	if e != nil {
		return WithdrawalOperation{}, e
	}
	defer release()
	ctx = work
	c.mu.Lock()
	busy := c.receiveOp != nil || c.backup != nil || c.withdrawalTask != nil
	c.mu.Unlock()
	if busy {
		return WithdrawalOperation{}, gymstable.ErrBusy
	}
	owner, e := c.withdrawal.Journal.TryAcquire(ctx)
	if e != nil {
		return WithdrawalOperation{}, e
	}
	defer func() { e = errors.Join(e, owner.Close()) }()
	o, e := owner.Lookup(id)
	if e == nil {
		o, e = c.withdrawalRecover(ctx, owner, o)
	}
	if e != nil {
		return WithdrawalOperation{}, e
	}
	out, e = withdrawalProjection(c.withdrawalBinding(), o)
	if e == nil {
		e = c.withdrawalCheck(ctx, v, gymaccount.View)
	}
	return out, e
}
func (c *PoolChild) publishWithdrawal(ctx context.Context, p *walletwithdraw.PreparedCommit, cap gymaccount.Capability, t *withdrawalTask) (o walletwithdraw.Operation, e error) {
	if p == nil {
		return o, walletwithdraw.ErrInvalid
	}
	defer func() { e = errors.Join(e, p.Close()) }()
	return p.Publish(c.withdrawalFence(ctx, c.account.Manifest.ViewID, cap, t))
}
func (c *PoolChild) withdrawalAction(ctx context.Context, r WithdrawalRequest, stop bool) (WithdrawalAction, error) {
	var out WithdrawalAction
	var done <-chan struct{}
	e := c.withdrawalFence(ctx, r.ViewID, gymaccount.View, nil)(func(check func() error) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if t := c.withdrawalTask; t != nil && t.action.ActionID == r.ActionID && t.action.OperationID == r.OperationID {
			out = cloneWithdrawalAction(t.action)
			if stop {
				t.stopped = true
				t.cancel()
				done = t.done
			}
			return nil
		}
		if a := c.withdrawalLast; a != nil && a.ActionID == r.ActionID && a.OperationID == r.OperationID {
			out = cloneWithdrawalAction(*a)
			if stop && a.State == "review" {
				a.State = "stopped"
				c.withdrawalReviewValue = nil
				out = cloneWithdrawalAction(*a)
			}
			return nil
		}
		return walletwithdraw.ErrNotFound
	})
	if e != nil {
		return WithdrawalAction{}, e
	}
	if done != nil {
		<-done
		return c.withdrawalAction(ctx, r, false)
	}
	return out, nil
}
func (c *PoolChild) withdrawalReview(ctx context.Context, r WithdrawalRequest) (out WithdrawalReview, e error) {
	if !c.stopMu.TryLock() {
		return WithdrawalReview{}, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	work, release, e := c.reserveWithdrawalRead(ctx)
	if e != nil {
		return WithdrawalReview{}, e
	}
	defer release()
	ctx = work
	if e := c.withdrawalCheck(ctx, r.ViewID, gymaccount.Withdrawal); e != nil {
		return WithdrawalReview{}, e
	}
	c.mu.Lock()
	old := c.withdrawalReviewValue
	busy := c.receiveOp != nil || c.backup != nil || c.withdrawalTask != nil
	c.mu.Unlock()
	if busy {
		return WithdrawalReview{}, gymstable.ErrBusy
	}
	if old != nil && old.ActionID == r.ActionID {
		if old.OperationID != r.OperationID || old.ReviewID != r.ReviewID || old.Kind != r.ReviewKind || old.Draft.DraftDigest != r.DraftDigest || old.TxID != r.TxID {
			return WithdrawalReview{}, walletwithdraw.ErrConflict
		}
		return *old, nil
	}
	owner, e := c.withdrawal.Journal.TryAcquire(ctx)
	if e != nil {
		return WithdrawalReview{}, e
	}
	defer func() { e = errors.Join(e, owner.Close()) }()
	o, e := owner.Lookup(r.OperationID)
	if e == nil {
		o, e = c.withdrawalRecover(ctx, owner, o)
	}
	if e != nil {
		return WithdrawalReview{}, e
	}
	p, e := withdrawalProjection(c.withdrawalBinding(), o)
	if e != nil {
		return WithdrawalReview{}, e
	}
	if p.Draft == nil || p.Draft.DraftDigest != r.DraftDigest || r.ReviewKind == "send" && p.Stage != "prepared" || r.ReviewKind == "retry" && (!stableOne(p.Stage, "signed", "dispatching") || p.TxID != r.TxID) {
		return WithdrawalReview{}, walletwithdraw.ErrConflict
	}
	// A new send review is actionable only after exact owned-node input and
	// wallet-tip inspection. Retry reviews keep their saved signed transaction:
	// its inputs may already be consumed by that exact payment.
	if r.ReviewKind == "send" {
		if e = c.withdrawalIO.Inspect(ctx, *o.Draft); e != nil {
			return WithdrawalReview{}, e
		}
	}
	now := time.Now().UnixMilli()
	expires := min(now+300000, c.account.AuthenticatedAtMS+300000, c.account.SessionExpiresAtMS)
	review := WithdrawalReview{Version: 1, Binding: c.withdrawalBinding(), OperationID: r.OperationID, ActionID: r.ActionID, ReviewID: r.ReviewID, Kind: r.ReviewKind, Draft: *p.Draft, TxID: r.TxID, ReviewedAtMS: now, ExpiresAtMS: expires}
	if ValidateWithdrawalReview(review) != nil {
		return WithdrawalReview{}, gymaccount.ErrUnauthorized
	}
	e = c.withdrawalFence(ctx, r.ViewID, gymaccount.Withdrawal, nil)(func(check func() error) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.receiveOp != nil || c.backup != nil || c.withdrawalTask != nil {
			return gymstable.ErrBusy
		}
		if a := c.withdrawalLast; a != nil && a.State == "review" && a.ActionID != r.ActionID {
			return gymstable.ErrBusy
		}
		c.withdrawalReviewValue = &review
		req := r
		c.withdrawalLastRequest = &req
		c.withdrawalLast = &WithdrawalAction{Version: 1, Binding: review.Binding, OperationID: r.OperationID, ActionID: r.ActionID, Kind: r.ReviewKind, State: "review", ReviewID: r.ReviewID, Operation: &p}
		return nil
	})
	return review, e
}

func ownedWithdrawalRequest(r WithdrawalRequest) WithdrawalRequest {
	if r.Intent != nil {
		i := *r.Intent
		r.Intent = &i
	}
	return r
}

// A synchronous lookup/review is also accounted work. stopMu excludes new work;
// its bounded context is linked to child close, and all cleanup is joined.
func (c *PoolChild) reserveWithdrawalRead(ctx context.Context) (context.Context, func(), error) {
	c.mu.Lock()
	if c.receiveOp != nil || c.backup != nil || c.withdrawalTask != nil || c.withdrawalScratch != 0 {
		c.mu.Unlock()
		return nil, nil, gymstable.ErrBusy
	}
	c.withdrawalScratch = WithdrawalWorkBytes
	c.mu.Unlock()
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	return work, func() { stop(); cancel(); c.mu.Lock(); c.withdrawalScratch = 0; c.mu.Unlock() }, nil
}
