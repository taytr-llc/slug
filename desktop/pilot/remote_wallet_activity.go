// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

// Native projections carry the original DAILY binding, never a fabricated
// legacy ManifestViewPin. A later failed job keeps a separately named capture.
type RemoteWalletActivityJob struct {
	Sequence     uint64
	State, Error string
}
type RemoteWalletActivityCapture struct {
	Sequence uint64
	Binding  RemoteDailyPayoutBinding
	Metadata WalletActivityCapture
}
type RemoteWalletActivityRead struct {
	Job     RemoteWalletActivityJob
	Capture *RemoteWalletActivityCapture
}
type RemoteWalletActivityPage struct {
	Capture            RemoteWalletActivityCapture
	After, Limit, Next uint32
	HasMore            bool
	Items              []WalletActivityItem
}
type RemoteWalletActivityDetail struct {
	Capture RemoteWalletActivityCapture
	Item    WalletActivityItem
	Outputs []WalletActivityOutput
}
type remoteWalletActivityCache struct {
	capture RemoteWalletActivityCapture
	payload *walletActivityCache
}
type remoteWalletActivityOperation struct {
	job       RemoteWalletActivityJob
	binding   RemoteDailyPayoutBinding
	previous  *remoteWalletActivityCache
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	cancelled bool // a.mu
}

func cloneRemoteWalletActivityCapture(c *remoteWalletActivityCache) *RemoteWalletActivityCapture {
	if c == nil {
		return nil
	}
	x := c.capture
	return &x
}
func (a *RemotePoolAttachment) activitySelected() error {
	if wire.TestPoW {
		return ErrRemotePoolProfile
	}
	if a == nil || a.config.Version != 2 {
		return ErrRemotePoolConfig
	}
	return nil
}

