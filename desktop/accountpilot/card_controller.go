// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
)

type cardAccountFactories struct {
	seed       seedAccountFactories
	openCard   func(context.Context, string, gymcard.Pin, *gymseed.Store) (*gymcard.Store, error)
	resumeCard func(context.Context, string, gymcard.Pin, *gymseed.Store) (*gymcard.Store, error)
}
type cardAccountOwner struct {
	store *gymcard.Store
	deps  cardAccountFactories
}

func NewCards(c pilot.Config) (*StableController, error) {
	d := defaultEventFactories()
	d.child = pilot.NewCardManifestAccount
	return newCards(c, cardAccountFactories{seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared}, gymcard.Open, gymcard.ResumePrepared})
}
func newCards(config pilot.Config, deps cardAccountFactories) (*StableController, error) {
	if wire.TestPoW {
		return nil, gymcard.ErrProfile
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, _, e := openCardAccountStore(config, deps)
	if e != nil {
		return nil, e
	}
	base := &ManifestController{config: config, store: store, lock: lock, state: "signed_out", eventOwner: &eventAccountOwner{deps: deps.seed.event}, seedOwner: &seedAccountOwner{deps: deps.seed}, cardOwner: &cardAccountOwner{deps: deps}, now: deps.seed.event.now, verify: deps.seed.event.verify}
	s := &StableController{base}
	base.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, e := deps.seed.event.child(cfg, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		if e != nil {
			return nil, e
		}
		return child, nil
	}
	base.attachChild = s.installCardComponents
	if e = s.ensureSeedEvents(context.Background()); e == nil {
		e = s.ensureSeeds(context.Background())
	}
	if e == nil {
		e = s.ensureCards(context.Background())
	}
	if e != nil {
		_ = s.Close()
		return nil, e
	}
	return s, nil
}
func (c *StableController) CardSnapshot() pilot.CardSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
	}
	return s.CardSnapshot()
}
func (c *StableController) CardStatus(ctx context.Context, v, date string) (gymcard.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.Status{}, e
	}
	return s.CardStatus(ctx, v, date)
}
func (c *StableController) CardRecord(ctx context.Context, v, date string) (gymcard.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.ReadResult{}, e
	}
	return s.CardRecord(ctx, v, date)
}
func (c *StableController) CardHistory(ctx context.Context, v, after string, limit uint32) (gymcard.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.HistoryPage{}, e
	}
	return s.CardHistory(ctx, v, after, limit)
}
func (c *StableController) BeginCardBuild(ctx context.Context, v, date string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.BeginCardBuild(ctx, v, date)
}
func (c *StableController) CardJob(ctx context.Context, v, id string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.CardJob(ctx, v, id)
}
func (c *StableController) CancelCardJob(ctx context.Context, v, id string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.CancelCardJob(ctx, v, id)
}
