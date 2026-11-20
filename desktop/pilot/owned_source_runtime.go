// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

const ownedSourceConfigName = "SLUG-OWNED-SOURCE.json"
const ownedSourceDirectory = "owned-chain"

// OwnedSourceConfig is chosen and durably retained by the outer app setup
// before this constructor. Existing store markers cannot supply these values.
type OwnedSourceConfig struct {
	Version        uint32                         `json:"version"`
	Ownership      fighterownership.Pin           `json:"ownership"`
	Store          fighterownership.StoreIdentity `json:"store"`
	PublicReadPort uint16                         `json:"public_read_port"`
}
type ownedRuntimeResources struct {
	config          OwnedSourceConfig
	publisher       *registry.ManifestPublication
	store           *fighterownership.Store
	node            *fighterownership.OwnedChainSource
	public          *fighterownership.PublicReadServer
	server          *http.Server
	listener        net.Listener
	serveDone       chan error
	replay          *fighterownership.Replay
	progress        fighterownership.Progress
	source          *fighterownership.PublicSource
	lastObservation fighterownership.ChainObservation
	closeOnce       sync.Once
	closeErr        error
}

func validateOwnedSourceConfig(c OwnedSourceConfig, options PoolOptions) error {
	if c.Version != 1 || registry.ValidatePublicManifestPin(c.Ownership) != nil || !payoutNonzeroID(c.Store.StoreID) || c.PublicReadPort == 0 || c.Ownership.Source.DescriptorID != options.CandidateDescriptorID {
		return ErrManifestInitialization
	}
	return nil
}

func NewOwnedPoolRuntime(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, expected OwnedSourceConfig) (*PoolRuntime, error) {
	return newPoolRuntimeWithOptions(ctx, config, accounts, options, &expected)
}

func (r *PoolRuntime) openOwnedIssuer(established bool) error {
	o := r.owned
	if o == nil {
		return ErrPoolRuntime
	}
	guard := filepath.Join(r.config.DataDir, ownedSourceConfigName)
	hasGuard, e := poolRuntimeExists(guard)
	if e != nil {
		return e
	}
	issuerPath := filepath.Join(r.config.DataDir, "issuer")
	hasIssuer, e := poolRuntimeExists(issuerPath)
	if e != nil {
		return e
	}
	sourcePath := filepath.Join(r.config.DataDir, poolRuntimeSourceName)
	hasSource, e := poolRuntimeExists(sourcePath)
	if e != nil {
		return e
	}
	if established && (!hasGuard || !hasSource || !hasIssuer) {
		return ErrManifestInitialization
	}
	if !hasGuard && (hasSource || hasIssuer) {
		return ErrManifestInitialization
	}
	// Exact independent source identity precedes the first authority DB write.
	if e = writePoolRuntimeRecord(guard, o.config); e != nil {
		return e
	}
	r.source = r.marker.source(o.config.Ownership.Source)
	if !r.marker.validSource(r.source) {
		return ErrManifestInitialization
	}
	if e = writePoolRuntimeRecord(sourcePath, r.source); e != nil {
		return e
	}
	if hasIssuer {
		r.issuer, e = registry.OpenPublicManifestAuthority(issuerPath, o.config.Ownership)
	} else {
		var descriptor fighterdescriptor.Descriptor
		descriptor, e = fighterdescriptor.ResolveCandidate(r.marker.DescriptorID)
		if e == nil {
			r.issuer, e = registry.CreatePublicManifestAuthority(issuerPath, descriptor, o.config.Ownership)
		}
	}
	return e
}
func (r *PoolRuntime) openOwnedSource(ctx context.Context, established bool) error {
	o := r.owned
	if o == nil || r.nodes == nil || r.issuer == nil {
		return ErrPoolRuntime
	}
	var e error
	o.publisher, e = r.issuer.PublicPublication(o.config.Ownership)
	if e != nil {
		return e
	}
	path := filepath.Join(r.config.DataDir, ownedSourceDirectory)
	exists, e := poolRuntimeExists(path)
	if e != nil {
		return e
	}
	if !exists && established {
		return ErrManifestInitialization
	}
	if exists {
		o.store, e = fighterownership.Open(path, o.config.Ownership, o.config.Store)
	} else {
		o.store, e = fighterownership.Create(path, o.config.Ownership, o.config.Store)
	}
	if e != nil {
		return e
	}
	o.node, e = fighterownership.NewOwnedChainSource(ctx, o.config.Ownership, r.nodes.rpcConfig(), r.life)
	if e != nil {
		return e
	}
	o.public, e = fighterownership.NewPublicReadServer(o.config.Ownership, o.publisher, o.store, r.life)
	if e != nil {
		return e
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(o.config.PublicReadPort)))
	o.listener, e = net.Listen("tcp", address)
	if e != nil {
		return e
	}
	o.server = &http.Server{Handler: o.public, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	o.serveDone = make(chan error, 1)
	go func() {
		e := o.server.Serve(o.listener)
		o.serveDone <- e
		if !errors.Is(e, http.ErrServerClosed) {
			r.cancel()
		}
	}()
	return nil
}
func (o *ownedRuntimeResources) Close() error {
	if o == nil {
		return nil
	}
	o.closeOnce.Do(func() {
		var failures []error
		if o.server != nil {
			failures = append(failures, o.server.Close())
		}
		if o.serveDone != nil {
			e := <-o.serveDone
			if !errors.Is(e, http.ErrServerClosed) {
				failures = append(failures, e)
			}
		}
		if o.public != nil {
			failures = append(failures, o.public.Close())
		}
		if o.replay != nil {
			failures = append(failures, o.replay.Close())
		}
		if o.node != nil {
			failures = append(failures, o.node.Close())
		}
		if o.store != nil {
			failures = append(failures, o.store.Close())
		}
		if o.publisher != nil {
			failures = append(failures, o.publisher.Close())
		}
		o.closeErr = errors.Join(failures...)
	})
	return o.closeErr
}

