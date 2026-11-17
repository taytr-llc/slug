// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"time"
)

const eventMetadataBytes = 24 << 10

func cloneEventJob(v EventJob) EventJob {
	b, _ := json.Marshal(v)
	var o EventJob
	_ = json.Unmarshal(b, &o)
	return o
}
func cloneEventPin(v gymevent.Pin) gymevent.Pin {
	b, _ := json.Marshal(v)
	var o gymevent.Pin
	_ = json.Unmarshal(b, &o)
	return o
}
func (s *StableController) EventSnapshot() EventSnapshot {
	blank := EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.eventEnabled || !s.eventAttached || !s.installed || s.stableClosed {
			return gymevent.ErrUnavailable
		}
		v := s.viewPin()
		out = EventSnapshot{Version: 1, State: "setup_incomplete", Error: "event_setup_incomplete", View: &v, ParticipantID: s.eventParticipant}
		if s.eventStore == nil {
			return nil
		}
		p := cloneEventPin(s.eventPin)
		out.State = "ready"
		out.Error = ""
		out.Pin = &p
		out.StoreID = s.eventID
		out.CoordinatorCreatedAtMS = s.eventCreated
		s.expireQueuedLocked()
		if s.eventLast != nil {
			out.LastJobID = s.eventLast.JobID
		}
		if s.eventFailure != "" {
			out.State = "failed"
			out.Error = s.eventFailure
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.event != nil {
			out.ActiveJobID = s.active.event.JobID
		}
		if s.queued != nil && s.queued.event != nil && s.queued.published {
			out.QueuedJobID = s.queued.event.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}
func (s *StableController) eventRead(ctx context.Context, v string, fn func(context.Context, *gymevent.Store, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymevent.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.eventEnabled && s.eventAttached && s.installed && !s.stableClosed && !s.stopping
	store := s.eventStore
	s.stableMu.Unlock()
	if !valid {
		return gymevent.ErrUnavailable
	}
	if store == nil {
		return errEventSetup
	}
	if s.entryStore == nil {
		return gymevent.ErrBusy
	}
	if ctx.Err() != nil {
		return gymevent.ErrBusy
	}
	if e := fn(ctx, store, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymevent.ErrOutcomeUnknown) {
			return gymevent.ErrBusy
		}
		return e
	}
	return s.shortView(ctx, v, func() error { return nil })
}
func (s *StableController) EventStatus(ctx context.Context, v, date string) (gymevent.EventStatus, error) {
	if !entryDateValid(date) {
		return gymevent.EventStatus{}, gymevent.ErrCalendar
	}
	var out gymevent.EventStatus
	e := s.eventRead(ctx, v, func(ctx context.Context, x *gymevent.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Observe(ctx, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymevent.EventStatus{}, e
	}
	return out, nil
}
func (s *StableController) EventRecord(ctx context.Context, v, date string) (gymevent.SnapshotReceipt, error) {
	if !entryDateValid(date) {
		return gymevent.SnapshotReceipt{}, gymevent.ErrCalendar
	}
	var out gymevent.SnapshotReceipt
	e := s.eventRead(ctx, v, func(ctx context.Context, x *gymevent.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Read(ctx, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymevent.SnapshotReceipt{}, e
	}
	return out, nil
}
func (s *StableController) EventHistory(ctx context.Context, v, after string, limit uint32) (gymevent.HistoryPage, error) {
	if limit < 1 || limit > 8 || after != "" && !canonicalManifestID(after) {
		return gymevent.HistoryPage{}, gymevent.ErrInvalid
	}
	var out gymevent.HistoryPage
	e := s.eventRead(ctx, v, func(ctx context.Context, x *gymevent.Store, source *gymentry.Store) error {
		var e error
		out, e = x.History(ctx, source, s.session(), after, limit)
		return e
	})
	if e != nil {
		return gymevent.HistoryPage{}, e
	}
	return out, nil
}
func (s *StableController) EventJob(ctx context.Context, v, id string) (EventJob, error) {
	if !canonicalManifestID(id) {
		return EventJob{}, gymevent.ErrInvalid
	}
	var out EventJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.eventEnabled || s.stableClosed {
			return gymevent.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.event != nil && (t == s.active || t.published) && t.event.JobID == id {
				out = cloneEventJob(*t.event)
				return nil
			}
		}
		if s.eventLast != nil && s.eventLast.JobID == id {
			out = cloneEventJob(*s.eventLast)
			return nil
		}
		return gymevent.ErrNotFound
	})
	if e != nil {
		return EventJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelEventJob(ctx context.Context, v, id string) (EventJob, error) {
	if !canonicalManifestID(id) {
		return EventJob{}, gymevent.ErrInvalid
	}
	var out EventJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.eventEnabled || s.stableClosed {
			return gymevent.ErrUnavailable
		}
		if s.active != nil && s.active.event != nil && s.active.event.JobID == id {
			s.active.cancel()
			out = cloneEventJob(*s.active.event)
			return nil
		}
		if s.queued != nil && s.queued.event != nil && s.queued.published && s.queued.event.JobID == id {
			s.cancelQueuedEventLocked()
			out = cloneEventJob(*s.eventLast)
			return nil
		}
		if s.eventLast != nil && s.eventLast.JobID == id {
			out = cloneEventJob(*s.eventLast)
			return nil
		}
		return gymevent.ErrNotFound
	})
	if e != nil {
		return EventJob{}, e
	}
	return out, nil
}
