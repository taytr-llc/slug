// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *PoolController) stableChild(view string) (poolChild, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.viewID != view {
		return nil, gymstable.ErrStaleView
	}
	child, ok := c.child, c.child != nil
	if !ok {
		return nil, gymstable.ErrUnauthorized
	}
	return child, nil
}
func (c *PoolController) StableSnapshot() pilot.StableSnapshot {
	c.mu.Lock()
	view := c.viewID
	c.mu.Unlock()
	child, err := c.stableChild(view)
	if err != nil {
		return pilot.StableSnapshot{Version: 1, State: "unavailable"}
	}
	return child.StableSnapshot()
}
func (c *PoolController) BeginStableSync(ctx context.Context, v string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableSync(ctx, v)
}
func (c *PoolController) BeginStableInventory(ctx context.Context, v string, a uint64, l uint32) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableInventory(ctx, v, a, l)
}
func (c *PoolController) BeginStableRoster(ctx context.Context, v string, f, l uint32) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableRoster(ctx, v, f, l)
}
func (c *PoolController) BeginStablePrepare(ctx context.Context, v string, r gymstable.SlotRequest) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStablePrepare(ctx, v, r)
}
func (c *PoolController) BeginStableApply(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableApply(ctx, v, id)
}
func (c *PoolController) StableJob(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.StableJob(ctx, v, id)
}
func (c *PoolController) StableOperation(ctx context.Context, v, id string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.StableOperation(ctx, v, id)
}
func (c *PoolController) StablePending(ctx context.Context, v string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.StablePending(ctx, v)
}
func (c *PoolController) AbandonStableOperation(ctx context.Context, v, id string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.AbandonStableOperation(ctx, v, id)
}
func (c *PoolController) CancelStableJob(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.CancelStableJob(ctx, v, id)
}

func (c *PoolController) BeginStableRecent(ctx context.Context, v string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableRecent(ctx, v)
}
