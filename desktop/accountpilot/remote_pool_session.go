// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func (c *RemotePoolController) LoginFixture(ctx context.Context, fixture string) (resultErr error) {
	if c == nil || fixture != c.fixture {
		return gymaccount.ErrUnauthorized
	}
	work, done, e := c.admit(ctx, "")
	if e != nil {
		return e
	}
	defer done()
	c.mu.Lock()
	if c.session.Token != "" || c.wallet != nil || c.attachment != nil {
		c.mu.Unlock()
		return gymstable.ErrBusy
	}
	c.state, c.lastError = "signing_in", ""
	c.mu.Unlock()
	defer func() {
		if resultErr != nil {
			c.mu.Lock()
			c.viewID = ""
			if !c.closed {
				c.state = "failed"
				c.lastError = "remote_unavailable"
			}
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, c.detach())
		}
	}()
	if e = c.auditControls(); e != nil {
		return e
	}
	if e = c.refusePendingReset(work); e != nil {
		return e
	}
	login, at, e := signedOwnedFixtureLogin(work, c.accounts, c.coordinator, c.driver, fixture)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.session = login
	c.authenticatedAtMS = at
	c.mu.Unlock()
	view, e := hostedRandomID(rand.Reader)
	if e != nil {
		return e
	}
	c.mu.Lock()
	if c.closed || c.shutdownErr != nil || c.life.Err() != nil {
		c.mu.Unlock()
		return gymaccount.ErrUnauthorized
	}
	c.viewID = view
	c.mu.Unlock()
	fence := c.epoch(login, view)
	current, e := c.enrolledOnly(work, &login, fence)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.session.Account = login.Account
	c.mu.Unlock()
	session, e := c.wallets.OpenFixture(work, fixture, login, fence, current)
	// Even a constructor error may return the exact failed cleanup owner.
	c.mu.Lock()
	c.wallet = session
	c.mu.Unlock()
	if e != nil {
		return e
	}
	openAttachment := pilot.OpenRemotePoolAttachment
	if c.daily {
		openAttachment = pilot.OpenDailyRemotePoolAttachment
	}
	var attachment *pilot.RemotePoolAttachment
	if c.recovery != nil {
		attachment, e = pilot.OpenRecoverableDailyRemotePoolAttachment(work, session, view, pilot.RecoverableRemotePoolInputs{AttachmentRoot: c.inputs.AttachmentRoot, Config: *c.recovery, NodeRPC: c.inputs.NodeRPC, PoolCredentialPath: c.inputs.PoolCredentialPath, AdmissionCredentialPath: c.inputs.AdmissionCredentialPath})
	} else {
		attachment, e = openAttachment(work, session, view, c.inputs)
	}
	c.mu.Lock()
	c.attachment = attachment
	c.mu.Unlock()
	if e != nil {
		return e
	}
	if session == nil || attachment == nil {
		return pilot.ErrRemotePoolConfig
	}
	if e = c.auditControls(); e != nil {
		return e
	}
	// Open is synchronous and stopped. No worker callback can observe a partial
	// child/view publication; explicit Start is a separate admitted API.
	e = c.withView(work, view, attachment, func(a gymaccount.Account, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || c.life.Err() != nil || c.viewID != view || c.attachment != attachment || c.wallet != session || c.session.Token != login.Token || login.ExpiresAtMS <= time.Now().UnixMilli() {
			return gymaccount.ErrUnauthorized
		}
		c.state, c.lastError = "signed_in", ""
		return nil
	})
	if e != nil {
		return e
	}
	// Arm expiry only after all fallible account disclosure checks. If expiry
	// starts now it may wait for this operation's done, but no error cleanup can
	// recursively wait for that same callback.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.viewID != view || c.session.Token != login.Token || c.attachment != attachment || login.ExpiresAtMS <= time.Now().UnixMilli() {
		return gymaccount.ErrUnauthorized
	}
	if e := work.Err(); e != nil {
		if c.daily {
			return e
		}
		return gymaccount.ErrUnauthorized
	}
	expiryDone := make(chan struct{})
	c.expiryDone = expiryDone
	c.expiry = time.AfterFunc(time.Until(time.UnixMilli(login.ExpiresAtMS)), func() { defer close(expiryDone); c.expire(login, view, attachment) })
	return nil
}

