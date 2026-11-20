// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func (c *OwnedGymController) begin(state, view string, fn func(context.Context) error) error {
	c.mu.Lock()
	if c.closed || c.operation || c.shutdownErr != nil {
		c.mu.Unlock()
		return gymstable.ErrBusy
	}
	if view != "" && (c.state != "signed_in" || c.viewID != view || c.child == nil) {
		c.mu.Unlock()
		return gymstable.ErrStaleView
	}
	work, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	c.operation = true
	c.state = state
	c.lastError = ""
	c.pendingResetID = ""
	c.cancel = cancel
	c.done = make(chan struct{})
	done := c.done
	if state == "signing_out" || state == "resetting" {
		c.viewID = ""
	}
	c.mu.Unlock()
	go func() {
		defer cancel()
		e := fn(work)
		var pending *FixturePendingReset
		if e != nil && c.driver != nil {
			probe, stop := context.WithTimeout(context.Background(), 5*time.Second)
			pending, _ = c.pendingReset(probe)
			stop()
		}
		c.mu.Lock()
		c.operation = false
		c.cancel = nil
		if !c.closed && e != nil {
			c.state = "failed"
			c.lastError = "account_unavailable"
			if pending != nil {
				c.fixture = pending.Fixture
				c.pendingResetID = pending.Request.EventID
				c.lastError = "hosted_reset_pending"
			}
		}
		close(done)
		c.mu.Unlock()
	}()
	return nil
}
func (c *OwnedGymController) LoginFixture(fixture string) error {
	if _, e := gymaccount.FixtureIdentity(fixture); e != nil {
		return e
	}
	c.mu.Lock()
	occupied := c.child != nil || c.session.Token != ""
	c.mu.Unlock()
	if occupied {
		return gymstable.ErrBusy
	}
	return c.begin("signing_in", "", func(ctx context.Context) error {
		if c.runtime == nil || !c.runtime.Ready() {
			if e := c.resumeSetup(ctx); e != nil {
				return e
			}
		}
		return c.login(ctx, fixture)
	})
}
func (c *OwnedGymController) login(ctx context.Context, fixture string) (resultErr error) {
	login, at, e := c.signedLogin(ctx, fixture)
	if e != nil {
		return e
	}
	view, e := hostedRandomID(rand.Reader)
	if e != nil {
		return errors.Join(e, c.accounts.Logout(context.Background(), login.Token))
	}
	c.mu.Lock()
	if c.closed || c.shutdownErr != nil {
		c.mu.Unlock()
		return errors.Join(gymaccount.ErrUnauthorized, c.accounts.Logout(context.Background(), login.Token))
	}
	c.session = login
	c.viewID = view
	c.fixture = fixture
	c.mu.Unlock()
	var child *pilot.OwnedGymChild
	var session *pilot.OwnedWalletSession
	defer func() {
		if resultErr != nil {
			c.mu.Lock()
			var wait <-chan struct{}
			if c.session.Token == login.Token && c.expiry != nil {
				if !c.expiry.Stop() {
					wait = c.expiryDone
				}
				c.expiry = nil
				c.expiryDone = nil
			}
			if c.viewID == view {
				c.viewID = ""
			}
			if c.child == child {
				c.child = nil
			}
			if c.session.Token == login.Token {
				c.session = gymaccount.Session{}
				c.mining = gymaccount.Session{}
			}
			c.mu.Unlock()
			e := c.accounts.Logout(context.Background(), login.Token)
			if wait != nil {
				<-wait
			}
			if child != nil {
				e = errors.Join(e, child.Close())
			} else if session != nil {
				e = errors.Join(e, session.Close())
			}
			if e != nil {
				c.mu.Lock()
				c.failedChild = child
				c.failedWallet = session
				c.shutdownErr = errors.Join(c.shutdownErr, e)
				c.mu.Unlock()
			}
			resultErr = errors.Join(resultErr, e)
		}
	}()
	fence := c.viewFence(login, view, func() *pilot.OwnedGymChild { return child })
	current, e := c.enroll(ctx, fixture, &login, fence)
	if e != nil {
		return e
	}
	c.mu.Lock()
	if c.viewID == view && c.session.Token == login.Token {
		c.session.Account = login.Account
	}
	c.mu.Unlock()
	session, e = c.wallets.OpenFixture(ctx, fixture, login, fence, current)
	if e != nil {
		return e
	}
	if _, e = c.runtime.PrepareOwnedWallet(ctx, session); e != nil {
		return e
	}
	mining, e := c.accounts.MintMining(ctx, login.Token)
	if e != nil {
		return e
	}
	c.mu.Lock()
	if c.closed || c.viewID != view {
		c.mu.Unlock()
		return gymaccount.ErrUnauthorized
	}
	c.mining = mining
	c.mu.Unlock()
	manifest := pilot.ManifestAccountConfig{Account: pilot.AccountConfig{Fixture: fixture, AccountID: login.Account.ID, AuthorityID: c.accounts.AuthorityID(), RequireExisting: true, Authorizer: miningAuthorizer{c.accounts, login.Account.ID}, MiningToken: func(ctx context.Context) (string, error) { return c.miningToken(ctx, login, view) }}, ViewID: view, Generation: login.Account.Generation}
	lease := func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return c.withCapability(ctx, login, view, child, cap, fn)
	}
	manifest.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return lease(ctx, gymaccount.View, fn)
	}
	stable := pilot.StableAccountConfig{Manifest: manifest, AuthenticatedAtMS: at, SessionExpiresAtMS: login.ExpiresAtMS, EvidencePermit: &c.evidencePermit, Lease: lease, HistoryPublicationLease: manifest.ViewLease, ResultNodeLease: manifest.ViewLease}
	child, e = pilot.NewOwnedGymChild(ctx, c.runtime, session, c.league, stable)
	if e != nil {
		return e
	}
	c.mu.Lock()
	if c.closed || c.viewID != view || c.session.Token != login.Token {
		c.mu.Unlock()
		return gymaccount.ErrUnauthorized
	}
	c.child = child
	c.mu.Unlock()
	e = c.withCapability(ctx, login, view, child, gymaccount.View, func(_ gymaccount.Account, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if c.closed || c.viewID != view || c.child != child {
			return gymaccount.ErrUnauthorized
		}
		c.state = "signed_in"
		c.lastError = ""
		c.pendingResetID = ""
		c.expiryDone = make(chan struct{})
		done := c.expiryDone
		c.expiry = time.AfterFunc(time.Until(time.UnixMilli(login.ExpiresAtMS)), func() { defer close(done); c.expire(view, child) })
		return nil
	})
	if e != nil {
		return e
	}
	return child.Activate(ctx)
}
func (c *OwnedGymController) viewFence(login gymaccount.Session, view string, child func() *pilot.OwnedGymChild) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			expected := child()
			if ctx.Err() != nil || c.closed || c.shutdownErr != nil || view == "" || c.viewID != view || c.session.Token != login.Token || c.session.Account.ID != login.Account.ID || c.session.Account.Generation != login.Account.Generation || expected != nil && c.child != expected {
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
func (c *OwnedGymController) withCapability(ctx context.Context, login gymaccount.Session, view string, child *pilot.OwnedGymChild, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
	if ctx == nil || fn == nil {
		return gymaccount.ErrUnauthorized
	}
	return c.accounts.WithSession(ctx, login.Token, cap, func(a gymaccount.Account, again func() error) error {
		check := func() error {
			if e := again(); e != nil {
				return e
			}
			if !c.mu.TryLock() {
				return gymaccount.ErrUnauthorized
			}
			defer c.mu.Unlock()
			if ctx.Err() != nil || c.closed || c.shutdownErr != nil || c.viewID != view || view == "" || c.session.Token != login.Token || a.ID != login.Account.ID || a.Generation != login.Account.Generation || child != nil && c.child != child {
				return gymaccount.ErrUnauthorized
			}
			if c.state != "signed_in" && c.state != "signing_in" && c.state != "resetting" {
				return gymaccount.ErrUnauthorized
			}
			switch cap {
			case gymaccount.FighterTransfer, gymaccount.Withdrawal, gymaccount.RecoveryExport, gymaccount.RosterManage, gymaccount.EntryManage, gymaccount.SeedPublish, gymaccount.ResultPublish:
				if c.state != "signed_in" || c.operation {
					return gymaccount.ErrUnauthorized
				}
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
func (c *OwnedGymController) miningToken(ctx context.Context, login gymaccount.Session, view string) (string, error) {
	c.miningMu.Lock()
	defer c.miningMu.Unlock()
	c.mu.Lock()
	valid := !c.closed && c.shutdownErr == nil && c.viewID == view && c.session.Token == login.Token && (c.state == "signed_in" || c.state == "signing_in" || c.state == "resetting")
	token := c.mining
	c.mu.Unlock()
	if !valid {
		return "", gymaccount.ErrUnauthorized
	}
	if token.Token != "" && time.Now().UnixMilli()+30000 < token.ExpiresAtMS {
		return token.Token, nil
	}
	next, e := c.accounts.MintMining(ctx, login.Token)
	if e != nil {
		return "", e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.viewID != view || c.session.Token != login.Token {
		return "", gymaccount.ErrUnauthorized
	}
	c.mining = next
	return next.Token, nil
}
func (c *OwnedGymController) detach(ctx context.Context) error {
	c.mu.Lock()
	child, token := c.child, c.session.Token
	c.child = nil
	c.viewID = ""
	c.session = gymaccount.Session{}
	c.mining = gymaccount.Session{}
	var wait <-chan struct{}
	if c.expiry != nil && !c.expiry.Stop() {
		wait = c.expiryDone
	}
	c.expiry = nil
	c.expiryDone = nil
	c.mu.Unlock()
	var e error
	if token != "" {
		e = c.accounts.Logout(ctx, token)
	}
	if child != nil {
		ce := child.Close()
		e = errors.Join(e, ce)
		if ce != nil {
			c.mu.Lock()
			c.failedChild = child
			c.mu.Unlock()
		}
	}
	if wait != nil {
		<-wait
	}
	if e != nil {
		c.mu.Lock()
		c.shutdownErr = errors.Join(c.shutdownErr, e)
		c.mu.Unlock()
	}
	return e
}
func (c *OwnedGymController) expire(view string, child *pilot.OwnedGymChild) {
	c.mu.Lock()
	if c.closed || c.operation || c.viewID != view || c.child != child {
		c.mu.Unlock()
		return
	}
	c.expiry = nil
	c.expiryDone = nil
	c.viewID = ""
	c.state = "expired"
	c.operation = true
	done := make(chan struct{})
	c.done = done
	c.mu.Unlock()
	_ = c.detach(context.Background())
	c.mu.Lock()
	c.operation = false
	close(done)
	c.mu.Unlock()
}
func (c *OwnedGymController) Logout(ctx context.Context, view string) error {
	if !hostedID(view) {
		return gymstable.ErrStaleView
	}
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			return ctx.Err()
		}
		return gymstable.ErrInvalid
	}
	return c.begin("signing_out", view, func(work context.Context) error {
		if e := c.detach(work); e != nil {
			return e
		}
		c.mu.Lock()
		c.state = "signed_out"
		c.fixture = ""
		c.mu.Unlock()
		return nil
	})
}
func (c *OwnedGymController) ResetFixtureAccount(ctx context.Context, view string) error {
	if !hostedID(view) {
		return gymstable.ErrStaleView
	}
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			return ctx.Err()
		}
		return gymstable.ErrInvalid
	}
	c.mu.Lock()
	fixture := c.fixture
	c.mu.Unlock()
	return c.begin("resetting", view, func(work context.Context) error { return c.reset(work, fixture, "") })
}
func (c *OwnedGymController) ResumePendingReset(ctx context.Context, eventID string) error {
	if ctx == nil || ctx.Err() != nil || !hostedID(eventID) {
		return gymstable.ErrInvalid
	}
	c.mu.Lock()
	expected, fixture := c.pendingResetID, c.fixture
	c.mu.Unlock()
	if expected != eventID {
		return gymstable.ErrConflict
	}
	return c.begin("resetting", "", func(work context.Context) error { return c.reset(work, fixture, eventID) })
}
func (c *OwnedGymController) reset(ctx context.Context, fixture, eventID string) error {
	p, e := c.driver.PendingReset(ctx, fixture)
	var request gymprovider.ResetRequest
	if e == nil {
		request = p.Request
		if eventID != "" && request.EventID != eventID {
			return gymstable.ErrConflict
		}
	} else if errors.Is(e, gymprovider.ErrNotFound) && eventID == "" {
		request, e = c.coordinator.IssueReset(ctx)
	}
	if e != nil {
		return e
	}
	token, e := c.driver.BeginResetEvidence(ctx, request, fixture)
	var ack gymprovider.ResetResult
	if e == nil {
		ack, e = c.coordinator.BeginReset(ctx, request.EventID, token)
	}
	token = ""
	// The signed authority barrier/revocation precedes every child join.
	e = errors.Join(e, c.detach(context.Background()))
	if e != nil {
		return e
	}
	if ack.Phase == "completed" {
		e = c.driver.ObserveResetComplete(ctx, request, ack)
	} else {
		if e = c.driver.CommitReset(ctx, request, ack); e != nil {
			return e
		}
		token, e = c.driver.CompleteResetEvidence(ctx, request)
		if e != nil {
			return e
		}
		ack, e = c.coordinator.CompleteReset(ctx, request.EventID, token)
		token = ""
		if e != nil {
			return e
		}
		e = c.driver.ObserveResetComplete(ctx, request, ack)
	}
	if e != nil {
		return e
	}
	if c.runtime == nil || !c.runtime.Ready() {
		if e = c.resumeSetup(ctx); e != nil {
			return e
		}
	}
	if eventID != "" {
		c.mu.Lock()
		c.state = "signed_out"
		c.lastError = ""
		c.pendingResetID = ""
		c.fixture = ""
		c.mu.Unlock()
		return nil
	}
	return c.login(ctx, fixture)
}
func (c *OwnedGymController) active(ctx context.Context, view string) (*pilot.OwnedGymChild, error) {
	if ctx == nil {
		return nil, gymstable.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.shutdownErr != nil || c.state != "signed_in" || c.child == nil || view == "" || c.viewID != view {
		return nil, gymstable.ErrStaleView
	}
	return c.child, nil
}
func (c *OwnedGymController) Scope(ctx context.Context, fixture string) (pilot.OwnedGymScope, error) {
	if ctx == nil {
		return pilot.OwnedGymScope{}, gymstable.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return pilot.OwnedGymScope{}, e
	}
	c.mu.Lock()
	if c.closed || c.operation || c.shutdownErr != nil || !c.hadReady || c.league == nil || c.wallets == nil {
		c.mu.Unlock()
		return pilot.OwnedGymScope{}, pilot.ErrPoolRuntime
	}
	league, wallets := c.league, c.wallets
	c.mu.Unlock()
	cfg := league.Config()
	index := -1
	for i, p := range cfg.Participants {
		if p.Fixture == fixture {
			index = i
		}
	}
	if index < 0 {
		return pilot.OwnedGymScope{}, gymstable.ErrInvalid
	}
	carrier, id, e := wallets.CarrierIdentity(fixture)
	if e != nil {
		return pilot.OwnedGymScope{}, e
	}
	p := cfg.Participants[index]
	out := pilot.OwnedGymScope{Version: 1, Fixture: fixture, Source: cfg.Source, PoolSource: cfg.PoolSource, SourceBindingID: cfg.SourceBindingID, StablePin: p.StablePin, StableStoreID: p.StableStoreID, EntryPin: p.EntryPin, EntryStoreID: p.EntryStoreID, EventPin: cfg.EventPin, EventStoreID: cfg.EventStoreID, Carrier: carrier, CarrierBindingID: id}
	if e = pilot.ValidateOwnedGymScope(out); e != nil {
		return pilot.OwnedGymScope{}, e
	}
	return out, nil
}
func (c *OwnedGymController) Snapshot() pilot.OwnedGymSnapshot {
	c.mu.Lock()
	child, login, view := c.child, c.session, c.viewID
	state, fixture, last, busy, closed := c.state, c.fixture, c.lastError, c.operation, c.closed
	pending := c.pendingResetID
	source := c.init.Source
	c.mu.Unlock()
	if closed {
		state = "signed_out"
		last = ""
		pending = ""
	} else if state == "signed_in" && (busy || child == nil) {
		state = "signing_in"
	}
	out := pilot.OwnedGymSnapshot{Version: 1, Mode: pilot.OwnedGymMode, State: state, Source: source, Account: pilot.AccountSnapshot{State: state, Fixture: fixture, LastError: last}, PendingResetID: pending}
	if last == "hosted_reset_pending" {
		out.Error = "hosted_reset_pending"
	} else if last != "" {
		out.Error = "owned_unavailable"
	}
	if closed || busy || state != "signed_in" || child == nil || view == "" {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	canExport := c.withCapability(ctx, login, view, child, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() }) == nil
	snapshot := child.Snapshot()
	e := c.withCapability(ctx, login, view, child, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		identity, e := gymaccount.FixtureIdentity(fixture)
		if e != nil || a.Enrollment == nil || *a.Enrollment != identity {
			return gymaccount.ErrUnauthorized
		}
		held := a.HoldUntilMS > time.Now().UnixMilli()
		v := pilot.OwnedGymView{Version: 1, ViewID: view, AccountAuthorityID: c.accounts.AuthorityID(), AccountID: a.ID, Generation: a.Generation, Fixture: fixture, RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}
		out.View = &v
		out.Child = &snapshot
		out.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: fixture, ID: a.ID, Generation: a.Generation, HoldUntilMS: a.HoldUntilMS, Held: held, CanExport: canExport && !held}
		return check()
	})
	if e != nil {
		out.View = nil
		out.Child = nil
		out.State = "expired"
		out.Account = pilot.AccountSnapshot{State: "expired", Fixture: fixture}
		// The original expiry timer owns revocation; transient fail-fast contention
		// must not tear down a still-valid session from a snapshot read.
	}
	return out
}
func (c *OwnedGymController) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.viewID = ""
		if c.cancel != nil {
			c.cancel()
		}
		done := c.done
		busy := c.operation
		c.mu.Unlock()
		if busy && done != nil {
			<-done
		}
		if e := c.detach(context.Background()); e != nil {
			c.closeErr = e
			return
		}
		c.mu.Lock()
		failure := c.shutdownErr
		failedChild, failedWallet := c.failedChild, c.failedWallet
		c.mu.Unlock()
		if failedChild != nil {
			failure = errors.Join(failure, failedChild.Close())
		}
		if failedWallet != nil {
			failure = errors.Join(failure, failedWallet.Close())
		}
		if failure != nil {
			c.closeErr = failure
			return
		}
		for _, close := range []func() error{
			func() error {
				if c.runtime != nil {
					return c.runtime.Close()
				}
				return nil
			}, func() error {
				if c.wallets != nil {
					return c.wallets.Close()
				}
				return nil
			}, func() error {
				if c.vault != nil {
					return c.vault.Close()
				}
				return nil
			}, func() error {
				if c.driver != nil {
					return c.driver.Close()
				}
				return nil
			}, func() error {
				if c.coordinator != nil {
					return c.coordinator.Close()
				}
				return nil
			}, func() error {
				if c.accounts != nil {
					return c.accounts.Close()
				}
				return nil
			}, func() error {
				if c.lock != nil {
					return c.lock.Close()
				}
				return nil
			},
		} {
			if e := close(); e != nil {
				c.closeErr = e
				return
			}
		}
	})
	return c.closeErr
}

