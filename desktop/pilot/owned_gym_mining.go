// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectworker"
	"math"
	"strings"
	"time"
)

func (c *OwnedGymChild) Start(ctx context.Context, view string) error {
	work, done, e := c.begin(ctx, view)
	if e != nil {
		return e
	}
	defer done()
	runctx, cancel := context.WithCancel(c.life)
	var run *poolChildRun
	// Start publishes only the original login's run state. The worker and
	// admission signer independently capture and check the separate mining token.
	e = c.capabilityFence(work, gymaccount.View, func(check func() error) error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if c.run != nil {
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		run = &poolChildRun{cancel: cancel, done: make(chan struct{}), base: c.snapshot.Attempts}
		c.run = run
		c.snapshot.State = "starting"
		c.snapshot.LastError = ""
		return nil
	})
	if e != nil || run == nil {
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
		return e
	}
	go c.runWorker(runctx, run)
	return nil
}
func (c *OwnedGymChild) Stop(ctx context.Context, view string) error {
	if c == nil || ctx == nil || view != c.view.ViewID {
		return gymstable.ErrStaleView
	}
	if !c.opMu.TryLock() {
		return gymstable.ErrBusy
	}
	defer c.opMu.Unlock()
	var run *poolChildRun
	var backup *poolBackupOperation
	if e := c.fence(ctx, func() error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		run = c.run
		backup = c.backup
		return nil
	}); e != nil {
		return e
	}
	if run != nil {
		run.cancel()
	}
	if backup != nil {
		backup.cancel()
	}
	if run != nil {
		<-run.done
	}
	if backup != nil {
		<-backup.done
	}
	return c.fence(ctx, func() error { return nil })
}

