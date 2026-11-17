// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
)

type StableController struct{ *ManifestController }

func NewStable(config pilot.Config) (*StableController, error) {
	if wire.TestPoW {
		return nil, errors.New("candidate stable requires production PoW")
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, _, err := openStableAccountStore(config)
	if err != nil {
		return nil, err
	}
	base := &ManifestController{config: config, store: store, lock: lock, state: "signed_out"}
	s := &StableController{base}
	base.newChild = func(config pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, err := pilot.NewStableManifestAccount(config, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		return child, err
	}
	base.attachChild = s.install
	return s, nil
}
func (c *StableController) stableChild(view string) (*pilot.StableController, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.viewID != view {
		return nil, gymstable.ErrStaleView
	}
	child, ok := c.child.(*pilot.StableController)
	if !ok {
		return nil, gymstable.ErrUnauthorized
	}
	return child, nil
}
func (c *StableController) StableSnapshot() pilot.StableSnapshot {
	c.mu.Lock()
	view := c.viewID
	c.mu.Unlock()
	child, err := c.stableChild(view)
	if err != nil {
		return pilot.StableSnapshot{Version: 1, State: "unavailable"}
	}
	return child.StableSnapshot()
}
func (c *StableController) BeginStableSync(ctx context.Context, v string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableSync(ctx, v)
}
func (c *StableController) BeginStableInventory(ctx context.Context, v string, a uint64, l uint32) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableInventory(ctx, v, a, l)
}
func (c *StableController) BeginStableRoster(ctx context.Context, v string, f, l uint32) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableRoster(ctx, v, f, l)
}
func (c *StableController) BeginStablePrepare(ctx context.Context, v string, r gymstable.SlotRequest) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStablePrepare(ctx, v, r)
}
func (c *StableController) BeginStableApply(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableApply(ctx, v, id)
}
func (c *StableController) StableJob(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.StableJob(ctx, v, id)
}
func (c *StableController) StableOperation(ctx context.Context, v, id string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.StableOperation(ctx, v, id)
}
func (c *StableController) StablePending(ctx context.Context, v string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.StablePending(ctx, v)
}
func (c *StableController) AbandonStableOperation(ctx context.Context, v, id string) (pilot.StableOperation, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableOperation{}, e
	}
	return s.AbandonStableOperation(ctx, v, id)
}
func (c *StableController) CancelStableJob(ctx context.Context, v, id string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.CancelStableJob(ctx, v, id)
}

func (c *StableController) BeginStableRecent(ctx context.Context, v string) (pilot.StableJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.StableJob{}, e
	}
	return s.BeginStableRecent(ctx, v)
}
