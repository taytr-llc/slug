// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recovery"
	"path/filepath"
	"time"
)

func (c *ManifestController) openWallet(create bool) (*walletRuntime, error) {
	path := filepath.Join(c.config.DataDir, "wallet")
	return openAccountFixtureWallet(path, c.account.Fixture, create)
}

func (c *ManifestController) restoreWallet(path string, bundle recovery.Export) (*walletRuntime, error) {
	return restoreAccountFixtureWallet(path, c.account.Fixture, bundle)
}

// Prepare initializes the retained fixture wallet and isolated node data without
// obtaining a mining job, hashing, broadcasting or starting the worker. Once
// initialization begins it drains even on cancellation, just like first Start.
func (c *ManifestController) Prepare(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
func (c *ManifestController) SignAccountChallenge(ctx context.Context, challenge gymaccount.EnrollmentChallenge) (gymaccount.Authorization, error) {
	if c.account == nil || challenge.AccountID != c.account.AccountID {
		return gymaccount.Authorization{}, errors.New("account challenge is unavailable")
	}
	op, done, err := c.beginOperation(ctx, "account-signing", 30*time.Second)
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

func (c *ManifestController) accountSigner(signer prospectworker.Signer) prospectworker.Signer {
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

func (c *ManifestController) AccountID() string { return c.account.AccountID }

type manifestSessionAuthority struct {
	origin  string
	pin     registry.ManifestAuthorityPin
	trust   string
	account *AccountConfig
}

func (a manifestSessionAuthority) ProtocolVersion() uint32                     { return 3 }
func (a manifestSessionAuthority) AuthorityOrigin() string                     { return a.origin }
func (a manifestSessionAuthority) AuthorityPin() registry.ManifestAuthorityPin { return a.pin }
func (a manifestSessionAuthority) TransportTrustID() string                    { return a.trust }
func (a manifestSessionAuthority) client(ctx context.Context) (*prospectservice.ManifestClient, error) {
	token, e := a.account.MiningToken(ctx)
	if e != nil || ctx.Err() != nil {
		return nil, errors.New("mining session unavailable")
	}
	cl, e := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: a.origin, Token: token, AuthorityPin: a.pin}, policy())
	if e != nil {
		return nil, e
	}
	if cl.ProtocolVersion() != 3 || cl.AuthorityOrigin() != a.origin || cl.AuthorityPin() != a.pin || cl.TransportTrustID() != a.trust {
		cl.Close()
		return nil, errors.New("authority identity mismatch")
	}
	return cl, nil
}
func (a manifestSessionAuthority) IssueManifestJob(ctx context.Context, key string) (prospectjob.ManifestJob, error) {
	cl, e := a.client(ctx)
	if e != nil {
		return prospectjob.ManifestJob{}, e
	}
	defer cl.Close()
	return cl.IssueManifestJob(ctx, key)
}
func (a manifestSessionAuthority) SubmitManifest(ctx context.Context, j prospectjob.ManifestJob, p prospect.Proof, s gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	cl, e := a.client(ctx)
	if e != nil {
		return prospectservice.ManifestAdmissionResult{}, e
	}
	defer cl.Close()
	result, e := cl.SubmitManifest(ctx, j, p, s)
	// Preserve the acknowledgement if Stop interrupts the fixture pacing delay.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	return result, e
}
