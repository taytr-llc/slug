//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"time"
)

func poolPipelineTestFactories(clock func() time.Time) poolPipelineFactories {
	f := defaultPoolPipelineFactories()
	f.createSeed = func(ctx context.Context, p string, pin gymseed.Pin, source *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.CreateForTestClock(ctx, p, pin, source, clock)
	}
	f.openSeed = func(ctx context.Context, p string, pin gymseed.Pin, source *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.OpenForTestClock(ctx, p, pin, source, clock)
	}
	f.resumeSeed = func(ctx context.Context, p string, pin gymseed.Pin, source *gymevent.Store) (*gymseed.Store, error) {
		return gymseed.ResumePreparedForTestClock(ctx, p, pin, source, clock)
	}
	f.createCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.CreateForTestClock(ctx, p, pin, source, clock)
	}
	f.openCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.OpenForTestClock(ctx, p, pin, source, clock)
	}
	f.resumeCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store) (*gymcard.Store, error) {
		return gymcard.ResumePreparedForTestClock(ctx, p, pin, source, clock)
	}
	f.createBout = func(ctx context.Context, p string, pin gymbout.Pin, source *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.CreateForTestClock(ctx, p, pin, source, clock)
	}
	f.openBout = func(ctx context.Context, p string, pin gymbout.Pin, source *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.OpenForTestClock(ctx, p, pin, source, clock)
	}
	f.resumeBout = func(ctx context.Context, p string, pin gymbout.Pin, source *gymcard.Store) (*gymbout.Store, error) {
		return gymbout.ResumePreparedForTestClock(ctx, p, pin, source, clock)
	}
	f.createResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.CreateForTestClock(ctx, p, pin, source, clock)
	}
	f.openResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.OpenForTestClock(ctx, p, pin, source, clock)
	}
	f.resumeResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.ResumePreparedForTestClock(ctx, p, pin, source, clock)
	}
	f.preflightCareer = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) error {
		return gymresult.CheckCareerActivationForTestClock(ctx, p, pin, source, clock)
	}
	f.openCareerResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.OpenCareerForTestClock(ctx, p, pin, source, clock)
	}
	f.resumeCareerResult = func(ctx context.Context, p string, pin gymresult.Pin, source *gymbout.Store) (*gymresult.Store, error) {
		return gymresult.ResumeCareerActivationForTestClock(ctx, p, pin, source, clock)
	}
	f.openSharedCareer = func(p string, pin gymstable.Pin, source *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
		return gymstable.OpenSharedCareerForTestClock(p, pin, source, a, clock)
	}
	f.resumeSharedCareer = func(p string, pin gymstable.Pin, source *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
		return gymstable.ResumeSharedCareerBindingForTestClock(p, pin, source, a, clock)
	}
	f.openCareerCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store, a careerstate.Activation) (*gymcard.Store, error) {
		return gymcard.OpenCareerForTestClock(ctx, p, pin, source, a, clock)
	}
	f.resumeCareerCard = func(ctx context.Context, p string, pin gymcard.Pin, source *gymseed.Store, a careerstate.Activation) (*gymcard.Store, error) {
		return gymcard.ResumeCareerBindingForTestClock(ctx, p, pin, source, a, clock)
	}
	return f
}
