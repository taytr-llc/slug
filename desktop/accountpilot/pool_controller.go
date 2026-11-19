// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"sync"
	"time"
)

type poolChild interface {
	pilot.PoolChildAPI
}

// PoolController owns one account view over a shared runtime. Invalidating a
// view never destroys the pool source or the other gym's retained history.
type PoolController struct {
	evidencePermit                    pilot.EvidencePermit
	mu, miningMu                      sync.Mutex
	config                            pilot.Config
	store                             *gymaccount.Store
	lock                              *leveldb.DB
	runtime                           *pilot.PoolRuntime
	sourceID                          string
	child                             poolChild
	fixture, state, lastError, viewID string
	closed, operation                 bool
	cancel                            context.CancelFunc
	done                              chan struct{}
	session, mining                   gymaccount.Session
	expiry                            *time.Timer
	expiryDone                        chan struct{}
	closeOnce                         sync.Once
	closeErr, shutdownErr             error
	provision                         func(context.Context, string, gymaccount.Session) (pilot.PoolRecipientBinding, error)
	newChild                          func(context.Context, *pilot.PoolRuntime, pilot.StableAccountConfig) (poolChild, error)
	checkpoint                        func(string)
}

func NewPool(ctx context.Context, config pilot.Config) (*PoolController, error) {
	return NewPoolWithOptions(ctx, config, pilot.PoolOptions{})
}

func NewPoolWithOptions(ctx context.Context, config pilot.Config, options pilot.PoolOptions) (*PoolController, error) {
	return newPoolWithOptions(ctx, config, options, func(ctx context.Context, config pilot.Config, accounts *gymaccount.Store) (*pilot.PoolRuntime, error) {
		return pilot.NewPoolRuntimeWithOptions(ctx, config, accounts, options)
	})
}

func newPool(ctx context.Context, config pilot.Config, newRuntime func(context.Context, pilot.Config, *gymaccount.Store) (*pilot.PoolRuntime, error)) (*PoolController, error) {
	return newPoolWithOptions(ctx, config, pilot.PoolOptions{}, newRuntime)
}
func newPoolWithOptions(ctx context.Context, config pilot.Config, options pilot.PoolOptions, newRuntime func(context.Context, pilot.Config, *gymaccount.Store) (*pilot.PoolRuntime, error)) (_ *PoolController, resultErr error) {
	options, optionErr := pilot.ResolvePoolOptions(options)
	if optionErr != nil {
		return nil, optionErr
	}
	if wire.TestPoW {
		return nil, errors.New("pool desktop requires production PoW")
	}
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit pool node required")
	}
	store, lock, err := openPoolAccountStoreWithOptions(config.DataDir, options)
	if err != nil {
		return nil, err
	}
	c := &PoolController{config: config, store: store, lock: lock, state: "signed_out"}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, c.Close())
		}
	}()
	c.runtime, err = newRuntime(ctx, pilot.Config{DataDir: filepath.Join(config.DataDir, "shared"), NodeBinary: config.NodeBinary}, store)
	if err != nil {
		return nil, err
	}
	c.provision = c.runtime.ProvisionFixture
	for _, fixture := range []string{"zero256", "incrementing256"} {
		principal, e := verifyFixture(ctx, fixture, false)
		if e != nil {
			return nil, e
		}
		login, e := store.Login(ctx, principal)
		if e != nil {
			return nil, e
		}
		_, setupErr := c.provision(ctx, fixture, login)
		revokeErr := store.Logout(context.Background(), login.Token)
		if e = errors.Join(setupErr, revokeErr); e != nil {
			return nil, e
		}
	}
	if err = c.runtime.StartServices(ctx); err != nil {
		return nil, err
	}
	c.sourceID, err = poolservice.BindingID(c.runtime.SourceConfig())
	if err != nil {
		return nil, err
	}
	c.newChild = func(ctx context.Context, r *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		return pilot.NewPoolChild(ctx, r, a)
	}
	return c, nil
}