// The vault is the sole hosted gym admission signer. No installed-wallet
// fallback exists, and the original mining token is checked after verification.
func (c *OwnedGymChild) signer(ctx context.Context) (prospectworker.Signer, error) {
	if c.wallet == nil || c.wallet.setup == nil || c.wallet.setup.vault == nil || c.authority.checkView(ctx) != nil {
		return nil, gymaccount.ErrUnauthorized
	}
	return func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		p := c.scope.PoolSource.Policy
		if len(proof.MerkleBranch) > 20 || proof.Version != prospect.Version2 || initial.Network != p.Network || initial.DNAVersion != p.DNAVersion || !strings.EqualFold(initial.Genesis, p.Genesis) || !strings.EqualFold(initial.ProspectTarget, p.ProspectTarget) {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		proof.MerkleBranch = append([]string(nil), proof.MerkleBranch...)
		if c.authority.checkView(ctx) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		token, e := c.account.Manifest.Account.MiningToken(ctx)
		if e != nil {
			return gymauth.Authorization{}, e
		}
		lease, e := mininglease.Capture(ctx, c.account.Manifest.Account.Authorizer, token)
		if e != nil || lease.PublicKey() != c.view.GymPublicKey {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		auth, e := c.wallet.setup.vault.SignAdmission(ctx, token, initial, proof, c.wallet.epoch)
		if e != nil {
			return gymauth.Authorization{}, e
		}
		if auth.PublicKey != c.view.GymPublicKey || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		if lease.Check() != nil || c.authority.checkView(ctx) != nil {
			return gymauth.Authorization{}, gymaccount.ErrUnauthorized
		}
		return auth, nil
	}, nil
}
func (c *OwnedGymChild) runWorker(ctx context.Context, run *poolChildRun) {
	signer, e := c.signer(ctx)
	var worker poolChildWorker
	if e == nil {
		worker, e = prospectworker.NewPool(c.workerConfig, c.authority, c.admission, c.node, signer, c.box)
	}
	if e == nil {
		c.mu.Lock()
		run.worker = worker
		c.mu.Unlock()
		e = worker.Run(ctx)
	}
	work, cancel := context.WithTimeout(c.life, 2*time.Second)
	defer cancel()
	_ = c.fence(work, func() error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if c.run == run {
			if worker != nil {
				c.copyWorkerLocked(run, worker.Status())
			}
			c.snapshot.HashesPerSecondMilli = 0
			c.snapshot.State = "stopped"
			if e != nil && ctx.Err() == nil {
				c.snapshot.State = "failed"
				c.snapshot.LastError = "worker_failed"
			}
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
func (c *OwnedGymChild) copyWorkerLocked(run *poolChildRun, s prospectworker.Status) {
	rate := s.HashesPerSecond * 1000
	if s.Attempts > gymstable.MaxSafeInteger-run.base || s.Pending > c.workerConfig.MaxRecords || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > float64(gymstable.MaxSafeInteger) {
		c.snapshot.LastError = "worker_failed"
		return
	}
	c.snapshot.Attempts = run.base + s.Attempts
	c.snapshot.Pending = s.Pending
	c.snapshot.HashesPerSecondMilli = uint64(rate)
	c.snapshot.LastError = s.LastError
	if !PoolStatusCode(c.snapshot.LastError) {
		c.snapshot.LastError = "worker_failed"
	}
	c.snapshot.State = "running"
	if s.State == "failed" {
		c.snapshot.State = "failed"
	}
}
func (c *OwnedGymChild) refreshLoop() {
	defer close(c.refreshDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	next := time.Time{}
	for {
		if c.life.Err() != nil {
			return
		}
		c.mu.Lock()
		run := c.run
		var worker poolChildWorker
		if run != nil {
			worker = run.worker
		}
		c.mu.Unlock()
		if worker != nil {
			status := worker.Status()
			_ = c.fence(c.life, func() error {
				if !c.mu.TryLock() {
					return gymstable.ErrBusy
				}
				defer c.mu.Unlock()
				if c.run == run {
					c.copyWorkerLocked(run, status)
				}
				return nil
			})
		}
		continueReplay := false
		work, done, e := c.begin(c.life, c.view.ViewID)
		if e == nil {
			// Keep each replay slice bounded and release the original action gate
			// between slices. Only successful incomplete work requests an early
			// continuation; errors and idle work retain the one-second cadence.
			continueReplay = c.refreshOwnedEvents(work)
			if !time.Now().Before(next) && c.walletRefreshAllowed() {
				c.refreshWallet(work)
				next = time.Now().Add(5 * time.Second)
			}
			done()
		}
		if !ownedWaitRefresh(c.life, ticker.C, continueReplay) {
			return
		}
	}
}

// UI operations and Close get a cancellation-aware fairness interval outside
// every child/account/runtime gate. This replaces only needless idle ticker
// waits between successful bounded slices, not any replay/session deadline.
func ownedWaitRefresh(ctx context.Context, tick <-chan time.Time, continueReplay bool) bool {
	if ctx.Err() != nil {
		return false
	}
	if continueReplay {
		timer := time.NewTimer(10 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return ctx.Err() == nil
		}
	}
	select {
	case <-ctx.Done():
		return false
	case <-tick:
		return ctx.Err() == nil
	}
}
func (c *OwnedGymChild) refreshWallet(ctx context.Context) {
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	w, we := c.wallet.runtime.Snapshot(work)
	tip, te := c.node.Resolve(work, c.scope.PoolSource.Policy)
	_ = c.fence(work, func() error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if we == nil && w.Height >= 0 && w.Spendable >= 0 && w.Immature >= 0 {
			c.snapshot.WalletHeight = uint64(w.Height)
			c.snapshot.SpendableAtoms = w.Spendable
			c.snapshot.ImmatureAtoms = w.Immature
			c.snapshot.WalletState = "syncing"
			if w.Synced {
				c.snapshot.WalletState = "ready"
			}
		} else {
			c.snapshot.WalletState = "failed"
		}
		if te == nil {
			c.snapshot.Height = uint64(tip.ParentHeight)
		}
		return nil
	})
}
