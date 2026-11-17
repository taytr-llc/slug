// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"sort"
)

func (s *Store) statusLocked(ev Event, asOf int64, roster uint64) (EventStatus, error) {
	row, exists, e := s.event(ev)
	if e != nil {
		return EventStatus{}, e
	}
	v := EventStatus{Version: Version1, StoreID: s.meta.StoreID, Event: ev, Exists: exists, Phase: "accepting", EntryID: row.EntryID, Revision: row.Revision, LatestOperationID: row.LatestOperationID, SavedIDs: []string{}, CurrentRosterRevision: roster, Eligibility: "not_checked", ObservedAtMS: asOf, ClosedObservedAtMS: row.ClosedObservedAtMS}
	if asOf >= ev.CutoffMS {
		v.Phase = "closed_pending_lock"
	}
	if row.ClosedObservedAtMS > asOf {
		return EventStatus{}, ErrConflict
	}
	if row.LatestOperationID != "" {
		o, e := s.operation(row.LatestOperationID)
		if e != nil || o.Receipt == nil {
			return EventStatus{}, ErrCorrupt
		}
		r := o.Receipt
		v.LatestReceiptDigest, e = ReceiptDigest(*r)
		if e != nil {
			return EventStatus{}, ErrCorrupt
		}
		if r.CommittedAtMS > asOf {
			return EventStatus{}, ErrConflict
		}
		v.SavedAtMS = r.CommittedAtMS
		v.SavedRosterRevision = r.RosterRevision
		for _, m := range r.Members {
			v.SavedIDs = append(v.SavedIDs, m.Assignment.ProspectID)
		}
	}
	if s.meta.PendingOperationID != "" {
		o, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return EventStatus{}, e
		}
		if o.Intent.Request.EventID == ev.EventID {
			v.PendingOperationID = o.Intent.OperationID
		}
	}
	v.RosterChanged = len(v.SavedIDs) > 0 && v.SavedRosterRevision != roster
	if validateEventStatus(v) != nil {
		return EventStatus{}, ErrCorrupt
	}
	return v, nil
}
func (s *Store) status(ctx context.Context, session gymstable.Session, saturday string, upcoming bool) (EventStatus, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return EventStatus{}, e
	}
	defer done()
	var out EventStatus
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now, e := s.observeLocked(nil, 0, nil)
		if e != nil {
			return e
		}
		if upcoming {
			saturday, e = upcomingDate(now)
			if e != nil {
				return e
			}
		}
		ev, e := EventForDate(s.meta.Pin, saturday)
		if e != nil {
			return e
		}
		now, e = s.observeLocked(&ev, 0, nil)
		if e != nil {
			return e
		}
		if upcoming && now >= ev.CutoffMS {
			return ErrConflict
		}
		out, e = s.statusLocked(ev, now, s.meta.ObservedRosterRevision)
		if e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		return EventStatus{}, e
	}
	return out, nil
}
func (s *Store) Status(ctx context.Context, session gymstable.Session, saturday string) (EventStatus, error) {
	return s.status(ctx, session, saturday, false)
}
func (s *Store) Upcoming(ctx context.Context, session gymstable.Session) (EventStatus, error) {
	return s.status(ctx, session, "", true)
}
func (s *Store) query(ctx context.Context, session gymstable.Session, id string, pending bool) (OperationResult, error) {
	if !pending && !hashID(id) {
		return OperationResult{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OperationResult{}, e
	}
	defer done()
	var out OperationResult
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		out = OperationResult{Version: Version1, StoreID: s.meta.StoreID}
		if pending {
			id = s.meta.PendingOperationID
			if id == "" {
				return check()
			}
		}
		o, e := s.operation(id)
		if e != nil {
			return e
		}
		ev, e := s.requestEvent(o.Intent.Request)
		if e != nil {
			return e
		}
		if _, e = s.observeLocked(&ev, 0, nil); e != nil {
			return e
		}
		out.Operation = &o
		return check()
	})
	if e != nil {
		return OperationResult{}, e
	}
	return out, nil
}
func (s *Store) Operation(ctx context.Context, session gymstable.Session, id string) (OperationResult, error) {
	return s.query(ctx, session, id, false)
}
func (s *Store) Pending(ctx context.Context, session gymstable.Session) (OperationResult, error) {
	return s.query(ctx, session, "", true)
}
func (s *Store) Receipt(ctx context.Context, session gymstable.Session, id string) (Receipt, error) {
	v, e := s.query(ctx, session, id, false)
	if e != nil {
		return Receipt{}, e
	}
	if v.Operation == nil || v.Operation.Receipt == nil {
		return Receipt{}, ErrNotFound
	}
	return *v.Operation.Receipt, nil
}
func (s *Store) History(ctx context.Context, session gymstable.Session, after string, limit uint32) (HistoryPage, error) {
	if !optionalID(after) || limit < 1 || limit > MaxPageSize {
		return HistoryPage{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return HistoryPage{}, e
	}
	defer done()
	events := make([]Event, 0, s.meta.EventCount)
	it := s.db.NewIterator(util.BytesPrefix([]byte("e:")), nil)
	for it.Next() {
		s.mark("history_row")
		if ctx.Err() != nil {
			it.Release()
			return HistoryPage{}, ErrCancelled
		}
		if len(events) >= MaxEvents {
			it.Release()
			return HistoryPage{}, ErrCorrupt
		}
		var r eventRow
		if decode(it.Value(), MaxEventSize, &r, true) != nil || validateEventRow(r, s.meta) != nil {
			it.Release()
			return HistoryPage{}, ErrCorrupt
		}
		events = append(events, r.Event)
	}
	e = it.Error()
	it.Release()
	if e != nil {
		return HistoryPage{}, ErrCorrupt
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CutoffMS != events[j].CutoffMS {
			return events[i].CutoffMS < events[j].CutoffMS
		}
		return events[i].EventID < events[j].EventID
	})
	start := 0
	if after != "" {
		found := false
		for i, v := range events {
			if v.EventID == after {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return HistoryPage{}, ErrNotFound
		}
	}
	end := min(start+int(limit), len(events))
	var out HistoryPage
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now, e := s.observeLocked(nil, 0, nil)
		if e != nil {
			return e
		}
		batch := new(leveldb.Batch)
		for _, ev := range events[start:end] {
			row, _, e := s.event(ev)
			if e != nil {
				return e
			}
			if now >= ev.CutoffMS && row.Phase == "accepting" {
				row.Phase = "closed_pending_lock"
				row.ClosedObservedAtMS = now
				if e = put(batch, key("e", ev.EventID), row, MaxEventSize); e != nil {
					return e
				}
			}
		}
		if batch.Len() > 0 {
			if e = s.commitLocked(batch, s.meta); e != nil {
				return e
			}
		}

		out = HistoryPage{Version: Version1, StoreID: s.meta.StoreID, AfterEventID: after, ObservedAtMS: now, Events: []EventStatus{}, HasMore: end < len(events)}
		for _, ev := range events[start:end] {
			v, e := s.statusLocked(ev, now, s.meta.ObservedRosterRevision)
			if e != nil {
				return e
			}
			out.Events = append(out.Events, v)
		}
		if out.HasMore {
			out.NextEventID = out.Events[len(out.Events)-1].Event.EventID
		}
		return check()
	})
	if e != nil {
		return HistoryPage{}, e
	}
	return out, nil
}
func (s *Store) Check(ctx context.Context, session gymstable.Session, saturday string) (CheckResult, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CheckResult{}, e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return CheckResult{}, e
	}
	row, _, e := s.event(ev)
	if e != nil {
		return CheckResult{}, e
	}
	ids := []string{}
	if row.LatestOperationID != "" {
		o, e := s.operation(row.LatestOperationID)
		if e != nil || o.Receipt == nil {
			return CheckResult{}, ErrCorrupt
		}
		for _, m := range o.Receipt.Members {
			ids = append(ids, m.Assignment.ProspectID)
		}
	}
	var selected *gymstable.EntrySelection
	if len(ids) > 0 {
		v, err := s.source.ReadEntrySelection(ctx, session, ids)
		if err != nil {
			s.mu.Lock()
			_, ce := s.observeLocked(&ev, 0, nil)
			s.mu.Unlock()
			if ce != nil {
				return CheckResult{}, ce
			}
			return CheckResult{}, mapped(err)
		}
		selected = &v
	}
	s.mark("after_check_selection")
	// Observe source metadata only after the proof fence; never hold an account
	// lease while waiting for the stable gate. This is not a fresh proof claim.
	info, e := sourceInfo(ctx, s.source, s.meta.Pin)
	if e != nil {
		return CheckResult{}, e
	}
	var out CheckResult
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		lower := int64(0)
		if selected != nil {
			lower = selected.AsOfMS
		}
		now, e := s.observeLocked(&ev, lower, &info)
		if e != nil {
			return e
		}
		current, _, e := s.event(ev)
		if e != nil {
			return e
		}
		if current.Revision != row.Revision || current.LatestOperationID != row.LatestOperationID {
			return ErrConflict
		}
		asOf := now
		revision := info.RosterRevision
		if selected != nil {
			asOf = selected.AsOfMS
			revision = selected.Status.RosterRevision
			if info.Checkpoint != selected.Status.Checkpoint || info.RosterRevision != revision || asOf < row.CreatedAtMS || (asOf < ev.CutoffMS) != (now < ev.CutoffMS) {
				return ErrConflict
			}
		}
		status, e := s.statusLocked(ev, asOf, revision)
		if e != nil {
			return e
		}
		out = CheckResult{Version: Version1, Status: status, AsOfMS: asOf, RosterRevision: revision, Members: []CheckMember{}}
		if selected != nil {
			for _, m := range selected.Members {
				elig := "eligible"
				if m.Slot == 0 {
					elig = "not_rostered"
				}
				if m.Item.Condition.Phase == "retired" {
					elig = "retired"
				}
				out.Members = append(out.Members, CheckMember{m.Slot, m.Item, elig})
			}
		}
		if _, e = ValidateCheckResult(out); e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		return CheckResult{}, e
	}
	return out, nil
}
