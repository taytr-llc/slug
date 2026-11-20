// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"sort"
)

func (s *OwnedStore) SnapshotSourceInfo(ctx context.Context) (OwnedSnapshotSourceInfo, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return OwnedSnapshotSourceInfo{}, e
	}
	defer done()
	if ctx.Err() != nil {
		return OwnedSnapshotSourceInfo{}, ErrCancelled
	}
	return OwnedSnapshotSourceInfo{1, s.meta.StoreID, s.meta.Pin, s.meta.ClockMS}, nil
}
func (s *OwnedStore) observe(ctx context.Context, session Session, cap gymaccount.Capability, ev *Event) (int64, error) {
	now, e := s.clock()
	if e != nil {
		return 0, e
	}
	m := s.meta
	m.ClockMS = now
	b := new(leveldb.Batch)
	if ev != nil {
		r, exists, e := s.event(*ev)
		if e != nil {
			return 0, e
		}
		if exists && now >= ev.CutoffMS && r.ClosedObservedAtMS == 0 {
			r.Phase = "closed_pending_lock"
			r.ClosedObservedAtMS = now
			if e = put(b, key("e", ev.EventID), r, MaxEventSize); e != nil {
				return 0, e
			}
		}
	}
	e = s.fence(ctx, session, cap, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		if !s.readOnly {
			return s.persist(b, m)
		}
		return nil
	})
	return now, e
}
func (s *OwnedStore) statusValue(ev Event, now int64, roster uint64) (OwnedEventStatus, error) {
	row, exists, e := s.event(ev)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	v := OwnedEventStatus{Version: 1, StoreID: s.meta.StoreID, Event: ev, Exists: exists, Phase: "accepting", EntryID: row.EntryID, Revision: row.Revision, LatestOperationID: row.LatestOperationID, SavedIDs: []string{}, CurrentRosterRevision: roster, Eligibility: "not_checked", ObservedAtMS: now, ClosedObservedAtMS: row.ClosedObservedAtMS}
	if now >= ev.CutoffMS {
		v.Phase = "closed_pending_lock"
		if exists && v.ClosedObservedAtMS == 0 {
			v.ClosedObservedAtMS = now
		}
	}
	if row.LatestOperationID != "" {
		o, e := s.operation(row.LatestOperationID)
		if e != nil || o.Receipt == nil {
			return OwnedEventStatus{}, ErrCorrupt
		}
		r := o.Receipt
		v.LatestReceiptDigest, e = OwnedReceiptDigest(s.meta.Pin, *r)
		if e != nil || r.CommittedAtMS > now {
			return OwnedEventStatus{}, ErrCorrupt
		}
		v.SavedAtMS = r.CommittedAtMS
		v.SavedRosterRevision = r.RosterRevision
		for _, m := range r.Members {
			v.SavedIDs = append(v.SavedIDs, m.Birth.ProspectID)
		}
	}
	if s.meta.PendingOperationID != "" {
		o, e := s.operation(s.meta.PendingOperationID)
		if e != nil {
			return OwnedEventStatus{}, e
		}
		if o.Intent.Request.EventID == ev.EventID {
			v.PendingOperationID = o.Intent.OperationID
		}
	}
	v.RosterChanged = len(v.SavedIDs) > 0 && v.SavedRosterRevision != roster
	return ValidateOwnedEventStatus(s.meta.Pin, v)
}
func ValidateOwnedEventStatus(p OwnedPin, v OwnedEventStatus) (OwnedEventStatus, error) {
	if validateEventStatus(EventStatus(v)) != nil {
		return OwnedEventStatus{}, ErrInvalid
	}
	sid, e := OwnedID(p)
	ev, ee := OwnedEventForDate(p, v.Event.Saturday)
	eid, _ := OwnedEntryID(sid, ev.EventID, p.StablePin.Stable.Identity.GymID)
	if e != nil || ee != nil || v.StoreID != sid || v.Event != ev || v.EntryID != eid {
		return OwnedEventStatus{}, ErrInvalid
	}
	v.SavedIDs = append([]string{}, v.SavedIDs...)
	return v, nil
}
func (s *OwnedStore) Status(ctx context.Context, session Session, saturday string) (OwnedEventStatus, error) {
	return s.status(ctx, session, saturday, false)
}
func (s *OwnedStore) Upcoming(ctx context.Context, session Session) (OwnedEventStatus, error) {
	return s.status(ctx, session, "", true)
}
func (s *OwnedStore) status(ctx context.Context, session Session, saturday string, upcoming bool) (OwnedEventStatus, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	defer done()
	now, e := s.clock()
	if e != nil {
		return OwnedEventStatus{}, e
	}
	if upcoming {
		saturday, e = upcomingDate(now)
		if e != nil {
			return OwnedEventStatus{}, e
		}
	}
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	info, e := s.source.Status(ctx, session)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	now, e = s.observe(ctx, session, gymaccount.View, &ev)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	v, e := s.statusValue(ev, now, info.RosterRevision)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return OwnedEventStatus{}, e
	}
	return v, nil
}
func (s *OwnedStore) Pending(ctx context.Context, session Session) (*OwnedOperation, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return nil, e
	}
	defer done()
	var out *OwnedOperation
	if id := s.meta.PendingOperationID; id != "" {
		v, e := s.operation(id)
		if e != nil {
			return nil, e
		}
		out = &v
	}
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *OwnedStore) Receipt(ctx context.Context, session Session, id string) (OwnedReceipt, error) {
	v, e := s.Operation(ctx, session, id)
	if e != nil {
		return OwnedReceipt{}, e
	}
	if v.Receipt == nil {
		return OwnedReceipt{}, ErrNotFound
	}
	return *v.Receipt, nil
}
func (s *OwnedStore) History(ctx context.Context, session Session, after string, limit uint32) (OwnedHistoryPage, error) {
	if !optionalID(after) || limit < 1 || limit > 8 {
		return OwnedHistoryPage{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	defer done()
	events := make([]Event, 0, s.meta.EventCount)
	it := s.db.NewIterator(util.BytesPrefix([]byte("e:")), nil)
	for it.Next() {
		if len(events) >= MaxEvents || len(it.Value()) > MaxEventSize {
			it.Release()
			return OwnedHistoryPage{}, ErrCorrupt
		}
		var r eventRow
		if ownedcodec.Decode(it.Value(), MaxEventSize, &r) != nil {
			it.Release()
			return OwnedHistoryPage{}, ErrCorrupt
		}
		events = append(events, r.Event)
	}
	e = it.Error()
	it.Release()
	if e != nil || len(events) != int(s.meta.EventCount) {
		return OwnedHistoryPage{}, ErrCorrupt
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CutoffMS > events[j].CutoffMS })
	start := 0
	if after != "" {
		found := false
		for n, ev := range events {
			if ev.EventID == after {
				start = n + 1
				found = true
				break
			}
		}
		if !found {
			return OwnedHistoryPage{}, ErrNotFound
		}
	}
	info, e := s.source.Status(ctx, session)
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	now, e := s.observe(ctx, session, gymaccount.View, nil)
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	out := OwnedHistoryPage{Version: 1, StoreID: s.meta.StoreID, AfterEventID: after, ObservedAtMS: now, Events: make([]OwnedEventStatus, 0, limit)}
	end := min(len(events), start+int(limit))
	for _, ev := range events[start:end] {
		v, e := s.statusValue(ev, now, info.RosterRevision)
		if e != nil {
			return OwnedHistoryPage{}, e
		}
		out.Events = append(out.Events, v)
	}
	out.HasMore = end < len(events)
	if out.HasMore {
		out.NextEventID = events[end-1].EventID
	}
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return OwnedHistoryPage{}, e
	}
	return out, nil
}
func (s *OwnedStore) WithSnapshotView(ctx context.Context, session Session, fn func(OwnedSnapshotSourceInfo, func() error) error) error {
	if fn == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	now, e := s.observe(ctx, session, gymaccount.View, nil)
	if e != nil {
		return e
	}
	return s.fence(ctx, session, gymaccount.View, func(check func() error) error {
		return fn(OwnedSnapshotSourceInfo{1, s.meta.StoreID, s.meta.Pin, now}, check)
	})
}
