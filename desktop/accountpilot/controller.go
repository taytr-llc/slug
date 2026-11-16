package accountpilot

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type miningAuthorizer struct {
	store     *gymaccount.Store
	accountID string
}

func (a miningAuthorizer) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return a.store.WithSession(ctx, token, gymaccount.Mining, func(account gymaccount.Account, recheck func() error) error {
		if account.Enrollment == nil || account.ID != a.accountID {
			return gymaccount.ErrForbidden
		}
		return fn(account.Enrollment.GymPublicKey, recheck)
	})
}

// Controller keeps credentials and recovery operations inside the helper.
// One asynchronous account operation is admitted at a time. Close cancels and
// drains that operation and the old gym before closing account storage.
type Controller struct {
	mu          sync.Mutex
	credentials sync.Mutex
	config      pilot.Config
	store       *gymaccount.Store
	child       *pilot.Controller
	fixture     string
	state       string
	lastError   string
	closed      bool
	operation   bool
	cancel      context.CancelFunc
	done        chan struct{}
	session     gymaccount.Session
	mining      gymaccount.Session
	expiry      *time.Timer
	closeOnce   sync.Once
	closeErr    error
	shutdownErr error
}

func New(config pilot.Config) (*Controller, error) {
	if wire.TestPoW {
		return nil, errors.New("account pilot requires production PoW")
	}
	if !filepath.IsAbs(config.NodeBinary) {
		return nil, errors.New("explicit node binary required")
	}
	store, err := openStore(config.DataDir)
	if err != nil {
		return nil, err
	}
	return &Controller{config: config, store: store, state: "signed_out"}, nil
}

