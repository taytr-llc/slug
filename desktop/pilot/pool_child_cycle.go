// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"reflect"

	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/seednode"
)

// The runtime derives this complete chain from the retained event authority.
// A child can open established guards, but cannot resume or create them.
func validatePoolPipelineChild(r poolChildResources) (*poolPipelineChildBinding, error) {
	if r.Pipeline == nil {
		if r.Seeds != nil || r.Cards != nil || r.Bouts != nil || r.Results != nil || r.CareerReader != nil || r.Publisher != nil {
			return nil, ErrManifestInitialization
		}
		return nil, nil // private pre-pipeline fixtures; the public runtime requires a pipeline
	}
	if r.League == nil || r.Seeds == nil || r.Cards == nil || r.Bouts == nil || r.Results == nil || r.CareerReader == nil || r.Publisher == nil || r.Factories == nil || r.Factories.openSharedCareer == nil {
		return nil, ErrManifestInitialization
	}
	x := *r.Pipeline
	seed, err := gymseed.CandidatePin(r.League.EventPin)
	if err != nil || !reflect.DeepEqual(seed, x.SeedPin) {
		return nil, ErrManifestInitialization
	}
	card, err := gymcard.CandidatePin(seed)
	if err != nil || !reflect.DeepEqual(card, x.CardPin) {
		return nil, ErrManifestInitialization
	}
	bout, err := gymbout.CandidatePin(card)
	if err != nil || !reflect.DeepEqual(bout, x.BoutPin) {
		return nil, ErrManifestInitialization
	}
	result, err := gymresult.CandidatePin(bout)
	if err != nil || !reflect.DeepEqual(result, x.ResultPin) {
		return nil, ErrManifestInitialization
	}
	sid, _ := gymseed.ID(seed)
	cid, _ := gymcard.ID(card)
	bid, _ := gymbout.ID(bout)
	rid, _ := gymresult.ID(result)
	if x.SeedStoreID != sid || x.CardStoreID != cid || x.BoutStoreID != bid || x.ResultStoreID != rid ||
		x.SeedSourceCreatedMS != r.League.EventCreatedMS || x.SeedCreatedMS < x.SeedSourceCreatedMS ||
		x.CardSourceCreatedMS != x.SeedCreatedMS || x.CardCreatedMS < x.CardSourceCreatedMS ||
		x.BoutSourceCreatedMS != x.CardCreatedMS || x.BoutCreatedMS < x.BoutSourceCreatedMS ||
		x.ResultSourceCreatedMS != x.BoutCreatedMS || x.ResultCreatedMS < x.ResultSourceCreatedMS ||
		careerstate.ValidateActivation(x.Career) != nil || x.Career.ResultStoreID != rid || x.Career.ResultCreatedAtMS != x.ResultCreatedMS || x.Career.SourceCreatedAtMS != x.ResultSourceCreatedMS {
		return nil, ErrManifestInitialization
	}
	p := r.Publisher
	if !reflect.DeepEqual(p.Source, r.League.EventPin) || p.EventStoreID != r.League.EventStoreID || p.EventCreatedAtMS != r.League.EventCreatedMS || p.Pool != r.Source || p.PoolBindingID != r.League.SourceBindingID || p.Issuer != r.Issuer || p.Events != r.Events || p.Life != r.Life || p.Life == nil || p.Life.Err() != nil {
		return nil, ErrManifestInitialization
	}
	// CandidatePin returns owned nested participant slices, not resource aliases.
	x.SeedPin, x.CardPin, x.BoutPin, x.ResultPin = seed, card, bout, result
	return &x, nil
}

func (s *StableController) preparePoolPipeline(r poolChildResources) error {
	x, err := validatePoolPipelineChild(r)
	if err != nil || x == nil {
		return err
	}
	a, err := r.CareerReader.Activation()
	if err != nil || a != x.Career || s.installed || s.stableStore != nil || s.sharedSource != r.Issuer {
		return ErrManifestInitialization
	}
	if err = s.ConfigureCareer(r.CareerReader, r.Factories.openSharedCareer); err != nil {
		return err
	}
	s.seedEnabled, s.cardEnabled, s.boutEnabled, s.resultEnabled = true, true, true, true
	return nil
}