func (c *OwnedGymController) CaptureStable(ctx context.Context, view string, id string) (pilot.OwnedStableCapture, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero pilot.OwnedStableCapture
		return zero, e
	}
	return child.CaptureStable(ctx, view, id)
}

func (c *OwnedGymController) StablePage(ctx context.Context, view string, id string, after, limit uint32) (gymstable.OwnedPage, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedPage
		return zero, e
	}
	return child.StablePage(ctx, view, id, after, limit)
}

func (c *OwnedGymController) PrepareSlot(ctx context.Context, view string, request gymstable.OwnedSlotRequest) (gymstable.OwnedIntent, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedIntent
		return zero, e
	}
	return child.PrepareSlot(ctx, view, request)
}

func (c *OwnedGymController) ApplySlot(ctx context.Context, view string, id string) (gymstable.OwnedMutationResult, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedMutationResult
		return zero, e
	}
	return child.ApplySlot(ctx, view, id)
}

func (c *OwnedGymController) SlotOperation(ctx context.Context, view string, id string) (gymstable.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedOperation
		return zero, e
	}
	return child.SlotOperation(ctx, view, id)
}

func (c *OwnedGymController) PendingSlot(ctx context.Context, view string) (*gymstable.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero *gymstable.OwnedOperation
		return zero, e
	}
	return child.PendingSlot(ctx, view)
}

