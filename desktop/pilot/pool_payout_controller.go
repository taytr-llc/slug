// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
	"time"
)

type poolPayoutCache struct {
	job     PoolPayoutJob
	history poolsettle.PayoutHistory
	bytes   uint64
}
type poolPayoutOperation struct {
	kind      string
	job       PoolPayoutJob
	cache     *poolPayoutCache
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	cancelled bool
}

func clonePayoutJob(j PoolPayoutJob) PoolPayoutJob {
	if j.Capture != nil {
		v := *j.Capture
		j.Capture = &v
	}
	return j
}
func (c *PoolChild) payoutBindingLocked() PoolPayoutBinding {
	return PoolPayoutBinding{View: c.stable.viewPin(), SourceBindingID: c.resources.Recipient.SourceBindingID, RecipientScript: c.resources.Recipient.Recipient.Script}
}

// Reuse the existing hardened original parent-epoch publication fence. It
// performs no evidence, file, career-store or proof work.
func (c *PoolChild) payoutFence(ctx context.Context, v string, fn func() error) error {
	if ctx == nil {
		return gymstable.ErrInvalid
	}
	s, e := c.readyStable()
	if e != nil {
		return e
	}
	return s.evidencePublish(ctx, v, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || !c.prepared {
			return gymstable.ErrClosed
		}
		if e := fn(); e != nil {
			return e
		}
		return check()
	})
}
func (c *PoolChild) PoolPayoutSnapshot(ctx context.Context) (PoolPayoutSnapshot, error) {
	out := BlankPoolPayoutSnapshot()
	if ctx == nil {
		return out, gymstable.ErrInvalid
	}
	s, e := c.readyStable()
	if e != nil {
		return out, nil
	}
	e = c.payoutFence(ctx, s.view.ViewID, func() error {
		b := c.payoutBindingLocked()
		if !payoutBindingValid(b) {
			return ErrPayoutUnavailable
		}
		out = PoolPayoutSnapshot{Version: 1, State: "idle", Binding: &b, Activity: "none"}
		if c.payoutLast != nil {
			out.LastJobID = c.payoutLast.JobID
		}
		if c.payoutCache != nil {
			out.CaptureJobID = c.payoutCache.job.JobID
		}
		if op := c.payoutOp; op != nil {
			out.State, out.Activity, out.ActiveJobID = "busy", op.kind, op.job.JobID
		}
		return nil
	})
	if e != nil {
		return BlankPoolPayoutSnapshot(), e
	}
	return out, nil
}
func (c *PoolChild) payoutLookupLocked(id string) (PoolPayoutJob, bool) {
	if op := c.payoutOp; op != nil && op.kind == "capture" && op.job.JobID == id {
		return clonePayoutJob(op.job), true
	}
	if j := c.payoutLast; j != nil && j.JobID == id {
		return clonePayoutJob(*j), true
	}
	if x := c.payoutCache; x != nil && x.job.JobID == id {
		return clonePayoutJob(x.job), true
	}
	return PoolPayoutJob{}, false
}
func (c *PoolChild) payoutLookup(ctx context.Context, v, id string, cancel bool) (PoolPayoutJob, error) {
	if !payoutNonzeroID(id) {
		return PoolPayoutJob{}, gymstable.ErrInvalid
	}
	var out PoolPayoutJob
	e := c.payoutFence(ctx, v, func() error {
		var ok bool
		out, ok = c.payoutLookupLocked(id)
		if !ok {
			return ErrPayoutNotFound
		}
		if cancel {
			if op := c.payoutOp; op != nil && op.kind == "capture" && op.job.JobID == id {
				op.cancelled = true
				op.cancel()
			}
		}
		return nil
	})
	if e != nil {
		return PoolPayoutJob{}, e
	}
	return out, nil
}
func (c *PoolChild) PoolPayoutJob(ctx context.Context, v, id string) (PoolPayoutJob, error) {
	return c.payoutLookup(ctx, v, id, false)
}
func (c *PoolChild) CancelPoolPayoutHistory(ctx context.Context, v, id string) (PoolPayoutJob, error) {
	return c.payoutLookup(ctx, v, id, true)
}
func (c *PoolChild) BeginPoolPayoutHistory(ctx context.Context, v, id string) (PoolPayoutJob, error) {
	if wire.TestPoW {
		return PoolPayoutJob{}, gymstable.ErrProfile
	}
	if ctx == nil || !payoutNonzeroID(id) {
		return PoolPayoutJob{}, gymstable.ErrInvalid
	}
	if !c.stopMu.TryLock() {
		return PoolPayoutJob{}, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	task, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	op := &poolPayoutOperation{kind: "capture", ctx: task, cancel: cancel, done: make(chan struct{})}
	var out PoolPayoutJob
	admitted := false
	e := c.payoutFence(ctx, v, func() error {
		if j, ok := c.payoutLookupLocked(id); ok {
			out = j
			return nil
		}
		if c.backup != nil || c.payoutOp != nil {
			return gymstable.ErrBusy
		}
		if c.payoutScratch != 0 {
			return gymstable.ErrCapacity
		}
		b := c.payoutBindingLocked()
		if !payoutBindingValid(b) {
			return ErrPayoutUnavailable
		}
		op.job = PoolPayoutJob{Version: 1, JobID: id, State: "running", Binding: b}
		c.payoutOp = op
		c.payoutScratch = PoolPayoutScratchBytes
		c.payoutLast = nil
		out = clonePayoutJob(op.job)
		admitted = true
		return nil
	})
	if !admitted {
		cancel()
		if e != nil {
			return PoolPayoutJob{}, e
		}
		return out, nil
	}
	go c.runPoolPayout(op)
	if e != nil {
		return PoolPayoutJob{}, ErrPayoutReplyUnknown
	}
	return out, nil
}
func (c *PoolChild) payoutOperationLocked(op *poolPayoutOperation) error {
	if c.payoutOp != op || c.backup != nil || c.payoutBindingLocked() != op.job.Binding {
		return gymstable.ErrStaleView
	}
	return op.ctx.Err()
}
func (c *PoolChild) runPoolPayout(op *poolPayoutOperation) {
	reader := c.payoutReader
	if reader == nil {
		reader = c.authority
	}
	result, e := reader.Payouts(op.ctx)
	var cache *poolPayoutCache
	if e == nil {
		var owned poolsettle.PayoutHistory
		owned, e = poolsettle.ValidatePayoutHistory(result.History, c.resources.Recipient.Config.Identity.GymID)
		if e == nil && (result.Version != 1 || result.BindingID != op.job.Binding.SourceBindingID || owned.RecipientScript != op.job.Binding.RecipientScript) {
			e = ErrPayoutUnavailable
		}
		if e == nil {
			result.History = owned
			raw, x := payoutJSON(result)
			if x != nil || len(raw)+1 > PoolPayoutCaptureBytes {
				e = gymstable.ErrCapacity
			} else {
				meta := PoolPayoutCapture{Version: 1, Count: uint32(len(owned.Items)), PaidAtoms: owned.Balance.PaidAtoms, ReservedAtoms: owned.Balance.ReservedAtoms, SettlementObservedAtMS: owned.SettlementObservedAtMS, CapturedAtMS: time.Now().UnixMilli()}
				j := op.job
				j.State, j.Capture = "done", &meta
				if ValidatePoolPayoutJob(j) != nil {
					e = ErrPayoutUnavailable
				} else {
					cache = &poolPayoutCache{job: j, history: owned, bytes: uint64(len(raw) + 1)}
				}
			}
			clear(raw)
		}
	}
	result = poolservice.PayoutHistoryResponse{}
	if e == nil {
		e = c.payoutFence(op.ctx, op.job.Binding.View.ViewID, func() error {
			if x := c.payoutOperationLocked(op); x != nil {
				return x
			}
			c.payoutCache = cache
			c.payoutCacheBytes = cache.bytes
			j := clonePayoutJob(cache.job)
			c.payoutLast = &j
			return nil
		})
	}
	if e != nil {
		j := op.job
		j.State, j.Error = "failed", payoutTaskError(e)
		c.mu.Lock()
		cancelled := op.cancelled
		c.mu.Unlock()
		if cancelled {
			j.State, j.Error = "cancelled", ""
		}
		publish, done := context.WithTimeout(c.ctx, 2*time.Second)
		_ = c.payoutFence(publish, j.Binding.View.ViewID, func() error {
			if c.payoutOp != op {
				return gymstable.ErrStaleView
			}
			if c.payoutCache == cache && cache != nil {
				return nil
			}
			copy := clonePayoutJob(j)
			c.payoutLast = &copy
			return nil
		})
		done()
	}
	cache = nil
	c.finishPayoutOperation(op)
}
func (c *PoolChild) finishPayoutOperation(op *poolPayoutOperation) {
	op.cancel()
	op.cache = nil
	c.mu.Lock()
	if c.payoutOp == op {
		c.payoutOp = nil
		c.payoutScratch = 0
	}
	close(op.done)
	c.mu.Unlock()
}

// Caller holds stopMu, never an account/cache lock. Cancellation is joined
// before backup/Close touches the retained wallet or borrowed source.
func (c *PoolChild) cancelPoolPayout() {
	c.mu.Lock()
	op := c.payoutOp
	if op != nil {
		op.cancelled = true
		op.cancel()
	}
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
}
func (c *PoolChild) PoolPayoutPage(ctx context.Context, v, id string, after, limit uint32) (PoolPayoutPage, error) {
	if !payoutNonzeroID(id) || after > 366 || limit == 0 || limit > 8 {
		return PoolPayoutPage{}, gymstable.ErrInvalid
	}
	var out PoolPayoutPage
	e := c.payoutFence(ctx, v, func() error {
		x := c.payoutCache
		if x == nil || x.job.JobID != id {
			return ErrPayoutNotFound
		}
		meta := *x.job.Capture
		if after > meta.Count {
			return gymstable.ErrInvalid
		}
		next := after + min(limit, meta.Count-after)
		out = PoolPayoutPage{Version: 1, Binding: x.job.Binding, JobID: id, Capture: meta, After: after, Limit: limit, Next: next, HasMore: next < meta.Count, Items: append([]poolsettle.PayoutItem{}, x.history.Items[after:next]...)}
		return nil
	})
	if e != nil {
		return PoolPayoutPage{}, e
	}
	if ValidatePoolPayoutPage(out) != nil {
		return PoolPayoutPage{}, ErrPayoutUnavailable
	}
	return out, nil
}
func (c *PoolChild) ReadPoolPayoutTransaction(parent context.Context, v, id, txid string, index uint32) (PoolPayoutObservation, error) {
	if wire.TestPoW {
		return PoolPayoutObservation{}, gymstable.ErrProfile
	}
	if parent == nil || !payoutNonzeroID(id) || !canonicalManifestID(txid) || index > 255 {
		return PoolPayoutObservation{}, gymstable.ErrInvalid
	}
	if !c.stopMu.TryLock() {
		return PoolPayoutObservation{}, gymstable.ErrBusy
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	op := &poolPayoutOperation{kind: "wallet", ctx: ctx, cancel: func() { stop(); cancel() }, done: make(chan struct{})}
	var out PoolPayoutObservation
	admitted := false
	e := c.payoutFence(ctx, v, func() error {
		if c.backup != nil || c.payoutOp != nil {
			return gymstable.ErrBusy
		}
		if c.payoutScratch != 0 {
			return gymstable.ErrCapacity
		}
		x := c.payoutCache
		if x == nil || x.job.JobID != id {
			return ErrPayoutNotFound
		}
		var selected *poolsettle.PayoutItem
		for i := range x.history.Items {
			p := &x.history.Items[i]
			if p.TxID == txid && p.OutputIndex == index {
				if selected != nil {
					return ErrPayoutUnavailable
				}
				selected = p
			}
		}
		if selected == nil {
			return ErrPayoutNotFound
		}
		op.job, op.cache = clonePayoutJob(x.job), x
		out = PoolPayoutObservation{Version: 1, Binding: x.job.Binding, JobID: id, Capture: *x.job.Capture, Payment: *selected}
		c.payoutOp, c.payoutScratch = op, PoolPayoutScratchBytes
		admitted = true
		return nil
	})
	c.stopMu.Unlock()
	if !admitted {
		op.cancel()
		return PoolPayoutObservation{}, e
	}
	defer c.finishPayoutOperation(op)
	if e != nil {
		return PoolPayoutObservation{}, e
	}
	observe := c.payoutObserve
	if observe == nil {
		observe = c.wallet.ObservePoolPayout
	}
	read, e := observe(ctx, walletPayoutSelection{TxID: txid, OutputIndex: index, Atoms: out.Payment.Atoms, RecipientScript: out.Binding.RecipientScript})
	if ctx.Err() != nil {
		return PoolPayoutObservation{}, ctx.Err()
	}
	if errors.Is(e, errWalletPayoutBusy) {
		return PoolPayoutObservation{}, gymstable.ErrBusy
	}
	if e != nil {
		out.State, out.Error, out.ObservedAtMS = "unavailable", "wallet_unavailable", time.Now().UnixMilli()
	} else {
		out.State, out.ObservedAtMS, out.WalletSynced = read.State, read.ObservedAtMS, true
		out.WalletHeight, out.WalletTip, out.Confirmations, out.BlockHash = read.WalletHeight, read.WalletTip, read.Confirmations, read.BlockHash
	}
	if ValidatePoolPayoutObservation(out) != nil {
		return PoolPayoutObservation{}, ErrPayoutUnavailable
	}
	e = c.payoutFence(ctx, v, func() error {
		if x := c.payoutOperationLocked(op); x != nil {
			return x
		}
		if c.payoutCache != op.cache {
			return ErrPayoutNotFound
		}
		return nil
	})
	if e != nil {
		return PoolPayoutObservation{}, e
	}
	return out, nil
}

var _ PoolPayoutSurfaceAPI = (*PoolChild)(nil)
