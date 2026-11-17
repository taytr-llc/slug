// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
)

func NewEntries(config pilot.Config) (*StableController, error) {
	if wire.TestPoW {
		return nil, errors.New("candidate stable requires production PoW")
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, _, err := openEntryAccountStore(config)
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
		child, err := pilot.NewEntryManifestAccount(config, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}})
		return child, err
	}
	base.attachChild = s.installEntries
	return s, nil
}
func (c *StableController) EntrySnapshot() pilot.EntrySnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntrySnapshot{Version: 1, State: "unavailable"}
	}
	return s.EntrySnapshot()
}
func (c *StableController) EntryUpcoming(ctx context.Context, v string) (gymentry.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return s.EntryUpcoming(ctx, v)
}
func (c *StableController) EntryStatus(ctx context.Context, v, date string) (gymentry.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return s.EntryStatus(ctx, v, date)
}
func (c *StableController) EntryHistory(ctx context.Context, v, after string, limit uint32) (gymentry.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.HistoryPage{}, e
	}
	return s.EntryHistory(ctx, v, after, limit)
}
func (c *StableController) BeginEntryPrepare(ctx context.Context, v string, r gymentry.Request) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryPrepare(ctx, v, r)
}
func (c *StableController) BeginEntryApply(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryApply(ctx, v, id)
}
func (c *StableController) BeginEntryCheck(ctx context.Context, v, date string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryCheck(ctx, v, date)
}
func (c *StableController) EntryJob(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.EntryJob(ctx, v, id)
}
func (c *StableController) CancelEntryJob(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.CancelEntryJob(ctx, v, id)
}
func (c *StableController) EntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.EntryOperation(ctx, v, id)
}
func (c *StableController) EntryPending(ctx context.Context, v string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.EntryPending(ctx, v)
}
func (c *StableController) AbandonEntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.AbandonEntryOperation(ctx, v, id)
}
