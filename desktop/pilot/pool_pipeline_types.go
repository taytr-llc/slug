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
)

const poolPipelineDirectory = "event-cycle-v1"
const poolPipelineIntentName = "INTENT.json"
const poolPipelineActivationName = "ACTIVATION.json"
const poolPipelineActivatingName = "ACTIVATING.json"
const poolPipelineReadyName = "READY.json"

type poolPipelineFactories struct {
	createSeed, openSeed, resumeSeed       func(context.Context, string, gymseed.Pin, *gymevent.Store) (*gymseed.Store, error)
	createCard, openCard, resumeCard       func(context.Context, string, gymcard.Pin, *gymseed.Store) (*gymcard.Store, error)
	createBout, openBout, resumeBout       func(context.Context, string, gymbout.Pin, *gymcard.Store) (*gymbout.Store, error)
	createResult, openResult, resumeResult func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error)
	preflightCareer                        func(context.Context, string, gymresult.Pin, *gymbout.Store) error
	openCareerResult, resumeCareerResult   func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error)
	openSharedCareer, resumeSharedCareer   func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error)
	openCareerCard, resumeCareerCard       func(context.Context, string, gymcard.Pin, *gymseed.Store, careerstate.Activation) (*gymcard.Store, error)
}

func defaultPoolPipelineFactories() poolPipelineFactories {
	return poolPipelineFactories{
		createSeed: gymseed.Create, openSeed: gymseed.Open, resumeSeed: gymseed.ResumePrepared,
		createCard: gymcard.Create, openCard: gymcard.Open, resumeCard: gymcard.ResumePrepared,
		createBout: gymbout.Create, openBout: gymbout.Open, resumeBout: gymbout.ResumePrepared,
		createResult: gymresult.Create, openResult: gymresult.Open, resumeResult: gymresult.ResumePrepared,
		preflightCareer: gymresult.CheckCareerActivation, openCareerResult: gymresult.OpenCareer, resumeCareerResult: gymresult.ResumeCareerActivation,
		openSharedCareer: gymstable.OpenSharedCareer, resumeSharedCareer: gymstable.ResumeSharedCareerBinding,
		openCareerCard: gymcard.OpenCareer, resumeCareerCard: gymcard.ResumeCareerBinding,
	}
}

type poolPipelineChildBinding struct {
	SeedPin                                gymseed.Pin
	SeedStoreID                            string
	SeedCreatedMS, SeedSourceCreatedMS     int64
	CardPin                                gymcard.Pin
	CardStoreID                            string
	CardCreatedMS, CardSourceCreatedMS     int64
	BoutPin                                gymbout.Pin
	BoutStoreID                            string
	BoutCreatedMS, BoutSourceCreatedMS     int64
	ResultPin                              gymresult.Pin
	ResultStoreID                          string
	ResultCreatedMS, ResultSourceCreatedMS int64
	Career                                 careerstate.Activation
}

type poolPipelineIntent struct {
	Application       string `json:"application"`
	Version           uint32 `json:"version"`
	SourceBindingID   string `json:"source_binding_id"`
	LeagueReadySHA256 string `json:"league_ready_sha256"`
	EventStoreID      string `json:"event_store_id"`
	EventCreatedMS    int64  `json:"event_created_ms"`
	SeedStoreID       string `json:"seed_store_id"`
	CardStoreID       string `json:"card_store_id"`
	BoutStoreID       string `json:"bout_store_id"`
	ResultStoreID     string `json:"result_store_id"`
}

// Separate immutable stage markers preserve the original creation times even
// before career activation. They never authorize a missing store to be created.
type poolPipelineStage struct {
	Version         uint32 `json:"version"`
	StoreID         string `json:"store_id"`
	CreatedMS       int64  `json:"created_ms"`
	SourceCreatedMS int64  `json:"source_created_ms"`
}
type poolPipelinePrepared struct {
	Version uint32             `json:"version"`
	Intent  poolPipelineIntent `json:"intent"`
	Seed    poolPipelineStage  `json:"seed"`
	Card    poolPipelineStage  `json:"card"`
	Bout    poolPipelineStage  `json:"bout"`
	Result  poolPipelineStage  `json:"result"`
}
type poolPipelineReady struct {
	Prepared poolPipelinePrepared   `json:"prepared"`
	Career   careerstate.Activation `json:"career"`
}
type poolPipelineBootstrap struct {
	intent     poolPipelineIntent
	stages     [4]*poolPipelineStage
	activating bool
	activation *careerstate.Activation
	ready      *poolPipelineReady
}
