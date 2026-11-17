// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/hex"
	"io"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
)

func (s *Store) checkRequest(r SlotRequest) error {
	if _, e := ValidateSlotRequest(r); e != nil {
		return e
	}
	if r.ExpectedRevision != s.meta.RosterRevision {
		return ErrConflict
	}
	if r.ProspectID != "" {
		if _, e := s.assignment(r.ProspectID); e != nil {
			return e
		}
		for i, id := range s.roster.Slots {
			if uint32(i+1) != r.Slot && id == r.ProspectID {
				return ErrConflict
			}
		}
	}
	return nil
}
func sameView(i Intent, session Session) bool {
	return i.Generation == session.Generation && i.ViewID == session.ViewID
}
func (s *Store) caughtUp(ctx context.Context) error {
	count, e := s.sourceInfo(ctx)
	if e != nil {
		return e
	}
	if count != s.meta.Checkpoint {
		return ErrSyncRequired
	}
	return nil
}

func (s *Store) PrepareSlot(ctx context.Context, session Session, request SlotRequest) (Intent, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return Intent{}, e
	}
	defer done()
	if _, e = ValidateSlotRequest(request); e != nil {
		return Intent{}, e
	}
	if s.meta.PendingOperationID != "" {
		o, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return Intent{}, e
		}
		if o.State != "prepared" {
			return Intent{}, ErrCorrupt
		}
		if !sameView(o.Intent, session) || o.Intent.Request != request {
			return Intent{}, ErrPending
		}
		if e = s.withLease(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return Intent{}, e
		}
		return o.Intent, nil
	}
	if e = s.caughtUp(ctx); e != nil {
		return Intent{}, e
	}
	if s.meta.OperationCount >= MaxOperations {
		return Intent{}, ErrCapacity
	}
	if e = s.checkRequest(request); e != nil {
		return Intent{}, e
	}
	var id string
	for attempt := 0; attempt < 8; attempt++ {
		var raw [32]byte
		if _, e = io.ReadFull(s.random, raw[:]); e != nil {
			return Intent{}, ErrCapacity
		}
		candidate := hex.EncodeToString(raw[:])
		exists, e := s.db.Has(idKey('o', candidate), nil)
		if e != nil {
			return Intent{}, ErrCorrupt
		}
		if !exists {
			id = candidate
			break
		}
	}
	if id == "" {
		return Intent{}, ErrCapacity
	}
	var result Intent
	committed := false
	e = s.withLease(ctx, session, gymaccount.RosterManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.PendingOperationID != "" {
			return ErrPending
		}
		if e := s.checkRequest(request); e != nil {
			return e
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		result = Intent{Version: Version1, Sequence: uint64(s.meta.OperationCount) + 1, OperationID: id, Generation: session.Generation, ViewID: session.ViewID, Request: request, PreparedAtMS: now}
		o := Operation{Version: Version1, Intent: result, State: "prepared"}
		m := s.meta
		m.OperationCount++
		m.PendingOperationID = id
		m.LastOperationID = id
		b := new(leveldb.Batch)
		if e = encodedPut(b, idKey('o', id), o, MaxOperationSize); e != nil {
			return e
		}
		// Every reserved slot permits one complete MaxOperationSize terminal row;
		// no subsequent admission depends on free operation count or byte growth.
		if e = check(); e != nil {
			return e
		}
		s.mark("before_prepare")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		committed = true
		s.mark("after_prepare")
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return Intent{}, ErrOutcomeUnknown
		}
		if e != nil {
			return Intent{}, e
		}
		return Intent{}, ErrCancelled
	}
	return result, nil
}

