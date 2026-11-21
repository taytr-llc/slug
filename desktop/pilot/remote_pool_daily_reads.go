// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

type remoteDailyPayoutOperation struct {
	job       RemoteDailyPayoutJob
	binding   RemoteDailyPayoutBinding
	previous  *RemoteDailyPayoutCapture
	request   poolsettle.DailyPayoutRequest
	cached    *poolsettle.DailyPayoutPage
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	lease     *remotePoolLease
	ownLease  bool
	cancelled bool // a.mu
}

// No account lease spans HTTP. The same original local View is reacquired for
// admission and publication, including its retained parent epoch and recipient.
func (a *RemotePoolAttachment) dailyPayoutFence(ctx context.Context, fn func() error) error {
	if e := a.check(ctx); e != nil {
		return e
	}
	return a.local.withView(ctx, func(_ gymaccount.Account, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.closing || a.life.Err() != nil {
			return ErrRemotePoolUnavailable
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		return fn()
	})
}

// BeginDailyPayoutRead admits one native read job. sequence must be exactly the
// next original-owner sequence, or an exact repeat of the retained last job.
// captureSequence/beforeBatch are both zero for a new first-page/balance capture;
// otherwise they select the exact retained capture and continuation window.
// Caller timeout after admission does not create a replacement operation.
func (a *RemotePoolAttachment) BeginDailyPayoutRead(ctx context.Context, view string, sequence, captureSequence, beforeBatch uint64) (RemoteDailyPayoutJob, error) {
	if wire.TestPoW {
		return RemoteDailyPayoutJob{}, ErrRemotePoolProfile
	}
	if a == nil || a.config.Version != 2 {
		return RemoteDailyPayoutJob{}, ErrRemotePoolConfig
	}
	if sequence == 0 || (captureSequence == 0) != (beforeBatch == 0) {
		return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteDailyPayoutJob{}, e
	}
	defer end()
	binding, e := a.dailyPayoutBinding()
	if e != nil {
		return RemoteDailyPayoutJob{}, e
	}
	var repeated RemoteDailyPayoutJob
	e = a.dailyPayoutFence(work, func() error {
		var err error
		repeated, err = a.selectDailyPayoutReadLocked(sequence, captureSequence, beforeBatch)
		return err
	})
	if e != nil {
		return RemoteDailyPayoutJob{}, e
	}
	if repeated.Sequence != 0 {
		return repeated, nil
	}
	life, cancel := context.WithTimeout(a.life, 30*time.Second)
	op := &remoteDailyPayoutOperation{job: RemoteDailyPayoutJob{Sequence: sequence, CaptureSequence: captureSequence, BeforeBatch: beforeBatch, State: "working"}, binding: binding, ctx: life, cancel: cancel, done: make(chan struct{})}
	op.lease, op.ownLease, e = a.credentials.dailyReadLease(life)
	if e != nil {
		cancel()
		return RemoteDailyPayoutJob{}, e
	}
	registered := false
	priorJob := RemoteDailyPayoutJob{}
	e = op.lease.dailyReadCheck(work)
	if e == nil {
		e = a.dailyPayoutFence(work, func() error {
			if a.dailyRead != nil || sequence != a.dailyReadSequence+1 {
				return ErrRemotePoolBusy
			}
			op.previous = a.dailyPayout
			if captureSequence != 0 {
				var err error
				op.request, op.cached, err = dailyPayoutContinuation(op.previous, captureSequence, beforeBatch)
				if err != nil {
					return err
				}
				if op.previous.Binding != binding {
					return ErrRemotePoolChanged
				}
			}
			priorJob = a.dailyReadLast
			a.dailyRead, a.dailyReadSequence = op, sequence
			a.active.Add(1)
			registered = true
			return nil
		})
	}
	if e != nil {
		cancel()
		if op.ownLease {
			e = errors.Join(e, op.lease.Close())
		}
		if registered {
			a.mu.Lock()
			if a.dailyRead == op {
				a.dailyRead = nil
				a.dailyReadSequence = sequence - 1
				a.dailyReadLast = priorJob
			}
			close(op.done)
			a.mu.Unlock()
			a.active.Done()
		}
		return RemoteDailyPayoutJob{}, e
	}
	go a.runDailyPayoutRead(op)
	return op.job, nil
}

// Caller holds a.mu; this selects correlation only, never admits an owner.
func (a *RemotePoolAttachment) selectDailyPayoutReadLocked(sequence, captureSequence, beforeBatch uint64) (RemoteDailyPayoutJob, error) {
	j := a.dailyReadLast
	if a.dailyRead != nil {
		j = a.dailyRead.job
	}
	if j.Sequence == sequence && sequence != 0 {
		if j.CaptureSequence != captureSequence || j.BeforeBatch != beforeBatch {
			return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
		}
		return j, nil
	}
	if a.dailyRead != nil {
		return RemoteDailyPayoutJob{}, ErrRemotePoolBusy
	}
	if sequence == 0 || a.dailyReadSequence == math.MaxUint64 || sequence != a.dailyReadSequence+1 {
		return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
	}
	return RemoteDailyPayoutJob{}, nil
}

func dailyPayoutReadError(e error) string {
	if e == nil {
		return ""
	}
	if errors.Is(e, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "unavailable"
	}
	if errors.Is(e, ErrRemotePoolBusy) || errors.Is(e, errRemotePoolCredentialBusy) {
		return "busy"
	}
	if e == poolsettle.ErrStaleCapture {
		return "stale_capture"
	}
	var remote *poolservice.ClientError
	if errors.As(e, &remote) && !remote.OutcomeUnknown {
		switch remote.Code {
		case "busy", "rate_limited", "stale_capture", "unauthorized", "unavailable":
			return remote.Code
		}
	}
	return "unavailable"
}

func (a *RemotePoolAttachment) runDailyPayoutRead(op *remoteDailyPayoutOperation) {
	var next *RemoteDailyPayoutCapture
	var p poolsettle.DailyPayoutPage
	var e error
	if op.cached != nil {
		p = cloneDailyPayoutPage(*op.cached)
		e = op.lease.dailyReadCheck(op.ctx)
		if e == nil {
			next = cloneDailyPayoutCapture(op.previous)
		}
	} else {
		p, e = op.lease.dailyPayoutPage(op.ctx, op.request)
		if e == nil {
			if op.job.CaptureSequence == 0 {
				next, e = freshDailyPayoutCapture(op.job.Sequence, op.binding, time.Now().UnixMilli(), p)
			} else {
				next, e = extendDailyPayoutCapture(op.previous, op.request, p)
			}
		}
	}
	stale := dailyPayoutReadError(e) == "stale_capture"
	// A stale reply labels only this same existing capture; it never erases its
	// dated pages or combines them with a newer revision.
	if stale && op.job.CaptureSequence != 0 && op.previous != nil {
		next = cloneDailyPayoutCapture(op.previous)
		next.Stale = true
	}
	if e == nil || stale {
		fenceErr := op.lease.dailyReadCheck(op.ctx)
		if fenceErr == nil {
			fenceErr = a.dailyPayoutFence(op.ctx, func() error {
				if a.dailyRead != op || a.dailyPayout != op.previous || op.cancelled {
					return ErrRemotePoolChanged
				}
				return nil
			})
		}
		if fenceErr != nil {
			e, next = errors.Join(e, fenceErr), nil
		}
	}
	if op.ownLease {
		if ce := op.lease.Close(); ce != nil {
			e, next = errors.Join(e, ce), nil
		}
	}
	// Publication is not a new authority. The final original View fence above
	// must complete first; every later disclosure takes the same fence again.
	a.mu.Lock()
	if a.dailyRead == op {
		job := op.job
		job.State, job.Error = "done", ""
		if e != nil {
			job.State, job.Error = "failed", dailyPayoutReadError(e)
		}
		if op.ctx.Err() != nil {
			next = nil
			job.State, job.Error = "failed", dailyPayoutReadError(op.ctx.Err())
		}
		if op.cancelled || a.closing || a.life.Err() != nil {
			next = nil
			job.State, job.Error = "cancelled", ""
		}
		if next != nil && a.dailyPayout == op.previous {
			a.dailyPayout = next
		}
		a.dailyReadLast, a.dailyRead = job, nil
	}
	op.previous, op.cached = nil, nil
	op.cancel()
	close(op.done)
	a.mu.Unlock()
	a.active.Done()
}

// DailyPayoutRead discloses only the exact retained job and a separately named
// completed capture. It does not silently return an unrelated latest job.
func (a *RemotePoolAttachment) DailyPayoutRead(ctx context.Context, view string, sequence uint64) (RemoteDailyPayoutRead, error) {
	if wire.TestPoW {
		return RemoteDailyPayoutRead{}, ErrRemotePoolProfile
	}
	if a == nil || a.config.Version != 2 {
		return RemoteDailyPayoutRead{}, ErrRemotePoolConfig
	}
	if sequence == 0 {
		return RemoteDailyPayoutRead{}, ErrDailyPayoutReadInvalid
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteDailyPayoutRead{}, e
	}
	defer end()
	l, own, e := a.credentials.dailyReadLease(work)
	if e != nil {
		return RemoteDailyPayoutRead{}, e
	}
	if own {
		defer l.Close()
	}
	if e = l.dailyReadCheck(work); e != nil {
		return RemoteDailyPayoutRead{}, e
	}
	var out RemoteDailyPayoutRead
	e = a.dailyPayoutFence(work, func() error {
		j := a.dailyReadLast
		if a.dailyRead != nil {
			j = a.dailyRead.job
		}
		if j.Sequence != sequence {
			return ErrDailyPayoutReadNotFound
		}
		out = RemoteDailyPayoutRead{Job: j, Capture: cloneDailyPayoutCapture(a.dailyPayout)}
		return nil
	})
	if e != nil {
		return RemoteDailyPayoutRead{}, e
	}
	return out, nil
}

func (a *RemotePoolAttachment) CancelDailyPayoutRead(ctx context.Context, view string, sequence uint64) error {
	if wire.TestPoW {
		return ErrRemotePoolProfile
	}
	if a == nil || a.config.Version != 2 {
		return ErrRemotePoolConfig
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return e
	}
	defer end()
	var op *remoteDailyPayoutOperation
	e = a.dailyPayoutFence(work, func() error {
		if a.dailyRead != nil && a.dailyRead.job.Sequence == sequence {
			op = a.dailyRead
			op.cancelled = true
			op.cancel()
			return nil
		}
		if sequence == 0 || a.dailyReadLast.Sequence != sequence {
			return ErrDailyPayoutReadNotFound
		}
		return nil
	})
	if op != nil {
		<-op.done
	}
	return errors.Join(e, a.check(work))
}

func (a *RemotePoolAttachment) cancelDailyPayoutRead() {
	a.mu.Lock()
	op := a.dailyRead
	if op != nil {
		op.cancelled = true
		op.cancel()
	}
	a.mu.Unlock()
	if op != nil {
		<-op.done
	}
}
