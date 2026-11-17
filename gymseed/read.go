// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"sort"
)

func absentStatus(p Pin, ev gymevent.Event, n int64) Status {
	id, _ := ID(p)
	return Status{Version: 1, StoreID: id, EventKey: eventKey(p, ev), Event: ev, Phase: "absent", PublicationState: "none", ScanHeight: -1, ScanTipHeight: -1, ObservedAtMS: n, NodeState: "unobserved", FundingState: "unobserved", EscalationDue: n >= ev.IncidentDeadlineMS}
}
func phase(r Record) string {
	if len(r.Conflicts) > 0 || r.ConflictOverflow {
		return "conflict"
	}
	if r.Finality != nil {
		return "finalized"
	}
	if r.Target != nil {
		return "provisional"
	}
	if r.Anchor != nil {
		return "waiting_target"
	}
	if r.Publication != nil {
		return "waiting_anchor"
	}
	return "prepared"
}
func due(r Record, n int64) bool {
	return n >= r.SourceReceipt.Input.Event.IncidentDeadlineMS && (r.Finality == nil || len(r.Conflicts) > 0 || r.ConflictOverflow)
}
func statusFor(p Pin, r Record, n int64, funding string, generation uint32) Status {
	v := absentStatus(p, r.SourceReceipt.Input.Event, n)
	v.Exists = true
	v.Phase = phase(r)
	v.ScanHeight = r.Cursor.Height
	v.ScanGeneration = r.Cursor.Generation
	v.FundingGeneration = generation
	v.PreparedAtMS = r.PreparedAtMS
	v.ChainObservedAtMS = r.Observation.ChainObservedAtMS
	v.NodeState = r.Observation.NodeState
	v.FundingState = funding
	v.CurrentFinality = r.Observation.CurrentFinality
	v.TargetDescendants = r.Observation.TargetDescendants
	v.CheckpointPresent = r.Checkpoint != nil
	v.ConflictCount = uint32(len(r.Conflicts))
	v.ConflictOverflow = r.ConflictOverflow
	v.IncidentAtMS = r.IncidentAtMS
	v.EscalationDue = due(r, n)
	if r.Publication != nil {
		v.PublicationState = r.Publication.State
	}
	if r.Anchor != nil {
		v.AnchorHeight = r.Anchor.Height
		v.AnchorHash = r.Anchor.Hash
	}
	if r.Target != nil {
		v.TargetHeight = r.Target.Height
		v.TargetHash = r.Target.Hash
	}
	if r.Finality != nil {
		v.FinalizedAtMS = r.Finality.ObservedAtMS
	}
	if r.Observation.Tip != nil {
		v.ScanTipHeight = int32(r.Observation.Tip.Height)
	}
	return v
}
func (s *Store) fundingState(ctx context.Context) (string, error) {
	if s.meta.FundingCursor.Height < 0 {
		if s.meta.FundingCursor.Generation > 0 {
			return "scanning", nil
		}
		return "unobserved", nil
	}
	if !s.meta.FundingComplete {
		return "scanning", nil
	}
	coins, e := s.funding(ctx)
	if e != nil {
		return "unavailable", e
	}
	for _, c := range coins {
		reserved, e := s.db.Has(coinKey('u', c.Outpoint), nil)
		if e != nil {
			return "unavailable", ErrCorrupt
		}
		mature := !c.Coinbase || int64(s.meta.FundingCursor.Height)+1-int64(c.BlockHeight) >= 100
		if !reserved && mature && c.ValueSatoshis >= 1294 {
			return "ready", nil
		}
	}
	return "waiting_for_funds", nil
}
func (s *Store) observeRecordsLocked(lower int64, records []Record) (int64, bool, error) {
	n, e := s.sampleLocked(lower)
	if e != nil {
		return 0, false, e
	}
	next := s.meta
	next.ClockMS = n
	b := new(leveldb.Batch)
	changed := n != s.meta.ClockMS
	for i := range records {
		if records[i].IncidentAtMS == 0 && due(records[i], n) {
			records[i].IncidentAtMS = n
			if e = put(b, key('r', records[i].EventKey), records[i], MaxRecordSize); e != nil {
				return 0, false, e
			}
			changed = true
		}
	}
	if changed {
		if e = s.commitLocked(b, next); e != nil {
			return n, true, e
		}
	}
	return n, changed, nil
}
func (s *Store) readOne(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Record, bool, Status, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, false, Status{}, e
	}
	defer done()
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin, date)
	if e != nil {
		return Record{}, false, Status{}, ErrCalendar
	}
	if e = s.withSource(work, events, entries, session, gymaccount.View, func(_ int64, check func() error) error { return check() }); e != nil {
		return Record{}, false, Status{}, e
	}
	r, exists, e := s.record(eventKey(s.meta.Pin, ev))
	if e != nil {
		return r, false, Status{}, e
	}
	fund, e := s.fundingState(work)
	if e != nil {
		return r, false, Status{}, e
	}
	rows := []Record{}
	if exists {
		rows = append(rows, r)
	}
	var out Status
	mutated := false
	e = s.withSource(work, events, entries, session, gymaccount.View, func(lower int64, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		n, m, e := s.observeRecordsLocked(lower, rows)
		mutated = mutated || m
		if e != nil {
			return e
		}
		out = absentStatus(s.meta.Pin, ev, n)
		if exists {
			r = rows[0]
			out = statusFor(s.meta.Pin, r, n, fund, s.meta.FundingCursor.Generation)
		}
		return nil
	})
	if e != nil || work.Err() != nil {
		if mutated {
			return Record{}, false, Status{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return Record{}, false, Status{}, e
	}
	return r, exists, out, nil
}
func (s *Store) Status(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Status, error) {
	_, _, v, e := s.readOne(ctx, events, entries, session, date)
	return v, e
}
func (s *Store) Record(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Record, error) {
	r, ok, _, e := s.readOne(ctx, events, entries, session, date)
	if e != nil {
		return Record{}, e
	}
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}
func (s *Store) History(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, after string, limit uint32) (HistoryPage, error) {
	if !optionalID(after) || limit < 1 || limit > 8 {
		return HistoryPage{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return HistoryPage{}, e
	}
	defer done()
	if e = s.withSource(work, events, entries, session, gymaccount.View, func(_ int64, check func() error) error { return check() }); e != nil {
		return HistoryPage{}, e
	}
	ids, e := s.recordIDs(work)
	if e != nil {
		return HistoryPage{}, e
	}
	type row struct {
		id     string
		cutoff int64
	}
	order := make([]row, 0, len(ids))
	for _, id := range ids {
		if work.Err() != nil {
			return HistoryPage{}, ErrCancelled
		}
		r, _, e := s.record(id)
		if e != nil {
			return HistoryPage{}, e
		}
		order = append(order, row{id, r.SourceReceipt.Input.Event.CutoffMS})
		s.mark("history-record-validated")
	}
	if work.Err() != nil {
		return HistoryPage{}, ErrCancelled
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].cutoff == order[j].cutoff {
			return order[i].id < order[j].id
		}
		return order[i].cutoff < order[j].cutoff
	})
	start := 0
	found := after == ""
	for i, x := range order {
		if x.id == after {
			start = i + 1
			found = true
			break
		}
	}
	if !found {
		return HistoryPage{}, ErrNotFound
	}
	end := start + int(limit)
	if end > len(order) {
		end = len(order)
	}
	rows := make([]Record, 0, end-start)
	for _, x := range order[start:end] {
		if work.Err() != nil {
			return HistoryPage{}, ErrCancelled
		}
		r, _, e := s.record(x.id)
		if e != nil {
			return HistoryPage{}, e
		}
		rows = append(rows, r)
	}
	if work.Err() != nil {
		return HistoryPage{}, ErrCancelled
	}
	fund, e := s.fundingState(work)
	if e != nil {
		return HistoryPage{}, e
	}
	out := HistoryPage{Version: 1, StoreID: s.meta.StoreID, AfterEventKey: after, HasMore: end < len(order), Events: []Status{}}
	mutated := false
	e = s.withSource(work, events, entries, session, gymaccount.View, func(lower int64, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		n, m, e := s.observeRecordsLocked(lower, rows)
		mutated = mutated || m
		if e != nil {
			return e
		}
		out.ObservedAtMS = n
		for _, r := range rows {
			out.Events = append(out.Events, statusFor(s.meta.Pin, r, n, fund, s.meta.FundingCursor.Generation))
		}
		if out.HasMore {
			out.NextEventKey = rows[len(rows)-1].EventKey
		}
		return nil
	})
	if e != nil || work.Err() != nil {
		if mutated {
			return HistoryPage{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return HistoryPage{}, e
	}
	return out, nil
}
