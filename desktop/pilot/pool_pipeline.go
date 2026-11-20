// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/seednode"
	"io"
	"path/filepath"
	"reflect"
)

func openPoolPipelineStage[T io.Closer](root, name, id string, sourceMS int64, recorded *poolPipelineStage, create, open, resume func(string) (T, error), info func(T) (poolPipelineStage, error)) (_ T, stage poolPipelineStage, resultErr error) {
	var zero T
	path := filepath.Join(root, name)
	exists, e := poolLeagueDirectoryExists(path)
	if e != nil || recorded != nil && !exists {
		return zero, stage, ErrPoolRuntime
	}
	var store T
	if recorded != nil {
		store, e = open(path)
	} else if !exists || poolLeagueEmpty(path) {
		store, e = create(path)
	} else {
		store, e = resume(path)
	}
	if e != nil {
		return zero, stage, e
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, store.Close())
		}
	}()
	stage, e = info(store)
	if e != nil {
		return zero, stage, e
	}
	if stage.Version != 1 || stage.StoreID != id || stage.SourceCreatedMS != sourceMS || stage.CreatedMS < sourceMS || stage.CreatedMS <= 0 || recorded != nil && *recorded != stage {
		return zero, stage, ErrPoolRuntime
	}
	if e = writePoolRuntimeRecord(filepath.Join(root, name+"-READY.json"), stage); e != nil {
		return zero, stage, e
	}
	keep = true
	return store, stage, nil
}

