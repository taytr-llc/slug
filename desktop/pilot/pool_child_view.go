// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
)

func (c *PoolChild) readyStable() (*StableController, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.prepared {
		return nil, gymstable.ErrClosed
	}
	return c.stable, nil
}
func (c *PoolChild) publish(ctx context.Context, fn func() error) error {
	s, err := c.readyStable()
	if err != nil {
		return err
	}
	return s.withView(ctx, func(check func() error) error {
		if err := check(); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return gymstable.ErrClosed
		}
		return fn()
	})
}
func (c *PoolChild) refreshLoop() {
	defer close(c.refreshDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	next := time.Time{}
	for {
		if c.ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		run := c.run
		var worker poolChildWorker
		if run != nil {
			worker = run.worker
		}
		c.mu.Unlock()
		if worker != nil {
			status := worker.Status()
			_ = c.publish(c.ctx, func() error {
				if c.run == run {
					c.copyWorkerLocked(run, status)
				}
				return nil
			})
		}
		if !time.Now().Before(next) {
			c.refreshMu.Lock()
			c.mu.Lock()
			paused := c.receiveOp != nil || c.backup != nil || c.closed
			c.mu.Unlock()
			if !paused {
				c.refreshWalletAndStable()
				_ = c.refreshHostedAllocation(c.ctx)
				_ = c.refreshPoolBalance(c.ctx)
			}
			c.refreshMu.Unlock()
			next = time.Now().Add(10 * time.Second)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (c *PoolChild) refreshWalletAndStable() {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	wallet, walletErr := c.wallet.Snapshot(ctx)
	node, nodeErr := c.resources.Node.Resolve(ctx, c.resources.Source.Policy)
	s := c.stable
	select {
	case s.laneGate <- struct{}{}:
	case <-ctx.Done():
		return
	}
	status, statusErr := s.stableStore.SharedStatus(ctx, s.session())
	<-s.laneGate
	_ = c.publish(ctx, func() error {
		if walletErr == nil && wallet.Height >= 0 && wallet.Spendable >= 0 && wallet.Immature >= 0 && uint64(wallet.Spendable) <= manifestMaxSafe && uint64(wallet.Immature) <= manifestMaxSafe {
			c.snapshot.WalletHeight = uint64(wallet.Height)
			c.snapshot.SpendableAtoms = wallet.Spendable
			c.snapshot.ImmatureAtoms = wallet.Immature
			c.snapshot.WalletState = "syncing"
			if wallet.Synced {
				c.snapshot.WalletState = "ready"
			}
		} else {
			c.snapshot.WalletState = "failed"
		}
		if nodeErr == nil {
			c.snapshot.Height = uint64(node.ParentHeight)
		}
		if statusErr == nil {
			c.snapshot.AssignmentCount = status.AssignmentCount
		}
		return nil
	})
}

// RefreshPoolBalance performs network/proof work outside the view lease. The
// public Snapshot method only copies the resulting timestamped cache.
func (c *PoolChild) RefreshPoolBalance(parent context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.mu.Lock()
	paused := c.backup != nil
	c.mu.Unlock()
	if paused {
		return gymstable.ErrBusy
	}
	return c.refreshPoolBalance(parent)
}

func (c *PoolChild) refreshPoolBalance(parent context.Context) error {
	if parent == nil {
		return gymstable.ErrInvalid
	}
	if _, err := c.readyStable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	result, err := c.authority.Balance(ctx)
	var serviceErr *poolservice.ClientError
	if errors.As(err, &serviceErr) && serviceErr.Code == "busy" {
		return err
	}
	if err == nil && (result.BindingID != c.resources.Recipient.SourceBindingID || result.Balance.GymID != c.resources.Recipient.Config.Identity.GymID) {
		err = gymstable.ErrCorrupt
	}
	publicationContext := ctx
	if err != nil {
		// A timed-out request cannot publish through its cancelled context.
		// This failure contains no balance payload, but still belongs only to
		// the immutable original login/view and this live child. Successful
		// responses retain the original-token and request-context boundary.
		var publicationCancel context.CancelFunc
		publicationContext, publicationCancel = context.WithTimeout(c.ctx, 2*time.Second)
		defer publicationCancel()
	}
	publication := c.publish(publicationContext, func() error {
		if err != nil {
			c.snapshot.Balance = PoolBalanceSnapshot{State: "failed", Error: "balance_unavailable"}
		} else {
			copy := result
			c.snapshot.Balance = PoolBalanceSnapshot{State: "ready", Value: &copy}
		}
		return nil
	})
	if publication != nil {
		return publication
	}
	return err
}

// Forward only the typed stable/cache surface. In particular this type does
// not inherit isolated-node lifecycle or wallet recovery methods.
func (c *PoolChild) StableSnapshot() StableSnapshot {
	s, e := c.readyStable()
	if e != nil {
		return StableSnapshot{Version: 1, State: "unavailable"}
	}
	return s.StableSnapshot()
}
func (c *PoolChild) BeginStableSync(ctx context.Context, v string) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStableSync(ctx, v) })
}
func (c *PoolChild) BeginStableRecent(ctx context.Context, v string) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStableRecent(ctx, v) })
}
func (c *PoolChild) BeginStableInventory(ctx context.Context, v string, a uint64, l uint32) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStableInventory(ctx, v, a, l) })
}
func (c *PoolChild) BeginStableRoster(ctx context.Context, v string, f, l uint32) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStableRoster(ctx, v, f, l) })
}
func (c *PoolChild) BeginStablePrepare(ctx context.Context, v string, r gymstable.SlotRequest) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStablePrepare(ctx, v, r) })
}
func (c *PoolChild) BeginStableApply(ctx context.Context, v, id string) (StableJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableJob, error) { return s.BeginStableApply(ctx, v, id) })
}
func (c *PoolChild) StableJob(ctx context.Context, v, id string) (StableJob, error) {
	s, e := c.readyStable()
	if e != nil {
		return StableJob{}, e
	}
	return s.StableJob(ctx, v, id)
}
func (c *PoolChild) StableOperation(ctx context.Context, v, id string) (StableOperation, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableOperation, error) { return s.StableOperation(ctx, v, id) })
}
func (c *PoolChild) StablePending(ctx context.Context, v string) (StableOperation, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableOperation, error) { return s.StablePending(ctx, v) })
}
func (c *PoolChild) AbandonStableOperation(ctx context.Context, v, id string) (StableOperation, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (StableOperation, error) { return s.AbandonStableOperation(ctx, v, id) })
}
func (c *PoolChild) CancelStableJob(ctx context.Context, v, id string) (StableJob, error) {
	s, e := c.readyStable()
	if e != nil {
		return StableJob{}, e
	}
	return s.CancelStableJob(ctx, v, id)
}
func (c *PoolChild) ManifestPortrait(ctx context.Context, seq uint64) (ManifestPortrait, error) {
	s, e := c.readyStable()
	if e != nil {
		return ManifestPortrait{}, e
	}
	return s.ManifestPortrait(ctx, seq)
}
func (c *PoolChild) ManifestReceipt(ctx context.Context, seq uint64) (ManifestReceipt, error) {
	s, e := c.readyStable()
	if e != nil {
		return ManifestReceipt{}, e
	}
	return s.ManifestReceipt(ctx, seq)
}
func (c *PoolChild) ExportManifestReceipt(ctx context.Context, seq uint64, view, id, path string) error {
	s, e := c.readyStable()
	if e != nil {
		return e
	}
	return s.ExportManifestReceipt(ctx, seq, view, id, path)
}
func (c *PoolChild) ReleaseManifestReceipt(ctx context.Context, view, id string) error {
	s, e := c.readyStable()
	if e != nil {
		return e
	}
	return s.ReleaseManifestReceipt(ctx, view, id)
}
