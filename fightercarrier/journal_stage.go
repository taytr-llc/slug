// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

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
	next                             journalState
	operation                        Operation
	noop                             bool
	name                             string
	file                             *os.File
	stamp                            fileStamp
	used, closed, published, adopted bool
	dispatchLeg                      string
}

func (o *Owner) stageReady() error {
	if err := o.check(); err != nil {
		return err
	}
	if o.pending != nil {
		return ErrBusy
	}
	return nil
}
func (o *Owner) time() (int64, error) {
	if err := o.stageReady(); err != nil {
		return 0, err
	}
	t := o.j.now()
	if !journalTime(t) || t <= o.j.state.ClockHighWaterMS {
		return 0, ErrClock
	}
	return t, nil
}
func (o *Owner) replay(v Operation) (*PreparedCommit, error) {
	if err := o.stageReady(); err != nil {
		return nil, err
	}
	p := &PreparedCommit{owner: o, operation: cloneJournalOperation(v), noop: true}
	o.pending = p
	return p, nil
}
func (o *Owner) stage(v Operation, fresh bool, leg string) (*PreparedCommit, error) {
	if err := o.stageReady(); err != nil {
		return nil, err
	}
	if err := o.j.quickCheck(); err != nil {
		return nil, err
	}
	if err := ValidateOperation(v, o.j.identity.Pin, o.j.binding); err != nil {
		return nil, err
	}
	v = cloneJournalOperation(v)
	next := o.j.state
	next.Records = append([]Operation(nil), next.Records...)
	if fresh {
		next.Records = append(next.Records, v)
	} else {
		if v.Sequence == 0 || int(v.Sequence) > len(next.Records) {
			return nil, ErrConflict
		}
		next.Records[v.Sequence-1] = v
	}
	next.ClockHighWaterMS = v.UpdatedAtMS
	if err := validateJournalState(o.ctx, next, o.j.identity, o.j.binding); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > journalMaxBytes {
		return nil, ErrCapacity
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, ErrStorage
	}
	name := ".state-" + hex.EncodeToString(random[:]) + ".tmp"
	if !validStageName(name) {
		return nil, ErrStorage
	}
	f, err := createAt(o.j.chain.dir(), name)
	if err != nil {
		return nil, ErrStorage
	}
	p := &PreparedCommit{owner: o, next: next, operation: v, name: name, file: f, dispatchLeg: leg}
	o.pending = p
	n, err := f.Write(raw)
	if err == nil && n != len(raw) {
		err = ErrStorage
	}
	if err == nil {
		err = o.j.syncFile(f)
	}
	if err == nil {
		p.stamp, err = stamp(f)
	}
	if err == nil {
		err = o.check()
	}
	if err != nil {
		return nil, errors.Join(ErrStorage, err, p.Close())
	}
	return p, nil
}
func (o *Owner) mutate(id string) (Operation, int64, error) {
	if err := o.stageReady(); err != nil {
		return Operation{}, 0, err
	}
	v, err := o.Lookup(id)
	if err != nil {
		return Operation{}, 0, err
	}
	if v.Revision == math.MaxUint32 {
		return Operation{}, 0, ErrCapacity
	}
	t, err := o.time()
	if err != nil {
		return Operation{}, 0, err
	}
	v.Revision++
	v.UpdatedAtMS = t
	return v, t, nil
}

