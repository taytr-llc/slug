// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

type hostedReceiveChild interface {
	HostedAllocationSnapshot() (pilot.HostedAllocationScope, string, string, error)
	BeginHostedReceive(context.Context, string, string, string) (pilot.HostedReceiveJob, error)
	LookupHostedReceive(context.Context, string, string, string) (pilot.HostedReceiveJob, error)
	ResumeHostedReceive(context.Context, string, string, string) (pilot.HostedReceiveJob, error)
	RedisplayHostedReceive(context.Context, string, string, string) (pilot.HostedReceiveJob, error)
	CancelHostedReceive(context.Context, string, string, string) (pilot.HostedReceiveJob, error)
}

func (c *PoolController) hostedAllocationMode() bool {
	return c.hosted != nil && c.hosted.allocation != nil
}
func (c *PoolController) poolMode() string {
	if c.hostedAllocationMode() {
		return pilot.HostedRecoveryPoolMode
	}
	if c.hosted != nil {
		return pilot.HostedPoolMode
	}
	return pilot.PoolMode
}
func (c *PoolController) recoveryOwner() *pilot.HostedRecoveryOwner {
	if !c.hostedAllocationMode() {
		return nil
	}
	return c.hosted.allocation.recovery
}
func (c *PoolController) HostedRecoverySnapshot() pilot.HostedRecoverySnapshot {
	blank := pilot.BlankHostedRecoverySnapshot()
	if !c.hostedAllocationMode() {
		blank.Error = "hosted_recovery_unavailable"
		return blank
	}
	c.mu.Lock()
	owner := c.recoveryOwner()
	session, view := c.session, c.viewID
	state, fixture, last, busy, closed := c.state, c.fixture, c.lastError, c.operation, c.closed
	c.mu.Unlock()
	blank.Account = pilot.AccountSnapshot{State: state, Fixture: fixture, LastError: last}
	if closed {
		blank.Account = pilot.AccountSnapshot{State: "signed_out"}
		return blank
	}
	if last == "hosted_reset_pending" {
		blank.Error = "hosted_reset_pending"
	}
	if state == "failed" && last == "account_unavailable" && fixture != "" && c.hosted.allocation.archive != nil {
		blank.Error = "hosted_recovery_unavailable"
	}
	if busy || state != "signed_in" || view == "" {
		if state == "signed_in" {
			blank.Account.State = "signing_in"
		}
		return blank
	}
	if owner == nil {
		pool := c.Snapshot()
		if pool.Account.State != "signed_in" {
			blank.Account = pool.Account
			return blank
		}
		child, session, view, e := c.active()
		if e != nil {
			return blank
		}
		api, ok := child.(hostedReceiveChild)
		if !ok {
			return blank
		}
		scope, pending, issued, e := api.HostedAllocationSnapshot()
		if e != nil {
			return blank
		}
		out := pilot.HostedRecoverySnapshot{Version: 1, Mode: pilot.HostedRecoveryPoolMode, Kind: "healthy", Account: pool.Account, Healthy: &pilot.HostedHealthySnapshot{Pool: pool, Allocation: scope, PendingReceiveID: pending, IssuedReceiveID: issued}}
		e = c.withView(context.Background(), session, view, child, func(_ gymaccount.Account, check func() error) error {
			if e := check(); e != nil {
				return e
			}
			return pilot.ValidateHostedRecoverySnapshot(out)
		})
		if e == nil {
			return out
		}
		blank.Account = pilot.AccountSnapshot{State: "expired", Fixture: fixture}
		return blank
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	canExport := c.withRecoveryOwner(ctx, session, view, owner, gymaccount.RecoveryExport, func(gymaccount.Account, func() error) error { return nil }) == nil
	out := blank
	e := c.withRecoveryOwner(ctx, session, view, owner, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		v := owner.Snapshot()
		held := a.HoldUntilMS > c.clockNow().UnixMilli()
		out = pilot.HostedRecoverySnapshot{Version: 1, Mode: pilot.HostedRecoveryPoolMode, Kind: "recovery_only", Account: pilot.AccountSnapshot{State: "signed_in", Fixture: fixture, ID: a.ID, Generation: a.Generation, HoldUntilMS: a.HoldUntilMS, Held: held, CanExport: canExport && !held}, Recovery: &v}
		if e := pilot.ValidateHostedRecoverySnapshot(out); e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		blank.Account = pilot.AccountSnapshot{State: "expired", Fixture: fixture}
		go c.expireRecoveryView(view, owner)
		return blank
	}
	return out
}
func (c *PoolController) withRecoveryOwner(ctx context.Context, session gymaccount.Session, view string, owner *pilot.HostedRecoveryOwner, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
	if owner == nil || fn == nil {
		return gymaccount.ErrUnauthorized
	}
	return c.store.WithSession(ctx, session.Token, cap, func(a gymaccount.Account, recheck func() error) error {
		return c.recoveryEpoch(session, view, func() *pilot.HostedRecoveryOwner { return owner })(ctx, func(parentCheck func() error) error {
			check := func() error {
				if recheck == nil || a.ID != session.Account.ID || a.Generation != session.Account.Generation {
					return gymaccount.ErrUnauthorized
				}
				if e := recheck(); e != nil {
					return e
				}
				return parentCheck()
			}
			if e := check(); e != nil {
				return e
			}
			if e := fn(a, check); e != nil {
				return e
			}
			return check()
		})
	})
}
func (c *PoolController) recoveryEpoch(session gymaccount.Session, view string, get func() *pilot.HostedRecoveryOwner) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			if ctx.Err() != nil || c.closed || !c.hostedAllocationMode() || c.viewID != view || c.session.Token != session.Token || c.session.Account.ID != session.Account.ID || c.session.Account.Generation != session.Account.Generation || c.recoveryOwner() != get() || c.shutdownErr != nil || !(c.state == "signed_in" || c.state == "signing_in" || c.state == "resetting") {
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
func (c *PoolController) activeRecovery(view string) (*pilot.HostedRecoveryOwner, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	owner := c.recoveryOwner()
	if c.closed || c.operation || c.state != "signed_in" || view == "" || c.viewID != view || owner == nil || c.session.Token == "" {
		return nil, pilot.ErrHostedStaleView
	}
	return owner, nil
}
func (c *PoolController) hostedHealthy(view string) (poolChild, hostedReceiveChild, gymaccount.Session, error) {
	child, session, v, e := c.active()
	if e != nil || v != view || !c.hostedAllocationMode() {
		return nil, nil, gymaccount.Session{}, pilot.ErrHostedStaleView
	}
	api, ok := child.(hostedReceiveChild)
	if !ok {
		return nil, nil, gymaccount.Session{}, pilot.ErrHostedRecoveryUnavailable
	}
	return child, api, session, nil
}
func (c *PoolController) StartHostedRecovery(ctx context.Context, view string) error {
	child, _, session, e := c.hostedHealthy(view)
	if e != nil {
		return e
	}
	if e = c.withView(ctx, session, view, child, func(_ gymaccount.Account, check func() error) error { return check() }); e != nil {
		return e
	}
	return child.Start()
}
func (c *PoolController) StopHostedRecovery(ctx context.Context, view string) error {
	child, _, session, e := c.hostedHealthy(view)
	if e != nil {
		return e
	}
	child.Stop()
	return c.withView(ctx, session, view, child, func(_ gymaccount.Account, check func() error) error { return check() })
}
func (c *PoolController) hostedReceive(ctx context.Context, command, view, action, operation string) (pilot.HostedReceiveJob, error) {
	child, api, session, e := c.hostedHealthy(view)
	if e != nil {
		return pilot.HostedReceiveJob{}, e
	}
	var job pilot.HostedReceiveJob
	switch command {
	case "begin":
		job, e = api.BeginHostedReceive(ctx, view, action, operation)
	case "lookup":
		job, e = api.LookupHostedReceive(ctx, view, action, operation)
	case "resume":
		job, e = api.ResumeHostedReceive(ctx, view, action, operation)
	case "redisplay":
		job, e = api.RedisplayHostedReceive(ctx, view, action, operation)
	case "cancel":
		job, e = api.CancelHostedReceive(ctx, view, action, operation)
	}
	if e != nil {
		return pilot.HostedReceiveJob{}, e
	}
	e = c.withView(ctx, session, view, child, func(_ gymaccount.Account, check func() error) error {
		if job.ViewID != view || job.ActionID != action || job.OperationID != operation {
			return gymaccount.ErrUnauthorized
		}
		return check()
	})
	if e != nil {
		return pilot.HostedReceiveJob{}, e
	}
	return job, nil
}
func (c *PoolController) BeginHostedReceive(ctx context.Context, v, a, o string) (pilot.HostedReceiveJob, error) {
	return c.hostedReceive(ctx, "begin", v, a, o)
}
func (c *PoolController) LookupHostedReceive(ctx context.Context, v, a, o string) (pilot.HostedReceiveJob, error) {
	return c.hostedReceive(ctx, "lookup", v, a, o)
}
func (c *PoolController) ResumeHostedReceive(ctx context.Context, v, a, o string) (pilot.HostedReceiveJob, error) {
	return c.hostedReceive(ctx, "resume", v, a, o)
}
func (c *PoolController) RedisplayHostedReceive(ctx context.Context, v, a, o string) (pilot.HostedReceiveJob, error) {
	return c.hostedReceive(ctx, "redisplay", v, a, o)
}
func (c *PoolController) CancelHostedReceive(ctx context.Context, v, a, o string) (pilot.HostedReceiveJob, error) {
	return c.hostedReceive(ctx, "cancel", v, a, o)
}
func (c *PoolController) ExportHostedRecovery(ctx context.Context, kind pilot.HostedOwnerKind, view string, pin recoveryfile.Pin) error {
	if kind == pilot.HostedHealthyOwner {
		child, _, _, e := c.hostedHealthy(view)
		if e != nil {
			return e
		}
		return child.ExportPoolRecovery(ctx, pin)
	}
	if kind != pilot.HostedMissingOwner {
		return pilot.ErrHostedStaleView
	}
	owner, e := c.activeRecovery(view)
	if e != nil {
		return e
	}
	return owner.Export(ctx, pin)
}
func (c *PoolController) VerifyHostedBackup(ctx context.Context, kind pilot.HostedOwnerKind, view string, pin recoveryfile.Pin) error {
	if kind == pilot.HostedHealthyOwner {
		child, _, _, e := c.hostedHealthy(view)
		if e != nil {
			return e
		}
		return child.VerifyPoolBackup(ctx, pin)
	}
	if kind != pilot.HostedMissingOwner {
		return pilot.ErrHostedStaleView
	}
	owner, e := c.activeRecovery(view)
	if e != nil {
		return e
	}
	return owner.Verify(ctx, pin)
}
func (c *PoolController) CancelHostedBackup(ctx context.Context, kind pilot.HostedOwnerKind, view string) error {
	if kind == pilot.HostedHealthyOwner {
		child, _, session, e := c.hostedHealthy(view)
		if e != nil {
			return e
		}
		child.CancelPoolBackup()
		return c.withView(ctx, session, view, child, func(_ gymaccount.Account, check func() error) error { return check() })
	}
	if kind != pilot.HostedMissingOwner {
		return pilot.ErrHostedStaleView
	}
	owner, e := c.activeRecovery(view)
	if e != nil {
		return e
	}
	return owner.CancelBackup(ctx)
}
func (c *PoolController) installRecoverySession(ctx context.Context, fixture string, session gymaccount.Session) (resultErr error) {
	view, e := hostedRandomID(rand.Reader)
	if e != nil {
		_ = c.store.Logout(context.Background(), session.Token)
		return e
	}
	c.mu.Lock()
	c.session, c.mining, c.viewID, c.fixture = session, gymaccount.Session{}, view, fixture
	c.mu.Unlock()
	var owner *pilot.HostedRecoveryOwner
	defer func() {
		if resultErr != nil {
			c.mu.Lock()
			c.viewID = ""
			c.hosted.allocation.recovery = nil
			c.session = gymaccount.Session{}
			c.mu.Unlock()
			resultErr = errors.Join(resultErr, c.cleanupRecovery(context.Background(), session.Token, owner))
		}
	}()
	fence := c.recoveryEpoch(session, view, func() *pilot.HostedRecoveryOwner { return owner })
	scope, e := c.hosted.allocation.setup.ExpectedScope(fixture)
	if e != nil {
		return e
	}
	owner, e = pilot.NewHostedRecoveryOwner(c.config.DataDir, c.store, c.hosted.vault, c.hosted.allocation.setup, c.hosted.allocation.archive, fixture, session, view, scope, fence)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.hosted.allocation.recovery = owner
	c.mu.Unlock()
	if e = owner.Prepare(ctx); e != nil {
		return e
	}
	return c.withRecoveryOwner(ctx, session, view, owner, gymaccount.View, func(_ gymaccount.Account, check func() error) error {
		// recoveryEpoch already holds the short parent publication lock.
		c.state = "signed_in"
		c.expiryDone = make(chan struct{})
		done := c.expiryDone
		c.expiry = time.AfterFunc(time.Until(time.UnixMilli(session.ExpiresAtMS)), func() { defer close(done); c.expireRecoveryView(view, owner) })
		return check()
	})
}
func (c *PoolController) cleanupRecovery(ctx context.Context, token string, owner *pilot.HostedRecoveryOwner) error {
	var e error
	if token != "" {
		e = c.store.Logout(ctx, token)
	}
	if owner != nil {
		closeErr := owner.Close()
		e = errors.Join(e, closeErr)
		if closeErr != nil {
			c.mu.Lock()
			c.hosted.allocation.failedRecovery = owner
			c.mu.Unlock()
		}
	}
	return c.noteCleanup(e)
}
func (c *PoolController) expireRecoveryView(view string, owner *pilot.HostedRecoveryOwner) {
	c.mu.Lock()
	if c.closed || c.operation || c.viewID != view || c.recoveryOwner() != owner {
		c.mu.Unlock()
		return
	}
	token := c.session.Token
	c.viewID = ""
	c.hosted.allocation.recovery = nil
	c.session = gymaccount.Session{}
	c.state = "expired"
	c.operation = true
	done := make(chan struct{})
	c.done = done
	c.mu.Unlock()
	_ = c.cleanupRecovery(context.Background(), token, owner)
	c.mu.Lock()
	c.operation = false
	close(done)
	c.mu.Unlock()
}
