package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type Controller struct {
	closeOnce   sync.Once
	shutdownErr error
	closeErr    error
	mu          sync.Mutex
	resources   sync.Mutex
	config      Config
	account     *AccountConfig
	manifest    manifest
	lock        *leveldb.DB
	snapshot    Snapshot
	cancel      context.CancelFunc
	done        chan struct{}
	closed      bool
	store       *registry.Store
	box         *prospectworker.Outbox
	wallet      *walletRuntime
	operation   string
}

func New(config Config) (*Controller, error) {
	return newController(config, nil)
}

func newController(config Config, account *AccountConfig) (*Controller, error) {
	if wire.TestPoW {
		return nil, errors.New("pilot requires production PoW")
	}
	if !filepath.IsAbs(config.DataDir) || !filepath.IsAbs(config.NodeBinary) || strings.ContainsAny(config.DataDir+config.NodeBinary, "\r\n\t\x00") {
		return nil, errors.New("explicit absolute pilot paths are required")
	}
	config.DataDir = filepath.Clean(config.DataDir)
	config.NodeBinary = filepath.Clean(config.NodeBinary)
	marker := filepath.Join(config.DataDir, "SLUG-PILOT.json")
	// Reject old identities before even opening their instance-lock database.
	// Network/genesis alone cannot make old cached chains or receipts v2 data.
	var before manifest
	if _, err := os.Lstat(marker); err == nil {
		if readJSON(marker, 8192, &before) != nil || !before.valid() || !matchesAccount(before, account) || before.Phase == "initializing" {
			return nil, errors.New("unsupported or incomplete pilot identity; retained data was not changed")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("pilot identity unavailable")
	}
	if account != nil && account.RequireExisting && before.Phase != "ready" {
		return nil, errors.New("established account wallet is unavailable")
	}
	info, err := os.Stat(config.NodeBinary)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("bundled node binary is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.NodeBinary, "--version")
	var output limitedBuffer
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if cmd.Run() != nil || strings.TrimSpace(output.String()) != "slugd version 0.27.0" {
		return nil, errors.New("normal SLUG node binary is required")
	}
	if err := os.Mkdir(config.DataDir, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.New("pilot directory unavailable")
	}
	if info, err := os.Lstat(config.DataDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("pilot directory must be a real directory")
	}
	if _, err := os.Lstat(marker); os.IsNotExist(err) {
		entries, err := os.ReadDir(config.DataDir)
		if err != nil {
			return nil, errors.New("pilot directory unavailable")
		}
		for _, entry := range entries {
			if entry.Name() != "instance-lock" {
				return nil, errors.New("existing directory is not a wallet pilot")
			}
		}
	}
	if plainChild(filepath.Join(config.DataDir, "instance-lock"), true) != nil {
		return nil, errors.New("invalid managed pilot directory")
	}
	lock, err := leveldb.OpenFile(filepath.Join(config.DataDir, "instance-lock"), nil)
	if err != nil {
		return nil, errors.New("pilot is already open or unavailable")
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	var m manifest
	if _, err := os.Lstat(marker); os.IsNotExist(err) {
		entries, err := os.ReadDir(config.DataDir)
		if err != nil || len(entries) != 1 || entries[0].Name() != "instance-lock" {
			return nil, errors.New("existing directory is not a wallet pilot")
		}
		port, err := unusedPort()
		if err != nil {
			return nil, errors.New("loopback port unavailable")
		}
		m = manifest{Version: 2, Mode: Mode, AuthorityPort: port, Phase: "new", Network: policy().Network, Genesis: genesis, ActivationHeight: 62, RecoveryProfile: "slug-phase-a-fixture-bip39-v1"}
		if account != nil {
			m.Version, m.Mode, m.AccountID, m.Fixture = 3, AccountMode, account.AccountID, account.Fixture
		}
		if atomicJSON(marker, m) != nil {
			return nil, errors.New("pilot initialization failed")
		}
		for _, name := range []string{"primary-chain", "peer-chain"} {
			if os.Mkdir(filepath.Join(config.DataDir, name), 0700) != nil {
				return nil, errors.New("pilot initialization failed")
			}
		}
	} else if readJSON(marker, 8192, &m) != nil || !m.valid() || !matchesAccount(m, account) || m.Phase == "initializing" {
		return nil, errors.New("unsupported or incomplete pilot identity")
	}
	for _, name := range []string{"wallet", "registry", "outbox", "primary-chain", "peer-chain"} {
		if plainChild(filepath.Join(config.DataDir, name), true) != nil {
			return nil, errors.New("invalid managed pilot directory")
		}
	}
	c := &Controller{config: config, account: account, manifest: m, lock: lock, snapshot: Snapshot{State: "stopped", Mode: m.Mode, WalletState: "closed", BackupState: "idle", Prospects: []Prospect{}}}
	if m.Phase == "new" {
		if err := c.checkNewData(); err != nil {
			return nil, err
		}
	} else {
		var ready manifest
		if readJSON(filepath.Join(config.DataDir, "SLUG-PILOT-READY.json"), 8192, &ready) != nil || ready != m || checkInitializedChains(config.DataDir, m) != nil {
			return nil, errors.New("pilot initialized data is unavailable")
		}
		wallet, err := c.openWallet(false)
		if err != nil {
			return nil, errors.New("pilot wallet unavailable")
		}
		identity := wallet.Identity()
		closeErr := wallet.Close()
		if identity != m.Identity || closeErr != nil {
			return nil, errors.New("pilot wallet identity mismatch")
		}
		c.snapshot.GymID, c.snapshot.GymPublicKey, c.snapshot.PayoutAddress = m.Identity.GymID, m.Identity.GymPublicKey, m.Payout.Address
		var status savedStatus
		if _, err := os.Lstat(filepath.Join(config.DataDir, "status.json")); err == nil {
			if readJSON(filepath.Join(config.DataDir, "status.json"), 2048, &status) != nil {
				return nil, errors.New("invalid pilot status")
			}
			c.snapshot.Height, c.snapshot.Attempts = status.Height, status.Attempts
			c.snapshot.WalletHeight, c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = status.WalletHeight, status.SpendableAtoms, status.ImmatureAtoms
		}
		if err := c.openStores(false); err != nil {
			return nil, err
		}
		refreshErr := c.refresh(nil, nil, 0)
		closeErr = c.closeStores()
		if refreshErr != nil || closeErr != nil {
			return nil, errors.New("pilot evidence is unavailable")
		}
	}
	keepLock = true
	return c, nil
}

// A never-started v2 root is reopenable, but it cannot adopt any existing chain,
// wallet or receipt data. First Start owns their one-time initialization.
func (c *Controller) checkNewData() error {
	entries, err := os.ReadDir(c.config.DataDir)
	if err != nil {
		return errors.New("new pilot directory unavailable")
	}
	allowed := map[string]bool{"instance-lock": true, "SLUG-PILOT.json": true, "primary-chain": true, "peer-chain": true}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return errors.New("new pilot contains unexpected retained data")
		}
	}
	for _, name := range []string{"primary-chain", "peer-chain"} {
		path := filepath.Join(c.config.DataDir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("new pilot chain directory unavailable")
		}
		contents, err := os.ReadDir(path)
		if err != nil || len(contents) != 0 {
			return errors.New("preexisting chain cannot be adopted")
		}
	}
	return nil
}

type limitedBuffer struct{ data []byte }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4096 {
		return 0, errors.New("oversized node version")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *limitedBuffer) String() string { return string(b.data) }
func (c *Controller) openStores(initialize bool) error {
	c.resources.Lock()
	defer c.resources.Unlock()
	var e error
	path := filepath.Join(c.config.DataDir, "registry")
	if initialize {
		c.store, e = registry.CreateMining(path, policy().Network, genesis)
	} else {
		c.store, e = registry.Open(path, false)
	}
	if e != nil {
		return errors.New("pilot registry unavailable")
	}
	info, e := c.store.Info()
	if e != nil || info.Version != registry.Version4 || info.Network != policy().Network || info.Genesis != genesis {
		c.store.Close()
		c.store = nil
		return errors.New("pilot registry identity mismatch")
	}
	path = filepath.Join(c.config.DataDir, "outbox")
	if initialize {
		c.box, e = prospectworker.CreateOutbox(path, c.manifest.workerConfig())
	} else {
		c.box, e = prospectworker.OpenOutbox(path, c.manifest.workerConfig())
	}
	if e != nil {
		c.store.Close()
		c.store = nil
		return errors.New("pilot outbox unavailable")
	}
	return nil
}
func (c *Controller) closeStores() error {
	c.resources.Lock()
	defer c.resources.Unlock()
	var err error
	if c.box != nil {
		err = errors.Join(err, c.box.Close())
		c.box = nil
	}
	if c.store != nil {
		err = errors.Join(err, c.store.Close())
		c.store = nil
	}
	if err != nil {
		return errors.New("pilot storage close failed")
	}
	return nil
}
func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.snapshot
	s.Prospects = append([]Prospect{}, s.Prospects...)
	return s
}
func (c *Controller) Start() error {
	return c.start(true)
}

