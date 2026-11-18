// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymentry"
	"time"
)

// Bout metadata reads include durable account and source-clock observations.
// This admission/operation budget preserves a shorter caller deadline; a Sync
// already in progress must still join and can return observation uncertainty.
// Cached polls, short View checks, other families and jobs keep their budgets.
const boutMetadataTimeout = 250 * time.Millisecond

func (s *StableController) BoutSnapshot() BoutSnapshot {
	blank := BoutSnapshot{Version: 1, State: "unavailable", Error: "bout_unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.boutEnabled || !s.boutAttached || !s.installed || s.stableClosed {
			return gymbout.ErrUnavailable
		}
		v := s.viewPin()
		out = BoutSnapshot{Version: 1, State: "setup_incomplete", Error: "bout_setup_incomplete", View: &v}
		if s.boutStore == nil {
			return nil
		}
		p := cloneBoutPin(s.boutPin)
		out.State = "ready"
		out.Error = ""
		out.Pin = &p
		out.StoreID = s.boutID
		out.CreatedAtMS = s.boutCreated
		out.SourceCreatedAtMS = s.boutSourceCreated
		s.expireQueuedLocked()
		if s.boutLast != nil {
			out.LastJobID = s.boutLast.JobID
		}
		if s.boutFailure != "" {
			out.State = "failed"
			out.Error = s.boutFailure
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.bout != nil {
			out.ActiveJobID = s.active.bout.JobID
		}
		if s.queued != nil && s.queued.bout != nil && s.queued.published {
			out.QueuedJobID = s.queued.bout.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}
func (s *StableController) boutRead(ctx context.Context, v string, fn func(context.Context, *gymbout.Store, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, boutMetadataTimeout)
	defer cancel()
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymbout.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.boutEnabled && s.boutAttached && s.installed && !s.stableClosed && !s.stopping
	store := s.boutStore
	s.stableMu.Unlock()
	if !valid {
		return gymbout.ErrUnavailable
	}
	if store == nil {
		return errBoutSetup
	}
	if s.entryStore == nil {
		return gymbout.ErrBusy
	}
	if ctx.Err() != nil {
		return gymbout.ErrBusy
	}
	if e := fn(ctx, store, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymbout.ErrOutcomeUnknown) {
			return gymbout.ErrBusy
		}
		return e
	}
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return gymbout.ErrOutcomeUnknown
	}
	return nil
}
func (s *StableController) BoutStatus(ctx context.Context, v, date, bid string) (gymbout.Status, error) {
	if !entryDateValid(date) {
		return gymbout.Status{}, gymbout.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return gymbout.Status{}, gymbout.ErrInvalid
	}
	var out gymbout.Status
	e := s.boutRead(ctx, v, func(ctx context.Context, x *gymbout.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Status(ctx, s.cardStore, s.seedStore, s.eventStore, source, s.session(), date, bid)
		return e
	})
	if e != nil {
		return gymbout.Status{}, e
	}
	return out, nil
}
func (s *StableController) BoutRecord(ctx context.Context, v, date, bid string) (gymbout.ReadResult, error) {
	if !entryDateValid(date) {
		return gymbout.ReadResult{}, gymbout.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return gymbout.ReadResult{}, gymbout.ErrInvalid
	}
	var out gymbout.ReadResult
	e := s.boutRead(ctx, v, func(ctx context.Context, x *gymbout.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Read(ctx, s.cardStore, s.seedStore, s.eventStore, source, s.session(), date, bid)
		return e
	})
	if e != nil {
		return gymbout.ReadResult{}, e
	}
	return out, nil
}
func (s *StableController) BoutCardStatus(ctx context.Context, v, date string) (gymbout.CardStatus, error) {
	if !entryDateValid(date) {
		return gymbout.CardStatus{}, gymbout.ErrCalendar
	}
	var out gymbout.CardStatus
	e := s.boutRead(ctx, v, func(ctx context.Context, x *gymbout.Store, source *gymentry.Store) error {
		var e error
		out, e = x.CardStatus(ctx, s.cardStore, s.seedStore, s.eventStore, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymbout.CardStatus{}, e
	}
	return out, nil
}
func (s *StableController) BoutJob(ctx context.Context, v, id string) (BoutJob, error) {
	if !canonicalManifestID(id) {
		return BoutJob{}, gymbout.ErrInvalid
	}
	var out BoutJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.boutEnabled || s.stableClosed {
			return gymbout.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.bout != nil && (t == s.active || t.published) && t.bout.JobID == id {
				out = cloneBoutJob(*t.bout)
				return nil
			}
		}
		if s.boutLast != nil && s.boutLast.JobID == id {
			out = cloneBoutJob(*s.boutLast)
			return nil
		}
		return gymbout.ErrNotFound
	})
	if e != nil {
		return BoutJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelBoutJob(ctx context.Context, v, id string) (BoutJob, error) {
	if !canonicalManifestID(id) {
		return BoutJob{}, gymbout.ErrInvalid
	}
	var out BoutJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.boutEnabled || s.stableClosed {
			return gymbout.ErrUnavailable
		}
		if s.active != nil && s.active.bout != nil && s.active.bout.JobID == id {
			s.active.cancel()
			out = cloneBoutJob(*s.active.bout)
			return nil
		}
		if s.queued != nil && s.queued.bout != nil && s.queued.published && s.queued.bout.JobID == id {
			s.cancelQueuedBoutLocked()
			out = cloneBoutJob(*s.boutLast)
			return nil
		}
		if s.boutLast != nil && s.boutLast.JobID == id {
			out = cloneBoutJob(*s.boutLast)
			return nil
		}
		return gymbout.ErrNotFound
	})
	if e != nil {
		return BoutJob{}, e
	}
	return out, nil
}