// This is verification of the original enrolled receipt only. No challenge,
// provisioning, recovery or EnsureEnrollment mutation is reachable here.
func (c *RemotePoolController) enrolledOnly(ctx context.Context, login *gymaccount.Session, epoch recoveryvault.EpochFence) (recoveryvault.CurrentBinding, error) {
	var zero recoveryvault.CurrentBinding
	identity, e := gymaccount.FixtureIdentity(c.fixture)
	if e != nil {
		return zero, e
	}
	expected, e := c.selectedCurrent()
	if e != nil {
		return zero, e
	}
	e = c.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != login.Account.ID || a.Generation != login.Account.Generation || a.Enrollment == nil || *a.Enrollment != identity || a.RecoveryRequired || a.PrincipalID != expected.VaultBinding.PrincipalID || a.ID != expected.VaultBinding.AccountID {
			return gymaccount.ErrUnauthorized
		}
		login.Account = a
		return check()
	})
	if e != nil {
		return zero, e
	}
	status, e := c.vault.Status(ctx, login.Token, epoch)
	if e != nil {
		return zero, e
	}
	if status.State != "enrolled" || status.Receipt == nil {
		return zero, pilot.ErrManifestInitialization
	}
	receipt := status.Receipt
	actual := recoveryvault.CurrentBinding{Version: 1, HostedVaultID: c.init.Hosted.HostedVaultID, VaultBinding: receipt.Binding, RetentionID: receipt.RetentionID, WalletOwnerID: c.inputs.Config.ExpectedLocal.WalletOwnerID, SourceBindingID: c.inputs.Config.ExpectedLocal.OriginalSourceBindingID}
	if actual != expected || receipt.Binding.AuthorityID != c.accounts.AuthorityID() || receipt.Binding.RootPublicKey != identity.RootPublicKey || receipt.Binding.GymPublicKey != identity.GymPublicKey || receipt.Binding.GymID != identity.GymID {
		return zero, pilot.ErrManifestInitialization
	}
	return actual, nil
}

func (c *RemotePoolController) epoch(login gymaccount.Session, view string) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			if !c.daily && ctx.Err() != nil {
				return gymaccount.ErrUnauthorized
			}
			if c.closed || c.shutdownErr != nil || c.life.Err() != nil || view == "" || c.viewID != view || c.session.Token != login.Token || c.session.Account.ID != login.Account.ID || c.session.Account.Generation != login.Account.Generation || c.state != "signed_in" && c.state != "signing_in" {
				return gymaccount.ErrUnauthorized
			}
			if e := ctx.Err(); e != nil {
				if c.daily {
					return e
				}
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(check); e != nil {
			return e
		}
		return check()
	}
}

func (c *RemotePoolController) withView(ctx context.Context, view string, attachment *pilot.RemotePoolAttachment, fn func(gymaccount.Account, func() error) error) error {
	c.mu.Lock()
	login := c.session
	c.mu.Unlock()
	if ctx == nil || fn == nil || login.Token == "" || view == "" {
		return gymaccount.ErrUnauthorized
	}
	return c.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, again func() error) error {
		check := func() error {
			if e := again(); e != nil {
				return e
			}
			if !c.mu.TryLock() {
				return gymstable.ErrBusy
			}
			defer c.mu.Unlock()
			if !c.daily && ctx.Err() != nil {
				return gymaccount.ErrUnauthorized
			}
			if c.closed || c.shutdownErr != nil || c.life.Err() != nil || c.viewID != view || c.session.Token != login.Token || a.ID != login.Account.ID || a.Generation != login.Account.Generation || c.attachment != attachment || c.state != "signed_in" && c.state != "signing_in" {
				return gymaccount.ErrUnauthorized
			}
			if e := ctx.Err(); e != nil {
				if c.daily {
					return e
				}
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(a, check); e != nil {
			return e
		}
		return check()
	})
}

func (c *RemotePoolController) Start(ctx context.Context, view string) error {
	if !hostedID(view) {
		return gymstable.ErrStaleView
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return e
	}
	defer done()
	c.mu.Lock()
	attachment := c.attachment
	c.mu.Unlock()
	if e = c.withView(work, view, attachment, func(_ gymaccount.Account, check func() error) error { return check() }); e != nil {
		return e
	}
	e = attachment.Start(work, view)
	return errors.Join(e, c.withView(work, view, attachment, func(_ gymaccount.Account, check func() error) error { return check() }))
}
func (c *RemotePoolController) Stop(ctx context.Context, view string) error {
	if !hostedID(view) {
		return gymstable.ErrStaleView
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return e
	}
	defer done()
	c.mu.Lock()
	attachment := c.attachment
	c.mu.Unlock()
	e = attachment.Stop(work, view)
	return errors.Join(e, c.withView(work, view, attachment, func(_ gymaccount.Account, check func() error) error { return check() }))
}
func (c *RemotePoolController) Logout(ctx context.Context, view string) error {
	if !hostedID(view) {
		return gymstable.ErrStaleView
	}
	_, done, e := c.admitMode(ctx, view, true)
	if e != nil {
		return e
	}
	defer done()
	c.stopExpiry()
	e = c.detach()
	c.mu.Lock()
	if !c.closed {
		if e == nil {
			c.state, c.lastError = "signed_out", ""
		} else {
			c.state, c.lastError = "failed", "remote_unavailable"
		}
	}
	c.mu.Unlock()
	return e
}

