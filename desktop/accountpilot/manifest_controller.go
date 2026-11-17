// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
)

type manifestChild interface {
	AccountID() string
	Prepare(context.Context) error
	SignAccountChallenge(context.Context, gymaccount.EnrollmentChallenge) (gymaccount.Authorization, error)
	Snapshot() pilot.ManifestSnapshot
	Start() error
	Stop()
	Close() error
	LoadManifestProspects(context.Context) error
	RefreshManifestProspects(context.Context) error
	ManifestPortrait(context.Context, uint64) (pilot.ManifestPortrait, error)
	ManifestReceipt(context.Context, uint64) (pilot.ManifestReceipt, error)
	ExportManifestReceipt(context.Context, uint64, string, string, string) error
	ReleaseManifestReceipt(context.Context, string, string) error
	ExportRecoveryChecked(context.Context, string, func() error) error
	VerifyBackup(context.Context, string) error
}

// ManifestController preserves schema1 fixture login while attaching only a
// separately typed manifest child. Its immutable view lease never substitutes a
// later token, even when a callback finishes after a reset or failed login.
type ManifestController struct {
	eventOwner                        *eventAccountOwner                                                // new explicit route only; closed after child drain
	verify                            func(context.Context, string, bool) (gymaccount.Principal, error) // immutable fixture-only dependency
	now                               func() time.Time                                                  // nil preserves wall-clock behavior
	mu                                sync.Mutex
	miningMu                          sync.Mutex
	config                            pilot.Config
	store                             *gymaccount.Store
	lock                              *leveldb.DB
	child                             manifestChild
	fixture, state, lastError, viewID string
	closed, operation                 bool
	cancel                            context.CancelFunc
	done                              chan struct{}
	session, mining                   gymaccount.Session
	expiry                            *time.Timer
	expiryDone                        chan struct{}
	closeOnce                         sync.Once
	closeErr, shutdownErr             error
	// Private behavioral-test seam, never a runtime configuration knob.
	newChild    func(pilot.Config, pilot.ManifestAccountConfig) (manifestChild, error)
	checkpoint  func(string)
	attachChild func(context.Context, manifestChild) error
}

func NewManifest(config pilot.Config) (*ManifestController, error) {
	if wire.TestPoW {
		return nil, errors.New("candidate pilot requires production PoW")
	}
	if !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, errors.New("explicit node binary required")
	}
	store, lock, err := openManifestAccountStore(config.DataDir)
	if err != nil {
		return nil, err
	}
	return &ManifestController{config: config, store: store, lock: lock, state: "signed_out", newChild: func(config pilot.Config, account pilot.ManifestAccountConfig) (manifestChild, error) {
		return pilot.NewManifestAccount(config, account)
	}}, nil
}

func (c *ManifestController) begin(state string, fn func(context.Context) error) error {
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
func (c *ManifestController) LoginFixture(fixture string) error {
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

func (c *ManifestController) login(ctx context.Context, fixture string) (resultErr error) {
	principal, err := c.verifyPublicFixture(ctx, fixture, false)
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
	var child manifestChild
	defer func() {
		if resultErr == nil {
			return
		}
		c.mu.Lock()
		if c.viewID == viewID {
			c.viewID = ""
		}
		c.child = nil
		c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
		c.mu.Unlock()
		resultErr = errors.Join(resultErr, c.cleanupAccount(context.Background(), session.Token, child))
	}()
	dir := filepath.Join(c.config.DataDir, "gyms", session.Account.ID)
	if session.Account.Enrollment != nil && !realDirectory(dir) {
		return errors.New("established account wallet unavailable")
	}
	account := pilot.ManifestAccountConfig{
		Account: pilot.AccountConfig{Fixture: fixture, AccountID: session.Account.ID, AuthorityID: c.store.AuthorityID(), RequireExisting: session.Account.Enrollment != nil,
			Authorizer: miningAuthorizer{c.store, session.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, session.Account.ID, viewID) }},
		ViewID: viewID, Generation: session.Account.Generation,
	}
	account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return c.withView(ctx, session, viewID, child, fn)
	}
	created, err := c.newChild(pilot.Config{DataDir: dir, NodeBinary: c.config.NodeBinary}, account)
	if err != nil {
		return err
	}
	// Failed factories may return typed nils; cleanup owns only a successful child.
	child = created
	c.mu.Lock()
	c.child, c.fixture = child, fixture
	c.mu.Unlock()
	if c.checkpoint != nil {
		c.checkpoint("manifest-child-created")
	}
	if err = child.Prepare(ctx); err != nil {
		return err
	}
	if session.Account.Enrollment == nil || session.Account.RecoveryRequired {
		action := "enroll"
		if session.Account.RecoveryRequired {
			action = "recover"
		}
		challenge, err := c.store.BeginChallenge(ctx, session.Token, action)
		if err != nil {
			return err
		}
		var signature gymaccount.Authorization
		err = c.store.WithSession(ctx, session.Token, gymaccount.Enrollment, func(account gymaccount.Account, recheck func() error) error {
			if account.ID != challenge.AccountID {
				return gymaccount.ErrForbidden
			}
			var err error
			signature, err = child.SignAccountChallenge(ctx, challenge)
			if err != nil {
				return err
			}
			return recheck()
		})
		if err != nil {
			return err
		}
		if _, _, err = c.store.CompleteChallenge(ctx, session.Token, challenge.ChallengeID, signature); err != nil {
			return err
		}
	}
	// Retained proof verification is bounded and stopped. The parent remains
	// blank/attaching until the child's complete verified page is published.
	if c.attachChild != nil {
		err = c.attachChild(ctx, child)
	} else {
		err = child.LoadManifestProspects(ctx)
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
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
		c.expiry = time.AfterFunc(time.UnixMilli(session.ExpiresAtMS).Sub(c.clockNow()), func() { defer close(done); c.expireView(viewID, child) })
		return nil
	})
}