func (c *OwnedGymController) AbandonSlot(ctx context.Context, view string, id string) (gymstable.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedOperation
		return zero, e
	}
	return child.AbandonSlot(ctx, view, id)
}

func (c *OwnedGymController) CaptureHistory(ctx context.Context, view string, prospect string) (gymstable.OwnedCareerHistoryCapture, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedCareerHistoryCapture
		return zero, e
	}
	return child.CaptureHistory(ctx, view, prospect)
}

func (c *OwnedGymController) HistoryPage(ctx context.Context, view string, id string, after, limit uint32) (gymstable.OwnedCareerHistoryPage, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymstable.OwnedCareerHistoryPage
		return zero, e
	}
	return child.HistoryPage(ctx, view, id, after, limit)
}

func (c *OwnedGymController) EntryUpcoming(ctx context.Context, view string) (gymentry.OwnedEventStatus, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedEventStatus
		return zero, e
	}
	return child.EntryUpcoming(ctx, view)
}

func (c *OwnedGymController) EntryStatus(ctx context.Context, view string, date string) (gymentry.OwnedEventStatus, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedEventStatus
		return zero, e
	}
	return child.EntryStatus(ctx, view, date)
}

func (c *OwnedGymController) PrepareEntry(ctx context.Context, view string, request gymentry.Request) (gymentry.OwnedIntent, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedIntent
		return zero, e
	}
	return child.PrepareEntry(ctx, view, request)
}

