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
	"github.com/taytr-llc/slug/btcec/v2"
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
	manifest    manifest
	lock        *leveldb.DB
	snapshot    Snapshot
	cancel      context.CancelFunc
	done        chan struct{}
	closed      bool
	store       *registry.Store
	box         *prospectworker.Outbox
}

func New(config Config) (*Controller, error) {
	if wire.TestPoW {
		return nil, errors.New("pilot requires production PoW")
	}
	if !filepath.IsAbs(config.DataDir) || !filepath.IsAbs(config.NodeBinary) || strings.ContainsAny(config.DataDir+config.NodeBinary, "\r\n\t\x00") {
		return nil, errors.New("explicit absolute pilot paths are required")
	}
	config.DataDir = filepath.Clean(config.DataDir)
	config.NodeBinary = filepath.Clean(config.NodeBinary)
	info, e := os.Stat(config.NodeBinary)
	if e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("bundled node binary is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.NodeBinary, "--version")
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || strings.TrimSpace(output.String()) != "slugd version 0.27.0" {
		return nil, errors.New("normal SLUG node binary is required")
	}
	if e = os.Mkdir(config.DataDir, 0700); e != nil && !os.IsExist(e) {
		return nil, errors.New("pilot directory is unavailable")
	}
	info, e = os.Lstat(config.DataDir)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("pilot directory must be a real directory")
	}
	marker := filepath.Join(config.DataDir, "SLUG-PILOT.json")
	// The only pre-marker artifact we may adopt is our own instance lock.
	// Acquire that lock before choosing or writing the durable identity.
	if _, e = os.Lstat(marker); os.IsNotExist(e) {
		entries, err := os.ReadDir(config.DataDir)
		if err != nil {
			return nil, errors.New("pilot directory is unavailable")
		}
		for _, entry := range entries {
			if entry.Name() != "instance-lock" {
				return nil, errors.New("existing directory is not a pilot")
			}
		}
	}
	if plainChild(filepath.Join(config.DataDir, "instance-lock"), true) != nil {
		return nil, errors.New("invalid managed pilot directory")
	}
	lock, e := leveldb.OpenFile(filepath.Join(config.DataDir, "instance-lock"), nil)
	if e != nil {
		return nil, errors.New("pilot is already open or unavailable")
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	fresh := false
	var m manifest
	if _, e = os.Lstat(marker); os.IsNotExist(e) {
		entries, err := os.ReadDir(config.DataDir)
		if err != nil || len(entries) != 1 || entries[0].Name() != "instance-lock" {
			return nil, errors.New("existing directory is not a pilot")
		}
		p, err := unusedPort()
		if err != nil {
			return nil, errors.New("loopback port unavailable")
		}
		m = manifest{Version: 1, Mode: Mode, AuthorityPort: p}
		if atomicJSON(marker, m) != nil {
			return nil, errors.New("pilot initialization failed")
		}
		fresh = true
	} else if readJSON(marker, 1024, &m) != nil || m.Version != 1 || m.Mode != Mode || m.AuthorityPort == 0 {
		return nil, errors.New("invalid pilot identity")
	}
	for _, name := range []string{"registry", "outbox", "primary-chain", "peer-chain"} {
		if plainChild(filepath.Join(config.DataDir, name), true) != nil {
			return nil, errors.New("invalid managed pilot directory")
		}
	}
	if !fresh {
		var ready manifest
		if readJSON(filepath.Join(config.DataDir, "SLUG-PILOT-READY.json"), 1024, &ready) != nil || ready != m {
			return nil, errors.New("pilot initialization is incomplete")
		}
	}
	for _, name := range []string{"primary-chain", "peer-chain"} {
		path := filepath.Join(config.DataDir, name)
		if fresh {
			if os.Mkdir(path, 0700) != nil {
				return nil, errors.New("pilot chain initialization failed")
			}
		} else if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			return nil, errors.New("pilot chain data is missing")
		}
	}
	if checkInitializedChains(config.DataDir, m) != nil {
		return nil, errors.New("pilot chain data is unavailable")
	}
	c := &Controller{config: config, manifest: m, lock: lock, snapshot: Snapshot{State: "stopped", Mode: Mode, Prospects: []Prospect{}}}
	if !fresh {
		var status savedStatus
		statusPath := filepath.Join(config.DataDir, "status.json")
		if _, e := os.Lstat(statusPath); e == nil {
			if readJSON(statusPath, 1024, &status) != nil {
				return nil, errors.New("invalid pilot status")
			}
			c.snapshot.Height = status.Height
			c.snapshot.Attempts = status.Attempts
		}
	}
	// Both stores are initialized once and retained; never erase a partial or
	// corrupt directory. Failure preserves it for inspection rather than reset.
	if e = c.openStores(fresh); e != nil {
		return nil, e
	}
	if e = c.refresh(nil, nil, 0); e != nil {
		closeErr := c.closeStores()
		return nil, errors.Join(errors.New("pilot evidence is unavailable"), closeErr)
	}
	if e := c.closeStores(); e != nil {
		return nil, e
	}
	if fresh && atomicJSON(filepath.Join(config.DataDir, "SLUG-PILOT-READY.json"), m) != nil {
		return nil, errors.New("pilot initialization failed")
	}
	keepLock = true
	return c, nil
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
	if _, err := os.Lstat(path); os.IsNotExist(err) && initialize {
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
	if _, err := os.Lstat(path); os.IsNotExist(err) && initialize {
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("pilot is closed")
	}
	if c.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	c.snapshot.State = "starting"
	c.snapshot.LastError = ""
	go c.run(ctx, c.done, c.snapshot.Attempts)
	return nil
}
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.snapshot.State = "stopping"
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
		err := c.lock.Close()
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
func (c *Controller) run(ctx context.Context, done chan struct{}, base uint64) {
	failed := ""
	defer func() {
		s := c.Snapshot()
		if atomicJSON(filepath.Join(c.config.DataDir, "status.json"), savedStatus{s.Height, s.Attempts}) != nil {
			failed = "storage_failed"
			c.mu.Lock()
			c.shutdownErr = errors.Join(c.shutdownErr, errors.New("pilot status save failed"))
			c.mu.Unlock()
		}
		c.mu.Lock()
		c.cancel = nil
		c.snapshot.HashesPerSecond = 0
		if failed != "" {
			c.snapshot.State = "failed"
			c.snapshot.LastError = failed
		} else {
			c.snapshot.State = "stopped"
		}
		close(done)
		c.mu.Unlock()
	}()
	// Bind the exact persisted authority origin before enabling the worker.
	// A conflict fails rather than selecting a new outbox identity.
	if checkInitializedChains(c.config.DataDir, c.manifest) != nil {
		failed = "chain_data_missing"
		return
	}
	if e := c.openStores(false); e != nil {
		failed = "storage_failed"
		return
	}
	defer func() {
		if err := c.closeStores(); err != nil {
			failed = "storage_close_failed"
			c.mu.Lock()
			c.shutdownErr = errors.Join(c.shutdownErr, err)
			c.mu.Unlock()
		}
	}()
	ports := make([]uint16, 4)
	seen := map[uint16]bool{c.manifest.AuthorityPort: true}
	for i := range ports {
		for {
			p, e := unusedPort()
			if e != nil {
				failed = "loopback_unavailable"
				return
			}
			if !seen[p] {
				ports[i] = p
				seen[p] = true
				break
			}
		}
	}
	peer, e := startNode(c.config.NodeBinary, c.config.DataDir, "peer", ports[0], ports[1], ports[3], true)
	if e != nil {
		failed = "node_start_failed"
		return
	}
	stopNode := func(node *childNode) {
		if node.stop() != nil {
			failed = "node_shutdown_failed"
			c.mu.Lock()
			c.shutdownErr = errors.Join(c.shutdownErr, errors.New("pilot node shutdown failed"))
			c.mu.Unlock()
		}
	}
	defer stopNode(peer)
	primary, e := startNode(c.config.NodeBinary, c.config.DataDir, "primary", ports[2], 0, ports[1], false)
	if e != nil {
		failed = "node_start_failed"
		return
	}
	defer stopNode(primary)
	startup, cancelStartup := context.WithTimeout(ctx, 30*time.Second)
	e = waitNode(startup, primary, true)
	if e == nil {
		e = waitNode(startup, peer, false)
	}
	cancelStartup()
	if e != nil {
		if ctx.Err() == nil {
			failed = "node_unavailable"
		}
		return
	}
	if markInitializedChains(c.config.DataDir, c.manifest) != nil {
		failed = "storage_failed"
		return
	}
	node, e := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: primary.rpc.url, Username: primary.rpc.user, Password: primary.rpc.password})
	if e != nil {
		failed = "node_unavailable"
		return
	}
	defer node.Close()
	token, _ := hex.DecodeString(fixtureToken)
	digest := sha256.Sum256(token)
	server, e := prospectservice.NewServer(prospectservice.ServerConfig{Version: 2, Listen: strings.TrimPrefix(c.manifest.origin(), "http://"), Clients: []prospectservice.Credential{{PublicKey: fixturePublicKey, TokenSHA256: hex.EncodeToString(digest[:])}}, MaxJobs: 1000, MaxRecords: 1000}, policy(), node, c.store)
	if e != nil {
		failed = "authority_unavailable"
		return
	}
	authorityCtx, cancelAuthority := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	ready := make(chan string, 1)
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
	service, e := prospectservice.NewClient(prospectservice.ClientConfig{Version: 2, URL: c.manifest.origin(), Token: fixtureToken}, policy())
	if e != nil {
		failed = "authority_unavailable"
		return
	}
	defer service.Close()
	scalar := make([]byte, 32)
	scalar[31] = 1 // Public fixture only. Never a wallet.
	key, _ := btcec.PrivKeyFromBytes(scalar)
	signer := func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		if e := ctx.Err(); e != nil {
			return gymauth.Authorization{}, e
		}
		return gymauth.SignAdmission(initial, proof, key)
	}
	worker, e := prospectworker.New(c.manifest.workerConfig(), pacedAuthority{service}, node, signer, c.box)
	if e != nil {
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
		case e := <-workerDone:
			if e != nil {
				failed = "worker_failed"
			}
			if c.refresh(worker, primary, base) != nil {
				failed = "storage_failed"
				c.mu.Lock()
				c.shutdownErr = errors.Join(c.shutdownErr, errors.New("pilot final evidence read failed"))
				c.mu.Unlock()
			}
			return
		case <-ticker.C:
			if c.refresh(worker, primary, base) != nil {
				failed = "storage_failed"
				cancelWorker()
				<-workerDone
				return
			}
		case <-primary.done:
			failed = "node_exited"
			cancelWorker()
			<-workerDone
			return
		case <-peer.done:
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
