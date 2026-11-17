// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
)

type seedAccountFactories struct {
	event      eventAccountFactories
	openSeed   func(context.Context, string, gymseed.Pin, *gymevent.Store) (*gymseed.Store, error)
	resumeSeed func(context.Context, string, gymseed.Pin, *gymevent.Store) (*gymseed.Store, error)
}
type seedAccountOwner struct {
	store *gymseed.Store
	deps  seedAccountFactories
}

func NewSeeds(c pilot.Config) (*StableController, error) {
	d := defaultEventFactories()
	d.child = pilot.NewSeedManifestAccount
	return newSeeds(c, seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared})
}
func newSeeds(config pilot.Config, deps seedAccountFactories) (*StableController, error) {
	if wire.TestPoW {
		return nil, gymseed.ErrProfile
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, _, e := openSeedAccountStore(config, deps.event)
	if e != nil {
		return nil, e
	}
	base := &ManifestController{config: config, store: store, lock: lock, state: "signed_out", eventOwner: &eventAccountOwner{deps: deps.event}, seedOwner: &seedAccountOwner{deps: deps}, now: deps.event.now, verify: deps.event.verify}
	s := &StableController{base}
	base.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, e := deps.event.child(cfg, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		if e != nil {
			return nil, e
		}
		return child, nil
	}
	base.attachChild = s.installSeedComponents
	if e = s.ensureSeedEvents(context.Background()); e == nil {
		e = s.ensureSeeds(context.Background())
	}
	if e != nil {
		_ = s.Close()
		return nil, e
	}
	return s, nil
}
func (c *StableController) SeedSnapshot() pilot.SeedSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
	}
	return s.SeedSnapshot()
}
func (c *StableController) SeedStatus(ctx context.Context, v, date string) (gymseed.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.Status{}, e
	}
	return s.SeedStatus(ctx, v, date)
}
func (c *StableController) SeedRecord(ctx context.Context, v, date string) (gymseed.Record, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.Record{}, e
	}
	return s.SeedRecord(ctx, v, date)
}
func (c *StableController) SeedHistory(ctx context.Context, v, after string, limit uint32) (gymseed.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.HistoryPage{}, e
	}
	return s.SeedHistory(ctx, v, after, limit)
}
func (c *StableController) BeginSeedPrepare(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedPrepare(ctx, v, date)
}
func (c *StableController) BeginSeedPublish(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedPublish(ctx, v, date)
}
func (c *StableController) BeginSeedReconcile(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedReconcile(ctx, v, date)
}
func (c *StableController) BeginSeedAttest(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedAttest(ctx, v, date)
}
func (c *StableController) SeedJob(ctx context.Context, v, id string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.SeedJob(ctx, v, id)
}
func (c *StableController) CancelSeedJob(ctx context.Context, v, id string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.CancelSeedJob(ctx, v, id)
}