func (c *OwnedGymController) ApplyEntry(ctx context.Context, view string, id string) (gymentry.OwnedMutationResult, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedMutationResult
		return zero, e
	}
	return child.ApplyEntry(ctx, view, id)
}

func (c *OwnedGymController) EntryOperation(ctx context.Context, view string, id string) (gymentry.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedOperation
		return zero, e
	}
	return child.EntryOperation(ctx, view, id)
}

func (c *OwnedGymController) PendingEntry(ctx context.Context, view string) (*gymentry.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero *gymentry.OwnedOperation
		return zero, e
	}
	return child.PendingEntry(ctx, view)
}

func (c *OwnedGymController) AbandonEntry(ctx context.Context, view string, id string) (gymentry.OwnedOperation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymentry.OwnedOperation
		return zero, e
	}
	return child.AbandonEntry(ctx, view, id)
}

func (c *OwnedGymController) EventStatus(ctx context.Context, view string, date string) (gymevent.OwnedEventStatus, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymevent.OwnedEventStatus
		return zero, e
	}
	return child.EventStatus(ctx, view, date)
}

func (c *OwnedGymController) EventRecord(ctx context.Context, view string, date string) (gymevent.OwnedSnapshotReceipt, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero gymevent.OwnedSnapshotReceipt
		return zero, e
	}
	return child.EventRecord(ctx, view, date)
}

