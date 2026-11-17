// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ManifestController struct {
	eventBytes            int // guarded by mu; retained/reserved event family bytes
	entryBytes            int // guarded by mu; retained/reserved entry family bytes
	previewBytes          int // guarded by mu, included in the shared stable cache ceiling
	stable                *StableController
	stableSelected        bool
	stableProtected       bool
	closeOnce             sync.Once
	shutdownErr, closeErr error
	mu                    sync.Mutex
	resources             sync.Mutex
	config                Config
	account               *AccountConfig
	view                  ManifestAccountConfig
	manifest              manifestIdentity
	lock                  *leveldb.DB
	snapshot              ManifestSnapshot
	cancel                context.CancelFunc
	done                  chan struct{}
	closed                bool
	store                 *registry.ManifestStore
	box                   *prospectworker.ManifestOutbox
	wallet                *walletRuntime
	operation             string
	cache                 map[uint64]*manifestCached
	lazyCancel            context.CancelFunc
	lazyDone              chan struct{}
	receiptSlot           *ManifestReceipt
}

func (c *ManifestController) Start() error {
	return c.start(true)
}

func (c *ManifestController) start(mine bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("pilot is closed")
	}
	if c.cancel != nil {
		if c.operation == "mining" {
			return nil
		}
		return errors.New("pilot operation is in progress")
	}
	if c.manifest.Phase == "initializing" {
		return errors.New("pilot initialization is incomplete")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.operation = "mining"
	c.done = make(chan struct{})
	c.snapshot.State = "starting"
	c.snapshot.LastError = ""
	go c.run(ctx, c.done, c.snapshot.Attempts, mine)
	return nil
}
func (c *ManifestController) Stop() {
	c.mu.Lock()
	cancel, done, lazyCancel, lazyDone := c.cancel, c.done, c.lazyCancel, c.lazyDone
	if cancel != nil && c.operation == "mining" {
		c.snapshot.State = "stopping"
	}
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if lazyCancel != nil {
		lazyCancel()
	}
	if done != nil {
		<-done
	}
	if lazyDone != nil {
		<-lazyDone
	}
}
func (c *ManifestController) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		cancel, done := c.cancel, c.done
		lazyCancel, lazyDone := c.lazyCancel, c.lazyDone
		c.clearManifestCacheLocked("idle", "")
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if lazyCancel != nil {
			lazyCancel()
		}
		if lazyDone != nil {
			<-lazyDone
		}
		if done != nil {
			<-done
		}
		// The tracked operation and lazy/export work have drained before the
		// instance lock is released. No old pilot controller is constructed.
		c.resources.Lock()
		err := c.lock.Close()
		c.resources.Unlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.closeErr = c.shutdownErr
		if err != nil {
			c.closeErr = errors.Join(c.closeErr, errors.New("pilot instance lock close failed"))
		}
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

