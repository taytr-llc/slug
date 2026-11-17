// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) requestEvent(r Request) (Event, error) {
	ev, e := EventForDate(s.meta.Pin, r.Saturday)
	if e != nil {
		return Event{}, e
	}
	if ev.EventID != r.EventID {
		return Event{}, ErrInvalid
	}
	return ev, nil
}
func (s *Store) Prepare(ctx context.Context, session gymstable.Session, r Request) (Intent, error) {
	r, e := ValidateRequest(r)
	if e != nil {
		return Intent{}, e
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return Intent{}, e
	}
	defer done()
	ev, e := s.requestEvent(r)
	if e != nil {
		return Intent{}, e
	}
	var out Intent
	var pendingReplay bool
	e = s.withLease(ctx, session, gymaccount.EntryManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now, e := s.observeLocked(&ev, 0, nil)
		if e != nil {
			return e
		}
		if s.meta.PendingOperationID != "" {
			o, e := s.operation(s.meta.PendingOperationID)
			if e != nil {
				return e
			}
			if o.Intent.Generation == session.Generation && o.Intent.ViewID == session.ViewID && equal(o.Intent.Request, r) {
				out = o.Intent
				pendingReplay = true
				return check()
			}
			return ErrPending
		}
		row, exists, e := s.event(ev)
		if e != nil {
			return e
		}
		if now >= ev.CutoffMS {
			return ErrClosed
		}
		if row.Revision != r.ExpectedRevision {
			return ErrConflict
		}
		if s.meta.OperationCount >= MaxOperations || (!exists && s.meta.EventCount >= MaxEvents) {
			return ErrCapacity
		}
		return check()
	})
	if e != nil {
		return Intent{}, e
	}
	if pendingReplay {
		return out, nil
	}
	id, e := s.randomID()
	if e != nil {
		return Intent{}, e
	}
	// Reserve the complete terminal row at Prepare; counts never grow on Apply.
	proposed := Intent{Version1, uint64(s.meta.OperationCount) + 1, id, session.Generation, session.ViewID, r, s.meta.ClockMS}
	if _, e = MarshalIntent(proposed); e != nil {
		return Intent{}, e
	}
	committed := false
	e = s.withLease(ctx, session, gymaccount.EntryManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.PendingOperationID != "" {
			return ErrPending
		}
		row, exists, e := s.event(ev)
		if e != nil {
			return e
		}
		if row.Revision != r.ExpectedRevision {
			return ErrConflict
		}
		if s.meta.OperationCount >= MaxOperations || (!exists && s.meta.EventCount >= MaxEvents) {
			return ErrCapacity
		}
		if e = check(); e != nil {
			return e
		}
		s.mark("before_prepare_guard")
		now, e := s.sampleLocked(0)
		if e != nil {
			return e
		}
		if now >= ev.CutoffMS {
			if _, e = s.observeAtLocked(now, &ev, nil); e != nil {
				return e
			}
			return ErrClosed
		}
		proposed.PreparedAtMS = now
		o := Operation{Version: Version1, Intent: proposed, State: "prepared"}
		b := new(leveldb.Batch)
		if e = put(b, key("o", id), o, MaxOperationSize); e != nil {
			return e
		}
		b.Put(seqKey(proposed.Sequence), rawID(id))
		m := s.meta
		m.ClockMS = now
		m.OperationCount++
		m.PendingOperationID = id
		m.LastOperationID = id
		if !exists {
			row.CreatedAtMS = now
			m.EventCount++
			if e = put(b, key("e", ev.EventID), row, MaxEventSize); e != nil {
				return e
			}
		}
		s.mark("before_prepare")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		committed = true
		out = proposed
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
	return out, nil
}
func selectionInfo(v gymstable.EntrySelection) gymstable.EntrySourceInfo {
	s := v.Status
	return gymstable.EntrySourceInfo{Version: s.Version, StoreID: s.StoreID, Pin: s.Pin, Checkpoint: s.Checkpoint, ObservedSourceCount: s.ObservedSourceCount, RosterRevision: s.RosterRevision}
}
func (s *Store) Apply(ctx context.Context, session gymstable.Session, id string) (MutationResult, error) {
	if !hashID(id) {
		return MutationResult{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return MutationResult{}, e
	}
	defer done()
	o, e := s.operation(id)
	if e != nil {
		return MutationResult{}, e
	}
	if o.Intent.Generation != session.Generation || o.Intent.ViewID != session.ViewID {
		return MutationResult{}, ErrStaleView
	}
	ev, e := s.requestEvent(o.Intent.Request)
	if e != nil {
		return MutationResult{}, e
	}
	var out MutationResult
	committed := false
	if o.State == "committed" {
		e = s.withLease(ctx, session, gymaccount.EntryManage, func(check func() error) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, e := s.observeLocked(&ev, 0, nil); e != nil {
				return e
			}
			if e := check(); e != nil {
				return e
			}
			out = MutationResult{Version1, *o.Receipt, true}
			return nil
		})
		if e != nil {
			return MutationResult{}, e
		}
		return out, nil
	}
	if o.State != "prepared" {
		return MutationResult{}, ErrConflict
	}
	commit := func(sel *gymstable.EntrySelection, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, e := s.operation(id)
		if e != nil {
			return e
		}
		if !equal(current, o) || s.meta.PendingOperationID != id {
			return ErrConflict
		}
		row, _, e := s.event(ev)
		if e != nil {
			return e
		}
		if row.Revision != o.Intent.Request.ExpectedRevision {
			return ErrConflict
		}
		lower := o.Intent.PreparedAtMS
		var info *gymstable.EntrySourceInfo
		members := []Member{}
		if sel != nil {
			i := selectionInfo(*sel)
			info = &i
			lower = max(lower, sel.AsOfMS)
			if sel.Status.RosterRevision != o.Intent.Request.ExpectedRosterRevision || len(sel.Members) != len(o.Intent.Request.ProspectIDs) {
				return ErrConflict
			}
			classes := map[string]bool{}
			for n, x := range sel.Members {
				if x.Item.Assignment.ProspectID != o.Intent.Request.ProspectIDs[n] {
					return ErrCorrupt
				}
				if x.Slot == 0 {
					return ErrNotRostered
				}
				if x.Item.Condition.Phase == "retired" {
					return ErrRetired
				}
				class := x.Item.Manifest.Physical.Class
				if classes[class] {
					return ErrClassConflict
				}
				classes[class] = true
				members = append(members, Member{Slot: x.Slot, Assignment: x.Item.Assignment, Class: class, ConditionAtSave: x.Item.Condition})
			}
		}
		next := s.meta
		if info != nil {
			if e = mergeSource(&next, *info); e != nil {
				return e
			}
		}
		// Reserve worst-width scalar/condition space before the final sample.
		// Source manifests are already fully verified; no regeneration follows.
		r := Receipt{Version: Version1, StoreID: s.meta.StoreID, EntryID: row.EntryID, Event: ev, OperationID: id, OperationSequence: o.Intent.Sequence, Generation: session.Generation, ViewID: session.ViewID, Action: o.Intent.Request.Action, PreviousRevision: row.Revision, Revision: row.Revision + 1, RosterRevision: o.Intent.Request.ExpectedRosterRevision, StableCheckpoint: next.ObservedCheckpoint, ObservedSourceCount: next.ObservedSourceCount, CommittedAtMS: MaxTimestampMS, Members: members}
		reserve := r
		reserve.Members = append([]Member{}, members...)
		for n := range reserve.Members {
			reserve.Members[n].ConditionAtSave.AsOfMS = MaxTimestampMS
			reserve.Members[n].ConditionAtSave.AgeWeeks = int64(MaxTimestampMS / 604800000)
		}
		pre := o
		pre.State = "committed"
		pre.Receipt = &reserve
		encoded, e := json.Marshal(pre)
		if e != nil || len(encoded)+256 > MaxOperationSize {
			return ErrCapacity
		}
		if e = check(); e != nil {
			return mapped(e)
		}
		s.mark("before_apply_guard")
		now, e := s.sampleLocked(lower)
		if e != nil {
			return e
		}
		reject := func(reason error) error {
			if _, e := s.observeAtLocked(now, &ev, info); e != nil {
				return e
			}
			return reason
		}
		if now >= ev.CutoffMS {
			return reject(ErrClosed)
		}
		for n := range members {
			c, e := fightercondition.Project(fightercondition.Version1, sel.Members[n].Item.Manifest.DNA, members[n].Assignment.BornAtMS, now, 0)
			if e != nil {
				return reject(ErrCorrupt)
			}
			if c.Phase == "retired" {
				return reject(ErrRetired)
			}
			members[n].ConditionAtSave = c
		}
		r.CommittedAtMS = now
		r.Members = members
		o.State = "committed"
		o.Receipt = &r
		row.Revision = r.Revision
		row.LatestOperationID = id
		b := new(leveldb.Batch)
		if e = put(b, key("o", id), o, MaxOperationSize); e != nil {
			return reject(e)
		}
		if e = put(b, key("e", ev.EventID), row, MaxEventSize); e != nil {
			return reject(e)
		}
		m := next
		m.ClockMS = now
		m.PendingOperationID = ""

		s.mark("before_apply")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		committed = true
		out = MutationResult{Version1, r, false}
		s.mark("after_apply")
		return nil
	}
	if o.Intent.Request.Action == "replace" {
		e = s.source.WithEntrySelection(ctx, session, o.Intent.Request.ExpectedRosterRevision, o.Intent.Request.ProspectIDs, func(sel gymstable.EntrySelection, check func() error) error { return commit(&sel, check) })
		e = mapped(e)
	} else {
		e = s.withLease(ctx, session, gymaccount.EntryManage, func(check func() error) error { return commit(nil, check) })
	}
	if e != nil || ctx.Err() != nil {
		if committed || s.poisoned {
			return MutationResult{}, ErrOutcomeUnknown
		}
		// A joined failed/slow proof still advances the authenticated local clock.
		s.mu.Lock()
		_, clockErr := s.observeLocked(&ev, 0, nil)
		s.mu.Unlock()
		if clockErr != nil {
			return MutationResult{}, clockErr
		}
		if e != nil {
			return MutationResult{}, e
		}
		return MutationResult{}, ErrCancelled
	}
	return out, nil
}
func (s *Store) Abandon(ctx context.Context, session gymstable.Session, id string) (OperationResult, error) {
	if !hashID(id) {
		return OperationResult{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.EntryManage)
	if e != nil {
		return OperationResult{}, e
	}
	defer done()
	var out OperationResult
	committed := false
	e = s.withLease(ctx, session, gymaccount.EntryManage, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		o, e := s.operation(id)
		if e != nil {
			return e
		}
		ev, e := s.requestEvent(o.Intent.Request)
		if e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		now, e := s.observeLocked(&ev, o.Intent.PreparedAtMS, nil)
		if e != nil {
			return e
		}
		if o.State == "prepared" {
			if s.meta.PendingOperationID != id {
				return ErrCorrupt
			}
			o.State = "abandoned"
			o.AbandonedAtMS = now
			b := new(leveldb.Batch)
			if e = put(b, key("o", id), o, MaxOperationSize); e != nil {
				return e
			}
			m := s.meta
			m.PendingOperationID = ""
			s.mark("before_abandon")
			if e = s.commitLocked(b, m); e != nil {
				return e
			}
			committed = true
			s.mark("after_abandon")
		}
		out = OperationResult{Version1, s.meta.StoreID, &o}
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return OperationResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return OperationResult{}, e
		}
		return OperationResult{}, ErrCancelled
	}
	return out, nil
}