func (c *PoolController) begin(state string, fn func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation {
		return errors.New("account operation unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	c.cancel, c.done, c.operation = cancel, make(chan struct{}), true
	c.state, c.lastError = state, ""
	if state == "signing_out" || state == "resetting" {
		c.viewID = "" // Revoke old callbacks before the asynchronous drain begins.
	}
	done := c.done
	go func() {
		err := fn(ctx)
		cancel()
		c.mu.Lock()
		c.cancel, c.operation = nil, false
		if err != nil {
			c.state, c.lastError = "failed", "account_unavailable"
		}
		close(done)
		c.mu.Unlock()
	}()
	return nil
}

func (c *PoolController) LoginFixture(fixture string) error {
	if _, err := gymaccount.FixtureIdentity(fixture); err != nil {
		return errors.New("unknown fixture account")
	}
	c.mu.Lock()
	allowed := c.child == nil
	c.mu.Unlock()
	if !allowed {
		return errors.New("sign out before choosing an account")
	}
	return c.begin("signing_in", func(ctx context.Context) error { return c.login(ctx, fixture) })
}

func (c *PoolController) login(ctx context.Context, fixture string) (resultErr error) {
	principal, err := verifyFixture(ctx, fixture, false)
	if err != nil {
		return err
	}
	session, err := c.store.Login(ctx, principal)
	if err != nil {
		return err
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		_ = c.store.Logout(context.Background(), session.Token)
		return err
	}
	viewID := hex.EncodeToString(nonce[:])
	c.mu.Lock()
	c.session, c.mining, c.viewID = session, gymaccount.Session{}, viewID
	c.mu.Unlock()
	var child poolChild
	defer func() {
		if resultErr != nil {
			c.mu.Lock()
			if c.viewID == viewID {
				c.viewID = ""
			}
			c.child = nil
			c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, c.cleanupAccount(context.Background(), session.Token, child))
		}
	}()
	if _, err = c.provision(ctx, fixture, session); err != nil {
		return err
	}
	token, err := c.store.MintMining(ctx, session.Token)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed || c.viewID != viewID {
		c.mu.Unlock()
		return gymaccount.ErrUnauthorized
	}
	c.mining = token
	c.mu.Unlock()
	a := pilot.ManifestAccountConfig{Account: pilot.AccountConfig{Fixture: fixture, AccountID: session.Account.ID, AuthorityID: c.store.AuthorityID(), RequireExisting: true, Authorizer: miningAuthorizer{c.store, session.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, session.Account.ID, viewID) }}, ViewID: viewID, Generation: session.Account.Generation}
	a.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return c.withView(ctx, session, viewID, child, fn)
	}
	stable := pilot.StableAccountConfig{EvidencePermit: &c.evidencePermit, Manifest: a, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return c.withCapability(ctx, session, viewID, child, cap, fn)
	}, HistoryPublicationLease: func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return c.withPoolHistoryPublicationLease(ctx, session, viewID, child, fn)
	}, ResultNodeLease: func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return c.withPoolResultNodeLease(ctx, session, viewID, child, fn)
	}}
	created, err := c.newChild(ctx, c.runtime, stable)
	if err != nil {
		return err
	}
	child = created
	c.mu.Lock()
	c.child, c.fixture = child, fixture
	c.mu.Unlock()
	if err = child.Prepare(ctx); err != nil {
		return err
	}
	return c.withView(ctx, session, viewID, child, func(_ gymaccount.Account, recheck func() error) error {
		if err := recheck(); err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || c.viewID != viewID || c.child != child {
			return gymaccount.ErrUnauthorized
		}
		c.state = "signed_in"
		c.expiryDone = make(chan struct{})
		done := c.expiryDone
		c.expiry = time.AfterFunc(time.Until(time.UnixMilli(session.ExpiresAtMS)), func() { defer close(done); c.expireView(viewID, child) })
		return nil
	})
}

func (c *PoolController) withView(ctx context.Context, session gymaccount.Session, viewID string, child poolChild, fn func(gymaccount.Account, func() error) error) error {
	return c.withCapability(ctx, session, viewID, child, gymaccount.View, fn)
}

