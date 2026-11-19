// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"time"
)

type resultAccountFactories struct {
	bout         boutAccountFactories
	openResult   func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error)
	resumeResult func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error)
}
type resultAccountOwner struct {
	store *gymresult.Store
	deps  resultAccountFactories
}

func NewResults(ctx context.Context, c pilot.Config) (*StableController, error) {
	d := defaultEventFactories()
	d.child = pilot.NewResultManifestAccount
	return newResults(ctx, c, resultAccountFactories{boutAccountFactories{cardAccountFactories{seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared}, gymcard.Open, gymcard.ResumePrepared}, gymbout.Open, gymbout.ResumePrepared}, gymresult.Open, gymresult.ResumePrepared})
}
func newResults(parent context.Context, config pilot.Config, deps resultAccountFactories) (*StableController, error) {
	return newResultsMode(parent, config, deps, nil)
}
func newResultsMode(parent context.Context, config pilot.Config, deps resultAccountFactories, career *careerAccountOwner) (*StableController, error) {
	if parent == nil {
		return nil, gymresult.ErrInvalid
	}
	if wire.TestPoW {
		return nil, gymresult.ErrProfile
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	// Legacy context-free calls are joined; the budget stops subsequent admission.
	d := &deps.bout.card.seed.event
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
	store, lock, _, e := openResultAccountStoreMode(ctx, config, deps, career)
	if e != nil {
		return nil, e
	}
	d.createAccount, d.openAccount, d.openStable = oldCreate, oldOpen, oldStable
	base := &ManifestController{careerOwner: career, config: config, store: store, lock: lock, state: "signed_out", eventOwner: &eventAccountOwner{deps: deps.bout.card.seed.event}, seedOwner: &seedAccountOwner{deps: deps.bout.card.seed}, cardOwner: &cardAccountOwner{deps: deps.bout.card}, boutOwner: &boutAccountOwner{deps: deps.bout}, resultOwner: &resultAccountOwner{deps: deps}, now: deps.bout.card.seed.event.now, verify: deps.bout.card.seed.event.verify}
	s := &StableController{base}
	base.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		base.mu.Lock()
		session, view := base.session, base.viewID
		base.mu.Unlock()
		var child *pilot.StableController
		child, e := deps.bout.card.seed.event.child(cfg, pilot.StableAccountConfig{Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			return base.withCapability(ctx, session, view, child, cap, fn)
		}, HistoryPublicationLease: func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			return base.withHistoryPublicationLease(ctx, session, view, child, fn)
		}, ResultNodeLease: func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			return base.withResultNodeLease(ctx, session, view, child, fn)
		}})
		if e != nil {
			return nil, e
		}
		if career != nil {
			if e := child.ConfigureCareer(career.reader, career.deps.openStable); e != nil {
				_ = child.Close()
				return nil, e
			}
		}
		return child, nil
	}
	base.attachChild = s.installResultComponents
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
		e = s.ensureResults(ctx)
	}
	if e == nil && career != nil {
		e = s.ensureCareers(ctx, nil)
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

// This purpose-fixed admission lease does not broaden ordinary View semantics.
// It fences only the node-owner reservation/transfer; no RPC or drain runs here.
func (c *ManifestController) withResultNodeLease(ctx context.Context, session gymaccount.Session, view string, child manifestChild, fn func(gymaccount.Account, func() error) error) error {
	return c.withCapability(ctx, session, view, child, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		guard := func() error {
			if e := check(); e != nil {
				return e
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.state != "signed_in" || c.operation {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := guard(); e != nil {
			return e
		}
		if e := fn(a, guard); e != nil {
			return e
		}
		return guard()
	})
}
func (c *StableController) ResultSnapshot() pilot.ResultSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultSnapshot{Version: 1, State: "unavailable", Error: "result_unavailable", NodeState: "offline", NodeOwner: "none"}
	}
	return s.ResultSnapshot()
}
func (c *StableController) ResultStatus(ctx context.Context, v, date, bid string) (gymresult.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymresult.Status{}, e
	}
	return s.ResultStatus(ctx, v, date, bid)
}
func (c *StableController) ResultRecord(ctx context.Context, v, date, bid string) (gymresult.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymresult.ReadResult{}, e
	}
	return s.ResultRecord(ctx, v, date, bid)
}
func (c *StableController) BeginResultPublish(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultPublish(ctx, v, date, bid)
}
func (c *StableController) BeginResultReconcile(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultReconcile(ctx, v, date, bid)
}
func (c *StableController) BeginResultAttest(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultAttest(ctx, v, date, bid)
}
func (c *StableController) ResultJob(ctx context.Context, v, id string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.ResultJob(ctx, v, id)
}
func (c *StableController) CancelResultJob(ctx context.Context, v, id string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.CancelResultJob(ctx, v, id)
}
func (c *StableController) BeginResultExport(ctx context.Context, v, date, bid, path string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultExport(ctx, v, date, bid, path)
}
