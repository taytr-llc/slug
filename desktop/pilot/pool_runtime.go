// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

var ErrPoolRuntime = errors.New("shared pool runtime unavailable")
var ErrPoolRuntimeBusy = errors.New("shared pool runtime busy")

type poolRuntimeSessions struct{ accounts *gymaccount.Store }

func (a poolRuntimeSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return a.accounts.WithSession(ctx, token, gymaccount.Mining, func(account gymaccount.Account, check func() error) error {
		if account.Enrollment == nil || account.RecoveryRequired {
			return gymaccount.ErrUnauthorized
		}
		return fn(account.Enrollment.GymPublicKey, check)
	})
}

type poolRuntimeBorrow struct {
	account string
	close   func() error
	done    chan struct{}
}

// PoolRuntime owns only the dedicated fixture pool environment. Its account
// store is borrowed, its children borrow sources, and nothing here mines.
type PoolRuntime struct {
	mu                           sync.Mutex
	gate                         chan struct{}
	config                       Config
	accounts                     *gymaccount.Store
	marker                       poolRuntimeMarker
	source                       poolshare.Config
	lock                         *leveldb.DB
	nodes                        *pilotNodes
	node                         *prospectnode.Client
	issuer                       *registry.ManifestStore
	pool                         *poolshare.Pool
	rounds                       *poolround.Store
	settlement                   *poolsettle.Store
	league                       *poolLeagueReady
	events                       *gymevent.Store
	leagueFactories              *poolLeagueFactories
	pipeline                     *poolPipelineReady
	pipelineBootstrap            *poolPipelineBootstrap
	seeds                        *gymseed.Store
	cards                        *gymcard.Store
	bouts                        *gymbout.Store
	results                      *gymresult.Store
	careerReader                 *careerstate.Reader
	backend                      *poolsettlefixture.Backend
	recipients                   map[string]PoolRecipientBinding
	verified                     map[string]bool
	borrows                      map[*poolRuntimeBorrow]bool
	life                         context.Context
	cancel                       context.CancelFunc
	servicesCancel               context.CancelFunc
	services                     sync.WaitGroup
	ready, closed, serviceFailed bool
	lastError                    string
	closeOnce                    sync.Once
	closeErr                     error
}

type poolChildResources struct {
	Config                                               Config
	Source                                               poolshare.Config
	PoolURL, PoolTrustID, AdmissionURL, AdmissionTrustID string
	Issuer                                               *registry.ManifestStore
	Node                                                 *prospectnode.Client
	RPC                                                  prospectnode.RPCConfig
	Recipient                                            PoolRecipientBinding
	StablePin                                            gymstable.Pin
	League                                               *poolLeagueChildBinding
	Events                                               *gymevent.Store
	Factories                                            *poolLeagueFactories
	Pipeline                                             *poolPipelineChildBinding
	Seeds                                                *gymseed.Store
	Cards                                                *gymcard.Store
	Bouts                                                *gymbout.Store
	Results                                              *gymresult.Store
	CareerReader                                         *careerstate.Reader
	Publisher                                            *seednode.SharedPoolOwnerBinding
	WorkerConfig                                         prospectworker.PoolConfig
	Life                                                 context.Context
	Release                                              func()
	RegisterClose                                        func(func() error) error
}

func NewPoolRuntime(ctx context.Context, config Config, accounts *gymaccount.Store) (*PoolRuntime, error) {
	return NewPoolRuntimeWithOptions(ctx, config, accounts, PoolOptions{})
}

