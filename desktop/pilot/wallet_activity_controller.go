// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

type walletActivityOperation struct {
	kind      string
	job       WalletActivityJob
	cache     *walletActivityCache
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	cancelled bool
}
type walletActivityProjection struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func cloneWalletActivityJob(j WalletActivityJob) WalletActivityJob {
	if j.Capture != nil {
		x := *j.Capture
		j.Capture = &x
	}
	return j
}
func (c *PoolChild) activityBindingLocked() WalletActivityBinding {
	f := c.resources.Recipient.Config.Identity
	return WalletActivityBinding{View: c.stable.viewPin(), SourceBindingID: c.resources.Recipient.SourceBindingID, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
}
func (c *PoolChild) WalletActivitySnapshot(ctx context.Context) (WalletActivitySnapshot, error) {
	if ctx == nil {
		return WalletActivitySnapshot{}, gymstable.ErrInvalid
	}
	s, e := c.readyStable()
	if e != nil {
		return BlankWalletActivitySnapshot(), nil
	}
	out := BlankWalletActivitySnapshot()
	e = c.payoutFence(ctx, s.view.ViewID, func() error {
		b := c.activityBindingLocked()
		if ValidateWalletActivityBinding(b) != nil {
			return ErrActivityUnavailable
		}
		out = WalletActivitySnapshot{Version: 1, State: "idle", Binding: &b, Activity: "none"}
		if c.activityLast != nil {
			out.LastJobID = c.activityLast.JobID
		}
		if c.activityCache != nil {
			out.CaptureJobID = c.activityCache.job.JobID
		}
		if op := c.activityOp; op != nil {
			out.State, out.Activity, out.ActiveJobID = "busy", op.kind, op.job.JobID
		}
		return nil
	})
	if e != nil {
		return BlankWalletActivitySnapshot(), e
	}
	return out, nil
}
func (c *PoolChild) activityLookupLocked(id string) (WalletActivityJob, bool) {
	if op := c.activityOp; op != nil && op.kind == "capture" && op.job.JobID == id {
		return cloneWalletActivityJob(op.job), true
	}
	if j := c.activityLast; j != nil && j.JobID == id {
		return cloneWalletActivityJob(*j), true
	}
	if x := c.activityCache; x != nil && x.job.JobID == id {
		return cloneWalletActivityJob(x.job), true
	}
	return WalletActivityJob{}, false
}
func (c *PoolChild) activityLookup(ctx context.Context, v, id string, cancel bool) (WalletActivityJob, error) {
	if ctx == nil || !payoutNonzeroID(id) {
		return WalletActivityJob{}, gymstable.ErrInvalid
	}
	var out WalletActivityJob
	e := c.payoutFence(ctx, v, func() error {
		var ok bool
		out, ok = c.activityLookupLocked(id)
		if !ok {
			return ErrActivityNotFound
		}
		if op := c.activityOp; cancel && op != nil && op.kind == "capture" && op.job.JobID == id {
			op.cancelled = true
			op.cancel()
		}
		return nil
	})
	if e != nil {
		return WalletActivityJob{}, e
	}
	return out, nil
}
func (c *PoolChild) WalletActivityJob(ctx context.Context, v, id string) (WalletActivityJob, error) {
	return c.activityLookup(ctx, v, id, false)
}
func (c *PoolChild) CancelWalletActivity(ctx context.Context, v, id string) (WalletActivityJob, error) {
	return c.activityLookup(ctx, v, id, true)
}
func (c *PoolChild) BeginWalletActivity(ctx context.Context, v, id string) (WalletActivityJob, error) {
	if wire.TestPoW {
		return WalletActivityJob{}, gymstable.ErrProfile
	}
	if ctx == nil || !payoutNonzeroID(id) {
		return WalletActivityJob{}, gymstable.ErrInvalid
	}
	if !c.stopMu.TryLock() {
		return WalletActivityJob{}, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	task, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	op := &walletActivityOperation{kind: "capture", ctx: task, cancel: cancel, done: make(chan struct{})}
	var out WalletActivityJob
	var previous *WalletActivityJob
	admitted := false
	e := c.payoutFence(ctx, v, func() error {
		if j, ok := c.activityLookupLocked(id); ok {
			out = j
			return nil
		}
		if c.backup != nil || c.activityOp != nil {
			return gymstable.ErrBusy
		}
		if c.activityScratch != 0 {
			return gymstable.ErrCapacity
		}
		b := c.activityBindingLocked()
		if ValidateWalletActivityBinding(b) != nil {
			return ErrActivityUnavailable
		}
		op.job = WalletActivityJob{Version: 1, JobID: id, State: "running", Binding: b}
		previous = c.activityLast
		c.activityLast = nil
		c.activityOp, c.activityScratch = op, WalletActivityScratchBytes
		out = cloneWalletActivityJob(op.job)
		admitted = true
		return nil
	})
	if !admitted {
		cancel()
		if e != nil {
			return WalletActivityJob{}, e
		}
		return out, nil
	}
	if e != nil {
		// Local reservation is reversible. A failed outer original-view fence must
		// never launch the wallet read, even if its callback was reached.
		cancel()
		c.mu.Lock()
		if c.activityOp == op {
			c.activityOp = nil
			c.activityScratch = 0
			c.activityLast = previous
		}
		close(op.done)
		c.mu.Unlock()
		return WalletActivityJob{}, e
	}
	go c.runWalletActivity(op)
	return out, nil
}
func (c *PoolChild) activityOperationLocked(op *walletActivityOperation) error {
	if c.activityOp != op || c.backup != nil || c.activityBindingLocked() != op.job.Binding {
		return gymstable.ErrStaleView
	}
	return op.ctx.Err()
}
func (c *PoolChild) runWalletActivity(op *walletActivityOperation) {
	defer c.finishWalletActivity(op)
	read := c.activityRead
	if read == nil {
		read = func(ctx context.Context) (walletActivityRead, error) {
			return c.wallet.captureWalletActivity(ctx, c.resources.RPC)
		}
	}
	source, e := read(op.ctx)
	var cache *walletActivityCache
	if e == nil {
		cache, e = buildWalletActivityCache(op.ctx, op.job, source)
	}
	source = walletActivityRead{}
	if e == nil {
		var previous *walletActivityCache
		var last *WalletActivityJob
		e = c.payoutFence(op.ctx, op.job.Binding.View.ViewID, func() error {
			if x := c.activityOperationLocked(op); x != nil {
				return x
			}
			previous, last = c.activityCache, c.activityLast
			c.activityCache = cache
			j := cloneWalletActivityJob(cache.job)
			c.activityLast = &j
			return nil
		})
		if e != nil {
			c.mu.Lock()
			if c.activityCache == cache {
				c.activityCache, c.activityLast = previous, last
			}
			c.mu.Unlock()
		}
	}
	if e != nil {
		j := op.job
		j.State, j.Error = "failed", walletActivityTaskError(e)
		c.mu.Lock()
		cancelled := op.cancelled
		c.mu.Unlock()
		if cancelled {
			j.State, j.Error = "cancelled", ""
		}
		publish, done := context.WithTimeout(c.ctx, 2*time.Second)
		_ = c.payoutFence(publish, j.Binding.View.ViewID, func() error {
			if c.activityOp != op {
				return gymstable.ErrStaleView
			}
			copy := cloneWalletActivityJob(j)
			c.activityLast = &copy
			return nil
		})
		done()
	}
}
func (c *PoolChild) finishWalletActivity(op *walletActivityOperation) {
	op.cancel()
	op.cache = nil
	c.mu.Lock()
	if c.activityOp == op {
		c.activityOp = nil
		c.activityScratch = 0
	}
	close(op.done)
	c.mu.Unlock()
}

// stopMu is held by Stop/backup/Close; never hold an account lease while joining.
func (c *PoolChild) cancelWalletActivity() {
	c.mu.Lock()
	op, projection := c.activityOp, c.activityProjection
	if op != nil {
		op.cancelled = true
		op.cancel()
	}
	if projection != nil {
		projection.cancel()
	}
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
	if projection != nil {
		<-projection.done
	}
}
func (c *PoolChild) finishActivityProjection(p *walletActivityProjection) {
	p.cancel()
	c.mu.Lock()
	if c.activityProjection == p {
		c.activityProjection = nil
	}
	close(p.done)
	c.mu.Unlock()
}
func (c *PoolChild) activityProjectionContext(parent context.Context) *walletActivityProjection {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	return &walletActivityProjection{ctx: ctx, cancel: func() { stop(); cancel() }, done: make(chan struct{})}
}
func (c *PoolChild) WalletActivityPage(parent context.Context, v, id string, after, limit uint32) (WalletActivityPage, error) {
	if parent == nil || !payoutNonzeroID(id) || after > 256 || limit == 0 || limit > 8 {
		return WalletActivityPage{}, gymstable.ErrInvalid
	}
	if !c.stopMu.TryLock() {
		return WalletActivityPage{}, gymstable.ErrBusy
	}
	p := c.activityProjectionContext(parent)
	admitted := false
	var out WalletActivityPage
	e := c.payoutFence(p.ctx, v, func() error {
		if c.backup != nil || c.activityProjection != nil {
			return gymstable.ErrBusy
		}
		x := c.activityCache
		if x == nil || x.job.JobID != id {
			return ErrActivityNotFound
		}
		if after > uint32(len(x.items)) {
			return gymstable.ErrInvalid
		}
		next := after + min(limit, uint32(len(x.items))-after)
		out = WalletActivityPage{Version: 1, Binding: x.job.Binding, JobID: id, Capture: *x.job.Capture, After: after, Limit: limit, Next: next, HasMore: next < uint32(len(x.items)), Items: make([]WalletActivityItem, 0, next-after)}
		for _, item := range x.items[after:next] {
			out.Items = append(out.Items, CloneWalletActivityItem(item))
		}
		c.activityProjection = p
		admitted = true
		return nil
	})
	c.stopMu.Unlock()
	if !admitted {
		p.cancel()
		return WalletActivityPage{}, e
	}
	defer c.finishActivityProjection(p)
	if e != nil {
		return WalletActivityPage{}, e
	}
	if ValidateWalletActivityPage(out) != nil {
		return WalletActivityPage{}, ErrActivityUnavailable
	}
	e = c.payoutFence(p.ctx, v, func() error {
		if c.activityProjection != p || c.backup != nil {
			return gymstable.ErrStaleView
		}
		if c.activityCache == nil || c.activityCache.job.JobID != id {
			return ErrActivityNotFound
		}
		return p.ctx.Err()
	})
	if e != nil {
		return WalletActivityPage{}, e
	}
	return out, nil
}
func (c *PoolChild) WalletActivityDetail(parent context.Context, v, id, txid string) (WalletActivityDetail, error) {
	if wire.TestPoW {
		return WalletActivityDetail{}, gymstable.ErrProfile
	}
	if parent == nil || !payoutNonzeroID(id) || !canonicalManifestID(txid) {
		return WalletActivityDetail{}, gymstable.ErrInvalid
	}
	if !c.stopMu.TryLock() {
		return WalletActivityDetail{}, gymstable.ErrBusy
	}
	p := c.activityProjectionContext(parent)
	op := &walletActivityOperation{kind: "detail", ctx: p.ctx, cancel: p.cancel, done: make(chan struct{})}
	var out WalletActivityDetail
	var selected wallet.OwnActivityRecord
	admitted := false
	e := c.payoutFence(p.ctx, v, func() error {
		if c.backup != nil || c.activityOp != nil || c.activityProjection != nil {
			return gymstable.ErrBusy
		}
		if c.activityScratch != 0 {
			return gymstable.ErrCapacity
		}
		x := c.activityCache
		if x == nil || x.job.JobID != id {
			return ErrActivityNotFound
		}
		index := -1
		for i, item := range x.items {
			if item.TxID == txid {
				index = i
				break
			}
		}
		if index < 0 {
			return ErrActivityNotFound
		}
		// This bounded selected record is safe while the exclusive main operation
		// prevents replacement. No whole-cache clone escapes into a detail reader.
		selected = x.records[index]
		op.job, op.cache = cloneWalletActivityJob(x.job), x
		out = WalletActivityDetail{Version: 1, Binding: x.job.Binding, JobID: id, Capture: *x.job.Capture, Item: CloneWalletActivityItem(x.items[index])}
		c.activityOp, c.activityScratch, c.activityProjection = op, WalletActivityScratchBytes, p
		admitted = true
		return nil
	})
	c.stopMu.Unlock()
	if !admitted {
		p.cancel()
		return WalletActivityDetail{}, e
	}
	defer c.finishActivityProjection(p)
	defer c.finishWalletActivity(op)
	if e != nil {
		return WalletActivityDetail{}, e
	}
	var item WalletActivityItem
	project := c.activityProject
	if project == nil {
		project = projectWalletActivity
	}
	item, out.Outputs, e = project(p.ctx, selected, out.Capture, true)
	selected = wallet.OwnActivityRecord{}
	if e != nil {
		return WalletActivityDetail{}, e
	}
	if !WalletActivityItemEqual(item, out.Item) || ValidateWalletActivityDetail(out) != nil {
		return WalletActivityDetail{}, ErrActivityUnavailable
	}
	e = c.payoutFence(p.ctx, v, func() error {
		if x := c.activityOperationLocked(op); x != nil {
			return x
		}
		if c.activityProjection != p || c.activityCache != op.cache {
			return ErrActivityNotFound
		}
		return nil
	})
	if e != nil {
		return WalletActivityDetail{}, e
	}
	return out, nil
}

var _ WalletActivitySurfaceAPI = (*PoolChild)(nil)
