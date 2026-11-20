// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
)

func ownedSameView(i OwnedIntent, s Session) bool {
	return i.Generation == s.Generation && i.ViewID == s.ViewID
}
func (s *OwnedStore) checkOwnedRequest(ctx context.Context, r OwnedSlotRequest) (*fighterownership.HeadLease, error) {
	if _, e := ValidateOwnedSlotRequest(s.meta.Pin, r); e != nil {
		return nil, e
	}
	if r.ExpectedRevision != s.meta.RosterRevision {
		return nil, ErrConflict
	}
	for n, id := range s.roster.Slots {
		if r.ProspectID != "" && uint32(n+1) != r.Slot && id == r.ProspectID {
			return nil, ErrConflict
		}
	}
	c, e := s.source.CaptureOwned(ctx, s.meta.Pin.Stable.Identity.GymPublicKey)
	if e != nil {
		return nil, e
	}
	if c.Head() != r.Head {
		return nil, ErrConflict
	}
	if r.ProspectID != "" {
		v, e := s.source.OwnedItem(ctx, c, r.ProspectID)
		if e != nil {
			return nil, e
		}
		if v.State.StateID != r.ExpectedStateID || v.State.OwnerPublicKey != s.meta.Pin.Stable.Identity.GymPublicKey || ownedState(s.meta.Pin, v.Admission, v.State) != nil {
			return nil, ErrConflict
		}
	}
	return s.source.AcquireHead(ctx, r.Head)
}