func (c *OwnedGymController) TransferPrepare(ctx context.Context, view string, id, prospect, recipient string) (pilot.OwnedTransferReview, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero pilot.OwnedTransferReview
		return zero, e
	}
	return child.TransferPrepare(ctx, view, id, prospect, recipient)
}

func (c *OwnedGymController) TransferResume(ctx context.Context, view string, id, prospect string) (pilot.OwnedTransferReview, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero pilot.OwnedTransferReview
		return zero, e
	}
	return child.TransferResume(ctx, view, id, prospect)
}

func (c *OwnedGymController) TransferConfirm(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero fightercarrier.Operation
		return zero, e
	}
	return child.TransferConfirm(ctx, view, id)
}

func (c *OwnedGymController) TransferReviewDelivery(ctx context.Context, view string, id, leg string) (pilot.OwnedTransferDeliveryReview, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero pilot.OwnedTransferDeliveryReview
		return zero, e
	}
	return child.TransferReviewDelivery(ctx, view, id, leg)
}

func (c *OwnedGymController) TransferDeliver(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero fightercarrier.Operation
		return zero, e
	}
	return child.TransferDeliver(ctx, view, id)
}

func (c *OwnedGymController) TransferObserve(ctx context.Context, view string, id, leg string) (fightercarrier.Operation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero fightercarrier.Operation
		return zero, e
	}
	return child.TransferObserve(ctx, view, id, leg)
}

