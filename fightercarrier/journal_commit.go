// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"errors"
	"sync/atomic"
)

func (j *Journal) quickCheck() error {
	if e := j.chain.check(); e != nil {
		return e
	}
	for _, v := range []struct {
		name string
		want fileStamp
	}{{journalMarkerName, j.markerStamp}, {journalStateName, j.stateStamp}} {
		got, e := relativeStamp(j.chain.dir(), v.name)
		if e != nil || got != v.want || !privateFile(got) {
			return ErrStorage
		}
	}
	return nil
}
func (p *PreparedCommit) Publish(fence Fence) (Operation, error) {
	if p == nil || p.owner == nil || p.closed || p.used || p.owner.pending != p {
		return Operation{}, ErrConflict
	}
	p.used = true
	o := p.owner
	j := o.j
	if e := o.check(); e != nil {
		return Operation{}, e
	}
	if fence == nil {
		return Operation{}, ErrUnauthorized
	}
	var calls atomic.Uint32
	var callbackState atomic.Uint32
	var sealed, failed atomic.Bool
	callbackDone := make(chan struct{})
	var inner error
	guardClock := func() error {
		if e := o.check(); e != nil {
			return e
		}
		if !p.noop {
			now := j.now()
			if !journalTime(now) || now < j.state.ClockHighWaterMS || now < p.operation.UpdatedAtMS {
				return ErrClock
			}
		}
		return nil
	}
	outer := fence(func(check func() error) error {
		calls.Add(1)
		if sealed.Load() || !callbackState.CompareAndSwap(0, 1) {
			failed.Store(true)
			return ErrUnauthorized
		}
		defer close(callbackDone)
		if calls.Load() != 1 || check == nil {
			failed.Store(true)
			return ErrUnauthorized
		}
		guard := func() error {
			if sealed.Load() {
				return ErrUnauthorized
			}
			if e := guardClock(); e != nil {
				return e
			}
			if e := check(); e != nil {
				return errors.Join(ErrUnauthorized, e)
			}
			if sealed.Load() {
				return ErrUnauthorized
			}
			return guardClock()
		}
		if e := guard(); e != nil {
			inner = e
			failed.Store(true)
			return e
		}
		if e := j.quickCheck(); e != nil {
			inner = e
			return e
		}
		if !p.noop {
			got, e := relativeStamp(j.chain.dir(), p.name)
			if e != nil || got != p.stamp || !privateFile(got) {
				inner = ErrStorage
				return inner
			}
			// A rename error may be an uncertain acknowledgment. Never infer old state.
			p.published = true
			if e = j.rename(j.chain.dir(), p.name, journalStateName); e != nil {
				inner = errors.Join(ErrOutcomeUnknown, ErrStorage)
				return inner
			}
			if e = j.syncFile(j.chain.dir()); e != nil {
				inner = errors.Join(ErrOutcomeUnknown, ErrStorage)
				return inner
			}
			got, e = relativeStamp(j.chain.dir(), journalStateName)
			if e != nil || got != p.stamp {
				inner = errors.Join(ErrOutcomeUnknown, ErrStorage)
				return inner
			}
		}
		if e := guard(); e != nil {
			inner = e
			failed.Store(true)
			return e
		}
		return nil
	})
	sealed.Store(true)
	// Reject retained callbacks after the outer return. If a broken fence ran
	// its callback asynchronously, join that admitted callback before examining
	// its outcome or releasing any file ownership. This wait is outside it.
	if !callbackState.CompareAndSwap(0, 2) {
		<-callbackDone
	}
	if calls.Load() != 1 || failed.Load() || inner != nil || outer != nil || o.check() != nil {
		e := errors.Join(inner, outer)
		if calls.Load() != 1 || failed.Load() {
			e = errors.Join(e, ErrUnauthorized)
		}
		if ce := o.check(); ce != nil {
			e = errors.Join(e, ce)
		}
		if p.published {
			j.poison = errors.Join(ErrPoisoned, ErrOutcomeUnknown, e)
			return Operation{}, j.poison
		}
		if e == nil {
			e = ErrUnauthorized
		}
		return Operation{}, e
	}
	if !p.noop {
		old := j.stateFile
		j.state = p.next
		j.stateFile = p.file
		j.stateStamp = p.stamp
		p.adopted = true
		if e := old.Close(); e != nil {
			j.poison = errors.Join(ErrPoisoned, ErrOutcomeUnknown, ErrStorage)
			return Operation{}, j.poison
		}
	}
	if p.dispatchLeg != "" {
		pair := p.operation.SignedPackage
		leg, attempt := pair.Commit, p.operation.CommitDispatch
		if p.dispatchLeg == "spend" {
			leg, attempt = pair.Spend, p.operation.SpendDispatch
		}
		o.delivery = &Delivery{owner: o, requestID: p.operation.RequestID, leg: p.dispatchLeg, attempt: attempt.Attempt, raw: leg.Raw, txid: leg.TxID}
	}
	result := cloneJournalOperation(p.operation)
	if e := p.Close(); e != nil {
		return Operation{}, e
	}
	return result, nil
}
func (p *PreparedCommit) Close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	var e error
	if p.file != nil && !p.adopted {
		e = p.file.Close()
	}
	if p.name != "" && !p.published {
		if re := removeAt(p.owner.j.chain.dir(), p.name); re != nil {
			e = errors.Join(e, re)
		}
		if se := p.owner.j.syncFile(p.owner.j.chain.dir()); se != nil {
			e = errors.Join(e, se)
		}
	}
	if p.owner.pending == p {
		p.owner.pending = nil
	}
	p.next = journalState{}
	p.operation = Operation{}
	if e != nil {
		p.owner.j.poison = errors.Join(p.owner.j.poison, ErrPoisoned, ErrStorage)
		return p.owner.j.poison
	}
	return nil
}