func (s *StableController) attachPoolPipeline(ctx context.Context, r poolChildResources) error {
	x, err := validatePoolPipelineChild(r)
	if err != nil || x == nil {
		return err
	}
	// These concrete-store reads and career checks may audit state. Keep every
	// one outside the original account lease; only bounded metadata is published.
	si, err := r.Seeds.Info(ctx)
	if err != nil || si.StoreID != x.SeedStoreID || si.CreatedAtMS != x.SeedCreatedMS || !reflect.DeepEqual(si.Pin, x.SeedPin) {
		return ErrManifestInitialization
	}
	ci, err := r.Cards.Info(ctx)
	if err != nil || ci.StoreID != x.CardStoreID || ci.CreatedAtMS != x.CardCreatedMS || ci.SourceCreatedAtMS != x.CardSourceCreatedMS || !reflect.DeepEqual(ci.Pin, x.CardPin) {
		return ErrManifestInitialization
	}
	bi, err := r.Bouts.Info(ctx)
	if err != nil || bi.StoreID != x.BoutStoreID || bi.CreatedAtMS != x.BoutCreatedMS || bi.SourceCreatedAtMS != x.BoutSourceCreatedMS || !reflect.DeepEqual(bi.Pin, x.BoutPin) {
		return ErrManifestInitialization
	}
	ri, err := r.Results.Info(ctx)
	if err != nil || ri.StoreID != x.ResultStoreID || ri.CreatedAtMS != x.ResultCreatedMS || ri.SourceCreatedAtMS != x.ResultSourceCreatedMS || !reflect.DeepEqual(ri.Pin, x.ResultPin) {
		return ErrManifestInitialization
	}
	a, err := r.Results.CareerInfo(ctx)
	if err != nil || a != x.Career {
		return ErrManifestInitialization
	}
	a, err = r.CareerReader.Activation()
	if err != nil || a != x.Career {
		return ErrManifestInitialization
	}
	publisher := *r.Publisher
	publisher.Source = cloneEventPin(r.Publisher.Source)
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.installed || !s.eventAttached || s.stableClosed || s.seedAttached || s.cardAttached || s.boutAttached || s.resultAttached || s.careerReader != r.CareerReader {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		metadata := MaxSeedMetadataSize + cardRetainedMetadataBytes + boutRetainedMetadataBytes + resultRetainedMetadataBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+metadata) {
			return gymstable.ErrCapacity
		}
		s.seedStore, s.seedPin, s.seedID, s.seedCreated = r.Seeds, x.SeedPin, x.SeedStoreID, x.SeedCreatedMS
		s.cardStore, s.cardPin, s.cardID = r.Cards, x.CardPin, x.CardStoreID
		s.cardCreated, s.cardSourceCreated = x.CardCreatedMS, x.CardSourceCreatedMS
		s.boutStore, s.boutPin, s.boutID = r.Bouts, x.BoutPin, x.BoutStoreID
		s.boutCreated, s.boutSourceCreated = x.BoutCreatedMS, x.BoutSourceCreatedMS
		s.resultStore, s.resultPin, s.resultID = r.Results, x.ResultPin, x.ResultStoreID
		s.resultCreated, s.resultSourceCreated = x.ResultCreatedMS, x.ResultSourceCreatedMS
		s.seedAttached, s.cardAttached, s.boutAttached, s.resultAttached = true, true, true, true
		s.seedBytes, s.cardBytes, s.boutBytes, s.resultBytes = MaxSeedMetadataSize, cardRetainedMetadataBytes, boutRetainedMetadataBytes, resultRetainedMetadataBytes
		s.sharedPublisher = &publisher
		return nil
	})
}

// This is a process-lifetime check for cached node state, not an ownership
// attestation. The concrete adapter revalidates ownership before every borrow.
func sharedPoolNodesReady(p *seednode.SharedPoolOwnerBinding) bool {
	if p == nil || p.Life == nil || p.Life.Err() != nil || p.Primary.Cmd == nil || p.Peer.Cmd == nil || p.Primary.Done == nil || p.Peer.Done == nil {
		return false
	}
	select {
	case <-p.Primary.Done:
		return false
	default:
	}
	select {
	case <-p.Peer.Done:
		return false
	default:
	}
	return true
}
