package pilot

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slugwallet/recovery"
)

func validAccountID(id string) bool {
	if len(id) != 64 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func matchesAccount(m manifest, account *AccountConfig) bool {
	if account == nil {
		return m.Version == 2 && m.Mode == Mode
	}
	return m.Version == 3 && m.Mode == AccountMode && m.AccountID == account.AccountID && m.Fixture == account.Fixture
}

// NewAccount opens only a new, separate account-fixture namespace. It cannot
// adopt or upgrade set-22/set-26 standalone pilot state.
func NewAccount(config Config, account AccountConfig) (*Controller, error) {
	if _, err := gymaccount.FixtureIdentity(account.Fixture); err != nil || !validAccountID(account.AccountID) || !validAccountID(account.AuthorityID) || account.Authorizer == nil || account.MiningToken == nil {
		return nil, errors.New("explicit fixture account authority is required")
	}
	return newController(config, &account)
}

func (c *Controller) openWallet(create bool) (*walletRuntime, error) {
	path := filepath.Join(c.config.DataDir, "wallet")
	if c.account == nil {
		return openFixtureWallet(path, create)
	}
	return openAccountFixtureWallet(path, c.account.Fixture, create)
}

func (c *Controller) restoreWallet(path string, bundle recovery.Export) (*walletRuntime, error) {
	if c.account == nil {
		return restoreFixtureWallet(path, bundle)
	}
	return restoreAccountFixtureWallet(path, c.account.Fixture, bundle)
}

// Prepare initializes the retained fixture wallet and isolated node data without
// obtaining a mining job, hashing, broadcasting or starting the worker. Once
// initialization begins it drains even on cancellation, just like first Start.
func (c *Controller) Prepare(ctx context.Context) error {
	c.mu.Lock()
	ready, busy, closed := c.manifest.Phase == "ready", c.cancel != nil, c.closed
	c.mu.Unlock()
	if busy || closed {
		return errors.New("pilot is unavailable")
	}
	if ready {
		return nil
	}
	if err := c.start(false); err != nil {
		return err
	}
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		c.Stop()
		<-done
		return ctx.Err()
	case <-done:
	}
	if c.Snapshot().State != "stopped" {
		return errors.New("fixture account wallet initialization failed")
	}
	return nil
}

// SignAccountChallenge is private helper work, never a webview method. The
// caller must separately hold the current enrollment session authority.
func (c *Controller) SignAccountChallenge(ctx context.Context, challenge gymaccount.EnrollmentChallenge) (gymaccount.Authorization, error) {
	if c.account == nil || challenge.AccountID != c.account.AccountID {
		return gymaccount.Authorization{}, errors.New("account challenge is unavailable")
	}
	op, done, err := c.beginOperation("account-signing", 30*time.Second)
	if err != nil {
		return gymaccount.Authorization{}, err
	}
	defer c.finishOperation(done)
	// Bind caller cancellation to this operation, not Controller.Stop: an
	// already-started AfterFunc callback can outlive stop() and must never
	// cancel a later operation on the same controller.
	signing, cancel := context.WithCancel(op)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	wallet, err := c.openWallet(false)
	if err != nil {
		return gymaccount.Authorization{}, err
	}
	auth, signErr := wallet.SignAccountChallenge(signing, c.account.AuthorityID, challenge)
	closeErr := wallet.Close()
	if closeErr != nil {
		c.noteShutdown("pilot account wallet close failed")
	}
	if err := errors.Join(signErr, closeErr); err != nil {
		return gymaccount.Authorization{}, errors.New("account signature unavailable")
	}
	if ctx.Err() != nil || signing.Err() != nil {
		return gymaccount.Authorization{}, errors.New("account signature canceled")
	}
	return auth, nil
}

type sessionAuthority struct {
	origin  string
	account *AccountConfig
}