// Caller holds a.mu. A repeated original sequence only recovers its job; it
// never dispatches a second capture. The two async read families are independent.
func (a *RemotePoolAttachment) selectWalletActivityLocked(sequence uint64) (RemoteWalletActivityJob, error) {
	j := a.ownActivityLast
	if a.ownActivity != nil {
		j = a.ownActivity.job
	}
	if sequence != 0 && j.Sequence == sequence {
		return j, nil
	}
	if a.ownActivity != nil {
		return RemoteWalletActivityJob{}, ErrRemotePoolBusy
	}
	if sequence == 0 || a.ownActivitySequence == math.MaxUint64 || sequence != a.ownActivitySequence+1 {
		return RemoteWalletActivityJob{}, ErrDailyPayoutReadInvalid
	}
	return RemoteWalletActivityJob{}, nil
}
func (a *RemotePoolAttachment) BeginWalletActivityRead(ctx context.Context, view string, sequence uint64) (RemoteWalletActivityJob, error) {
	if e := a.activitySelected(); e != nil {
		return RemoteWalletActivityJob{}, e
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteWalletActivityJob{}, e
	}
	defer end()
	binding, e := a.dailyPayoutBinding()
	if e != nil {
		return RemoteWalletActivityJob{}, e
	}
	life, cancel := context.WithTimeout(a.life, 30*time.Second)
	op := &remoteWalletActivityOperation{job: RemoteWalletActivityJob{Sequence: sequence, State: "working"}, binding: binding, ctx: life, cancel: cancel, done: make(chan struct{})}
	var repeated, prior RemoteWalletActivityJob
	registered := false
	e = a.dailyPayoutFence(work, func() error {
		var err error
		repeated, err = a.selectWalletActivityLocked(sequence)
		if err != nil || repeated.Sequence != 0 {
			return err
		}
		op.previous, prior = a.ownActivityCache, a.ownActivityLast
		a.ownActivity, a.ownActivitySequence = op, sequence
		a.active.Add(1)
		registered = true
		return nil
	})
	if e != nil || !registered {
		cancel()
		if registered {
			a.mu.Lock()
			if a.ownActivity == op {
				a.ownActivity = nil
				a.ownActivitySequence = sequence - 1
				a.ownActivityLast = prior
			}
			close(op.done)
			a.mu.Unlock()
			a.active.Done()
		}
		if e != nil {
			return RemoteWalletActivityJob{}, e
		}
		return repeated, nil
	}
	go a.runWalletActivityRead(op)
	return op.job, nil
}
func (a *RemotePoolAttachment) runWalletActivityRead(op *remoteWalletActivityOperation) {
	source, e := a.local.session.runtime.captureWalletActivity(op.ctx, a.activityRPC)
	var next *remoteWalletActivityCache
	if e == nil {
		identity := walletIdentity{RootPublicKey: op.binding.View.RootPublicKey, GymPublicKey: op.binding.View.GymPublicKey, GymID: op.binding.View.GymID}
		var payload *walletActivityCache
		var meta WalletActivityCapture
		payload, meta, e = projectWalletActivityCapture(op.ctx, identity, source)
		if e == nil {
			next = &remoteWalletActivityCache{capture: RemoteWalletActivityCapture{Sequence: op.job.Sequence, Binding: op.binding, Metadata: meta}, payload: payload}
		}
	}
	source = walletActivityRead{}
	// Complete the original outer View recheck before making next observable.
	if e == nil {
		e = a.dailyPayoutFence(op.ctx, func() error {
			if a.ownActivity != op || a.ownActivityCache != op.previous || op.cancelled {
				return ErrRemotePoolChanged
			}
			return nil
		})
	}
	a.finishWalletActivityRead(op, next, e)
}
func (a *RemotePoolAttachment) finishWalletActivityRead(op *remoteWalletActivityOperation, next *remoteWalletActivityCache, e error) {
	a.mu.Lock()
	if a.ownActivity == op {
		j := op.job
		j.State, j.Error = "done", ""
		if e != nil {
			next = nil
			j.State, j.Error = "failed", walletActivityTaskError(e)
		}
		if op.ctx.Err() != nil {
			next = nil
			j.State, j.Error = "failed", "activity_unavailable"
		}
		if op.cancelled || a.closing || a.life.Err() != nil {
			next = nil
			j.State, j.Error = "cancelled", ""
		}
		if next != nil && a.ownActivityCache == op.previous {
			a.ownActivityCache = next
		}
		a.ownActivityLast, a.ownActivity = j, nil
	}
	op.previous = nil
	op.cancel()
	close(op.done)
	a.mu.Unlock()
	a.active.Done()
}
func (a *RemotePoolAttachment) WalletActivityRead(ctx context.Context, view string, sequence uint64) (RemoteWalletActivityRead, error) {
	if e := a.activitySelected(); e != nil {
		return RemoteWalletActivityRead{}, e
	}
	if sequence == 0 {
		return RemoteWalletActivityRead{}, ErrDailyPayoutReadInvalid
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteWalletActivityRead{}, e
	}
	defer end()
	var out RemoteWalletActivityRead
	e = a.dailyPayoutFence(work, func() error {
		j := a.ownActivityLast
		if a.ownActivity != nil {
			j = a.ownActivity.job
		}
		if j.Sequence != sequence {
			return ErrActivityNotFound
		}
		out = RemoteWalletActivityRead{Job: j, Capture: cloneRemoteWalletActivityCapture(a.ownActivityCache)}
		return nil
	})
	if e != nil {
		return RemoteWalletActivityRead{}, e
	}
	return out, nil
}
func (a *RemotePoolAttachment) CancelWalletActivityRead(ctx context.Context, view string, sequence uint64) error {
	if e := a.activitySelected(); e != nil {
		return e
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return e
	}
	defer end()
	var op *remoteWalletActivityOperation
	e = a.dailyPayoutFence(work, func() error {
		if a.ownActivity != nil && a.ownActivity.job.Sequence == sequence {
			op = a.ownActivity
			op.cancelled = true
			op.cancel()
			return nil
		}
		if sequence == 0 || a.ownActivityLast.Sequence != sequence {
			return ErrActivityNotFound
		}
		return nil
	})
	if op != nil {
		<-op.done
	}
	return errors.Join(e, a.check(work))
}
func (a *RemotePoolAttachment) cancelWalletActivityRead() {
	a.mu.Lock()
	op := a.ownActivity
	if op != nil {
		op.cancelled = true
		op.cancel()
	}
	a.mu.Unlock()
	if op != nil {
		<-op.done
	}
}
func remoteActivityPage(c *remoteWalletActivityCache, sequence uint64, after, limit uint32) (RemoteWalletActivityPage, error) {
	if c == nil || sequence == 0 || c.capture.Sequence != sequence {
		return RemoteWalletActivityPage{}, ErrActivityNotFound
	}
	if c.payload == nil || after > c.capture.Metadata.Count || limit == 0 || limit > 8 || len(c.payload.items) != int(c.capture.Metadata.Count) {
		return RemoteWalletActivityPage{}, ErrDailyPayoutReadInvalid
	}
	end := after + limit
	if end > c.capture.Metadata.Count {
		end = c.capture.Metadata.Count
	}
	out := RemoteWalletActivityPage{Capture: c.capture, After: after, Limit: limit, Next: end, HasMore: end < c.capture.Metadata.Count, Items: make([]WalletActivityItem, 0, end-after)}
	for _, item := range c.payload.items[after:end] {
		out.Items = append(out.Items, CloneWalletActivityItem(item))
	}
	return out, nil
}
func (a *RemotePoolAttachment) WalletActivityPage(ctx context.Context, view string, sequence uint64, after, limit uint32) (RemoteWalletActivityPage, error) {
	if e := a.activitySelected(); e != nil {
		return RemoteWalletActivityPage{}, e
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteWalletActivityPage{}, e
	}
	defer end()
	var out RemoteWalletActivityPage
	e = a.dailyPayoutFence(work, func() error {
		var x error
		out, x = remoteActivityPage(a.ownActivityCache, sequence, after, limit)
		return x
	})
	if e != nil {
		return RemoteWalletActivityPage{}, e
	}
	return out, nil
}
func (a *RemotePoolAttachment) WalletActivityDetail(ctx context.Context, view string, sequence uint64, txid string) (RemoteWalletActivityDetail, error) {
	if e := a.activitySelected(); e != nil {
		return RemoteWalletActivityDetail{}, e
	}
	if !payoutLowerHex(txid, 64, false) {
		return RemoteWalletActivityDetail{}, ErrDailyPayoutReadInvalid
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteWalletActivityDetail{}, e
	}
	defer end()
	var cache *remoteWalletActivityCache
	index := -1
	e = a.dailyPayoutFence(work, func() error {
		cache = a.ownActivityCache
		if cache == nil || sequence == 0 || cache.capture.Sequence != sequence {
			return ErrActivityNotFound
		}
		for i, item := range cache.payload.items {
			if item.TxID == txid {
				if index != -1 {
					return ErrActivityUnavailable
				}
				index = i
			}
		}
		if index < 0 {
			return ErrActivityNotFound
		}
		return nil
	})
	if e != nil {
		return RemoteWalletActivityDetail{}, e
	}
	item, outputs, e := projectWalletActivity(work, cache.payload.records[index], cache.capture.Metadata, true)
	if e != nil {
		return RemoteWalletActivityDetail{}, e
	}
	if !WalletActivityItemEqual(item, cache.payload.items[index]) {
		return RemoteWalletActivityDetail{}, ErrActivityUnavailable
	}
	e = a.dailyPayoutFence(work, func() error {
		if a.ownActivityCache != cache {
			return ErrActivityNotFound
		}
		return nil
	})
	if e != nil {
		return RemoteWalletActivityDetail{}, e
	}
	return RemoteWalletActivityDetail{Capture: cache.capture, Item: item, Outputs: outputs}, nil
}
