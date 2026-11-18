// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) readOne(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Record, bool, Status, error) {
	if !callValid(ctx, seeds, events, entries) {
		return Record{}, false, Status{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, false, Status{}, e
	}
	defer done()
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return Record{}, false, Status{}, ErrCalendar
	}
	id, _ := CardID(s.meta.Pin, ev)
	if e = s.authorize(work, seeds, events, entries, session); e != nil {
		return Record{}, false, Status{}, e
	}
	r, ok, e := s.record(id)
	if e != nil {
		return Record{}, false, Status{}, e
	}
	var ptr *Record
	if ok {
		ptr = &r
	}
	v, e := s.observe(work, seeds, events, entries, session, []gymevent.Event{ev}, []*Record{ptr})
	if e != nil {
		return Record{}, false, Status{}, e
	}
	return r, ok, v[0], nil
}
func (s *Store) Read(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (ReadResult, error) {
	r, ok, status, e := s.readOne(ctx, seeds, events, entries, session, date)
	if e != nil {
		return ReadResult{}, e
	}
	if !ok {
		return ReadResult{}, ErrNotFound
	}
	return ReadResult{1, r, status}, nil
}
func (s *Store) Status(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Status, error) {
	_, _, status, e := s.readOne(ctx, seeds, events, entries, session, date)
	return status, e
}
func (s *Store) History(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, after string, limit uint32) (HistoryPage, error) {
	if !callValid(ctx, seeds, events, entries) || !optionalID(after) || limit < 1 || limit > 8 {
		return HistoryPage{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return HistoryPage{}, e
	}
	defer done()
	if e = s.authorize(work, seeds, events, entries, session); e != nil {
		return HistoryPage{}, e
	}
	start := 0
	found := after == ""
	for i, row := range s.index {
		if work.Err() != nil {
			return HistoryPage{}, ErrCancelled
		}
		if row.id == after {
			start = i + 1
			found = true
			break
		}
	}
	if !found {
		return HistoryPage{}, ErrNotFound
	}
	end := min(start+int(limit), len(s.index))
	records := make([]*Record, 0, end-start)
	evs := make([]gymevent.Event, 0, end-start)
	for _, row := range s.index[start:end] {
		if work.Err() != nil {
			return HistoryPage{}, ErrCancelled
		}
		r, ok, e := s.record(row.id)
		if e != nil {
			return HistoryPage{}, e
		}
		if !ok || r.Source.SourceReceipt.Input.Event.CutoffMS != row.cutoff {
			return HistoryPage{}, ErrCorrupt
		}
		records = append(records, &r)
		evs = append(evs, r.Source.SourceReceipt.Input.Event)
	}
	if work.Err() != nil {
		return HistoryPage{}, ErrCancelled
	}
	// Empty pages still take an original-view observation; no implicit Build.
	statuses, e := s.observe(work, seeds, events, entries, session, evs, records)
	if e != nil {
		return HistoryPage{}, e
	}
	out := HistoryPage{Version: 1, StoreID: s.meta.StoreID, AfterCardID: after, HasMore: end < len(s.index), ObservedAtMS: s.meta.ClockMS, Cards: statuses}
	if out.HasMore {
		out.NextCardID = s.index[end-1].id
	}
	return out, nil
}