func (a sessionAuthority) ProtocolVersion() uint32 { return 2 }
func (a sessionAuthority) AuthorityOrigin() string { return a.origin }
func (a sessionAuthority) client(ctx context.Context) (*prospectservice.Client, error) {
	token, err := a.account.MiningToken(ctx)
	if err != nil {
		return nil, errors.New("mining session unavailable")
	}
	return prospectservice.NewClient(prospectservice.ClientConfig{Version: 2, URL: a.origin, Token: token}, policy())
}
func (a sessionAuthority) IssueJob(ctx context.Context, key string) (prospectjob.Job, error) {
	client, err := a.client(ctx)
	if err != nil {
		return prospectjob.Job{}, err
	}
	defer client.Close()
	return client.IssueJob(ctx, key)
}
func (a sessionAuthority) Submit(ctx context.Context, job prospectjob.Job, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
	client, err := a.client(ctx)
	if err != nil {
		return prospectservice.AdmissionResult{}, err
	}
	defer client.Close()
	return (pacedAuthority{client}).Submit(ctx, job, proof, auth)
}

func (c *Controller) miningService() (prospectworker.Authority, func(), error) {
	if c.account != nil {
		return sessionAuthority{origin: c.manifest.origin(), account: c.account}, func() {}, nil
	}
	client, err := prospectservice.NewClient(prospectservice.ClientConfig{Version: 2, URL: c.manifest.origin(), Token: fixtureToken}, policy())
	if err != nil {
		return nil, nil, err
	}
	return pacedAuthority{client}, client.Close, nil
}

func (c *Controller) accountSigner(signer prospectworker.Signer) prospectworker.Signer {
	return func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymAuth gymauth.Authorization, err error) {
		token, err := c.account.MiningToken(ctx)
		if err != nil {
			return gymAuth, errors.New("mining session unavailable")
		}
		err = c.account.Authorizer.WithSession(ctx, token, func(key string, recheck func() error) error {
			if key != c.manifest.Identity.GymPublicKey || recheck == nil || recheck() != nil {
				return errors.New("mining session unavailable")
			}
			var signErr error
			gymAuth, signErr = signer(ctx, initial, proof)
			if signErr != nil {
				return signErr
			}
			return recheck()
		})
		if err != nil {
			return gymauth.Authorization{}, errors.New("mining authorization unavailable")
		}
		return gymAuth, nil
	}
}

// delegatedAccountSigner leaves the account lease before calling the custody
// boundary, which owns its own preflight/decryption/final authorization. Holding
// the lease across that call would deadlock a shared authority and prevent reset
// from progressing during provider I/O. Recheck the same credential and verify
// the returned typed signature before handing it to the worker.
func (c *Controller) delegatedAccountSigner() prospectworker.Signer {
	return func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		denied := errors.New("mining authorization unavailable")
		key := c.manifest.Identity.GymPublicKey
		if ctx.Err() != nil || len(proof.MerkleBranch) > 20 {
			return gymauth.Authorization{}, denied
		}
		// Own the evidence checked across waits. Give the callback a separate
		// branch so it cannot mutate this baseline or the worker's input.
		proof.MerkleBranch = append([]string(nil), proof.MerkleBranch...)
		if _, err := gymauth.AdmissionDigest(initial, proof, key); err != nil {
			return gymauth.Authorization{}, denied
		}
		token, err := c.account.MiningToken(ctx)
		if err != nil {
			return gymauth.Authorization{}, denied
		}
		check := func(expected string, recheck func() error) error {
			if ctx.Err() != nil || expected != key || recheck == nil {
				return denied
			}
			return recheck()
		}
		if err = c.account.Authorizer.WithSession(ctx, token, check); err != nil {
			return gymauth.Authorization{}, denied
		}
		request := proof
		request.MerkleBranch = append([]string(nil), proof.MerkleBranch...)
		auth, err := c.account.AdmissionSigner(ctx, token, initial, request)
		if err != nil || ctx.Err() != nil {
			return gymauth.Authorization{}, denied
		}
		err = c.account.Authorizer.WithSession(ctx, token, func(expected string, recheck func() error) error {
			if auth.PublicKey != key || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
				return denied
			}
			return check(expected, recheck)
		})
		if err != nil {
			return gymauth.Authorization{}, denied
		}
		return auth, nil
	}
}

// AccountID is the immutable fixture binding, never a credential.
func (c *Controller) AccountID() string {
	if c.account == nil {
		return ""
	}
	return c.account.AccountID
}