func (c *Controller) start(mine bool) error {
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
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		if c.operation == "mining" {
			c.snapshot.State = "stopping"
		}
		c.cancel()
	}
}
func (c *Controller) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		cancel, done := c.cancel, c.done
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
		// A stopped receipt export may own a temporary outbox handle. Drain it
		// before releasing the application instance lock as well.
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

// Receipt exports the original authority receipt retained in the outbox. It
// neither rehashes work nor makes a new claim about current chain membership.
func (c *Controller) Receipt(sequence uint64) (record registry.Record, resultErr error) {
	if sequence == 0 {
		return registry.Record{}, errors.New("receipt not found")
	}
	c.resources.Lock()
	defer c.resources.Unlock()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return registry.Record{}, errors.New("pilot is closed")
	}
	if c.manifest.Phase != "ready" {
		return registry.Record{}, errors.New("receipt not found")
	}
	box := c.box
	if box == nil {
		var e error
		box, e = prospectworker.OpenOutbox(filepath.Join(c.config.DataDir, "outbox"), c.manifest.workerConfig())
		if e != nil {
			return registry.Record{}, errors.New("receipt unavailable")
		}
		defer func() {
			if box.Close() != nil {
				resultErr = errors.Join(resultErr, errors.New("receipt storage close failed"))
			}
		}()
	}
	var after uint64
	for {
		entries, e := box.List(after, 100)
		if e != nil {
			return registry.Record{}, errors.New("receipt unavailable")
		}
		if len(entries) == 0 {
			return registry.Record{}, errors.New("receipt not found")
		}
		for _, entry := range entries {
			after = entry.Sequence
			if entry.Receipt != nil && entry.Receipt.Record.Sequence == sequence {
				return entry.Receipt.Record, nil
			}
		}
	}
}
func (c *Controller) refresh(worker *prospectworker.Worker, node *childNode, base uint64) error {
	c.resources.Lock()
	defer c.resources.Unlock()
	if c.box == nil {
		return nil
	}
	prospects := make([]Prospect, 0)
	var after, count, pending uint64
	for {
		entries, e := c.box.List(after, 100)
		if e != nil {
			return e
		}
		if len(entries) == 0 {
			break
		}
		for _, entry := range entries {
			after = entry.Sequence
			if (entry.BlockState != "not-applicable" && entry.BlockState != "canonical" && entry.BlockState != "rejected") || (entry.AdmissionState != "not-applicable" && entry.AdmissionState != "accepted" && entry.AdmissionState != "expired" && entry.AdmissionState != "rejected") {
				pending++
			}
			if entry.Receipt == nil {
				continue
			}
			r := entry.Receipt.Record
			count++
			prospects = append(prospects, Prospect{r.Sequence, r.Verified.ProspectID, r.Verified.WorkHash, r.Birth.BornAtMS, r.Verified.DNA.Rarity, r.Verified.DNA.TalentBudget, r.Verified.DNA.Attributes})
		}
	}
	sort.Slice(prospects, func(i, j int) bool { return prospects[i].Sequence > prospects[j].Sequence })
	if len(prospects) > 100 {
		prospects = prospects[:100]
	}
	var status prospectworker.Status
	if worker != nil {
		status = worker.Status()
	}
	var height uint64
	validHeight := false
	if node != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		validHeight = node.rpc.call(ctx, "getblockcount", &height) == nil
		cancel()
	}
	var view walletView
	validWallet := false
	if c.wallet != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		view, err = c.wallet.Snapshot(ctx)
		validWallet = err == nil && view.Height >= 0 && view.Spendable >= 0 && view.Immature >= 0
		cancel()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot.Prospects = prospects
	c.snapshot.ProspectCount = count
	c.snapshot.Pending = pending
	if worker != nil {
		c.snapshot.Attempts = base + status.Attempts
		c.snapshot.HashesPerSecond = status.HashesPerSecond
		c.snapshot.Pending = status.Pending
		c.snapshot.RecoveryError = status.RecoveryError
		c.snapshot.LastError = status.LastError
	}
	if validHeight {
		c.snapshot.Height = height
	}
	if validWallet {
		c.snapshot.WalletHeight = uint64(view.Height)
		c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = view.Spendable, view.Immature
		c.snapshot.WalletState = "syncing"
		if view.Synced {
			c.snapshot.WalletState = "ready"
		}
	} else if c.wallet != nil {
		// Retain the last coherent amounts but never advertise current sync
		// after a disconnected or changing RPC observation.
		c.snapshot.WalletState = "syncing"
	}
	return nil
}