func NewPoolRuntimeWithOptions(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions) (_ *PoolRuntime, resultErr error) {
	options, optionErr := ResolvePoolOptions(options)
	if optionErr != nil {
		return nil, optionErr
	}
	if wire.TestPoW || ctx == nil || ctx.Err() != nil || accounts == nil || !validAccountID(accounts.AuthorityID()) || !filepath.IsAbs(config.DataDir) || filepath.Clean(config.DataDir) != config.DataDir || !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary || strings.ContainsAny(config.DataDir+config.NodeBinary, "\r\n\t\x00") {
		return nil, ErrPoolRuntime
	}
	if e := CheckPoolRuntimeSelection(config.DataDir, accounts.AuthorityID(), options); e != nil {
		return nil, e
	}
	if i, e := os.Stat(config.NodeBinary); e != nil || !i.Mode().IsRegular() {
		return nil, ErrPoolRuntime
	}
	check, stop := context.WithTimeout(ctx, 5*time.Second)
	cmd := exec.CommandContext(check, config.NodeBinary, "--version")
	var version limitedBuffer
	cmd.Stdout, cmd.Stderr = &version, io.Discard
	e := cmd.Run()
	stop()
	if e != nil || strings.TrimSpace(version.String()) != "slugd version 0.27.0" {
		return nil, ErrPoolRuntime
	}
	if e = os.Mkdir(config.DataDir, 0700); e == nil {
		if e = syncDirectory(filepath.Dir(config.DataDir)); e != nil {
			return nil, ErrPoolRuntime
		}
	} else if !os.IsExist(e) {
		return nil, ErrPoolRuntime
	}
	if i, e := os.Lstat(config.DataDir); e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return nil, ErrPoolRuntime
	}
	life, cancel := context.WithCancel(context.Background())
	r := &PoolRuntime{config: config, accounts: accounts, gate: make(chan struct{}, 1), life: life, cancel: cancel, recipients: make(map[string]PoolRecipientBinding), verified: make(map[string]bool), borrows: make(map[*poolRuntimeBorrow]bool)}
	keep := false
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, r.Close())
		}
	}()
	markerPath := filepath.Join(config.DataDir, poolRuntimeMarkerName)
	exists, e := poolRuntimeExists(markerPath)
	if e != nil {
		return nil, e
	}
	if !exists {
		entries, e := os.ReadDir(config.DataDir)
		if e != nil || len(entries) != 0 {
			return nil, ErrPoolRuntime
		}
		poolPort, e := unusedPort()
		if e != nil {
			return nil, e
		}
		admissionPort, e := unusedPort()
		if e != nil || admissionPort == poolPort {
			return nil, ErrPoolRuntime
		}
		r.marker = poolRuntimeMarker{"slug-shared-pool-runtime", 1, accounts.AuthorityID(), policy().Network, genesis, chaincfg.PhaseAActivationHeight, recovery.ProfileID, options.CandidateDescriptorID, true, poolPort, admissionPort}
		if e = writePoolRuntimeRecord(markerPath, r.marker); e != nil {
			return nil, e
		}
	} else if readManifestJSON(markerPath, poolRuntimeMarkerLimit, &r.marker) != nil || !r.marker.validFor(accounts.AuthorityID(), options.CandidateDescriptorID) {
		return nil, ErrPoolRuntime
	}
	if e = poolRuntimeRootFiles(config.DataDir); e != nil {
		return nil, e
	}
	if e = plainChild(filepath.Join(config.DataDir, "instance-lock"), true); e != nil {
		return nil, e
	}
	r.lock, e = leveldb.OpenFile(filepath.Join(config.DataDir, "instance-lock"), nil)
	if e != nil {
		return nil, ErrPoolRuntimeBusy
	}
	coreExists, e := poolRuntimeExists(filepath.Join(config.DataDir, poolRuntimeCoreName))
	if e != nil {
		return nil, e
	}
	readyExists, e := poolRuntimeExists(filepath.Join(config.DataDir, poolRuntimeReadyName))
	if e != nil {
		return nil, e
	}
	if readyExists && !coreExists {
		return nil, ErrPoolRuntime
	}
	if coreExists {
		for _, name := range []string{poolRuntimeSourceName, "issuer", "shares", "rounds", "primary-chain", "peer-chain", "gyms"} {
			if exists, e := poolRuntimeExists(filepath.Join(config.DataDir, name)); e != nil || !exists {
				return nil, ErrPoolRuntime
			}
		}
		if e = requireChainFiles(config.DataDir); e != nil {
			return nil, e
		}
	}
	if e = r.openPoolIssuer(coreExists); e != nil {
		return nil, e
	}
	startup, cancelStartup := context.WithTimeout(ctx, 45*time.Second)
	defer cancelStartup()
	r.nodes, e = r.openPoolNodes(startup)
	if e != nil {
		return nil, e
	}
	r.node, e = prospectnode.NewClient(r.nodes.rpcConfig())
	if e != nil {
		return nil, e
	}
	sessions := poolRuntimeSessions{accounts}
	sharesPath := filepath.Join(config.DataDir, "shares")
	sharesExist, e := poolRuntimeExists(sharesPath)
	if e != nil {
		return nil, e
	}
	if sharesExist {
		r.pool, e = poolshare.Open(sharesPath, r.source, r.issuer, r.node, sessions)
	} else {
		r.pool, e = poolshare.Create(sharesPath, r.source, r.issuer, r.node, sessions)
	}
	if e != nil {
		return nil, e
	}
	limits := poolround.Limits{Version: 1, MaxRounds: 256, MaxBytes: 256 << 20}
	roundsPath := filepath.Join(config.DataDir, "rounds")
	roundsExist, e := poolRuntimeExists(roundsPath)
	if e != nil {
		return nil, e
	}
	if roundsExist {
		r.rounds, e = poolround.Open(roundsPath, limits, r.pool, r.node)
	} else {
		r.rounds, e = poolround.Create(roundsPath, limits, r.pool, r.node)
	}
	if e != nil {
		return nil, e
	}
	if e = r.loadPoolRecipients(); e != nil {
		return nil, e
	}
	core, e := r.coreRecord()
	if e != nil {
		return nil, e
	}
	if e = writePoolRuntimeRecord(filepath.Join(config.DataDir, poolRuntimeCoreName), core); e != nil {
		return nil, e
	}
	if readyExists {
		ready, e := r.readyRecord()
		if e != nil || readPoolRuntimeRecord(filepath.Join(config.DataDir, poolRuntimeReadyName), ready) != nil {
			return nil, ErrPoolRuntime
		}
		if exists, e := poolRuntimeExists(filepath.Join(config.DataDir, "settlement")); e != nil || !exists {
			return nil, ErrPoolRuntime
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	keep = true
	go func() {
		select {
		case <-life.Done():
		case <-r.nodes.primary.done:
		case <-r.nodes.peer.done:
		}
		_ = r.Close()
	}()
	return r, nil
}

func (r *PoolRuntime) openPoolIssuer(established bool) error {
	root := r.config.DataDir
	sourcePath := filepath.Join(root, poolRuntimeSourceName)
	issuerPath := filepath.Join(root, "issuer")
	hasSource, e := poolRuntimeExists(sourcePath)
	if e != nil {
		return e
	}
	hasIssuer, e := poolRuntimeExists(issuerPath)
	if e != nil {
		return e
	}
	if hasSource {
		if !hasIssuer || readManifestJSON(sourcePath, poolRuntimeMarkerLimit, &r.source) != nil || !r.marker.validSource(r.source) {
			return ErrPoolRuntime
		}
		if _, e = poolservice.BindingID(r.source); e != nil {
			return e
		}
		r.issuer, e = registry.OpenManifestAuthority(issuerPath, r.source.AuthorityPin, false)
		return e
	}
	if established || hasIssuer {
		// An interrupted creation without an independently persisted source
		// pin cannot turn inspection of an orphan issuer into trusted config.
		return ErrPoolRuntime
	}
	r.issuer, e = registry.CreateManifestAuthority(issuerPath, r.marker.Network, r.marker.Genesis, r.marker.DescriptorID)
	if e != nil {
		return e
	}
	info, e := r.issuer.Info()
	if e != nil {
		return e
	}
	r.source = r.marker.source(info.Pin)
	if _, e = poolservice.BindingID(r.source); e != nil {
		return e
	}
	return writePoolRuntimeRecord(sourcePath, r.source)
}

func (r *PoolRuntime) SourceConfig() poolshare.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.source
}
func (r *PoolRuntime) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready && !r.closed && r.life.Err() == nil
}
func (r *PoolRuntime) LastError() string { r.mu.Lock(); defer r.mu.Unlock(); return r.lastError }
func (r *PoolRuntime) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrPoolRuntime
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.life.Err() != nil {
		return nil, nil, ErrPoolRuntime
	}
	select {
	case r.gate <- struct{}{}:
	default:
		return nil, nil, ErrPoolRuntimeBusy
	}
	work, cancel := context.WithTimeout(ctx, 60*time.Second)
	stop := context.AfterFunc(r.life, cancel)
	return work, func() { stop(); cancel(); <-r.gate }, nil
}

