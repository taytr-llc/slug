//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"time"
)

func NewCareersForTestClock(ctx context.Context, c pilot.Config, clock func() time.Time) (*StableController, error) {
	if clock == nil {
		return nil, careerstate.ErrInvalid
	}
	f := careerAccountFactories{
		preflight: func(ctx context.Context, p string, pin gymresult.Pin, b *gymbout.Store) error {
			return gymresult.CheckCareerActivationForTestClock(ctx, p, pin, b, clock)
		},
		openResult: func(ctx context.Context, p string, pin gymresult.Pin, b *gymbout.Store) (*gymresult.Store, error) {
			return gymresult.OpenCareerForTestClock(ctx, p, pin, b, clock)
		},
		resumeResult: func(ctx context.Context, p string, pin gymresult.Pin, b *gymbout.Store) (*gymresult.Store, error) {
			return gymresult.ResumeCareerActivationForTestClock(ctx, p, pin, b, clock)
		},
		openStable: func(p string, pin gymstable.Pin, m *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
			return gymstable.OpenCareerForTestClock(p, pin, m, a, clock)
		},
		resumeStable: func(p string, pin gymstable.Pin, m *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
			return gymstable.ResumeCareerBindingForTestClock(p, pin, m, a, clock)
		},
		openCard: func(ctx context.Context, p string, pin gymcard.Pin, s *gymseed.Store, a careerstate.Activation) (*gymcard.Store, error) {
			return gymcard.OpenCareerForTestClock(ctx, p, pin, s, a, clock)
		},
		resumeCard: func(ctx context.Context, p string, pin gymcard.Pin, s *gymseed.Store, a careerstate.Activation) (*gymcard.Store, error) {
			return gymcard.ResumeCareerBindingForTestClock(ctx, p, pin, s, a, clock)
		},
	}
	return newCareers(ctx, c, resultFactoriesForTestClock(clock), f)
}
