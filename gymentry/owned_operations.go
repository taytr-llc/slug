// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func (s *OwnedStore) requestEvent(r Request) (Event, error) {
	ev, e := OwnedEventForDate(s.meta.Pin, r.Saturday)
	if e != nil || ev.EventID != r.EventID {
		return Event{}, ErrInvalid
	}
	return ev, nil
}
func ownedSelectionMembers(v gymstable.OwnedEntrySelection) ([]OwnedMember, error) {
	out := make([]OwnedMember, 0, len(v.Members))
	classes := map[string]bool{}
	for _, m := range v.Members {
		if m.Slot == 0 {
			return nil, ErrNotRostered
		}
		if m.Item.Condition.Phase == "retired" {
			return nil, ErrRetired
		}
		class := m.Item.Manifest.Physical.Class
		if classes[class] {
			return nil, ErrClassConflict
		}
		classes[class] = true
		out = append(out, OwnedMember{m.Slot, m.Item.Birth, m.Item.Ownership, class, m.Item.Condition})
	}
	return out, nil
}
func (s *OwnedStore) Prepare(ctx context.Context, session Session, r Request) (OwnedIntent, error) {
	r, e := ValidateRequest(r)
	if e != nil {
		return OwnedIntent{}, e
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return OwnedIntent{}, e
	}
	defer done()
	ev, e := s.requestEvent(r)
	if e != nil {
		return OwnedIntent{}, e
	}
	if s.readOnly {
		return OwnedIntent{}, ErrForbidden
	}
	if _, e = s.observe(ctx, session, gymaccount.EntryManage, &ev); e != nil {
		return OwnedIntent{}, e
	}
	if s.meta.PendingOperationID != "" {
		o, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return OwnedIntent{}, e
		}
		if o.Intent.Generation != session.Generation || o.Intent.ViewID != session.ViewID || !ownedcodec.Equal(o.Intent.Request, r) {
			return OwnedIntent{}, ErrPending
		}
		if e = s.fence(ctx, session, gymaccount.EntryManage, nil); e != nil {
			return OwnedIntent{}, e
		}
		return o.Intent, nil
	}
	row, exists, e := s.event(ev)
	if e != nil {
		return OwnedIntent{}, e
	}
	if row.Revision != r.ExpectedRevision {
		return OwnedIntent{}, ErrConflict
	}
	if s.meta.OperationCount >= MaxOperations || !exists && s.meta.EventCount >= MaxEvents {
		return OwnedIntent{}, ErrCapacity
	}
	id, e := s.randomID()
	if e != nil {
		return OwnedIntent{}, e
	}
	var result OwnedIntent
	committed := false
	prepare := func(v gymstable.OwnedEntrySelection) (func(func() error) error, error) {
		now, e := s.clock()
		if e != nil {
			return nil, e
		}
		if now >= ev.CutoffMS {
			return nil, ErrClosed
		}
		states := make([]string, 0, len(v.Members))
		if r.Action == "replace" {
			if _, e = ownedSelectionMembers(v); e != nil {
				return nil, e
			}
			for _, m := range v.Members {
				states = append(states, m.Item.Ownership.StateID)
			}
		}
		result = OwnedIntent{1, uint64(s.meta.OperationCount) + 1, id, session.Generation, session.ViewID, r, now}
		op := OwnedOperation{Version: 1, Intent: result, State: "prepared"}
		raw, e := MarshalOwnedOperation(s.meta.Pin, op)
		if e != nil {
			return nil, e
		}
		b := new(leveldb.Batch)
		b.Put(key("o", id), raw)
		b.Put(seqKey(result.Sequence), rawID(id))
		if e = put(b, key("p", id), ownedPreparation{1, id, v.Status.Head, states}, 4096); e != nil {
			return nil, e
		}
		m := s.meta
		m.ClockMS = now
		m.OperationCount++
		m.PendingOperationID = id
		m.LastOperationID = id
		if !exists {
			m.EventCount++
			row.CreatedAtMS = now
			if e = put(b, key("e", ev.EventID), row, MaxEventSize); e != nil {
				return nil, e
			}
		}
		return func(check func() error) error {
			fresh, e := s.clock()
			if e != nil {
				return e
			}
			if fresh >= ev.CutoffMS {
				return ErrClosed
			}
			if e = check(); e != nil {
				return e
			}
			if e = s.persist(b, m); e != nil {
				return e
			}
			committed = true
			return nil
		}, nil
	}
	if r.Action == "replace" {
		e = s.source.WithEntrySelection(ctx, session, r.ExpectedRosterRevision, r.ProspectIDs, prepare)
	} else {
		status, se := s.source.SourceInfo(ctx)
		if se != nil {
			return OwnedIntent{}, se
		}
		commit, se := prepare(gymstable.OwnedEntrySelection{Status: status})
		if se != nil {
			return OwnedIntent{}, se
		}
		e = s.fence(ctx, session, gymaccount.EntryManage, commit)
	}
	if e != nil {
		if committed {
			return OwnedIntent{}, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedIntent{}, e
	}
	return result, nil
}
func (s *OwnedStore) Apply(ctx context.Context, session Session, id string) (OwnedMutationResult, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	if o.Intent.Generation != session.Generation || o.Intent.ViewID != session.ViewID {
		return OwnedMutationResult{}, ErrStaleView
	}
	if o.State == "abandoned" {
		return OwnedMutationResult{}, ErrConflict
	}
	if o.State == "committed" {
		if e = s.fence(ctx, session, gymaccount.EntryManage, nil); e != nil {
			return OwnedMutationResult{}, e
		}
		return OwnedMutationResult{1, *o.Receipt, true}, nil
	}
	if s.readOnly {
		return OwnedMutationResult{}, ErrForbidden
	}
	if s.meta.PendingOperationID != id {
		return OwnedMutationResult{}, ErrConflict
	}
	saved, e := s.preparation(o)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	request := o.Intent.Request
	ev, e := s.requestEvent(request)
	if e != nil {
		return OwnedMutationResult{}, e
	}
	row, exists, e := s.event(ev)
	if e != nil || !exists {
		return OwnedMutationResult{}, ErrCorrupt
	}
	if row.Revision != request.ExpectedRevision {
		return OwnedMutationResult{}, ErrConflict
	}
	var result OwnedMutationResult
	committed := false
	prepare := func(v gymstable.OwnedEntrySelection) (func(func() error) error, error) {
		now := v.AsOfMS
		if now < s.meta.ClockMS {
			return nil, ErrClock
		}
		if !timestamp(now) || now >= ev.CutoffMS {
			return nil, ErrClosed
		}
		members := []OwnedMember{}
		if request.Action == "replace" {
			var e error
			members, e = ownedSelectionMembers(v)
			if e != nil {
				return nil, e
			}
			if len(members) != len(saved.StateIDs) {
				return nil, ErrConflict
			}
			for n, m := range members {
				if m.Ownership.StateID != saved.StateIDs[n] {
					return nil, ErrConflict
				}
			}
		}
		receipt := OwnedReceipt{1, s.meta.StoreID, row.EntryID, ev, id, o.Intent.Sequence, o.Intent.Generation, o.Intent.ViewID, request.Action, row.Revision, row.Revision + 1, request.ExpectedRosterRevision, v.Status.Head, now, members}
		o.State = "committed"
		o.Receipt = &receipt
		raw, e := MarshalOwnedOperation(s.meta.Pin, o)
		if e != nil {
			return nil, e
		}
		b := new(leveldb.Batch)
		b.Put(key("o", id), raw)
		row.Revision++
		row.LatestOperationID = id
		if e = put(b, key("e", ev.EventID), row, MaxEventSize); e != nil {
			return nil, e
		}
		m := s.meta
		m.ClockMS = now
		m.PendingOperationID = ""
		result = OwnedMutationResult{1, receipt, false}
		return func(check func() error) error {
			fresh, e := s.clock()
			if e != nil {
				return e
			}
			if fresh >= ev.CutoffMS {
				return ErrClosed
			}
			if e = check(); e != nil {
				return e
			}
			if e = s.persist(b, m); e != nil {
				return e
			}
			committed = true
			return nil
		}, nil
	}
	if request.Action == "replace" {
		e = s.source.WithEntrySelection(ctx, session, request.ExpectedRosterRevision, request.ProspectIDs, prepare)
	} else {
		status, se := s.source.SourceInfo(ctx)
		if se != nil {
			return OwnedMutationResult{}, se
		}
		now, se := s.clock()
		if se != nil {
			return OwnedMutationResult{}, se
		}
		commit, se := prepare(gymstable.OwnedEntrySelection{Status: status, AsOfMS: now})
		if se != nil {
			return OwnedMutationResult{}, se
		}
		e = s.fence(ctx, session, gymaccount.EntryManage, commit)
	}
	if e != nil {
		if committed {
			return OwnedMutationResult{}, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedMutationResult{}, e
	}
	return result, nil
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
func (s *OwnedStore) Abandon(ctx context.Context, session Session, id string) (OwnedOperation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return OwnedOperation{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return OwnedOperation{}, e
	}
	if o.State != "prepared" {
		if e = s.fence(ctx, session, gymaccount.EntryManage, nil); e != nil {
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
	b.Put(key("o", id), raw)
	m := s.meta
	m.ClockMS = now
	m.PendingOperationID = ""
	committed := false
	e = s.fence(ctx, session, gymaccount.EntryManage, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		if e := s.persist(b, m); e != nil {
			return e
		}
		committed = true
		return nil
	})
	if e != nil {
		if committed {
			return OwnedOperation{}, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedOperation{}, e
	}
	return o, nil
}