func (c *PoolController) withCapability(ctx context.Context, session gymaccount.Session, viewID string, child poolChild, capability gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
	if fn == nil || child == nil {
		return gymaccount.ErrUnauthorized
	}
	return c.store.WithSession(ctx, session.Token, capability, func(account gymaccount.Account, recheck func() error) error {
		check := func() error {
			if err := recheck(); err != nil {
				return err
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed || c.viewID != viewID || c.child != child || c.session.Token != session.Token || account.ID != session.Account.ID || account.Generation != session.Account.Generation || child.AccountID() != account.ID || (c.state != "signed_in" && c.state != "signing_in" && c.state != "resetting") {
				return gymaccount.ErrUnauthorized
			}
			if (capability == gymaccount.RosterManage || capability == gymaccount.EntryManage || capability == gymaccount.SeedPublish || capability == gymaccount.ResultPublish) && (c.state != "signed_in" || c.operation) {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if err := check(); err != nil {
			return err
		}
		if err := fn(account, check); err != nil {
			return err
		}
		return check()
	})
}

func (c *PoolController) miningToken(ctx context.Context, accountID, viewID string) (string, error) {
	c.miningMu.Lock()
	defer c.miningMu.Unlock()
	c.mu.Lock()
	session, mining := c.session, c.mining
	valid := !c.closed && (c.state == "signed_in" || c.state == "signing_in" || c.state == "resetting") && c.viewID == viewID && session.Account.ID == accountID && session.Token != ""
	c.mu.Unlock()
	if !valid {
		return "", gymaccount.ErrUnauthorized
	}
	if mining.Token != "" && c.clockNow().UnixMilli()+30000 < mining.ExpiresAtMS {
		return mining.Token, nil
	}
	next, err := c.store.MintMining(ctx, session.Token)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.viewID != viewID || c.session.Token != session.Token {
		return "", gymaccount.ErrUnauthorized
	}
	c.mining = next
	return next.Token, nil
}

func (c *PoolController) expireView(viewID string, child poolChild) {
	c.mu.Lock()
	if c.closed || c.operation || c.viewID != viewID || c.child != child {
		c.mu.Unlock()
		return
	}
	token := c.session.Token
	c.viewID = ""
	c.child = nil
	c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
	c.state = "expired"
	c.operation = true
	done := make(chan struct{})
	c.done = done
	c.mu.Unlock()
	_ = c.cleanupAccount(context.Background(), token, child)
	c.mu.Lock()
	c.operation = false
	close(done)
	c.mu.Unlock()
}

func (c *PoolController) noteCleanup(err error) error {
	if err == nil {
		return nil
	}
	failure := errors.New("account cleanup failed")
	c.mu.Lock()
	c.shutdownErr = errors.Join(c.shutdownErr, failure)
	c.mu.Unlock()
	return failure
}

func (c *PoolController) cleanupAccount(ctx context.Context, token string, child poolChild) error {
	var err error
	if child != nil {
		child.Stop()
	}
	if token != "" {
		err = c.store.Logout(ctx, token)
	}
	if child != nil {
		err = errors.Join(err, child.Close())
	}
	return c.noteCleanup(err)
}

func (c *PoolController) detach(ctx context.Context) error {
	c.mu.Lock()
	child, token := c.child, c.session.Token
	c.child = nil
	c.viewID = ""
	c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
	var wait <-chan struct{}
	if c.expiry != nil && !c.expiry.Stop() {
		wait = c.expiryDone
	}
	c.expiry, c.expiryDone = nil, nil
	c.mu.Unlock()
	err := c.cleanupAccount(ctx, token, child)
	if wait != nil {
		<-wait
	}
	return err
}

func (c *PoolController) Logout() error {
	return c.begin("signing_out", func(ctx context.Context) error {
		if err := c.detach(ctx); err != nil {
			return err
		}
		c.mu.Lock()
		c.state, c.fixture = "signed_out", ""
		c.mu.Unlock()
		return nil
	})
}

func (c *PoolController) ResetFixtureAccount() error {
	c.mu.Lock()
	fixture, child := c.fixture, c.child
	c.mu.Unlock()
	if child == nil || fixture == "" {
		return errors.New("sign in to the fixture first")
	}
	return c.begin("resetting", func(ctx context.Context) error {
		// Invalidate first and drain without any account/controller lease. The
		// same retained wallet is reopened only after the durable reset.
		if err := c.detach(ctx); err != nil {
			return err
		}
		principal, err := verifyFixture(ctx, fixture, true)
		if err != nil {
			return err
		}
		if _, _, err = c.store.ApplyReset(ctx, principal); err != nil {
			return err
		}
		return c.login(ctx, fixture)
	})
}

func (c *PoolController) active() (poolChild, gymaccount.Session, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.child == nil || c.viewID == "" || c.session.Token == "" || c.child.AccountID() != c.session.Account.ID {
		return nil, gymaccount.Session{}, "", errors.New("account unavailable")
	}
	return c.child, c.session, c.viewID, nil
}

func (c *PoolController) Start() error {
	child, _, _, err := c.active()
	if err != nil {
		return err
	}
	// The child captures this immutable view and fences only asynchronous worker
	// publication. Never nest its authority calls or wait for a drain here.
	return child.Start()
}

func (c *PoolController) Stop() {
	c.mu.Lock()
	child, cancel := c.child, c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if child != nil {
		child.Stop()
	}
}

func (c *PoolController) Snapshot() pilot.PoolSnapshot {
	c.mu.Lock()
	child, session, viewID := c.child, c.session, c.viewID
	state, fixture, last, busy, closed := c.state, c.fixture, c.lastError, c.operation, c.closed
	c.mu.Unlock()
	blank := pilot.BlankPoolSnapshot()
	blank.Account = pilot.AccountSnapshot{State: state, Fixture: fixture, LastError: last}
	if closed || busy || state != "signed_in" || child == nil || viewID == "" {
		if closed {
			blank.Account.State = "signed_out"
		} else if state == "signed_in" {
			if busy {
				blank.Account.State = "signing_in"
			} else {
				blank.Account.State = "expired"
			}
		}
		return blank
	}
	result := blank
	capabilityCtx, capabilityCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	canExport := c.withCapability(capabilityCtx, session, viewID, child, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() }) == nil
	capabilityCancel()
	err := c.withView(context.Background(), session, viewID, child, func(a gymaccount.Account, check func() error) error {
		v := child.Snapshot()
		if v.Mode != pilot.PoolMode || v.View == nil || v.View.ViewID != viewID || v.View.AccountID != a.ID || v.View.AccountAuthorityID != c.store.AuthorityID() || v.View.Generation != a.Generation || v.SourceBindingID != c.sourceID {
			return gymaccount.ErrForbidden
		}
		held := a.HoldUntilMS > c.clockNow().UnixMilli()
		v.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: fixture, ID: a.ID, Generation: a.Generation, HoldUntilMS: a.HoldUntilMS, Held: held, CanExport: canExport && !held}
		if err := check(); err != nil {
			return err
		}
		if pilot.ValidatePoolSnapshot(v) != nil {
			return gymaccount.ErrForbidden
		}
		result = v
		return nil
	})
	if err != nil {
		blank.Account.State = "expired"
		go c.expireView(viewID, child)
		return blank
	}
	return result
}
func (c *PoolController) RefreshPoolBalance(ctx context.Context) error {
	child, _, _, e := c.active()
	if e != nil {
		return e
	}
	return child.RefreshPoolBalance(ctx)
}
func (c *PoolController) ManifestPortrait(ctx context.Context, n uint64) (pilot.ManifestPortrait, error) {
	child, _, _, e := c.active()
	if e != nil {
		return pilot.ManifestPortrait{}, e
	}
	return child.ManifestPortrait(ctx, n)
}
func (c *PoolController) ManifestReceipt(ctx context.Context, n uint64) (pilot.ManifestReceipt, error) {
	child, _, _, e := c.active()
	if e != nil {
		return pilot.ManifestReceipt{}, e
	}
	return child.ManifestReceipt(ctx, n)
}
func (c *PoolController) ExportManifestReceipt(ctx context.Context, n uint64, v, id, path string) error {
	if e := c.backupPath(path); e != nil {
		return e
	}
	child, _, view, e := c.active()
	if e != nil || v != view {
		return gymaccount.ErrUnauthorized
	}
	return child.ExportManifestReceipt(ctx, n, v, id, path)
}
func (c *PoolController) ReleaseManifestReceipt(ctx context.Context, v, id string) error {
	child, _, view, e := c.active()
	if e != nil || v != view {
		return gymaccount.ErrUnauthorized
	}
	return child.ReleaseManifestReceipt(ctx, v, id)
}
func (c *PoolController) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.viewID = ""
		cancel, done := c.cancel, c.done
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		c.Stop()
		if done != nil {
			<-done
		}
		err := c.detach(context.Background())
		if c.runtime != nil {
			err = errors.Join(err, c.runtime.Close())
		}
		if c.store != nil {
			err = errors.Join(err, c.store.Close())
		}
		if c.lock != nil {
			err = errors.Join(err, c.lock.Close())
		}
		c.mu.Lock()
		c.closeErr = errors.Join(c.shutdownErr, err)
		c.mu.Unlock()
	})
	return c.closeErr
}
func (c *PoolController) clockNow() time.Time { return time.Now() }
