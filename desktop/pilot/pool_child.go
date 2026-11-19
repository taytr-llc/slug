// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

type poolChildRun struct {
	cancel context.CancelFunc
	done   chan struct{}
	worker poolChildWorker
	base   uint64
}
type poolChildWorker interface {
	Run(context.Context) error
	Status() prospectworker.Status
}

// PoolChild owns one original account view. Shared chain and source resources
// are borrowed until every worker, proof lane and wallet operation has joined.
type PoolChild struct {
	stable                                     *StableController
	account                                    StableAccountConfig
	resources                                  poolChildResources
	wallet                                     *walletRuntime
	box                                        *prospectworker.PoolOutbox
	lock                                       *leveldb.DB
	authority                                  *PoolSessionAuthority
	admission                                  *poolChildAdmission
	ctx                                        context.Context
	cancel                                     context.CancelFunc
	refreshDone                                chan struct{}
	mu                                         sync.Mutex
	stopMu                                     sync.Mutex
	refreshMu                                  sync.Mutex
	closed                                     bool
	prepared, prepareAttempted, refreshStarted bool
	run                                        *poolChildRun
	backup                                     *poolBackupOperation
	backupCloseErr                             error
	backupExport                               func(context.Context) (recovery.Export, error)
	snapshot                                   PoolSnapshot
	closeOnce                                  sync.Once
	closeDone                                  chan struct{}
	closeErr                                   error
	newWorker                                  func(prospectworker.Signer) (poolChildWorker, error)
	signer                                     func(context.Context) (prospectworker.Signer, error)
}

func NewPoolChild(ctx context.Context, runtime *PoolRuntime, account StableAccountConfig) (*PoolChild, error) {
	if wire.TestPoW {
		return nil, gymstable.ErrProfile
	}
	if ctx == nil || ctx.Err() != nil || runtime == nil || account.Lease == nil {
		return nil, gymstable.ErrInvalid
	}
	r, err := runtime.childResources(account.Manifest.Account.AccountID, account.Manifest.Account.Fixture)
	if err != nil {
		return nil, err
	}
	if r.Pipeline == nil {
		r.Release()
		return nil, ErrManifestInitialization
	}
	c, err := newPoolChild(ctx, r, account)
	if err != nil {
		r.Release()
		return nil, err
	}
	if r.RegisterClose == nil {
		_ = c.Close()
		return nil, gymstable.ErrInvalid
	}
	if err = r.RegisterClose(c.Close); err != nil {
		_ = c.Close()
		return nil, err
	}
	go func() {
		select {
		case <-r.Life.Done():
			_ = c.Close()
		case <-c.closeDone:
		}
	}()
	return c, err
}

func newPoolChild(ctx context.Context, r poolChildResources, account StableAccountConfig) (_ *PoolChild, resultErr error) {
	league, err := validatePoolLeagueChild(r)
	if err != nil {
		return nil, err
	}
	r.League = league
	pipeline, err := validatePoolPipelineChild(r)
	if err != nil {
		return nil, err
	}
	r.Pipeline = pipeline
	a := account.Manifest
	f, err := gymaccount.FixtureIdentity(a.Account.Fixture)
	if err != nil || a.Account.AdmissionSigner != nil || r.Recipient.Config.AccountID != a.Account.AccountID || r.Recipient.Config.AccountAuthorityID != a.Account.AuthorityID || r.Recipient.Config.Fixture != a.Account.Fixture || r.Recipient.Config.Identity != f || r.StablePin.Version != gymstable.SharedPinVersion2 || r.StablePin.Source != r.Source.AuthorityPin || r.StablePin.Identity != f || r.StablePin.AccountID != a.Account.AccountID || r.StablePin.AccountAuthorityID != a.Account.AuthorityID || r.Issuer == nil || r.Node == nil || r.Life == nil || r.Life.Err() != nil || r.Release == nil {
		return nil, gymstable.ErrInvalid
	}
	wc, err := prospectworker.ValidatePoolConfig(r.WorkerConfig)
	if err != nil || wc.Source != r.Source || wc.PublicKey != f.GymPublicKey || wc.AuthorityOrigin != r.PoolURL || wc.TransportTrustID != r.PoolTrustID || wc.AdmissionOrigin != r.AdmissionURL || wc.AdmissionTrustID != r.AdmissionTrustID {
		return nil, gymstable.ErrInvalid
	}
	clientConfig := PoolAuthorityConfig{Version: 1, URL: r.PoolURL, PublicKey: f.GymPublicKey, TransportTrustID: r.PoolTrustID, Source: r.Source}
	authority, err := NewPoolSessionAuthority(clientConfig, a)
	if err != nil {
		return nil, err
	}
	admissionFence, err := NewPoolSessionAuthority(clientConfig, a)
	if err != nil {
		_ = authority.Close()
		return nil, err
	}
	life, cancel := context.WithCancel(context.Background())
	c := &PoolChild{account: account, resources: r, authority: authority, admission: &poolChildAdmission{fence: admissionFence, config: wc}, ctx: life, cancel: cancel, refreshDone: make(chan struct{}), closeDone: make(chan struct{}), snapshot: BlankPoolSnapshot()}
	return c, nil
}

