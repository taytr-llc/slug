// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"time"
)

type boutAccountFactories struct {
	card       cardAccountFactories
	openBout   func(context.Context, string, gymbout.Pin, *gymcard.Store) (*gymbout.Store, error)
	resumeBout func(context.Context, string, gymbout.Pin, *gymcard.Store) (*gymbout.Store, error)
}
type boutAccountOwner struct {
	store *gymbout.Store
	deps  boutAccountFactories
}

func NewBouts(ctx context.Context, c pilot.Config) (*StableController, error) {
	d := defaultEventFactories()
	d.child = pilot.NewBoutManifestAccount
	return newBouts(ctx, c, boutAccountFactories{cardAccountFactories{seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared}, gymcard.Open, gymcard.ResumePrepared}, gymbout.Open, gymbout.ResumePrepared})
}
func newBouts(parent context.Context, config pilot.Config, deps boutAccountFactories) (*StableController, error) {
	if parent == nil {
		return nil, gymbout.ErrInvalid
	}
	if wire.TestPoW {
		return nil, gymbout.ErrProfile
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	// Legacy context-free calls are joined; the budget stops subsequent admission.
	d := &deps.card.seed.event
	oldCreate, oldOpen, oldStable := d.createAccount, d.openAccount, d.openStable
	d.createAccount = func(path string, c gymaccount.Config) (*gymaccount.Store, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		s, e := oldCreate(path, c)
		if e == nil && ctx.Err() != nil {
			e = ctx.Err()
			if s != nil {
				_ = s.Close()
				s = nil
			}
		}
		return s, e
	}
	d.openAccount = func(path string, c gymaccount.Config) (*gymaccount.Store, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		s, e := oldOpen(path, c)
		if e == nil && ctx.Err() != nil {
			e = ctx.Err()
			if s != nil {
				_ = s.Close()
				s = nil
			}
		}
		return s, e
	}
	d.openStable = func(path string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		s, e := oldStable(path, pin, source)
		if e == nil && ctx.Err() != nil {
			e = ctx.Err()
			if s != nil {
				_ = s.Close()
				s = nil
			}
		}
		return s, e
	}
	store, lock, _, e := openBoutAccountStore(ctx, config, deps)
	if e != nil {
		return nil, e
	}
	d.createAccount, d.openAccount, d.openStable = oldCreate, oldOpen, oldStable
	base := &ManifestController{config: config, store: store, lock: lock, state: "signed_out", eventOwner: &eventAccountOwner{deps: deps.card.seed.event}, seedOwner: &seedAccountOwner{deps: deps.card.seed}, cardOwner: &cardAccountOwner{deps: deps.card}, boutOwner: &boutAccountOwner{deps: deps}, now: deps.card.seed.event.now, verify: deps.card.seed.event.verify}
	s := &StableController{base}
	base.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, e := deps.card.seed.event.child(cfg, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		if e != nil {
			return nil, e
		}
		return child, nil
	}
	base.attachChild = s.installBoutComponents
	if e = s.ensureSeedEvents(ctx); e == nil {
		e = s.ensureSeeds(ctx)
	}
	if e == nil {
		e = s.ensureCards(ctx)
	}
	if e == nil {
		e = s.ensureBouts(ctx)
	}
	if e == nil {
		e = ctx.Err()
	}
	if e != nil {
		_ = s.Close()
		return nil, e
	}
	return s, nil
}
func (c *StableController) BoutSnapshot() pilot.BoutSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutSnapshot{Version: 1, State: "unavailable", Error: "bout_unavailable"}
	}
	return s.BoutSnapshot()
}
func (c *StableController) BoutStatus(ctx context.Context, v, date, bid string) (gymbout.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.Status{}, e
	}
	return s.BoutStatus(ctx, v, date, bid)
}
func (c *StableController) BoutRecord(ctx context.Context, v, date, bid string) (gymbout.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.ReadResult{}, e
	}
	return s.BoutRecord(ctx, v, date, bid)
}
func (c *StableController) BoutCardStatus(ctx context.Context, v, date string) (gymbout.CardStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.CardStatus{}, e
	}
	return s.BoutCardStatus(ctx, v, date)
}
func (c *StableController) BeginBoutRun(ctx context.Context, v, date, bid string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.BeginBoutRun(ctx, v, date, bid)
}
func (c *StableController) BoutJob(ctx context.Context, v, id string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.BoutJob(ctx, v, id)
}
func (c *StableController) CancelBoutJob(ctx context.Context, v, id string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.CancelBoutJob(ctx, v, id)
}
