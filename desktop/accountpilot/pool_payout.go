// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *PoolController) payoutChild(v string) (pilot.PoolPayoutSurfaceAPI, error) {
	child, e := c.stableChild(v)
	if e != nil {
		return nil, e
	}
	s, ok := child.(pilot.PoolPayoutSurfaceAPI)
	if !ok {
		return nil, pilot.ErrPayoutUnavailable
	}
	return s, nil
}
func (c *PoolController) PoolPayoutSnapshot(ctx context.Context) (pilot.PoolPayoutSnapshot, error) {
	if ctx == nil {
		return pilot.PoolPayoutSnapshot{}, gymstable.ErrInvalid
	}
	c.mu.Lock()
	v, signed := c.viewID, c.state == "signed_in" && !c.closed && !c.operation
	c.mu.Unlock()
	if !signed {
		return pilot.BlankPoolPayoutSnapshot(), nil
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutSnapshot{}, e
	}
	return s.PoolPayoutSnapshot(ctx)
}
func (c *PoolController) BeginPoolPayoutHistory(ctx context.Context, v, id string) (pilot.PoolPayoutJob, error) {
	if ctx == nil {
		return pilot.PoolPayoutJob{}, gymstable.ErrInvalid
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutJob{}, e
	}
	return s.BeginPoolPayoutHistory(ctx, v, id)
}
func (c *PoolController) PoolPayoutJob(ctx context.Context, v, id string) (pilot.PoolPayoutJob, error) {
	if ctx == nil {
		return pilot.PoolPayoutJob{}, gymstable.ErrInvalid
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutJob{}, e
	}
	return s.PoolPayoutJob(ctx, v, id)
}
func (c *PoolController) CancelPoolPayoutHistory(ctx context.Context, v, id string) (pilot.PoolPayoutJob, error) {
	if ctx == nil {
		return pilot.PoolPayoutJob{}, gymstable.ErrInvalid
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutJob{}, e
	}
	return s.CancelPoolPayoutHistory(ctx, v, id)
}
func (c *PoolController) PoolPayoutPage(ctx context.Context, v, id string, a, l uint32) (pilot.PoolPayoutPage, error) {
	if ctx == nil {
		return pilot.PoolPayoutPage{}, gymstable.ErrInvalid
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutPage{}, e
	}
	return s.PoolPayoutPage(ctx, v, id, a, l)
}
func (c *PoolController) ReadPoolPayoutTransaction(ctx context.Context, v, id, tx string, n uint32) (pilot.PoolPayoutObservation, error) {
	if ctx == nil {
		return pilot.PoolPayoutObservation{}, gymstable.ErrInvalid
	}
	s, e := c.payoutChild(v)
	if e != nil {
		return pilot.PoolPayoutObservation{}, e
	}
	return s.ReadPoolPayoutTransaction(ctx, v, id, tx, n)
}

var _ pilot.PoolPayoutSurfaceAPI = (*PoolController)(nil)