// Prepare is called only after the parent has attached this exact child to its
// original-view lease. Construction itself cannot publish authenticated data.
func (c *PoolChild) Prepare(parent context.Context) error {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	c.mu.Lock()
	closed, prepared, attempted := c.closed, c.prepared, c.prepareAttempted
	c.prepareAttempted = true
	c.mu.Unlock()
	if closed {
		return gymstable.ErrClosed
	}
	if prepared {
		return nil
	}
	if attempted {
		return ErrManifestInitialization
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	r, account := c.resources, c.account
	a := account.Manifest
	f, _ := gymaccount.FixtureIdentity(a.Account.Fixture)
	wc := r.WorkerConfig
	if err := c.authority.checkView(ctx); err != nil {
		return err
	}
	token, err := a.Account.MiningToken(ctx)
	if err != nil || c.authority.checkToken(ctx, token) != nil {
		return poolAccessError(false)
	}
	lockPath := filepath.Join(r.Config.DataDir, "pool-child-lock")
	if err = plainChild(lockPath, true); err != nil {
		return err
	}
	c.lock, err = leveldb.OpenFile(lockPath, nil)
	if err != nil {
		return gymstable.ErrBusy
	}
	c.box, err = openPoolChildOutbox(r)
	if err != nil {
		return err
	}
	c.wallet, err = openAccountFixtureWallet(filepath.Join(r.Config.DataDir, "wallet"), a.Account.Fixture, false)
	if err != nil {
		return err
	}
	if err = c.wallet.Attach(ctx, r.RPC); err != nil {
		return err
	}
	if err = waitPoolChildWallet(ctx, c.wallet); err != nil {
		return err
	}
	binding, err := ensurePoolRecipientBinding(ctx, r.Config.DataDir, r.Recipient.Config, a, c.wallet, false)
	if err != nil || binding != r.Recipient {
		return ErrPoolRecipient
	}
	endpoint, err := url.Parse(r.AdmissionURL)
	if err != nil {
		return gymstable.ErrInvalid
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port == 0 {
		return gymstable.ErrInvalid
	}
	base := &ManifestController{config: r.Config, account: &a.Account, view: a, manifest: manifestIdentity{Phase: "ready", AccountID: a.Account.AccountID, AccountAuthorityID: a.Account.AuthorityID, Fixture: a.Account.Fixture, AuthorityPort: uint16(port), AuthorityPin: r.Source.AuthorityPin, TransportTrustID: r.AdmissionTrustID, Identity: walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}}, snapshot: BlankManifestSnapshot(), cache: make(map[uint64]*manifestCached)}
	pin := r.StablePin
	s := &StableController{ManifestController: base, stableConfig: account, laneGate: make(chan struct{}, 1), wake: make(chan struct{}, 1), stableState: "syncing", sharedPin: &pin, sharedSource: r.Issuer}
	base.stable = s
	c.stable = s
	if err = s.preparePoolLeague(r); err != nil {
		return err
	}
	if err = s.preparePoolPipeline(r); err != nil {
		return err
	}
	if err = s.openPair(r.Issuer); err != nil {
		return err
	}
	status, err := s.stableStore.SharedStatus(ctx, s.session())
	if err != nil {
		return err
	}
	s.stableStatus, s.installed, s.needSync = &status.Status, true, true
	if err = s.attachPoolLeague(ctx, r); err != nil {
		return err
	}
	if err = s.attachPoolPipeline(ctx, r); err != nil {
		return err
	}
	view := base.viewPin()
	c.newWorker = func(signer prospectworker.Signer) (poolChildWorker, error) {
		return prospectworker.NewPoolWithHooks(wc, c.authority, c.admission, r.Node, poolChildSigner(c.authority, signer), c.box, s.observeStable, s.cycle)
	}
	c.signer = func(ctx context.Context) (prospectworker.Signer, error) { return c.wallet.Signer(ctx, r.Source.Policy) }
	if c.authority.checkToken(ctx, token) != nil {
		return poolAccessError(false)
	}
	err = base.withView(ctx, func(check func() error) error {
		if err := check(); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.snapshot.View = &view
		c.snapshot.GymID, c.snapshot.GymPublicKey = f.GymID, f.GymPublicKey
		c.snapshot.PayoutAddress = r.Recipient.Recipient.Address
		c.snapshot.SourceBindingID = r.Recipient.SourceBindingID
		c.snapshot.AssignmentCount = status.AssignmentCount
		c.snapshot.WalletState = "ready"
		c.snapshot.BackupState = "idle"
		c.prepared, c.refreshStarted = true, true
		return nil
	})
	if err != nil {
		c.mu.Lock()
		c.refreshStarted = false
		c.prepared = false
		c.mu.Unlock()
		return err
	}
	s.startStoppedLoop()
	go c.refreshLoop()
	return nil
}

func waitPoolChildWallet(ctx context.Context, w *walletRuntime) error {
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		v, e := w.Snapshot(ctx)
		if e == nil && v.Synced {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type poolChildMarker struct {
	Version   uint32                    `json:"version"`
	Worker    prospectworker.PoolConfig `json:"worker"`
	Recipient PoolRecipientBinding      `json:"recipient"`
	Stable    gymstable.Pin             `json:"stable"`
}

func openPoolChildOutbox(r poolChildResources) (*prospectworker.PoolOutbox, error) {
	want := poolChildMarker{1, r.WorkerConfig, r.Recipient, r.StablePin}
	intent, ready, path := filepath.Join(r.Config.DataDir, "SLUG-POOL-CHILD.json"), filepath.Join(r.Config.DataDir, "SLUG-POOL-CHILD-READY.json"), filepath.Join(r.Config.DataDir, "pool-outbox")
	check := func(name string) error {
		var got poolChildMarker
		if readManifestJSON(name, 16384, &got) != nil || !reflect.DeepEqual(got, want) {
			return ErrManifestInitialization
		}
		return syncDirectory(filepath.Dir(name))
	}
	if _, e := os.Lstat(intent); os.IsNotExist(e) {
		if _, e = os.Lstat(path); !os.IsNotExist(e) {
			return nil, ErrManifestInitialization
		}
		raw, _ := json.Marshal(want)
		if e = atomicBytes(intent, append(raw, '\n')); e != nil {
			return nil, e
		}
	} else if e != nil || check(intent) != nil {
		return nil, ErrManifestInitialization
	}
	_, readyErr := os.Lstat(ready)
	if readyErr == nil && check(ready) != nil {
		return nil, ErrManifestInitialization
	}
	if readyErr != nil && !os.IsNotExist(readyErr) {
		return nil, readyErr
	}
	var box *prospectworker.PoolOutbox
	var err error
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		if readyErr == nil {
			return nil, ErrManifestInitialization
		}
		box, err = prospectworker.CreatePoolOutbox(path, r.WorkerConfig)
	} else if e != nil {
		return nil, e
	} else {
		box, err = prospectworker.OpenPoolOutbox(path, r.WorkerConfig)
	}
	if err != nil {
		return nil, err
	}
	// The child READY marker commits both storage components. An interrupted
	// first installation may resume; a ready child's missing stable is loss.
	stablePath := filepath.Join(r.Config.DataDir, "stable")
	var stable *gymstable.Store
	if _, e := os.Lstat(stablePath); os.IsNotExist(e) {
		if readyErr == nil || r.League != nil {
			_ = box.Close()
			return nil, ErrManifestInitialization
		}
		stable, err = gymstable.CreateShared(stablePath, r.StablePin, r.Issuer)
	} else if e != nil {
		err = e
	} else {
		if r.Pipeline != nil {
			if r.Factories == nil || r.Factories.openSharedCareer == nil {
				_ = box.Close()
				return nil, ErrManifestInitialization
			}
			stable, err = r.Factories.openSharedCareer(stablePath, r.StablePin, r.Issuer, r.Pipeline.Career)
		} else if r.Factories != nil {
			stable, err = r.Factories.openStable(stablePath, r.StablePin, r.Issuer)
		} else {
			stable, err = gymstable.OpenShared(stablePath, r.StablePin, r.Issuer)
		}
	}
	if err != nil {
		_ = box.Close()
		return nil, err
	}
	if err = stable.Close(); err != nil {
		_ = box.Close()
		return nil, err
	}
	if os.IsNotExist(readyErr) {
		raw, _ := json.Marshal(want)
		if err = atomicBytes(ready, append(raw, '\n')); err != nil {
			_ = box.Close()
			return nil, err
		}
	}
	return box, nil
}

func (c *PoolChild) AccountID() string { return c.account.Manifest.Account.AccountID }
func (c *PoolChild) Snapshot() PoolSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.snapshot
	if out.View != nil {
		v := *out.View
		out.View = &v
	}
	if out.Balance.Value != nil {
		v := *out.Balance.Value
		out.Balance.Value = &v
	}
	return out
}
func (c *PoolChild) Start() error {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	s, err := c.readyStable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	var run *poolChildRun
	err = s.withView(ctx, func(check func() error) error {
		if err := check(); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || !c.prepared {
			return gymstable.ErrClosed
		}
		if c.backup != nil || c.backupCloseErr != nil {
			return gymstable.ErrBusy
		}
		if c.run != nil {
			return nil
		}
		run = &poolChildRun{cancel: cancel, done: make(chan struct{}), base: c.snapshot.Attempts}
		c.run = run
		c.snapshot.State = "starting"
		c.snapshot.LastError = ""
		return nil
	})
	if err != nil || run == nil {
		cancel()
		if run != nil {
			c.mu.Lock()
			if c.run == run {
				c.run = nil
				c.snapshot.State = "stopped"
			}
			close(run.done)
			c.mu.Unlock()
		}
		return err
	}
	s.stableMu.Lock()
	s.paused = false
	s.needSync = true
	s.stableMu.Unlock()
	s.startStoppedLoop()
	s.signal()
	go c.runWorker(ctx, run)
	return nil
}
func (c *PoolChild) runWorker(ctx context.Context, run *poolChildRun) {
	var err error
	signer, e := c.signer(ctx)
	err = e
	var worker poolChildWorker
	if err == nil {
		worker, err = c.newWorker(signer)
	}
	if err == nil {
		c.mu.Lock()
		run.worker = worker
		c.mu.Unlock()
		err = worker.Run(ctx)
	}
	_ = c.publish(c.ctx, func() error {
		if c.run == run {
			if worker != nil {
				c.copyWorkerLocked(run, worker.Status())
			}
			if err != nil && ctx.Err() == nil {
				c.snapshot.State = "failed"
				c.snapshot.LastError = "worker_failed"
			} else {
				c.snapshot.State = "stopped"
			}
			c.snapshot.HashesPerSecond = 0
		}
		return nil
	})
	c.mu.Lock()
	if c.run == run {
		c.run = nil
	}
	close(run.done)
	c.mu.Unlock()
}
func (c *PoolChild) copyWorkerLocked(run *poolChildRun, s prospectworker.Status) {
	if s.Attempts > manifestMaxSafe-run.base || s.Pending > c.resources.WorkerConfig.MaxRecords || math.IsNaN(s.HashesPerSecond) || math.IsInf(s.HashesPerSecond, 0) {
		c.snapshot.LastError = "worker_failed"
		return
	}
	c.snapshot.Attempts = run.base + s.Attempts
	c.snapshot.Pending = s.Pending
	c.snapshot.HashesPerSecond = s.HashesPerSecond
	c.snapshot.LastError = s.LastError
	c.snapshot.RecoveryError = s.RecoveryError
	if c.snapshot.LastError == "not_found" {
		c.snapshot.LastError = "unknown_job"
	}
	if !PoolStatusCode(c.snapshot.LastError) {
		c.snapshot.LastError = "worker_failed"
	}
	if !PoolStatusCode(c.snapshot.RecoveryError) {
		c.snapshot.RecoveryError = "worker_failed"
	}
	if c.snapshot.State != "stopping" {
		c.snapshot.State = "running"
		if s.State == "failed" {
			c.snapshot.State = "failed"
		}
	}
}
func (c *PoolChild) Stop() { c.stopMu.Lock(); defer c.stopMu.Unlock(); c.stop() }
func (c *PoolChild) stop() {
	c.cancelPoolBackup()
	c.mu.Lock()
	run := c.run
	if run != nil {
		c.snapshot.State = "stopping"
		run.cancel()
	}
	c.mu.Unlock()
	s := c.stable
	if s != nil {
		s.stableMu.Lock()
		s.paused = true
		s.needSync = false
		s.cancelTasksLocked()
		s.stableMu.Unlock()
		s.stopStoppedLoop()
	}
	if run != nil {
		<-run.done
	}
	if s != nil {
		s.mu.Lock()
		cancel, done := s.lazyCancel, s.lazyDone
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
	}
}
func (c *PoolChild) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		c.stopMu.Lock()
		defer c.stopMu.Unlock()
		c.mu.Lock()
		c.closed = true
		c.snapshot = BlankPoolSnapshot()
		c.mu.Unlock()
		s := c.stable
		if s != nil {
			s.mu.Lock()
			s.closed = true
			s.clearManifestCacheLocked("idle", "")
			s.mu.Unlock()
			s.stableMu.Lock()
			s.stableClosed = true
			s.stableStatus = nil
			s.last = nil
			s.previewLast = nil
			s.entryLast, s.eventLast = nil, nil
			s.seedLast, s.cardLast, s.boutLast, s.resultLast = nil, nil, nil, nil
			s.clearCareerReadLocked()
			s.invalidateHistoryLocked()
			s.entryPending, s.eventFailure = "", ""
			s.stableMu.Unlock()
		}
		c.stop()
		if c.refreshStarted {
			<-c.refreshDone
		}
		c.mu.Lock()
		err := c.backupCloseErr
		c.mu.Unlock()
		if s != nil {
			s.laneGate <- struct{}{}
			err = errors.Join(err, s.closePair())
			<-s.laneGate
		}
		err = errors.Join(err, c.authority.Close(), c.admission.Close())
		if c.box != nil {
			err = errors.Join(err, c.box.Close())
		}
		if c.wallet != nil {
			err = errors.Join(err, c.wallet.Close())
		}
		if c.lock != nil {
			err = errors.Join(err, c.lock.Close())
		}
		c.resources.Release()
		c.mu.Lock()
		c.closeErr = err
		c.mu.Unlock()
		close(c.closeDone)
	})
	<-c.closeDone
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}