func (c *OwnedGymController) TransferOperation(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero fightercarrier.Operation
		return zero, e
	}
	return child.TransferOperation(ctx, view, id)
}

func (c *OwnedGymController) TransferAbandon(ctx context.Context, view string, id string) (fightercarrier.Operation, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		var zero fightercarrier.Operation
		return zero, e
	}
	return child.TransferAbandon(ctx, view, id)
}

func (c *OwnedGymController) Start(ctx context.Context, view string) error {
	child, e := c.active(ctx, view)
	if e != nil {
		return e
	}
	return child.Start(ctx, view)
}

func (c *OwnedGymController) Stop(ctx context.Context, view string) error {
	child, e := c.active(ctx, view)
	if e != nil {
		return e
	}
	return child.Stop(ctx, view)
}

func (c *OwnedGymController) ExportRecovery(ctx context.Context, view string, pin recoveryfile.Pin) error {
	child, e := c.active(ctx, view)
	if e != nil {
		return e
	}
	return child.ExportRecovery(ctx, view, pin)
}

func (c *OwnedGymController) VerifyBackup(ctx context.Context, view string, pin recoveryfile.Pin) error {
	child, e := c.active(ctx, view)
	if e != nil {
		return e
	}
	return child.VerifyBackup(ctx, view, pin)
}

func (c *OwnedGymController) CancelBackup(ctx context.Context, view string) error {
	child, e := c.active(ctx, view)
	if e != nil {
		return e
	}
	return child.CancelBackup(ctx, view)
}

func (c *OwnedGymController) TransferFinality(ctx context.Context, view, id string) (pilot.OwnedTransferFinality, error) {
	child, e := c.active(ctx, view)
	if e != nil {
		return pilot.OwnedTransferFinality{}, e
	}
	return child.TransferFinality(ctx, view, id)
}
