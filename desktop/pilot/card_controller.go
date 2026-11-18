// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"time"
)

// Card metadata reads include durable account and source-clock observations.
// This admission/operation budget preserves a shorter caller deadline; a Sync
// already in progress must still join and can return observation uncertainty.
// Cached polls, short View checks, other families and jobs keep their budgets.
const cardMetadataTimeout = 500 * time.Millisecond

func (s *StableController) CardSnapshot() CardSnapshot {
	blank := CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.cardEnabled || !s.cardAttached || !s.installed || s.stableClosed {
			return gymcard.ErrUnavailable
		}
		v := s.viewPin()
		out = CardSnapshot{Version: 1, State: "setup_incomplete", Error: "card_setup_incomplete", View: &v}
		if s.cardStore == nil {
			return nil
		}
		p := cloneCardPin(s.cardPin)
		out.State = "ready"
		out.Error = ""
		out.Pin = &p
		out.StoreID = s.cardID
		out.CreatedAtMS = s.cardCreated
		out.SourceCreatedAtMS = s.cardSourceCreated
		s.expireQueuedLocked()
		if s.cardLast != nil {
			out.LastJobID = s.cardLast.JobID
		}
		if s.cardFailure != "" {
			out.State = "failed"
			out.Error = s.cardFailure
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.card != nil {
			out.ActiveJobID = s.active.card.JobID
		}
		if s.queued != nil && s.queued.card != nil && s.queued.published {
			out.QueuedJobID = s.queued.card.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}
func (s *StableController) cardRead(ctx context.Context, v string, fn func(context.Context, *gymcard.Store, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, cardMetadataTimeout)
	defer cancel()
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymcard.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.cardEnabled && s.cardAttached && s.installed && !s.stableClosed && !s.stopping
	store := s.cardStore
	s.stableMu.Unlock()
	if !valid {
		return gymcard.ErrUnavailable
	}
	if store == nil {
		return errCardSetup
	}
	if s.entryStore == nil {
		return gymcard.ErrBusy
	}
	if ctx.Err() != nil {
		return gymcard.ErrBusy
	}
	if e := fn(ctx, store, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymcard.ErrOutcomeUnknown) {
			return gymcard.ErrBusy
		}
		return e
	}
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return gymcard.ErrOutcomeUnknown
	}
	return nil
}
func (s *StableController) CardStatus(ctx context.Context, v, date string) (gymcard.Status, error) {
	if !entryDateValid(date) {
		return gymcard.Status{}, gymcard.ErrCalendar
	}
	var out gymcard.Status
	e := s.cardRead(ctx, v, func(ctx context.Context, x *gymcard.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Status(ctx, s.seedStore, s.eventStore, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymcard.Status{}, e
	}
	return out, nil
}
func (s *StableController) CardRecord(ctx context.Context, v, date string) (gymcard.ReadResult, error) {
	if !entryDateValid(date) {
		return gymcard.ReadResult{}, gymcard.ErrCalendar
	}
	var out gymcard.ReadResult
	e := s.cardRead(ctx, v, func(ctx context.Context, x *gymcard.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Read(ctx, s.seedStore, s.eventStore, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymcard.ReadResult{}, e
	}
	return out, nil
}
func (s *StableController) CardHistory(ctx context.Context, v, after string, limit uint32) (gymcard.HistoryPage, error) {
	if limit < 1 || limit > 8 || after != "" && !canonicalManifestID(after) {
		return gymcard.HistoryPage{}, gymcard.ErrInvalid
	}
	var out gymcard.HistoryPage
	e := s.cardRead(ctx, v, func(ctx context.Context, x *gymcard.Store, source *gymentry.Store) error {
		var e error
		out, e = x.History(ctx, s.seedStore, s.eventStore, source, s.session(), after, limit)
		return e
	})
	if e != nil {
		return gymcard.HistoryPage{}, e
	}
	return out, nil
}
func (s *StableController) CardJob(ctx context.Context, v, id string) (CardJob, error) {
	if !canonicalManifestID(id) {
		return CardJob{}, gymcard.ErrInvalid
	}
	var out CardJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.cardEnabled || s.stableClosed {
			return gymcard.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.card != nil && (t == s.active || t.published) && t.card.JobID == id {
				out = cloneCardJob(*t.card)
				return nil
			}
		}
		if s.cardLast != nil && s.cardLast.JobID == id {
			out = cloneCardJob(*s.cardLast)
			return nil
		}
		return gymcard.ErrNotFound
	})
	if e != nil {
		return CardJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelCardJob(ctx context.Context, v, id string) (CardJob, error) {
	if !canonicalManifestID(id) {
		return CardJob{}, gymcard.ErrInvalid
	}
	var out CardJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.cardEnabled || s.stableClosed {
			return gymcard.ErrUnavailable
		}
		if s.active != nil && s.active.card != nil && s.active.card.JobID == id {
			s.active.cancel()
			out = cloneCardJob(*s.active.card)
			return nil
		}
		if s.queued != nil && s.queued.card != nil && s.queued.published && s.queued.card.JobID == id {
			s.cancelQueuedCardLocked()
			out = cloneCardJob(*s.cardLast)
			return nil
		}
		if s.cardLast != nil && s.cardLast.JobID == id {
			out = cloneCardJob(*s.cardLast)
			return nil
		}
		return gymcard.ErrNotFound
	})
	if e != nil {
		return CardJob{}, e
	}
	return out, nil
}