func (c *Controller) begin(state string, fn func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation {
		return errors.New("account operation unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	c.cancel, c.done, c.operation = cancel, make(chan struct{}), true
	c.state, c.lastError = state, ""
	done := c.done
	go func() {
		err := fn(ctx)
		cancel()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.cancel, c.operation = nil, false
		if err != nil {
			c.state, c.lastError = "failed", "account_unavailable"
		}
		close(done)
	}()
	return nil
}

func (c *Controller) LoginFixture(fixture string) error {
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

func (c *Controller) login(ctx context.Context, fixture string) (resultErr error) {
	principal, err := verifyFixture(ctx, fixture, false)
	if err != nil {
		return err
	}
	session, err := c.store.Login(ctx, principal)
	if err != nil {
		return err
	}
	c.credentials.Lock()
	c.session, c.mining = session, gymaccount.Session{}
	c.credentials.Unlock()
	var child *pilot.Controller
	defer func() {
		if resultErr == nil {
			return
		}
		var closeChild func() error
		if child != nil {
			closeChild = child.Close
		}
		resultErr = errors.Join(resultErr, c.cleanupAccount(context.Background(), session.Token, closeChild))
		c.credentials.Lock()
		c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
		c.credentials.Unlock()
		c.mu.Lock()
		c.child = nil
		c.mu.Unlock()
	}()
	dir := filepath.Join(c.config.DataDir, "gyms", session.Account.ID)
	// An enrolled account may never silently recreate a missing wallet tree.
	if session.Account.Enrollment != nil && !realDirectory(dir) {
		return errors.New("established account wallet unavailable")
	}
	child, err = pilot.NewAccount(pilot.Config{DataDir: dir, NodeBinary: c.config.NodeBinary}, pilot.AccountConfig{
		Fixture: fixture, AccountID: session.Account.ID, AuthorityID: c.store.AuthorityID(),
		RequireExisting: session.Account.Enrollment != nil,
		Authorizer:      miningAuthorizer{c.store, session.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, session.Account.ID) },
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.child, c.fixture = child, fixture
	c.mu.Unlock()
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
			var signErr error
			signature, signErr = child.SignAccountChallenge(ctx, challenge)
			if signErr != nil {
				return signErr
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
	if err = ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.state = "signed_in"
	if c.expiry != nil {
		c.expiry.Stop()
	}
	c.expiry = time.AfterFunc(time.Until(time.UnixMilli(session.ExpiresAtMS)), child.Stop)
	c.mu.Unlock()
	return nil
}

func (c *Controller) miningToken(ctx context.Context, accountID string) (string, error) {
	c.credentials.Lock()
	defer c.credentials.Unlock()
	if c.session.Token == "" || c.session.Account.ID != accountID {
		return "", gymaccount.ErrUnauthorized
	}
	if c.mining.Token != "" && time.Now().UnixMilli()+30000 < c.mining.ExpiresAtMS {
		return c.mining.Token, nil
	}
	session, err := c.store.MintMining(ctx, c.session.Token)
	if err != nil {
		return "", err
	}
	c.mining = session
	return session.Token, nil
}

func (c *Controller) Logout() error {
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

func (c *Controller) detach(ctx context.Context) error {
	c.mu.Lock()
	child := c.child
	c.child = nil
	if c.expiry != nil {
		c.expiry.Stop()
		c.expiry = nil
	}
	c.mu.Unlock()
	if child != nil {
		child.Stop()
	}
	c.credentials.Lock()
	token := c.session.Token
	c.session, c.mining = gymaccount.Session{}, gymaccount.Session{}
	c.credentials.Unlock()
	var closeChild func() error
	if child != nil {
		closeChild = child.Close
	}
	return c.cleanupAccount(ctx, token, closeChild)
}

// Retain cleanup failures after dropping a child handle. Otherwise a failed
// login/logout can lose the only shutdown error and later Quit appear clean.
func (c *Controller) cleanupAccount(ctx context.Context, token string, closeChild func() error) error {
	var cleanupErr error
	if token != "" {
		cleanupErr = c.store.Logout(ctx, token)
	}
	if closeChild != nil {
		cleanupErr = errors.Join(cleanupErr, closeChild())
	}
	if cleanupErr == nil {
		return nil
	}
	failure := errors.New("account cleanup failed")
	c.mu.Lock()
	c.shutdownErr = errors.Join(c.shutdownErr, failure)
	c.mu.Unlock()
	return failure
}

func (c *Controller) ResetFixtureAccount() error {
	c.mu.Lock()
	fixture := c.fixture
	child := c.child
	c.mu.Unlock()
	if child == nil || fixture == "" {
		return errors.New("sign in to the fixture first")
	}
	return c.begin("resetting", func(ctx context.Context) error {
		// Cancel mining before changing authorization. The reset is durable even
		// if subsequent wallet reopening/proof fails; never replace that wallet.
		child.Stop()
		principal, err := verifyFixture(ctx, fixture, true)
		if err != nil {
			return err
		}
		if _, _, err = c.store.ApplyReset(ctx, principal); err != nil {
			return err
		}
		if err = c.detach(ctx); err != nil {
			return err
		}
		return c.login(ctx, fixture)
	})
}

func (c *Controller) Snapshot() pilot.Snapshot {
	c.mu.Lock()
	c.credentials.Lock()
	child, state, fixture, last, busy := c.child, c.state, c.fixture, c.lastError, c.operation
	token, accountID := c.session.Token, c.session.Account.ID
	c.credentials.Unlock()
	c.mu.Unlock()
	result := pilot.Snapshot{State: "stopped", Mode: pilot.AccountMode, WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}, Account: &pilot.AccountSnapshot{State: state, Fixture: fixture, LastError: last}}
	if busy || state != "signed_in" {
		return result
	}
	if token == "" || child == nil {
		return result
	}
	if accountID != child.AccountID() {
		result.Account.State = "expired"
		return result
	}
	var account gymaccount.Account
	err := c.store.WithSession(context.Background(), token, gymaccount.View, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		account = a
		return recheck()
	})
	if err != nil {
		child.Stop()
		result.Account.State = "expired"
		return result
	}
	status := result.Account
	result = child.Snapshot()
	result.Account = status
	status.ID, status.Generation, status.HoldUntilMS = account.ID, account.Generation, account.HoldUntilMS
	err = c.store.WithSession(context.Background(), token, gymaccount.RecoveryExport, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		return recheck()
	})
	status.CanExport = err == nil
	status.Held = errors.Is(err, gymaccount.ErrHeld)
	return result
}

func (c *Controller) active() (*pilot.Controller, string, error) {
	c.mu.Lock()
	c.credentials.Lock()
	child, busy, closed, state := c.child, c.operation, c.closed, c.state
	token, accountID := c.session.Token, c.session.Account.ID
	c.credentials.Unlock()
	c.mu.Unlock()
	if child == nil || busy || closed || state != "signed_in" || token == "" || accountID != child.AccountID() {
		return nil, "", errors.New("account unavailable")
	}
	return child, token, nil
}
func (c *Controller) Start() error {
	child, _, err := c.active()
	if err != nil {
		return err
	}
	token, err := c.miningToken(context.Background(), child.AccountID())
	if err != nil {
		return err
	}
	return c.store.WithSession(context.Background(), token, gymaccount.Mining, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		if err := recheck(); err != nil {
			return err
		}
		return child.Start()
	})
}
func (c *Controller) Stop() {
	c.mu.Lock()
	child := c.child
	c.mu.Unlock()
	if child != nil {
		child.Stop()
	}
}
func (c *Controller) Receipt(sequence uint64) (record registry.Record, err error) {
	child, token, err := c.active()
	if err != nil {
		return record, err
	}
	err = c.store.WithSession(context.Background(), token, gymaccount.View, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		var e error
		record, e = child.Receipt(sequence)
		if e != nil {
			return e
		}
		return recheck()
	})
	if err != nil {
		return registry.Record{}, err
	}
	return record, nil
}
func (c *Controller) ExportRecovery(path string) error {
	if err := c.backupPath(path); err != nil {
		return err
	}
	child, token, err := c.active()
	if err != nil {
		return err
	}
	return c.store.WithSession(context.Background(), token, gymaccount.RecoveryExport, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		return child.ExportRecoveryChecked(path, recheck)
	})
}
func (c *Controller) VerifyBackup(path string) error {
	if err := c.backupPath(path); err != nil {
		return err
	}
	child, token, err := c.active()
	if err != nil {
		return err
	}
	return c.store.WithSession(context.Background(), token, gymaccount.View, func(a gymaccount.Account, recheck func() error) error {
		if a.ID != child.AccountID() {
			return gymaccount.ErrForbidden
		}
		if err := recheck(); err != nil {
			return err
		}
		return child.VerifyBackup(path)
	})
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
		c.Stop()
		if done != nil {
			<-done
		}
		err := errors.Join(c.detach(context.Background()), c.store.Close())
		c.mu.Lock()
		c.closeErr = errors.Join(c.shutdownErr, err)
		c.mu.Unlock()
	})
	return c.closeErr
}