func (s *Store) ApplySlot(ctx context.Context, session Session, operationID string) (MutationResult, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return MutationResult{}, e
	}
	defer done()
	o, e := s.operation(operationID)
	if e != nil {
		return MutationResult{}, e
	}
	if !sameView(o.Intent, session) {
		return MutationResult{}, ErrStaleView
	}
	if o.State == "abandoned" {
		return MutationResult{}, ErrConflict
	}
	if o.State == "committed" {
		if e = s.withLease(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return MutationResult{}, e
		}
		return MutationResult{Version: Version1, Receipt: *o.Receipt, Replayed: true}, nil
	}
	if e = s.caughtUp(ctx); e != nil {
		return MutationResult{}, e
	}
	if e = s.checkRequest(o.Intent.Request); e != nil {
		return MutationResult{}, e
	}
	var target Assignment
	if id := o.Intent.Request.ProspectID; id != "" {
		target, e = s.assignment(id)
		if e != nil {
			return MutationResult{}, e
		}
		item, e := s.readSource(ctx, target.Sequence, id)
		if e != nil {
			return MutationResult{}, e
		}
		if item.Assignment != target {
			return MutationResult{}, ErrCorrupt
		}
	}
	var result MutationResult
	committed := false
	e = s.withLease(ctx, session, gymaccount.RosterManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, e := s.operation(operationID)
		if e != nil {
			return e
		}
		if !equalEncoded(current, o) || s.meta.PendingOperationID != operationID {
			return ErrConflict
		}
		if e = s.checkRequest(o.Intent.Request); e != nil {
			return e
		}
		if target.ProspectID != "" {
			a, e := s.assignment(target.ProspectID)
			if e != nil {
				return e
			}
			if a != target {
				return ErrConflict
			}
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		if target.AcceptedAtMS > now {
			return ErrClock
		}
		r := s.roster
		request := o.Intent.Request
		old := r.Slots[request.Slot-1]
		changed := old != request.ProspectID
		r.Slots[request.Slot-1] = request.ProspectID
		if changed {
			r.Revision++
		}
		receipt := MutationReceipt{Version: Version1, OperationID: operationID, Generation: o.Intent.Generation, ViewID: o.Intent.ViewID, Request: request, PreviousProspectID: old, Changed: changed, PreviousRevision: s.meta.RosterRevision, Revision: r.Revision, Slots: r.Slots, CommittedAtMS: now}
		o.State = "committed"
		o.Receipt = &receipt
		m := s.meta
		m.PendingOperationID = ""
		m.RosterRevision = r.Revision
		b := new(leveldb.Batch)
		if e = encodedPut(b, []byte("roster"), r, MaxMetadataSize); e != nil {
			return e
		}
		if e = encodedPut(b, idKey('o', operationID), o, MaxOperationSize); e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		s.mark("before_apply")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		s.roster = r
		committed = true
		s.mark("after_apply")
		result = MutationResult{Version: Version1, Receipt: receipt}
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return MutationResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return MutationResult{}, e
		}
		return MutationResult{}, ErrCancelled
	}
	return result, nil
}

func (s *Store) Operation(ctx context.Context, session Session, operationID string) (Operation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return Operation{}, e
	}
	defer done()
	o, e := s.operation(operationID)
	if e != nil {
		return Operation{}, e
	}
	if e = s.withLease(ctx, session, gymaccount.View, nil); e != nil {
		return Operation{}, e
	}
	return o, nil
}
func (s *Store) Pending(ctx context.Context, session Session) (*Operation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return nil, e
	}
	defer done()
	var result *Operation
	if id := s.meta.PendingOperationID; id != "" {
		o, e := s.operation(id)
		if e != nil {
			return nil, e
		}
		if o.State != "prepared" {
			return nil, ErrCorrupt
		}
		result = &o
	}
	if e = s.withLease(ctx, session, gymaccount.View, nil); e != nil {
		return nil, e
	}
	return result, nil
}
func (s *Store) Abandon(ctx context.Context, session Session, operationID string) (Operation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return Operation{}, e
	}
	defer done()
	o, e := s.operation(operationID)
	if e != nil {
		return Operation{}, e
	}
	if o.State != "prepared" {
		if e = s.withLease(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return Operation{}, e
		}
		return o, nil
	}
	committed := false
	e = s.withLease(ctx, session, gymaccount.RosterManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, e := s.operation(operationID)
		if e != nil {
			return e
		}
		if !equalEncoded(current, o) || s.meta.PendingOperationID != operationID {
			return ErrConflict
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		o.State = "abandoned"
		o.AbandonedAtMS = now
		b := new(leveldb.Batch)
		if e = encodedPut(b, idKey('o', operationID), o, MaxOperationSize); e != nil {
			return e
		}
		m := s.meta
		m.PendingOperationID = ""
		if e = check(); e != nil {
			return e
		}
		s.mark("before_abandon")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		committed = true
		s.mark("after_abandon")
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return Operation{}, ErrOutcomeUnknown
		}
		if e != nil {
			return Operation{}, e
		}
		return Operation{}, ErrCancelled
	}
	return o, nil
}
