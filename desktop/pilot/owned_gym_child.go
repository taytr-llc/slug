// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectworker"
)

// OwnedGymChild retains one concrete login and its original parent fence. The
// owned stable/event family is never installed in a legacy StableController.
// Construction must complete before the parent publishes this child's View.
type OwnedGymChild struct {
	account        StableAccountConfig
	view           OwnedGymView
	scope          OwnedGymScope
	runtime        *PoolRuntime
	wallet         *OwnedWalletSession
	league         *OwnedLeague
	stable         *gymstable.OwnedStore
	entries        *gymentry.OwnedStore
	events         *gymevent.OwnedStore
	transfers      *OwnedTransferActions
	authority      *PoolSessionAuthority
	admission      *poolChildAdmission
	box            *prospectworker.PoolOutbox
	workerConfig   prospectworker.PoolConfig
	node           *prospectnode.Client
	rpc            prospectnode.RPCConfig
	config         Config
	life           context.Context
	cancel         context.CancelFunc
	stopRuntime    func() bool
	release        func()
	mu             sync.Mutex
	opMu           sync.Mutex
	closed         atomic.Bool
	closeOnce      sync.Once
	closeErr       error
	run            *poolChildRun
	refreshDone    chan struct{}
	snapshot       OwnedGymChildSnapshot
	capture        *gymstable.OwnedRead
	captureID      string
	captureValue   OwnedStableCapture
	history        *gymstable.OwnedCareerHistoryRead
	historyID      string
	backup         *poolBackupOperation
	backupCloseErr error
}

func (c *OwnedGymChild) session() gymstable.Session {
	return gymstable.Session{Generation: c.view.Generation, ViewID: c.view.ViewID, Lease: c.account.Lease}
}

// The lifecycle gate is acquired outside every account fence. Close cancels
// admitted work first, then joins this gate before releasing any borrowed owner.
func (c *OwnedGymChild) begin(ctx context.Context, view string) (context.Context, func(), error) {
	if c == nil || ctx == nil {
		return nil, nil, gymstable.ErrInvalid
	}
	if view != c.view.ViewID || !payoutNonzeroID(view) {
		return nil, nil, gymstable.ErrStaleView
	}
	if !c.opMu.TryLock() {
		return nil, nil, gymstable.ErrBusy
	}
	if c.closed.Load() || c.life.Err() != nil {
		c.opMu.Unlock()
		return nil, nil, gymstable.ErrClosed
	}
	c.mu.Lock()
	busy := c.backup != nil || c.backupCloseErr != nil
	c.mu.Unlock()
	if busy {
		c.opMu.Unlock()
		return nil, nil, gymstable.ErrBusy
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(c.life, cancel)
	done := func() { stop(); cancel(); c.opMu.Unlock() }
	if e := c.fence(work, func() error { return nil }); e != nil {
		done()
		return nil, nil, e
	}
	return work, done, nil
}

func (c *OwnedGymChild) fence(ctx context.Context, fn func() error) error {
	return c.capabilityFence(ctx, gymaccount.View, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return check()
	})
}

// No mutable wallet/source work is permitted inside fn. The original account
// callback may only publish already prepared bounded metadata. Malformed outer
// callbacks cannot swallow a refusal or disclose a successful result.
func (c *OwnedGymChild) capabilityFence(ctx context.Context, cap gymaccount.Capability, fn func(func() error) error) error {
	if ctx == nil || c == nil || c.account.Lease == nil || fn == nil {
		return gymaccount.ErrUnauthorized
	}
	var callback sync.Mutex
	var calls atomic.Uint32
	active := true
	var inner error
	outer := c.account.Lease(ctx, cap, func(a gymaccount.Account, recheck func() error) error {
		calls.Add(1)
		if !callback.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer callback.Unlock()
		if !active || calls.Load() != 1 || recheck == nil {
			inner = gymaccount.ErrUnauthorized
			return inner
		}
		check := func() error {
			if ctx.Err() != nil || c.life.Err() != nil || c.closed.Load() {
				return gymaccount.ErrUnauthorized
			}
			if a.ID != c.view.AccountID || a.Generation != c.view.Generation || a.RecoveryRequired || a.Enrollment == nil || a.Enrollment.RootPublicKey != c.view.RootPublicKey || a.Enrollment.GymPublicKey != c.view.GymPublicKey || a.Enrollment.GymID != c.view.GymID {
				return gymaccount.ErrUnauthorized
			}
			return recheck()
		}
		if inner = check(); inner != nil {
			return inner
		}
		if inner = fn(check); inner != nil {
			return inner
		}
		inner = check()
		return inner
	})
	callback.Lock()
	active = false
	e := inner
	callback.Unlock()
	if calls.Load() != 1 {
		return errors.Join(gymaccount.ErrUnauthorized, outer, e)
	}
	return errors.Join(outer, e)
}

func (c *OwnedGymChild) Snapshot() OwnedGymChildSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.snapshot
	if v.OwnershipHead != nil {
		head := *v.OwnershipHead
		v.OwnershipHead = &head
	}
	return v
}

func (c *OwnedGymChild) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.cancel != nil {
			c.cancel()
		}
		// Stop worker and pending backup outside the action gate; either may need
		// to finish a callback before the gate can be joined.
		c.mu.Lock()
		run, backup := c.run, c.backup
		c.mu.Unlock()
		if run != nil {
			run.cancel()
			<-run.done
		}
		if backup != nil {
			backup.cancel()
			<-backup.done
		}
		c.opMu.Lock()
		defer c.opMu.Unlock()
		if c.refreshDone != nil {
			<-c.refreshDone
		}
		c.closeErr = errors.Join(c.closeErr, c.backupCloseErr)
		if c.transfers != nil {
			c.closeErr = errors.Join(c.closeErr, c.transfers.Close())
		}
		if c.admission != nil {
			c.closeErr = errors.Join(c.closeErr, c.admission.Close())
		}
		if c.authority != nil {
			c.closeErr = errors.Join(c.closeErr, c.authority.Close())
		}
		if c.box != nil {
			c.closeErr = errors.Join(c.closeErr, c.box.Close())
		}
		if c.wallet != nil {
			c.closeErr = errors.Join(c.closeErr, c.wallet.Close())
		}
		c.capture = nil
		c.captureValue = OwnedStableCapture{}
		c.history = nil
		if c.stopRuntime != nil {
			c.stopRuntime()
		}
		// Cleanup failure keeps this concrete child registered with the parent. It
		// cannot silently free the wallet/runtime namespace for a replacement.
		if c.closeErr == nil && c.release != nil {
			c.release()
		}
	})
	return c.closeErr
}

// Activate is called after the parent has published this exact child and View.
// Keeping construction synchronous avoids reading a provisional callback closure
// concurrently with its first child assignment.
func (c *OwnedGymChild) Activate(ctx context.Context) error {
	work, done, e := c.begin(ctx, c.view.ViewID)
	if e != nil {
		return e
	}
	defer done()
	var started chan struct{}
	e = c.fence(work, func() error {
		if !c.mu.TryLock() {
			return gymstable.ErrBusy
		}
		defer c.mu.Unlock()
		if c.refreshDone != nil {
			return nil
		}
		started = make(chan struct{})
		c.refreshDone = started
		return nil
	})
	if e != nil {
		if started != nil {
			close(started)
		}
		return e
	}
	if started != nil {
		go c.refreshLoop()
	}
	return nil
}