// No controller/cache lock is held on entry to the account lease. The parent
// epoch check is short, and child callbacks may then take their own cache lock.
func (c *ManifestController) withView(ctx context.Context, session gymaccount.Session, viewID string, child manifestChild, fn func(gymaccount.Account, func() error) error) error {
	return c.withCapability(ctx, session, viewID, child, gymaccount.View, fn)
}
func (c *ManifestController) withCapability(ctx context.Context, session gymaccount.Session, viewID string, child manifestChild, capability gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
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
			if (capability == gymaccount.RosterManage || capability == gymaccount.EntryManage) && (c.state != "signed_in" || c.operation) {
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
func (c *ManifestController) miningToken(ctx context.Context, accountID, viewID string) (string, error) {
	c.miningMu.Lock()
	defer c.miningMu.Unlock()
	c.mu.Lock()
	session, mining := c.session, c.mining
	valid := !c.closed && c.state == "signed_in" && c.viewID == viewID && session.Account.ID == accountID && session.Token != ""
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

func (c *ManifestController) expireView(viewID string, child manifestChild) {
	c.mu.Lock()
	if c.closed || c.viewID != viewID || c.child != child {
		c.mu.Unlock()
		return
	}
	c.viewID = ""
	c.state = "expired"
	c.mu.Unlock()
	child.Stop()
	c.noteCleanup(child.Close())
}
func (c *ManifestController) noteCleanup(err error) error {
	if err == nil {
		return nil
	}
	failure := errors.New("account cleanup failed")
	c.mu.Lock()
	c.shutdownErr = errors.Join(c.shutdownErr, failure)
	c.mu.Unlock()
	return failure
}
func (c *ManifestController) cleanupAccount(ctx context.Context, token string, child manifestChild) error {
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
func (c *ManifestController) detach(ctx context.Context) error {
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
func (c *ManifestController) Logout() error {
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
func (c *ManifestController) ResetFixtureAccount() error {
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
		principal, err := c.verifyPublicFixture(ctx, fixture, true)
		if err != nil {
			return err
		}
		if _, _, err = c.store.ApplyReset(ctx, principal); err != nil {
			return err
		}
		return c.login(ctx, fixture)
	})
}

func (c *ManifestController) active() (manifestChild, gymaccount.Session, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.child == nil || c.viewID == "" || c.session.Token == "" || c.child.AccountID() != c.session.Account.ID {
		return nil, gymaccount.Session{}, "", errors.New("account unavailable")
	}
	return c.child, c.session, c.viewID, nil
}
func (c *ManifestController) Snapshot() pilot.ManifestSnapshot {
	c.mu.Lock()
	child, session, viewID := c.child, c.session, c.viewID
	state, fixture, last, busy, closed := c.state, c.fixture, c.lastError, c.operation, c.closed
	c.mu.Unlock()
	blank := pilot.BlankManifestSnapshot()
	blank.Account = pilot.AccountSnapshot{State: state, Fixture: fixture, LastError: last}
	if closed || busy || state != "signed_in" || child == nil || viewID == "" {
		return blank
	}
	// Probe the distinct capability before final View capture, never nested.
	exportErr := c.store.WithSession(context.Background(), session.Token, gymaccount.RecoveryExport, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != session.Account.ID || a.Generation != session.Account.Generation {
			return gymaccount.ErrForbidden
		}
		return recheck()
	})
	result := blank
	err := c.withView(context.Background(), session, viewID, child, func(account gymaccount.Account, recheck func() error) error {
		candidate := child.Snapshot()
		if candidate.View == nil || candidate.View.ViewID != viewID || candidate.View.AccountID != account.ID || candidate.View.Generation != account.Generation || candidate.Mode != pilot.ManifestMode {
			return gymaccount.ErrForbidden
		}
		candidate.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: fixture, ID: account.ID, Generation: account.Generation, HoldUntilMS: account.HoldUntilMS, Held: errors.Is(exportErr, gymaccount.ErrHeld), CanExport: exportErr == nil}
		if err := recheck(); err != nil {
			return err
		}
		result = candidate
		return nil
	})
	if err != nil {
		blank.Account.State = "expired"
		child.Stop()
		return blank
	}
	return result
}
func (c *ManifestController) Start() error {
	child, session, viewID, err := c.active()
	if err != nil {
		return err
	}
	token, err := c.miningToken(context.Background(), session.Account.ID, viewID)
	if err != nil {
		return err
	}
	if c.checkpoint != nil {
		c.checkpoint("start-token")
	}
	return c.store.WithSession(context.Background(), token, gymaccount.Mining, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() || a.Generation != session.Account.Generation {
			return gymaccount.ErrForbidden
		}
		if err := recheck(); err != nil {
			return err
		}
		// Start only publishes an asynchronous child operation. Serialize this
		// short publication with view invalidation; never hold mu over a drain.
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || c.operation || c.state != "signed_in" || c.viewID != viewID || c.child != child || c.session.Token != session.Token {
			return gymaccount.ErrUnauthorized
		}
		return child.Start()
	})
}
func (c *ManifestController) Stop() {
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
func (c *ManifestController) RefreshManifestProspects(ctx context.Context) error {
	child, _, _, err := c.active()
	if err != nil {
		return err
	}
	return child.RefreshManifestProspects(ctx)
}
func (c *ManifestController) ManifestPortrait(ctx context.Context, seq uint64) (pilot.ManifestPortrait, error) {
	child, _, _, err := c.active()
	if err != nil {
		return pilot.ManifestPortrait{}, err
	}
	return child.ManifestPortrait(ctx, seq)
}
func (c *ManifestController) ManifestReceipt(ctx context.Context, seq uint64) (pilot.ManifestReceipt, error) {
	child, _, _, err := c.active()
	if err != nil {
		return pilot.ManifestReceipt{}, err
	}
	return child.ManifestReceipt(ctx, seq)
}
func (c *ManifestController) ExportManifestReceipt(ctx context.Context, seq uint64, viewID, prospectID, path string) error {
	if err := c.backupPath(path); err != nil {
		return err
	}
	child, _, current, err := c.active()
	if err != nil {
		return err
	}
	if current != viewID {
		return gymaccount.ErrUnauthorized
	}
	return child.ExportManifestReceipt(ctx, seq, viewID, prospectID, path)
}
func (c *ManifestController) ReleaseManifestReceipt(ctx context.Context, viewID, prospectID string) error {
	c.mu.Lock()
	child, current := c.child, c.viewID
	c.mu.Unlock()
	if child == nil || current != viewID {
		return nil
	}
	return child.ReleaseManifestReceipt(ctx, viewID, prospectID)
}
func (c *ManifestController) ExportRecovery(ctx context.Context, path string) error {
	if err := c.backupPath(path); err != nil {
		return err
	}
	child, session, viewID, err := c.active()
	if err != nil {
		return err
	}
	return c.store.WithSession(ctx, session.Token, gymaccount.RecoveryExport, func(a gymaccount.Account, recheck func() error) error {
		guard := func() error {
			if err := recheck(); err != nil {
				return err
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed || c.viewID != viewID || c.child != child || a.ID != session.Account.ID || a.Generation != session.Account.Generation {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if err := guard(); err != nil {
			return err
		}
		return child.ExportRecoveryChecked(ctx, path, guard)
	})
}
func (c *ManifestController) VerifyBackup(ctx context.Context, path string) error {
	if err := c.backupPath(path); err != nil {
		return err
	}
	child, session, viewID, err := c.active()
	if err != nil {
		return err
	}
	return c.withView(ctx, session, viewID, child, func(_ gymaccount.Account, recheck func() error) error {
		if err := recheck(); err != nil {
			return err
		}
		return child.VerifyBackup(ctx, path)
	})
}
func (c *ManifestController) Close() error {
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
		if c.store != nil {
			if c.eventOwner != nil && c.eventOwner.store != nil {
				err = errors.Join(err, c.eventOwner.store.Close())
				c.eventOwner.store = nil
			}
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

func (c *ManifestController) clockNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *ManifestController) verifyPublicFixture(ctx context.Context, fixture string, reset bool) (gymaccount.Principal, error) {
	if c.verify != nil {
		return c.verify(ctx, fixture, reset)
	}
	return verifyFixture(ctx, fixture, reset)
}