// The parent gate excludes children while this complete ownership chain is
// prepared. All audits and durable writes run outside account/session leases.
func (r *PoolRuntime) preparePipeline(ctx context.Context) (resultErr error) {
	if r.pipeline != nil && r.results != nil {
		return nil
	}
	if r.league == nil || r.events == nil || r.issuer == nil || ctx == nil || ctx.Err() != nil {
		return ErrPoolRuntime
	}
	if e := r.loadPipelineBootstrap(*r.league, true); e != nil {
		return e
	}
	intent, bind, e := derivePoolPipeline(*r.league)
	if e != nil {
		return e
	}
	root := filepath.Join(r.config.DataDir, poolPipelineDirectory)
	if e = poolLeagueMkdir(root); e != nil {
		return e
	}
	if e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineIntentName), intent); e != nil {
		return e
	}
	b := r.pipelineBootstrap
	if b == nil {
		b = &poolPipelineBootstrap{intent: intent}
		r.pipelineBootstrap = b
	}
	f := r.leagueFactories
	if f == nil {
		return ErrPoolRuntime
	}
	var seeds *gymseed.Store
	var cards *gymcard.Store
	var bouts *gymbout.Store
	var results *gymresult.Store
	keep := false
	defer func() {
		if !keep {
			if results != nil {
				resultErr = errors.Join(resultErr, results.Close())
			}
			if bouts != nil {
				resultErr = errors.Join(resultErr, bouts.Close())
			}
			if cards != nil {
				resultErr = errors.Join(resultErr, cards.Close())
			}
			if seeds != nil {
				resultErr = errors.Join(resultErr, seeds.Close())
			}
		}
	}()
	var stage poolPipelineStage
	seeds, stage, e = openPoolPipelineStage(root, "seeds", intent.SeedStoreID, intent.EventCreatedMS, b.stages[0],
		func(p string) (*gymseed.Store, error) { return f.createSeed(ctx, p, bind.SeedPin, r.events) },
		func(p string) (*gymseed.Store, error) { return f.openSeed(ctx, p, bind.SeedPin, r.events) },
		func(p string) (*gymseed.Store, error) { return f.resumeSeed(ctx, p, bind.SeedPin, r.events) },
		func(s *gymseed.Store) (poolPipelineStage, error) {
			i, e := s.Info(ctx)
			if e != nil || !reflect.DeepEqual(i.Pin, bind.SeedPin) {
				return poolPipelineStage{}, ErrPoolRuntime
			}
			return poolPipelineStage{1, i.StoreID, i.CreatedAtMS, intent.EventCreatedMS}, nil
		})
	if e != nil {
		return e
	}
	b.stages[0] = &stage
	openCard := func(p string) (*gymcard.Store, error) {
		if b.activation != nil {
			if b.ready != nil {
				return f.openCareerCard(ctx, p, bind.CardPin, seeds, *b.activation)
			}
			return f.resumeCareerCard(ctx, p, bind.CardPin, seeds, *b.activation)
		}
		return f.openCard(ctx, p, bind.CardPin, seeds)
	}
	var cardStage poolPipelineStage
	cards, cardStage, e = openPoolPipelineStage(root, "cards", intent.CardStoreID, stage.CreatedMS, b.stages[1],
		func(p string) (*gymcard.Store, error) { return f.createCard(ctx, p, bind.CardPin, seeds) }, openCard,
		func(p string) (*gymcard.Store, error) { return f.resumeCard(ctx, p, bind.CardPin, seeds) },
		func(s *gymcard.Store) (poolPipelineStage, error) {
			i, e := s.Info(ctx)
			if e != nil || !reflect.DeepEqual(i.Pin, bind.CardPin) {
				return poolPipelineStage{}, ErrPoolRuntime
			}
			return poolPipelineStage{1, i.StoreID, i.CreatedAtMS, i.SourceCreatedAtMS}, nil
		})
	if e != nil {
		return e
	}
	b.stages[1] = &cardStage
	var boutStage poolPipelineStage
	bouts, boutStage, e = openPoolPipelineStage(root, "bouts", intent.BoutStoreID, cardStage.CreatedMS, b.stages[2],
		func(p string) (*gymbout.Store, error) { return f.createBout(ctx, p, bind.BoutPin, cards) },
		func(p string) (*gymbout.Store, error) { return f.openBout(ctx, p, bind.BoutPin, cards) },
		func(p string) (*gymbout.Store, error) { return f.resumeBout(ctx, p, bind.BoutPin, cards) },
		func(s *gymbout.Store) (poolPipelineStage, error) {
			i, e := s.Info(ctx)
			if e != nil || !reflect.DeepEqual(i.Pin, bind.BoutPin) {
				return poolPipelineStage{}, ErrPoolRuntime
			}
			return poolPipelineStage{1, i.StoreID, i.CreatedAtMS, i.SourceCreatedAtMS}, nil
		})
	if e != nil {
		return e
	}
	b.stages[2] = &boutStage
	openResult := func(p string) (*gymresult.Store, error) {
		if b.ready != nil {
			return f.openCareerResult(ctx, p, bind.ResultPin, bouts)
		}
		if b.activating {
			return f.resumeCareerResult(ctx, p, bind.ResultPin, bouts)
		}
		return f.openResult(ctx, p, bind.ResultPin, bouts)
	}
	var resultStage poolPipelineStage
	results, resultStage, e = openPoolPipelineStage(root, "results", intent.ResultStoreID, boutStage.CreatedMS, b.stages[3],
		func(p string) (*gymresult.Store, error) { return f.createResult(ctx, p, bind.ResultPin, bouts) }, openResult,
		func(p string) (*gymresult.Store, error) { return f.resumeResult(ctx, p, bind.ResultPin, bouts) },
		func(s *gymresult.Store) (poolPipelineStage, error) {
			i, e := s.Info(ctx)
			if e != nil || !reflect.DeepEqual(i.Pin, bind.ResultPin) {
				return poolPipelineStage{}, ErrPoolRuntime
			}
			return poolPipelineStage{1, i.StoreID, i.CreatedAtMS, i.SourceCreatedAtMS}, nil
		})
	if e != nil {
		return e
	}
	b.stages[3] = &resultStage
	prepared, e := b.prepared()
	if e != nil {
		return e
	}
	resultPath := filepath.Join(root, "results")
	if !b.activating {
		if e = results.Close(); e != nil {
			return e
		}
		results = nil
		if e = f.preflightCareer(ctx, resultPath, bind.ResultPin, bouts); e != nil {
			return e
		}
		if e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineActivatingName), prepared); e != nil {
			return e
		}
		b.activating = true
		results, e = f.resumeCareerResult(ctx, resultPath, bind.ResultPin, bouts)
		if e != nil {
			return e
		}
	} else if e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineActivatingName), prepared); e != nil {
		return e
	}
	activation, e := results.CareerInfo(ctx)
	if e != nil {
		return e
	}
	if b.activation != nil && *b.activation != activation {
		return ErrPoolRuntime
	}
	if e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineActivationName), activation); e != nil {
		return e
	}
	b.activation = &activation
	reader, e := results.CareerReader()
	if e != nil {
		return e
	}
	for _, p := range r.league.Intent.Participants {
		path := filepath.Join(r.config.DataDir, "gyms", p.AccountID, "stable")
		var open = f.resumeSharedCareer
		if b.ready != nil {
			open = f.openSharedCareer
		}
		stable, e := open(path, p.EntryPin.StablePin, r.issuer, activation)
		if e != nil {
			return e
		}
		if b.ready != nil {
			e = stable.AttachCareer(ctx, reader)
		} else {
			e = stable.BindCareer(ctx, reader)
		}
		e = errors.Join(e, stable.Close())
		if e != nil {
			return e
		}
	}
	if b.ready != nil {
		e = cards.AttachCareer(ctx, reader)
	} else {
		e = cards.BindCareer(ctx, reader)
	}
	if e != nil {
		return e
	}
	ready := poolPipelineReady{prepared, activation}
	if e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineReadyName), ready); e != nil {
		return e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || ctx.Err() != nil || r.life.Err() != nil {
		return ErrPoolRuntime
	}
	r.pipeline = &ready
	r.seeds = seeds
	r.cards = cards
	r.bouts = bouts
	r.results = results
	r.careerReader = reader
	b.ready = &ready
	keep = true
	return nil
}