func (r *PoolRuntime) childResources(accountID, fixture string) (poolChildResources, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.ready || r.life.Err() != nil || !r.verified[fixture] {
		return poolChildResources{}, ErrPoolRuntime
	}
	b, ok := r.recipients[fixture]
	if !ok || b.Config.AccountID != accountID {
		return poolChildResources{}, ErrPoolRuntime
	}
	for borrow := range r.borrows {
		if borrow.account == accountID {
			return poolChildResources{}, ErrPoolRuntimeBusy
		}
	}
	poolURL, admissionURL := poolRuntimeOrigin(r.marker.PoolPort), poolRuntimeOrigin(r.marker.AdmissionPort)
	poolTrust, e := poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: poolURL, Token: fixtureToken, PublicKey: b.Config.Identity.GymPublicKey, Source: r.source})
	if e != nil {
		return poolChildResources{}, e
	}
	admissionTrust, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: admissionURL, Token: fixtureToken, AuthorityPin: r.source.AuthorityPin})
	if e != nil {
		return poolChildResources{}, e
	}
	pin := poolStablePin(b)
	league, e := r.leagueChild(accountID, pin)
	if e != nil {
		return poolChildResources{}, e
	}
	pipeline, publisher, e := r.pipelineChild()
	if e != nil {
		return poolChildResources{}, e
	}
	worker := prospectworker.PoolConfig{Version: 1, Source: r.source, PublicKey: b.Config.Identity.GymPublicKey, AuthorityOrigin: poolURL, TransportTrustID: poolTrust, AdmissionOrigin: admissionURL, AdmissionTrustID: admissionTrust, MaxRecords: 1000, MaxBytes: 256 << 20}
	if _, e = prospectworker.ValidatePoolConfig(worker); e != nil {
		return poolChildResources{}, e
	}
	borrow := &poolRuntimeBorrow{account: accountID, done: make(chan struct{})}
	r.borrows[borrow] = true
	var once sync.Once
	release := func() {
		once.Do(func() { r.mu.Lock(); defer r.mu.Unlock(); delete(r.borrows, borrow); close(borrow.done) })
	}
	register := func(closeChild func() error) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed || !r.borrows[borrow] || borrow.close != nil || closeChild == nil {
			return ErrPoolRuntime
		}
		borrow.close = closeChild
		return nil
	}
	return poolChildResources{Config: Config{DataDir: filepath.Join(r.config.DataDir, "gyms", accountID), NodeBinary: r.config.NodeBinary}, Source: r.source, PoolURL: poolURL, PoolTrustID: poolTrust, AdmissionURL: admissionURL, AdmissionTrustID: admissionTrust, Issuer: r.issuer, Node: r.node, RPC: r.nodes.rpcConfig(), Recipient: b, StablePin: pin, League: league, Events: r.events, Factories: r.leagueFactories, Pipeline: pipeline, Seeds: r.seeds, Cards: r.cards, Bouts: r.bouts, Results: r.results, CareerReader: r.careerReader, Publisher: publisher, WorkerConfig: worker, Life: r.life, Release: release, RegisterClose: register}, nil
}

