// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
)

type PreparedCommit struct {
	owner                            *Owner
	next                             state
	operation                        Operation
	noop                             bool
	name                             string
	file                             *os.File
	stamp                            fileStamp
	used, closed, published, adopted bool
	dispatch                         bool
}

func (o *Owner) stageReady() error {
	if e := o.check(); e != nil {
		return e
	}
	if o.pending != nil {
		return ErrBusy
	}
	return nil
}
func (o *Owner) time() (int64, error) {
	if e := o.stageReady(); e != nil {
		return 0, e
	}
	t := o.j.now()
	if !validTime(t) || t < o.j.state.ClockHighWaterMS {
		return 0, ErrClock
	}
	return t, nil
}
func (o *Owner) replay(v Operation) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	p := &PreparedCommit{owner: o, operation: ownedOperation(v), noop: true}
	o.pending = p
	return p, nil
}
func (o *Owner) stage(v Operation, newRecord, dispatch bool) (p *PreparedCommit, err error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	if e := o.j.quickCheck(); e != nil {
		return nil, e
	}
	next := o.j.state
	next.Records = append([]Operation(nil), next.Records...)
	if newRecord {
		next.Records = append(next.Records, v)
	} else {
		next.Records[v.Sequence-1] = v
	}
	next.ClockHighWaterMS = v.UpdatedAtMS
	if e := validateState(o.ctx, next, o.j.identity, o.j.binding); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(next)
	if e != nil || len(raw) > MaxStateBytes {
		return nil, ErrCapacity
	}
	var random [32]byte
	if _, e = rand.Read(random[:]); e != nil {
		return nil, ErrStorage
	}
	name := ".state-" + hex.EncodeToString(random[:]) + ".tmp"
	f, e := createAt(o.j.chain.dir(), name)
	if e != nil {
		return nil, ErrStorage
	}
	p = &PreparedCommit{owner: o, next: next, operation: ownedOperation(v), name: name, file: f, dispatch: dispatch}
	o.pending = p
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = ErrStorage
	}
	if e == nil {
		e = o.j.syncFile(f)
	}
	if e == nil {
		p.stamp, e = stamp(f)
	}
	if e == nil {
		e = o.check()
	}
	if e != nil {
		ce := p.Close()
		return nil, errors.Join(ErrStorage, e, ce)
	}
	return p, nil
}
func (o *Owner) mutate(id string) (Operation, int64, error) {
	if e := o.stageReady(); e != nil {
		return Operation{}, 0, e
	}
	v, e := o.Lookup(id)
	if e != nil {
		return Operation{}, 0, e
	}
	if v.Revision == math.MaxUint32 {
		return Operation{}, 0, ErrCapacity
	}
	t, e := o.time()
	if e != nil {
		return Operation{}, 0, e
	}
	v.Revision++
	v.UpdatedAtMS = t
	return v, t, nil
}
func (o *Owner) StageIntent(r Request) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	script, e := requestScript(r)
	if e != nil {
		return nil, e
	}
	if old, e := o.Lookup(r.OperationID); e == nil {
		if old.Intent.Destination != r.Recipient || old.Intent.RecipientAtoms != r.AmountAtoms {
			return nil, ErrConflict
		}
		return o.replay(old)
	} else if !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	s, e := o.Snapshot()
	if e != nil {
		return nil, e
	}
	if s.Count >= MaxOperations {
		return nil, ErrCapacity
	}
	if s.UnresolvedOperationID != "" {
		return nil, ErrConflict
	}
	t, e := o.time()
	if e != nil {
		return nil, e
	}
	in := Intent{1, r.OperationID, o.j.binding, t, r.Recipient, hex.EncodeToString(script), r.AmountAtoms, 1000, 100000, 1, 84, 84, 0}
	v := Operation{Version: 1, Sequence: s.Count + 1, OperationID: r.OperationID, Revision: 1, UpdatedAtMS: t, Stage: StageIntent, Intent: in, IntentDigest: intentDigest(in)}
	return o.stage(v, true, false)
}
func (o *Owner) StagePrepared(id string, d Draft) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Draft != nil {
		if !same(*old.Draft, d) {
			return nil, ErrConflict
		}
		return o.replay(old)
	}
	if old.Stage != StageIntent {
		return nil, ErrConflict
	}
	dg, e := DraftDigest(d)
	if e != nil {
		return nil, e
	}
	v, t, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	d.Inputs = append([]Input(nil), d.Inputs...)
	v.Stage = StagePrepared
	v.Draft = &d
	v.DraftDigest = dg
	v.PreparedAtMS = t
	return o.stage(v, false, false)
}
func auth(a AuthorizationInput, t int64) Authorization {
	return Authorization{a.ActionID, a.ReviewID, a.ViewID, a.Generation, t}
}
func (o *Owner) StageSigned(id string, raw []byte, a AuthorizationInput) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	if !validAction(a) {
		return nil, ErrInvalid
	}
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Draft == nil {
		return nil, ErrConflict
	}
	s, e := ValidateSigned(*old.Draft, raw)
	if e != nil {
		return nil, e
	}
	if old.Signed != nil {
		if s != *old.Signed {
			return nil, ErrConflict
		}
		return o.replay(old)
	}
	if old.Stage != StagePrepared {
		return nil, ErrConflict
	}
	v, t, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	au := auth(a, t)
	v.Stage = StageSigned
	v.Signed = &s
	v.SignAuthorization = &au
	return o.stage(v, false, false)
}
func (o *Owner) StageDispatch(id string, a AuthorizationInput) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	if !validAction(a) {
		return nil, ErrInvalid
	}
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Signed == nil {
		return nil, ErrConflict
	}
	attempt := uint32(1)
	if old.Dispatch != nil {
		prev := old.Dispatch.Authorization
		if prev.ActionID == a.ActionID {
			if prev.ReviewID != a.ReviewID || prev.ViewID != a.ViewID || prev.Generation != a.Generation {
				return nil, ErrConflict
			}
			return o.replay(old)
		}
		if old.Dispatch.Attempt == math.MaxUint32 {
			return nil, ErrCapacity
		}
		attempt = old.Dispatch.Attempt + 1
		if old.SignAuthorization.ActionID == a.ActionID {
			return nil, ErrConflict
		}
	}
	if old.Stage != StageSigned && old.Stage != StageDispatching {
		return nil, ErrConflict
	}
	v, t, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	v.Stage = StageDispatching
	v.Dispatch = &Dispatch{Attempt: attempt, Authorization: auth(a, t), IntentAtMS: t, Result: DeliveryPending}
	return o.stage(v, false, true)
}
func (o *Owner) StageDeliveryResult(id string, attempt uint32, result DeliveryResult, code string) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	want := ""
	switch result {
	case DeliveryAcknowledged:
	case DeliveryUnknown:
		want = "node_unavailable"
	case DeliveryRejected:
		want = "node_rejected"
	default:
		return nil, ErrInvalid
	}
	if code != want {
		return nil, ErrInvalid
	}
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Dispatch == nil || old.Dispatch.Attempt != attempt {
		return nil, ErrConflict
	}
	if old.Dispatch.Result != DeliveryPending {
		if old.Dispatch.Result == result && old.Dispatch.Error == code {
			return o.replay(old)
		}
		return nil, ErrConflict
	}
	v, t, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	v.Dispatch.Result = result
	v.Dispatch.ResultAtMS = t
	v.Dispatch.Error = code
	return o.stage(v, false, false)
}
func (o *Owner) StageObservation(id string, in ObservationInput) (*PreparedCommit, error) {
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Signed == nil {
		return nil, ErrConflict
	}
	v, t, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	v.Observation = &Observation{1, old.Signed.TxID, t, in.State, in.NodeTip, in.NodeHeight, in.Confirmations, in.BlockHash, in.Error}
	if !observationValid(*v.Observation, old.Signed.TxID, v.SignAuthorization.AuthorizedAtMS, t) {
		return nil, ErrInvalid
	}
	if in.State == ObservationMempool || in.State == ObservationConfirmed {
		v.Stage = StageSubmitted
	}
	return o.stage(v, false, false)
}
func (o *Owner) StageAbandon(id string) (*PreparedCommit, error) {
	if e := o.stageReady(); e != nil {
		return nil, e
	}
	old, e := o.Lookup(id)
	if e != nil {
		return nil, e
	}
	if old.Stage == StageAbandoned {
		return o.replay(old)
	}
	if old.Stage != StageIntent && old.Stage != StagePrepared {
		return nil, ErrConflict
	}
	v, _, e := o.mutate(id)
	if e != nil {
		return nil, e
	}
	v.Stage = StageAbandoned
	return o.stage(v, false, false)
}
