// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"time"
)

type eventAccountFactories struct {
	createAccount func(string, gymaccount.Config) (*gymaccount.Store, error)
	openAccount   func(string, gymaccount.Config) (*gymaccount.Store, error)
	openStable    func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error)
	openEntry     func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error)
	createEvent   func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
	openEvent     func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
	resumeEvent   func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
	child         func(pilot.Config, pilot.StableAccountConfig) (*pilot.StableController, error)
	now           func() time.Time
	verify        func(context.Context, string, bool) (gymaccount.Principal, error)
}
type eventAccountOwner struct {
	store *gymevent.Store
	deps  eventAccountFactories
}

func defaultEventFactories() eventAccountFactories {
	return eventAccountFactories{gymaccount.Create, gymaccount.Open, gymstable.Open, gymentry.Open, gymevent.Create, gymevent.Open, gymevent.ResumePrepared, pilot.NewEventManifestAccount, time.Now, verifyFixture}
}
func NewEvents(config pilot.Config) (*StableController, error) {
	return newEvents(config, defaultEventFactories())
}
func newEvents(config pilot.Config, deps eventAccountFactories) (*StableController, error) {
	if wire.TestPoW {
		return nil, gymevent.ErrProfile
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, _, e := openEventAccountStore(config, deps)
	if e != nil {
		return nil, e
	}
	base := &ManifestController{config: config, store: store, lock: lock, state: "signed_out", eventOwner: &eventAccountOwner{deps: deps}, now: deps.now, verify: deps.verify}
	s := &StableController{base}
	base.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, e := deps.child(cfg, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		if e != nil {
			return nil, e
		}
		return child, nil
	}
	base.attachChild = s.installEventComponents
	if e = s.ensureEvents(context.Background()); e != nil {
		_ = s.Close()
		return nil, e
	}
	return s, nil
}
func (c *StableController) EventSnapshot() pilot.EventSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
	}
	return s.EventSnapshot()
}
func (c *StableController) EventStatus(ctx context.Context, v, date string) (gymevent.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.EventStatus{}, e
	}
	return s.EventStatus(ctx, v, date)
}
func (c *StableController) EventRecord(ctx context.Context, v, date string) (gymevent.SnapshotReceipt, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.SnapshotReceipt{}, e
	}
	return s.EventRecord(ctx, v, date)
}
func (c *StableController) EventHistory(ctx context.Context, v, after string, limit uint32) (gymevent.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.HistoryPage{}, e
	}
	return s.EventHistory(ctx, v, after, limit)
}
func (c *StableController) BeginEventCapture(ctx context.Context, v, date string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.BeginEventCapture(ctx, v, date)
}
func (c *StableController) EventJob(ctx context.Context, v, id string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.EventJob(ctx, v, id)
}
func (c *StableController) CancelEventJob(ctx context.Context, v, id string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.CancelEventJob(ctx, v, id)
}
