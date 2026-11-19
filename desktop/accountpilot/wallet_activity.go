// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *PoolController) walletActivityChild(v string) (pilot.WalletActivitySurfaceAPI, error) {
	child, e := c.stableChild(v)
	if e != nil {
		return nil, e
	}
	s, ok := child.(pilot.WalletActivitySurfaceAPI)
	if !ok {
		return nil, pilot.ErrActivityUnavailable
	}
	return s, nil
}
func (c *PoolController) WalletActivitySnapshot(ctx context.Context) (pilot.WalletActivitySnapshot, error) {
	if ctx == nil {
		return pilot.WalletActivitySnapshot{}, gymstable.ErrInvalid
	}
	c.mu.Lock()
	v, signed := c.viewID, c.state == "signed_in" && !c.closed && !c.operation
	c.mu.Unlock()
	if !signed {
		return pilot.BlankWalletActivitySnapshot(), nil
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivitySnapshot{}, e
	}
	return s.WalletActivitySnapshot(ctx)
}
func (c *PoolController) BeginWalletActivity(ctx context.Context, v, id string) (pilot.WalletActivityJob, error) {
	if ctx == nil {
		return pilot.WalletActivityJob{}, gymstable.ErrInvalid
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivityJob{}, e
	}
	return s.BeginWalletActivity(ctx, v, id)
}
func (c *PoolController) WalletActivityJob(ctx context.Context, v, id string) (pilot.WalletActivityJob, error) {
	if ctx == nil {
		return pilot.WalletActivityJob{}, gymstable.ErrInvalid
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivityJob{}, e
	}
	return s.WalletActivityJob(ctx, v, id)
}
func (c *PoolController) CancelWalletActivity(ctx context.Context, v, id string) (pilot.WalletActivityJob, error) {
	if ctx == nil {
		return pilot.WalletActivityJob{}, gymstable.ErrInvalid
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivityJob{}, e
	}
	return s.CancelWalletActivity(ctx, v, id)
}
func (c *PoolController) WalletActivityPage(ctx context.Context, v, id string, a, l uint32) (pilot.WalletActivityPage, error) {
	if ctx == nil {
		return pilot.WalletActivityPage{}, gymstable.ErrInvalid
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivityPage{}, e
	}
	return s.WalletActivityPage(ctx, v, id, a, l)
}
func (c *PoolController) WalletActivityDetail(ctx context.Context, v, id, tx string) (pilot.WalletActivityDetail, error) {
	if ctx == nil {
		return pilot.WalletActivityDetail{}, gymstable.ErrInvalid
	}
	s, e := c.walletActivityChild(v)
	if e != nil {
		return pilot.WalletActivityDetail{}, e
	}
	return s.WalletActivityDetail(ctx, v, id, tx)
}

var _ pilot.WalletActivitySurfaceAPI = (*PoolController)(nil)
