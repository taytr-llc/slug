// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"path/filepath"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

type careerAccountFactories struct {
	preflight                func(context.Context, string, gymresult.Pin, *gymbout.Store) error
	openResult, resumeResult func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error)
	openStable, resumeStable func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error)
	openCard, resumeCard     func(context.Context, string, gymcard.Pin, *gymseed.Store, careerstate.Activation) (*gymcard.Store, error)
}
type careerAccountOwner struct {
	deps   careerAccountFactories
	reader *careerstate.Reader
}

func defaultCareerFactories() careerAccountFactories {
	return careerAccountFactories{gymresult.CheckCareerActivation, gymresult.OpenCareer, gymresult.ResumeCareerActivation, gymstable.OpenCareer, gymstable.ResumeCareerBinding, gymcard.OpenCareer, gymcard.ResumeCareerBinding}
}
func NewCareers(ctx context.Context, c pilot.Config) (*StableController, error) {
	d := defaultEventFactories()
	d.child = pilot.NewResultManifestAccount
	deps := resultAccountFactories{boutAccountFactories{cardAccountFactories{seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared}, gymcard.Open, gymcard.ResumePrepared}, gymbout.Open, gymbout.ResumePrepared}, gymresult.Open, gymresult.ResumePrepared}
	return newCareers(ctx, c, deps, defaultCareerFactories())
}
func newCareers(ctx context.Context, c pilot.Config, d resultAccountFactories, f careerAccountFactories) (*StableController, error) {
	o := &careerAccountOwner{deps: f}
	path := filepath.Join(c.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	oldStable, oldCard, oldResult := d.bout.card.seed.event.openStable, d.bout.card.openCard, d.openResult
	d.bout.card.seed.event.openStable = func(p string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
		if m, e := readCareerMarker(path); e == nil && m.CareerActivation != nil {
			if m.CareerPhase == "ready" {
				return f.openStable(p, pin, source, *m.CareerActivation)
			}
			return f.resumeStable(p, pin, source, *m.CareerActivation)
		}
		return oldStable(p, pin, source)
	}
	d.bout.card.openCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store) (*gymcard.Store, error) {
		if m, e := readCareerMarker(path); e == nil && m.CareerActivation != nil {
			if m.CareerPhase == "ready" {
				return f.openCard(ctx, p, pin, source, *m.CareerActivation)
			}
			return f.resumeCard(ctx, p, pin, source, *m.CareerActivation)
		}
		return oldCard(ctx, p, pin, source)
	}
	d.openResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		if m, e := readCareerMarker(path); e == nil {
			if m.CareerPhase == "ready" {
				return f.openResult(ctx, p, pin, source)
			}
			if m.CareerPhase == "prepared" {
				return f.resumeResult(ctx, p, pin, source)
			}
		}
		return oldResult(ctx, p, pin, source)
	}
	return newResultsMode(ctx, c, d, o)
}
func (c *StableController) CareerInventory(ctx context.Context, v, jobID string, after uint64, limit uint32) (gymstable.CareerPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymstable.CareerPage{}, e
	}
	return s.CareerInventory(ctx, v, jobID, after, limit)
}
func (c *StableController) CareerRoster(ctx context.Context, v, jobID string, first, limit uint32) (gymstable.CareerRosterPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymstable.CareerRosterPage{}, e
	}
	return s.CareerRoster(ctx, v, jobID, first, limit)
}

func (c *StableController) CareerView(v string) (pilot.ManifestViewPin, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ManifestViewPin{}, e
	}
	return s.CareerView(v)
}
