// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) Observe(ctx context.Context, source *gymentry.Store, session gymstable.Session, saturday string) (EventStatus, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return EventStatus{}, e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return EventStatus{}, e
	}
	var out EventStatus
	e = s.sourceView(work, source, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		n, e := s.observeLocked(info.ObservedAtMS)
		if e != nil {
			return e
		}
		out, e = s.status(ev, n)
		return e
	})
	if e != nil || work.Err() != nil {
		if e == nil {
			e = ErrCancelled
		}
		return EventStatus{}, mapped(e)
	}
	return out, nil
}
func (s *Store) Read(ctx context.Context, source *gymentry.Store, session gymstable.Session, saturday string) (SnapshotReceipt, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return SnapshotReceipt{}, e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return SnapshotReceipt{}, e
	}
	if e = s.sourceView(work, source, session, func(_ gymentry.SnapshotSourceInfo, check func() error) error { return check() }); e != nil {
		return SnapshotReceipt{}, e
	}
	// All semantic work is outside the final original-account lease.
	out, ok, e := s.receipt(ev)
	if e != nil {
		return SnapshotReceipt{}, e
	}
	if ok && validateSnapshotReceipt(s.meta.Pin, out) != nil {
		return SnapshotReceipt{}, ErrCorrupt
	}
	e = s.sourceView(work, source, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		if _, e := s.observeLocked(info.ObservedAtMS); e != nil {
			return e
		}
		_, exists, e := s.event(ev)
		if e != nil {
			return e
		}
		if !exists {
			return ErrNotFound
		}
		if !ok {
			return ErrIncomplete
		}
		return nil
	})
	if e != nil || work.Err() != nil {
		if e == nil {
			e = ErrCancelled
		}
		return SnapshotReceipt{}, mapped(e)
	}
	return out, nil
}
func (s *Store) History(ctx context.Context, source *gymentry.Store, session gymstable.Session, after string, limit uint32) (HistoryPage, error) {
	if !optionalID(after) || limit == 0 || limit > MaxPageSize {
		return HistoryPage{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return HistoryPage{}, e
	}
	defer done()
	if e = s.sourceView(work, source, session, func(_ gymentry.SnapshotSourceInfo, check func() error) error { return check() }); e != nil {
		return HistoryPage{}, e
	}
	rows, e := s.rows(work)
	if e != nil {
		return HistoryPage{}, e
	}
	start := 0
	found := after == ""
	if after != "" {
		for i, r := range rows {
			if r.Event.EventID == after {
				start = i + 1
				found = true
				break
			}
		}
	}
	out := HistoryPage{Version: 1, StoreID: s.meta.StoreID, AfterEventID: after, NextEventID: "", Events: []EventStatus{}}
	e = s.sourceView(work, source, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		n, e := s.observeLocked(info.ObservedAtMS)
		if e != nil {
			return e
		}
		out.ObservedAtMS = n
		if !found {
			return ErrNotFound
		}
		end := min(len(rows), start+int(limit))
		out.HasMore = end < len(rows)
		for _, r := range rows[start:end] {
			v, e := s.status(r.Event, n)
			if e != nil {
				return e
			}
			out.Events = append(out.Events, v)
			if out.HasMore {
				out.NextEventID = r.Event.EventID
			}
		}
		return nil
	})
	if e != nil || work.Err() != nil {
		if e == nil {
			e = ErrCancelled
		}
		return HistoryPage{}, mapped(e)
	}
	return out, nil
}