// commitOwned stages all bounded bytes before entering the original account
// fence. A local write followed by an outer lease failure is never a refusal.
func (s *OwnedStore) commitOwned(ctx context.Context, session Session, cap gymaccount.Capability, b *leveldb.Batch, m ownedMetadata, head *fighterownership.HeadLease, install func()) error {
	if s.readOnly {
		return ErrForbidden
	}
	if e := encodedPut(b, []byte("meta"), m, OwnedMaxStatusSize); e != nil {
		return e
	}
	committed := false
	e := s.fence(ctx, session, cap, func(check func() error) error {
		if head != nil {
			if e := head.Check(); e != nil {
				return e
			}
		}
		if e := check(); e != nil {
			return e
		}
		if e := s.persist(b, m); e != nil {
			return e
		}
		committed = true
		if install != nil {
			install()
		}
		return nil
	})
	if e != nil && committed {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return e
}
func (s *OwnedStore) PrepareSlot(ctx context.Context, session Session, r OwnedSlotRequest) (OwnedIntent, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return OwnedIntent{}, e
	}
	defer done()
	if _, e = ValidateOwnedSlotRequest(s.meta.Pin, r); e != nil {
		return OwnedIntent{}, e
	}
	if s.meta.PendingOperationID != "" {
		o, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return OwnedIntent{}, e
		}
		if o.State != "prepared" {
			return OwnedIntent{}, ErrCorrupt
		}
		if !ownedSameView(o.Intent, session) || o.Intent.Request != r {
			return OwnedIntent{}, ErrPending
		}
		if e = s.fence(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return OwnedIntent{}, e
		}
		return o.Intent, nil
	}
	if s.meta.OperationCount >= MaxOperations {
		return OwnedIntent{}, ErrCapacity
	}
	head, e := s.checkOwnedRequest(ctx, r)
	if e != nil {
		return OwnedIntent{}, e
	}
	defer head.Close()
	id, e := s.newID()
	if e != nil {
		return OwnedIntent{}, e
	}
	now, e := s.clock()
	if e != nil {
		return OwnedIntent{}, e
	}
	intent := OwnedIntent{1, uint64(s.meta.OperationCount) + 1, id, session.Generation, session.ViewID, r, now}
	o := OwnedOperation{Version: 1, Intent: intent, State: "prepared"}
	raw, e := MarshalOwnedOperation(s.meta.Pin, o)
	if e != nil {
		return OwnedIntent{}, e
	}
	b := new(leveldb.Batch)
	b.Put(idKey('o', id), raw)
	m := s.meta
	m.ClockMS = now
	m.OperationCount++
	m.PendingOperationID = id
	m.LastOperationID = id
	if e = s.commitOwned(ctx, session, gymaccount.RosterManage, b, m, head, nil); e != nil {
		return OwnedIntent{}, e
	}
	return intent, nil
}
func (s *OwnedStore) ApplySlot(ctx context.Context, session Session, id string) (OwnedMutationResult, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	if !ownedSameView(o.Intent, session) {
		return OwnedMutationResult{}, ErrStaleView
	}
	if o.State == "abandoned" {
		return OwnedMutationResult{}, ErrConflict
	}
	if o.State == "committed" {
		if e = s.fence(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return OwnedMutationResult{}, e
		}
		return OwnedMutationResult{1, *o.Receipt, true}, nil
	}
	if s.meta.PendingOperationID != id {
		return OwnedMutationResult{}, ErrConflict
	}
	head, e := s.checkOwnedRequest(ctx, o.Intent.Request)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	defer head.Close()
	now, e := s.clock()
	if e != nil {
		return OwnedMutationResult{}, e
	}
	r := s.roster
	request := o.Intent.Request
	old := r.Slots[request.Slot-1]
	changed := old != request.ProspectID
	r.Slots[request.Slot-1] = request.ProspectID
	if changed {
		r.Revision++
	}
	receipt := OwnedMutationReceipt{1, id, o.Intent.Generation, o.Intent.ViewID, request, old, changed, s.meta.RosterRevision, r.Revision, r.Slots, now}
	o.State = "committed"
	o.Receipt = &receipt
	raw, e := MarshalOwnedOperation(s.meta.Pin, o)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	b := new(leveldb.Batch)
	b.Put(idKey('o', id), raw)
	if e = encodedPut(b, []byte("roster"), r, 4096); e != nil {
		return OwnedMutationResult{}, e
	}
	m := s.meta
	m.ClockMS = now
	m.PendingOperationID = ""
	m.RosterRevision = r.Revision
	if e = s.commitOwned(ctx, session, gymaccount.RosterManage, b, m, head, func() { s.roster = r; s.read = nil; s.history = nil }); e != nil {
		return OwnedMutationResult{}, e
	}
	return OwnedMutationResult{1, receipt, false}, nil
}
func (s *OwnedStore) Operation(ctx context.Context, session Session, id string) (OwnedOperation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedOperation{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return OwnedOperation{}, e
	}
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return OwnedOperation{}, e
	}
	return o, nil
}
func (s *OwnedStore) Pending(ctx context.Context, session Session) (*OwnedOperation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return nil, e
	}
	defer done()
	var o *OwnedOperation
	if s.meta.PendingOperationID != "" {
		v, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return nil, e
		}
		if v.State != "prepared" {
			return nil, ErrCorrupt
		}
		o = &v
	}
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return nil, e
	}
	return o, nil
}
func (s *OwnedStore) Abandon(ctx context.Context, session Session, id string) (OwnedOperation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.RosterManage)
	if e != nil {
		return OwnedOperation{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return OwnedOperation{}, e
	}
	if o.State != "prepared" {
		if e = s.fence(ctx, session, gymaccount.RosterManage, nil); e != nil {
			return OwnedOperation{}, e
		}
		return o, nil
	}
	if s.meta.PendingOperationID != id {
		return OwnedOperation{}, ErrConflict
	}
	now, e := s.clock()
	if e != nil {
		return OwnedOperation{}, e
	}
	o.State = "abandoned"
	o.AbandonedAtMS = now
	raw, e := MarshalOwnedOperation(s.meta.Pin, o)
	if e != nil {
		return OwnedOperation{}, e
	}
	b := new(leveldb.Batch)
	b.Put(idKey('o', id), raw)
	m := s.meta
	m.ClockMS = now
	m.PendingOperationID = ""
	if e = s.commitOwned(ctx, session, gymaccount.RosterManage, b, m, nil, nil); e != nil {
		return OwnedOperation{}, e
	}
	return o, nil
}