func (o *Owner) StageIntent(request Request) (*PreparedCommit, error) {
	if err := o.stageReady(); err != nil {
		return nil, err
	}
	digest, err := RequestDigest(request)
	if err != nil {
		return nil, err
	}
	if request.OwnershipPinID != o.j.identity.Pin.OwnershipPinID {
		return nil, ErrConflict
	}
	if old, err := o.Lookup(request.RequestID); err == nil {
		if old.RequestDigest != digest || !sameJournal(old.Request, request) {
			return nil, ErrConflict
		}
		return o.replay(old)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	snapshot, err := o.Snapshot()
	if err != nil {
		return nil, err
	}
	if snapshot.Count >= journalMaxRecords {
		return nil, ErrCapacity
	}
	if snapshot.ActiveRequestID != "" {
		return nil, ErrConflict
	}
	t, err := o.time()
	if err != nil {
		return nil, err
	}
	v := Operation{Version: 1, Sequence: snapshot.Count + 1, RequestID: request.RequestID, Revision: 1, CreatedAtMS: t, UpdatedAtMS: t, Stage: "intent", Request: request, RequestDigest: digest}
	return o.stage(v, true, "")
}
func (o *Owner) StagePrepared(id string, preparation Preparation) (*PreparedCommit, error) {
	old, err := o.Lookup(id)
	if err != nil {
		return nil, err
	}
	if old.Preparation != nil {
		if !sameJournal(*old.Preparation, preparation) {
			return nil, ErrConflict
		}
		return o.replay(old)
	}
	if old.Stage != "intent" || !sameJournal(old.Request, preparation.Request) {
		return nil, ErrConflict
	}
	digest, err := PreparationDigest(preparation)
	if err != nil {
		return nil, err
	}
	v, _, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	v.Stage = "prepared"
	v.Preparation = &preparation
	v.PreparationDigest = digest
	return o.stage(v, false, "")
}

type AuthorizationInput struct {
	ActionID, ReviewID, ViewID string
	Generation                 uint64
}

func (o *Owner) authorization(a AuthorizationInput, t int64) Authorization {
	return Authorization{Mode: o.j.identity.Pin.OwnerMode, ActionID: a.ActionID, ReviewID: a.ReviewID, ViewID: a.ViewID, Generation: a.Generation, AuthorizedAtMS: t}
}
func (o *Owner) StageSigned(id string, pair SignedPackage, approval AuthorizationInput) (*PreparedCommit, error) {
	old, err := o.Lookup(id)
	if err != nil {
		return nil, err
	}
	if old.SignedPackage != nil {
		if !sameJournal(*old.SignedPackage, pair) {
			return nil, ErrConflict
		}
		return o.replay(old)
	}
	if old.Stage != "prepared" || old.Preparation == nil {
		return nil, ErrConflict
	}
	digest, err := SignedPackageDigest(pair)
	if err != nil {
		return nil, err
	}
	v, t, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	authorization := o.authorization(approval, t)
	v.Stage = "signed"
	v.SignedPackage = &pair
	v.SignedPackageDigest = digest
	v.SignAuthorization = &authorization
	return o.stage(v, false, "")
}

// StageDispatch persists an attempt before minting the one-use delivery token.
// Retry always reuses the exact retained transaction and a fresh action fence.
func (o *Owner) StageDispatch(id, leg string, approval AuthorizationInput) (*PreparedCommit, error) {
	old, err := o.Lookup(id)
	if err != nil {
		return nil, err
	}
	if old.SignedPackage == nil || journalTerminal(old) {
		return nil, ErrConflict
	}
	var previous *Dispatch
	switch leg {
	case "commit":
		if old.Request.Kind != "transfer" || (old.Stage != "signed" && old.Stage != "commit_dispatching") {
			return nil, ErrConflict
		}
		previous = old.CommitDispatch
	case "spend":
		if old.Request.Kind == "transfer" {
			if old.CommitDispatch == nil || old.CommitObservation == nil || (old.CommitObservation.State != "mempool" && old.CommitObservation.State != "canonical") {
				return nil, ErrConflict
			}
			if old.Stage != "commit_observed" && old.Stage != "spend_dispatching" {
				return nil, ErrConflict
			}
		} else if old.Stage != "signed" && old.Stage != "spend_dispatching" {
			return nil, ErrConflict
		}
		previous = old.SpendDispatch
	default:
		return nil, ErrInvalid
	}
	if previous != nil && previous.Authorization.ActionID == approval.ActionID {
		a := previous.Authorization
		if a.ReviewID != approval.ReviewID || a.ViewID != approval.ViewID || a.Generation != approval.Generation {
			return nil, ErrConflict
		}
		return o.replay(old)
	}
	v, t, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	v.Stage = leg + "_dispatching"
	attempt := uint32(1)
	if previous != nil {
		if previous.Attempt == math.MaxUint32 {
			return nil, ErrCapacity
		}
		attempt = previous.Attempt + 1
	}
	d := &Dispatch{Attempt: attempt, Authorization: o.authorization(approval, t), IntentAtMS: t, Result: "pending"}
	if leg == "commit" {
		v.CommitDispatch = d
	} else {
		v.SpendDispatch = d
	}
	return o.stage(v, false, leg)
}
func (o *Owner) StageDeliveryResult(id, leg string, attempt uint32, result, errorCode string) (*PreparedCommit, error) {
	v, t, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	var d *Dispatch
	if leg == "commit" {
		d = v.CommitDispatch
	} else if leg == "spend" {
		d = v.SpendDispatch
	} else {
		return nil, ErrInvalid
	}
	if d == nil || d.Attempt != attempt || journalTerminal(v) || result == "pending" {
		return nil, ErrConflict
	}
	if d.Result != "pending" {
		return nil, ErrConflict
	}
	d.Result = result
	d.ResultAtMS = t
	d.Error = errorCode
	return o.stage(v, false, "")
}
func (o *Owner) StageObservation(id, leg string, observation Observation) (*PreparedCommit, error) {
	v, t, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	if v.SignedPackage == nil || journalTerminal(v) {
		return nil, ErrConflict
	}
	observation.ObservedAtMS = t
	positive := observation.State == "mempool" || observation.State == "canonical"
	switch leg {
	case "commit":
		if v.Request.Kind != "transfer" || v.CommitDispatch == nil || v.SpendDispatch != nil {
			return nil, ErrConflict
		}
		v.CommitObservation = &observation
		if positive {
			v.Stage = "commit_observed"
		} else {
			v.Stage = "commit_dispatching"
		}
	case "spend":
		if v.SpendDispatch == nil {
			return nil, ErrConflict
		}
		v.SpendObservation = &observation
		if positive {
			v.Stage = "complete"
		}
	default:
		return nil, ErrInvalid
	}
	return o.stage(v, false, "")
}
func (o *Owner) StageAbandon(id string) (*PreparedCommit, error) {
	old, err := o.Lookup(id)
	if err != nil {
		return nil, err
	}
	if old.Stage == "abandoned" {
		return o.replay(old)
	}
	if old.SignedPackage != nil || (old.Stage != "intent" && old.Stage != "prepared") {
		return nil, ErrConflict
	}
	v, _, err := o.mutate(id)
	if err != nil {
		return nil, err
	}
	v.Stage = "abandoned"
	return o.stage(v, false, "")
}