func (r *PoolRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.ready = false
		r.cancel()
		borrows := make([]*poolRuntimeBorrow, 0, len(r.borrows))
		closers := make([]func() error, 0, len(r.borrows))
		for b := range r.borrows {
			borrows = append(borrows, b)
			if b.close != nil {
				closers = append(closers, b.close)
			}
		}
		r.mu.Unlock()
		var failures []error
		for _, closeChild := range closers {
			failures = append(failures, closeChild())
		}
		for _, b := range borrows {
			<-b.done
		}
		r.gate <- struct{}{}
		r.services.Wait()
		if r.results != nil {
			failures = append(failures, r.results.Close())
		}
		if r.bouts != nil {
			failures = append(failures, r.bouts.Close())
		}
		if r.cards != nil {
			failures = append(failures, r.cards.Close())
		}
		if r.seeds != nil {
			failures = append(failures, r.seeds.Close())
		}
		if r.events != nil {
			failures = append(failures, r.events.Close())
		}
		if r.settlement != nil {
			failures = append(failures, r.settlement.Close())
		}
		if r.rounds != nil {
			failures = append(failures, r.rounds.Close())
		}
		if r.pool != nil {
			failures = append(failures, r.pool.Close())
		}
		if r.issuer != nil {
			failures = append(failures, r.issuer.Close())
		}
		if r.backend != nil {
			r.backend.Close()
		}
		if r.node != nil {
			r.node.Close()
		}
		if r.nodes != nil {
			failures = append(failures, r.nodes.Close())
		}
		if r.lock != nil {
			failures = append(failures, r.lock.Close())
		}
		<-r.gate
		r.closeErr = errors.Join(failures...)
	})
	return r.closeErr
}