type pacedAuthority struct{ *prospectservice.Client }

func (a pacedAuthority) Submit(ctx context.Context, j prospectjob.Job, p prospect.Proof, s gymauth.Authorization) (prospectservice.AdmissionResult, error) {
	result, e := a.Client.Submit(ctx, j, p, s)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	// Preserve the original successful ACK even when Stop canceled the pause.
	return result, e
}
func (c *Controller) run(ctx context.Context, done chan struct{}, base uint64, mine bool) {
	failed := ""
	fresh := c.manifest.Phase == "new"
	defer func() {
		c.saveStatus(&failed)
		if fresh {
			var durable manifest
			if readJSON(filepath.Join(c.config.DataDir, "SLUG-PILOT.json"), 8192, &durable) != nil || durable.Phase != "ready" {
				c.resources.Lock()
				c.mu.Lock()
				c.manifest.Phase, c.manifest.Identity, c.manifest.Payout = "initializing", walletIdentity{}, walletPayout{}
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
		if atomicJSON(filepath.Join(c.config.DataDir, "SLUG-PILOT.json"), m) != nil {
			failed = "storage_failed"
			return
		}
		c.resources.Lock()
		c.mu.Lock()
		c.manifest = m
		c.mu.Unlock()
		c.resources.Unlock()
	} else if c.manifest.Phase != "ready" || checkInitializedChains(c.config.DataDir, c.manifest) != nil {
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
		if !m.valid() {
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
		if markInitializedChains(c.config.DataDir, c.manifest) != nil || atomicJSON(filepath.Join(c.config.DataDir, "SLUG-PILOT-READY.json"), c.manifest) != nil || atomicJSON(filepath.Join(c.config.DataDir, "SLUG-PILOT.json"), c.manifest) != nil {
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
	node, err := prospectnode.NewClient(nodes.rpcConfig())
	if err != nil {
		failed = "node_unavailable"
		return
	}
	defer node.Close()
	token, _ := hex.DecodeString(fixtureToken)
	digest := sha256.Sum256(token)
	serverConfig := prospectservice.ServerConfig{Version: 2, Listen: strings.TrimPrefix(c.manifest.origin(), "http://"), Clients: []prospectservice.Credential{{PublicKey: identity.GymPublicKey, TokenSHA256: hex.EncodeToString(digest[:])}}, MaxJobs: 1000, MaxRecords: 1000}
	var server *prospectservice.Server
	if c.account == nil {
		server, err = prospectservice.NewServer(serverConfig, policy(), node, c.store)
	} else {
		serverConfig.Clients = nil
		server, err = prospectservice.NewSessionServer(serverConfig, policy(), node, c.store, c.account.Authorizer)
	}
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
	service, closeService, err := c.miningService()
	if err != nil {
		failed = "authority_unavailable"
		return
	}
	defer closeService()
	signer, err := wallet.Signer(ctx, policy())
	if err != nil {
		failed = "wallet_signer_unavailable"
		return
	}
	if c.account != nil {
		signer = c.accountSigner(signer)
	}
	worker, err := prospectworker.New(c.manifest.workerConfig(), service, node, signer, c.box)
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

func (c *Controller) noteShutdown(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdownErr = errors.Join(c.shutdownErr, errors.New(message))
}
func (c *Controller) saveStatus(failed *string) {
	// A failed first initialization must retain its original durable intent,
	// and a never-started pilot must stay empty/reopenable.
	var durable manifest
	if readJSON(filepath.Join(c.config.DataDir, "SLUG-PILOT.json"), 8192, &durable) != nil || durable.Phase != "ready" {
		return
	}
	s := c.Snapshot()
	status := savedStatus{Height: s.Height, Attempts: s.Attempts, WalletHeight: s.WalletHeight, SpendableAtoms: s.SpendableAtoms, ImmatureAtoms: s.ImmatureAtoms}
	if atomicJSON(filepath.Join(c.config.DataDir, "status.json"), status) != nil {
		*failed = "storage_failed"
		c.noteShutdown("pilot status save failed")
	}
}