func (c *RemotePoolController) stopExpiry() {
	c.mu.Lock()
	timer, done := c.expiry, c.expiryDone
	c.expiry, c.expiryDone = nil, nil
	c.mu.Unlock()
	if timer != nil && !timer.Stop() && done != nil {
		<-done
	}
}
func (c *RemotePoolController) expire(login gymaccount.Session, view string, attachment *pilot.RemotePoolAttachment) {
	c.mu.Lock()
	if c.closed || c.viewID != view || c.session.Token != login.Token || c.attachment != attachment {
		c.mu.Unlock()
		return
	}
	c.viewID, c.state = "", "expired"
	c.expiring = true
	cancel, done, busy := c.opCancel, c.opDone, c.operation
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if busy && done != nil {
		<-done
	}
	e := c.detach()
	c.mu.Lock()
	c.expiring = false
	if e != nil {
		c.lastError = "remote_unavailable"
	}
	c.mu.Unlock()
}

// Called only after invalidating the original epoch and joining admitted APIs,
// or by the single admitted logout/login-cleanup operation. A failed child
// stops dependency cleanup; exact pointers and the root lock stay retained.
func (c *RemotePoolController) detach() error {
	c.mu.Lock()
	attachment, session, token := c.attachment, c.wallet, c.session.Token
	prior := c.shutdownErr
	c.viewID = ""
	c.mu.Unlock()
	// An audit descriptor failure retains the root, but must not leave the
	// attachment/backend running. Idempotent child Close methods join their work;
	// failed children still stop release of the wallet/dependencies below them.
	var e error
	if token != "" && c.accounts != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		e = c.accounts.Logout(ctx, token)
		cancel()
	}
	if attachment != nil {
		ce := attachment.Close()
		e = errors.Join(e, ce)
		if ce != nil {
			c.mu.Lock()
			c.shutdownErr = errors.Join(c.shutdownErr, e)
			retained := c.shutdownErr
			c.mu.Unlock()
			return retained
		}
	}
	if session != nil {
		e = errors.Join(e, session.Close())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e != nil || prior != nil {
		c.shutdownErr = errors.Join(c.shutdownErr, e)
		return c.shutdownErr
	}
	c.attachment, c.wallet = nil, nil
	c.session = gymaccount.Session{}
	c.authenticatedAtMS = 0
	return nil
}

func (c *RemotePoolController) Snapshot(ctx context.Context) (RemotePoolControllerSnapshot, error) {
	var out RemotePoolControllerSnapshot
	work, done, e := c.admit(ctx, "")
	if e != nil {
		return out, e
	}
	defer done()
	c.mu.Lock()
	state, view, attachment, last := c.state, c.viewID, c.attachment, c.lastError
	c.mu.Unlock()
	version := uint32(1)
	if c.daily {
		version = 2
	}
	out = RemotePoolControllerSnapshot{Version: version, ConfigID: c.configID, State: state, Account: pilot.AccountSnapshot{State: state, Fixture: c.fixture, LastError: last}, Error: last}
	if state != "signed_in" || attachment == nil || view == "" {
		return out, nil
	}
	remote, e := attachment.Snapshot(work, view)
	if e != nil {
		return RemotePoolControllerSnapshot{}, e
	}
	e = c.withView(work, view, attachment, func(a gymaccount.Account, check func() error) error {
		identity, e := gymaccount.FixtureIdentity(c.fixture)
		if e != nil || a.Enrollment == nil || *a.Enrollment != identity || a.RecoveryRequired {
			return gymaccount.ErrUnauthorized
		}
		held := a.HoldUntilMS > time.Now().UnixMilli()
		out.View = &pilot.RemotePoolView{Version: 1, ViewID: view, AccountAuthorityID: c.accounts.AuthorityID(), AccountID: a.ID, Generation: a.Generation, Fixture: c.fixture, RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}
		out.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: c.fixture, ID: a.ID, Generation: a.Generation, HoldUntilMS: a.HoldUntilMS, Held: held}
		out.Remote = &remote
		return check()
	})
	if e != nil {
		return RemotePoolControllerSnapshot{}, e
	}
	return out, nil
}