// PublicOwnershipConfig is public configuration for an independently restored
// gym. It discloses no wallet, account, operation journal or source database.
func (r *PoolRuntime) PublicOwnershipConfig() (OwnedSourceConfig, fighterownership.PublicReadConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.life.Err() != nil || r.owned == nil {
		return OwnedSourceConfig{}, fighterownership.PublicReadConfig{}, ErrPoolRuntime
	}
	c := r.owned.config
	return c, fighterownership.PublicReadConfig{URL: "http://127.0.0.1:" + strconv.Itoa(int(c.PublicReadPort)) + "/slug/ownership/v1"}, nil
}

// OwnershipOwners borrows the exact concrete pair. Their own operations retain
// lifetime and fresh-node checks; a DTO cannot replace either owner.
func (r *PoolRuntime) OwnershipOwners() (*fighterownership.Store, *fighterownership.OwnedChainSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.life.Err() != nil || r.owned == nil {
		return nil, nil, ErrPoolRuntime
	}
	return r.owned.store, r.owned.node, nil
}

// RefreshOwnership performs a bounded slice of actual verification. Callers
// retry on contention or a moving chain; no cached head becomes ready on error.
func (r *PoolRuntime) RefreshOwnership(ctx context.Context, steps uint32) (progress fighterownership.Progress, resultErr error) {
	defer func() {
		if resultErr != nil {
			progress.Ready = false
			progress.Head = nil
			progress.Cutoff = nil
		}
	}()
	if steps < 1 || steps > 128 {
		return fighterownership.Progress{}, ErrPoolRuntime
	}
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return fighterownership.Progress{}, e
	}
	defer done()
	o := r.owned
	if o == nil {
		return fighterownership.Progress{}, ErrPoolRuntime
	}
	if o.replay == nil {
		// A moving tip may leave a synchronized chain publication waiting for
		// its exact old body to be signed. Drain that concrete intent before
		// admission PublishNext, which deliberately cannot sign chain facts.
		source, e := fighterownership.NewPublicSource(ctx, o.config.Ownership, o.publisher)
		if e != nil {
			return o.progress, e
		}
		if source, _, e = o.store.ResumeChainPublication(ctx, source, o.node); e != nil {
			return o.progress, e
		}
		// A signed historical cutoff may likewise have an interrupted exact
		// reference publication. Drain it before admission/current-tip work,
		// including PrepareOwnedSource before any league exists. Use the fresh
		// source returned by chain recovery; neither drain grants a ready Head.
		if _, _, e = o.store.ResumeCutoffPublication(ctx, source); e != nil {
			return o.progress, e
		}
		status, e := o.publisher.Status(ctx)
		if e != nil {
			return o.progress, e
		}
		for n := 0; n < 8 && status.PublicEvidencePending; n++ {
			status, e = o.publisher.PublishNext(ctx)
			if e != nil {
				return o.progress, e
			}
		}
		if status.PublicEvidencePending {
			return o.progress, fighterownership.ErrUnavailable
		}
		o.source, e = fighterownership.NewPublicSource(ctx, o.config.Ownership, o.publisher)
		if e != nil {
			return o.progress, e
		}
		observation, e := o.node.Observe(ctx)
		if e != nil {
			return o.progress, e
		}
		unchangedObservation := observation == o.lastObservation
		if !unchangedObservation {
			o.source, _, e = o.store.RecordChainObservation(ctx, o.source, o.node)
			if e != nil {
				return o.progress, e
			}
			o.lastObservation = observation
			status, e = o.publisher.Status(ctx)
			if e != nil {
				return o.progress, e
			}
		}
		// An exact completed observation/source prefix can be reused. A fresh
		// process always replays because lastObservation begins empty.
		if h, e := o.store.Head(ctx); e == nil && unchangedObservation && o.progress.Ready && h.ChainHeight == observation.Height && h.ChainHash == observation.Hash && h.PublicationSequence == status.PublicationSequence && h.AdmissionSequence == status.SignedAdmissionSequence {
			return o.progress, nil
		}
		o.replay, e = o.store.BeginReplay(ctx, o.source, o.node)
		if errors.Is(e, fighterownership.ErrStale) {
			// A new source/node frontier cannot resume an older partial job.
			// Retire only its exact joined work record; immutable evidence stays.
			partial, inspectErr := o.store.InspectReplay(ctx)
			if inspectErr != nil {
				return o.progress, inspectErr
			}
			if e = o.store.AbandonReplay(ctx, partial.WorkID); e != nil {
				return o.progress, e
			}
			o.replay, e = o.store.BeginReplay(ctx, o.source, o.node)
		}
		if e != nil {
			return o.progress, e
		}
		o.progress = fighterownership.Progress{WorkID: o.replay.WorkID()}
	}
	for n := uint32(0); n < steps; n++ {
		p, e := o.replay.Step(ctx)
		if p.WorkID != "" {
			o.progress = p
		}
		if e != nil {
			_ = o.replay.Close()
			o.replay = nil
			o.progress.Ready = false
			return o.progress, e
		}
		o.progress = p
		if p.Ready {
			_ = o.replay.Close()
			o.replay = nil
			return p, nil
		}
	}
	return o.progress, nil
}