func (c *ManifestController) run(ctx context.Context, done chan struct{}, base uint64, mine bool) {
	failed := ""
	fresh := c.manifest.Phase == "new"
	defer func() {
		c.saveStatus(&failed)
		if fresh {
			var durable manifestIdentity
			if readManifestJSON(filepath.Join(c.config.DataDir, "SLUG-MANIFEST-PILOT.json"), 8192, &durable) != nil || durable.Phase != "ready" {
				c.resources.Lock()
				c.mu.Lock()
				c.manifest.Phase = "initializing"
				c.mu.Unlock()
				c.resources.Unlock()
			}
		}
		c.mu.Lock()
		c.cancel, c.operation = nil, ""
		c.snapshot.HashesPerSecond = 0
		c.snapshot.WalletState = "closed"
		if failed != "" {
			c.snapshot.State, c.snapshot.LastError = "failed", failed
		} else {
			c.snapshot.State = "stopped"
		}
		close(done)
		c.mu.Unlock()
	}()
	if fresh {
		if c.checkNewData() != nil {
			failed = "initialization_failed"
			return
		}
		m := c.manifest
		m.Phase = "initializing"
		if atomicJSON(filepath.Join(c.config.DataDir, "SLUG-MANIFEST-PILOT.json"), m) != nil {
			failed = "storage_failed"
			return
		}
		c.resources.Lock()
		c.mu.Lock()
		c.manifest = m
		c.mu.Unlock()
		c.resources.Unlock()
	} else if c.manifest.Phase != "ready" || checkManifestChains(c.config.DataDir, c.manifest) != nil {
		failed = "chain_data_missing"
		return
	}
	// First initialization cannot abandon a newly committed wallet on normal
	// Stop/Quit. Drain this bounded transaction to its durable ready identity;
	// actual failures leave the explicit incomplete phase for inspection.
	startupBase := ctx
	if fresh {
		startupBase = context.Background()
	}
	startup, cancelStartup := context.WithTimeout(startupBase, 30*time.Second)
	defer cancelStartup()
	nodes, err := c.openNodes(startup)
	if err != nil {
		if errors.Is(err, errPilotNodeClose) {
			failed = "node_shutdown_failed"
		} else if ctx.Err() == nil || fresh {
			failed = "node_start_failed"
		}
		return
	}
	defer func() {
		if nodes.Close() != nil {
			failed = "node_shutdown_failed"
			c.noteShutdown("pilot node shutdown failed")
		}
	}()
	wallet, err := c.openWallet(fresh)
	if err != nil {
		failed = "wallet_unavailable"
		return
	}
	c.resources.Lock()
	c.wallet = wallet
	c.resources.Unlock()
	defer func() {
		c.resources.Lock()
		c.wallet = nil
		c.resources.Unlock()
		if wallet.Close() != nil {
			failed = "wallet_shutdown_failed"
			c.noteShutdown("pilot wallet shutdown failed")
		}
	}()
	c.mu.Lock()
	c.snapshot.WalletState = "connecting"
	c.mu.Unlock()
	if wallet.Attach(startup, nodes.rpcConfig()) != nil {
		failed = "wallet_connection_failed"
		return
	}
	var expected *walletPayout
	if !fresh {
		expected = &c.manifest.Payout
	}
	payout, err := wallet.EnsurePayout(startup, expected)
	if err != nil {
		failed = "wallet_payout_failed"
		return
	}
	if waitWallet(startup, wallet, nodes.primary) != nil {
		failed = "wallet_connection_failed"
		return
	}
	identity := wallet.Identity()
	if !fresh && identity != c.manifest.Identity {
		failed = "wallet_identity_mismatch"
		return
	}
	if fresh {
		m := c.manifest
		m.Phase, m.Identity, m.Payout = "ready", identity, payout
		if !m.validReadyWallet() {
			failed = "wallet_identity_mismatch"
			return
		}
		// The in-memory identity is needed for store creation, but a failed
		// initialization remains durably marked initializing, never adopted.
		c.resources.Lock()
		c.mu.Lock()
		c.manifest = m
		c.mu.Unlock()
		c.resources.Unlock()
	}
	if err := c.openStores(fresh); err != nil {
		failed = "storage_failed"
		return
	}
	defer func() {
		if c.closeStores() != nil {
			failed = "storage_close_failed"
			c.noteShutdown("pilot storage close failed")
		}
	}()
	if fresh {
		if markManifestChains(c.config.DataDir, c.manifest) != nil || atomicJSON(filepath.Join(c.config.DataDir, "SLUG-MANIFEST-PILOT-READY.json"), c.manifest) != nil || atomicJSON(filepath.Join(c.config.DataDir, "SLUG-MANIFEST-PILOT.json"), c.manifest) != nil {
			failed = "storage_failed"
			return
		}
	}
	c.mu.Lock()
	c.snapshot.PayoutAddress, c.snapshot.GymID, c.snapshot.GymPublicKey = payout.Address, identity.GymID, identity.GymPublicKey
	c.mu.Unlock()
	if c.refresh(nil, nodes.primary, base) != nil {
		failed = "wallet_status_failed"
		return
	}
	if ctx.Err() != nil || !mine {
		return
	}
	cancelStartup()
	if c.stable != nil {
		if err := c.stable.openLive(c.store); err != nil {
			failed = "storage_failed"
			return
		}
		defer c.stable.closeLive()
	}
	node, err := prospectnode.NewClient(nodes.rpcConfig())
	if err != nil {
		failed = "node_unavailable"
		return
	}
	defer node.Close()
	serverConfig := prospectservice.ManifestServerConfig{Version: 3, Listen: strings.TrimPrefix(c.manifest.origin(), "http://"), AuthorityPin: c.manifest.AuthorityPin, MaxJobs: 1000, MaxRecords: 1000}
	server, err := prospectservice.NewManifestSessionServer(serverConfig, policy(), node, c.store, c.account.Authorizer)
	if err != nil {
		failed = "authority_unavailable"
		return
	}
	authorityCtx, cancelAuthority := context.WithCancel(context.Background())
	serverDone, ready := make(chan error, 1), make(chan string, 1)
	go func() { serverDone <- server.Run(authorityCtx, func(origin string) { ready <- origin }) }()
	serverFinished := false
	defer func() {
		cancelAuthority()
		if !serverFinished {
			<-serverDone
		}
	}()
	select {
	case origin := <-ready:
		if origin != c.manifest.origin() {
			failed = "authority_identity_mismatch"
			return
		}
	case <-serverDone:
		serverFinished = true
		failed = "authority_port_busy"
		return
	case <-ctx.Done():
		return
	}
	service := manifestSessionAuthority{origin: c.manifest.origin(), pin: c.manifest.AuthorityPin, trust: c.manifest.TransportTrustID, account: c.account}
	signer, err := wallet.Signer(ctx, policy())
	if err != nil {
		failed = "wallet_signer_unavailable"
		return
	}
	signer = c.accountSigner(signer)

	observer := c.observeManifest
	var cycle prospectworker.ManifestCycleHook
	if c.stable != nil {
		observer = c.stable.observeStable
		cycle = c.stable.cycle
	}
	worker, err := prospectworker.NewManifestWithHooks(c.manifest.workerConfig(), service, node, signer, c.box, observer, cycle)
	if err != nil {
		failed = "worker_unavailable"
		return
	}
	c.mu.Lock()
	if ctx.Err() == nil {
		c.snapshot.State = "running"
	}
	c.mu.Unlock()
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(workerCtx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-workerDone:
			if err != nil {
				failed = "worker_failed"
				if errors.Is(err, prospectworker.ErrManifestObserver) {
					failed = "observer_unavailable"
				}
			}
			if c.refresh(worker, nodes.primary, base) != nil {
				failed = "storage_failed"
				c.noteShutdown("pilot final evidence read failed")
			}
			return
		case <-ticker.C:
			if c.refresh(worker, nodes.primary, base) != nil {
				failed = "storage_failed"
				cancelWorker()
				<-workerDone
				return
			}
		case <-nodes.primary.done:
			failed = "node_exited"
			cancelWorker()
			<-workerDone
			return
		case <-nodes.peer.done:
			failed = "node_exited"
			cancelWorker()
			<-workerDone
			return
		case <-serverDone:
			serverFinished = true
			failed = "authority_exited"
			cancelWorker()
			<-workerDone
			return
		}
	}
}

func (c *ManifestController) noteShutdown(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdownErr = errors.Join(c.shutdownErr, errors.New(message))
}
func (c *ManifestController) saveStatus(failed *string) {
	// A failed first initialization must retain its original durable intent,
	// and a never-started pilot must stay empty/reopenable.
	var durable manifestIdentity
	if readManifestJSON(filepath.Join(c.config.DataDir, "SLUG-MANIFEST-PILOT.json"), 8192, &durable) != nil || durable.Phase != "ready" {
		return
	}
	s := c.Snapshot()
	status := savedStatus{Height: s.Height, Attempts: s.Attempts, WalletHeight: s.WalletHeight, SpendableAtoms: s.SpendableAtoms, ImmatureAtoms: s.ImmatureAtoms}
	if atomicJSON(filepath.Join(c.config.DataDir, "status.json"), status) != nil {
		*failed = "storage_failed"
		c.noteShutdown("pilot status save failed")
	}
}