// Called only with the runtime mutex held. The child receives owned value pins
// and borrowed concrete owners, never authority reconstructed from disk.
func (r *PoolRuntime) pipelineChild() (*poolPipelineChildBinding, *seednode.SharedPoolOwnerBinding, error) {
	if r.pipeline == nil || r.league == nil || r.results == nil || r.bouts == nil || r.cards == nil || r.seeds == nil || r.careerReader == nil || r.nodes == nil || r.nodes.primary == nil || r.nodes.peer == nil {
		return nil, nil, ErrPoolRuntime
	}
	intent, b, e := derivePoolPipeline(*r.league)
	if e != nil || intent != r.pipeline.Prepared.Intent {
		return nil, nil, ErrPoolRuntime
	}
	p := r.pipeline.Prepared
	b.SeedCreatedMS, b.SeedSourceCreatedMS = p.Seed.CreatedMS, p.Seed.SourceCreatedMS
	b.CardCreatedMS, b.CardSourceCreatedMS = p.Card.CreatedMS, p.Card.SourceCreatedMS
	b.BoutCreatedMS, b.BoutSourceCreatedMS = p.Bout.CreatedMS, p.Bout.SourceCreatedMS
	b.ResultCreatedMS, b.ResultSourceCreatedMS = p.Result.CreatedMS, p.Result.SourceCreatedMS
	b.Career = r.pipeline.Career
	source := cloneEventPin(r.league.Intent.EventPin)
	owner := &seednode.SharedPoolOwnerBinding{Source: source, EventStoreID: r.league.Intent.EventStoreID, EventCreatedAtMS: r.league.EventCreatedMS, Pool: r.source, PoolBindingID: intent.SourceBindingID, Accounts: r.accounts, Issuer: r.issuer, Events: r.events, Home: r.config.DataDir, Primary: seednode.OwnedProcess{Cmd: r.nodes.primary.cmd, Done: r.nodes.primary.done}, Peer: seednode.OwnedProcess{Cmd: r.nodes.peer.cmd, Done: r.nodes.peer.done}, Life: r.life, RuntimeMarkerSHA256: poolCanonicalHash(r.marker), LeagueIntentSHA256: poolCanonicalHash(r.league.Intent), LeagueReadySHA256: poolCanonicalHash(*r.league)}
	return &b, owner, nil
}
