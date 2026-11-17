// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"time"
)

func cloneEntry(j EntryJob) EntryJob {
	raw, _ := json.Marshal(j)
	var out EntryJob
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s *StableController) EntrySnapshot() EntrySnapshot {
	blank := EntrySnapshot{Version: 1, State: "unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.entryEnabled || !s.installed || s.stableClosed || s.entryStoreID == "" {
			return gymentry.ErrUnavailable
		}
		s.expireQueuedLocked()
		v := s.viewPin()
		out = EntrySnapshot{Version: 1, State: "idle", View: &v, StoreID: s.entryStoreID, PendingOperationID: s.entryPending}
		if s.entryLast != nil {
			out.LastJobID = s.entryLast.JobID
			if s.entryLast.State == "failed" {
				out.State = "failed"
				out.Error = s.entryLast.Error
			}
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.entry != nil {
			out.ActiveJobID = s.active.entry.JobID
		}
		if s.queued != nil && s.queued.entry != nil && s.queued.published {
			out.QueuedJobID = s.queued.entry.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}

// entryRead never waits behind proof work. The short original View acquisition
// is cancellable, and a started core sync write is joined before this returns.
func (s *StableController) entryRead(ctx context.Context, view string, mutation bool, fn func(context.Context, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if e := s.shortView(ctx, view, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymentry.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.entryEnabled && s.installed && !s.stableClosed && !s.stopping
	s.stableMu.Unlock()
	if !valid || s.entryStore == nil {
		return gymentry.ErrUnavailable
	}
	if e := fn(ctx, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymentry.ErrOutcomeUnknown) {
			return gymentry.ErrBusy
		}
		return e
	}
	if e := s.shortView(ctx, view, func() error { return nil }); e != nil {
		if mutation {
			return gymentry.ErrOutcomeUnknown
		}
		return e
	}
	return nil
}
func (s *StableController) EntryUpcoming(ctx context.Context, v string) (gymentry.EventStatus, error) {
	var out gymentry.EventStatus
	e := s.entryRead(ctx, v, false, func(ctx context.Context, store *gymentry.Store) error {
		var e error
		out, e = store.Upcoming(ctx, s.session())
		return e
	})
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return out, nil
}
func (s *StableController) EntryStatus(ctx context.Context, v, date string) (gymentry.EventStatus, error) {
	if !entryDateValid(date) {
		return gymentry.EventStatus{}, gymentry.ErrCalendar
	}
	var out gymentry.EventStatus
	e := s.entryRead(ctx, v, false, func(ctx context.Context, store *gymentry.Store) error {
		var e error
		out, e = store.Status(ctx, s.session(), date)
		return e
	})
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return out, nil
}
func (s *StableController) EntryHistory(ctx context.Context, v, after string, limit uint32) (gymentry.HistoryPage, error) {
	if limit < 1 || limit > 8 || after != "" && !canonicalManifestID(after) {
		return gymentry.HistoryPage{}, gymentry.ErrInvalid
	}
	var out gymentry.HistoryPage
	e := s.entryRead(ctx, v, false, func(ctx context.Context, store *gymentry.Store) error {
		var e error
		out, e = store.History(ctx, s.session(), after, limit)
		return e
	})
	if e != nil {
		return gymentry.HistoryPage{}, e
	}
	return out, nil
}
func (s *StableController) entryOperation(ctx context.Context, v, id, kind string) (gymentry.OperationResult, error) {
	if kind != "pending" && !canonicalManifestID(id) {
		return gymentry.OperationResult{}, gymentry.ErrInvalid
	}
	var out gymentry.OperationResult
	e := s.entryRead(ctx, v, kind == "abandon", func(ctx context.Context, store *gymentry.Store) error {
		var e error
		switch kind {
		case "operation":
			out, e = store.Operation(ctx, s.session(), id)
		case "pending":
			out, e = store.Pending(ctx, s.session())
		case "abandon":
			out, e = store.Abandon(ctx, s.session(), id)
		}
		if e != nil {
			return e
		}
		var pending gymentry.OperationResult
		if kind == "pending" {
			pending = out
		} else if kind == "abandon" {
			pending, e = store.Pending(ctx, s.session())
			if e != nil {
				return gymentry.ErrOutcomeUnknown
			}
		} else {
			return nil
		}
		e = s.shortView(ctx, v, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			s.entryPending = ""
			if pending.Operation != nil {
				s.entryPending = pending.Operation.Intent.OperationID
			}
			return nil
		})
		if e != nil && kind == "abandon" {
			return gymentry.ErrOutcomeUnknown
		}
		return e
	})
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return out, nil
}
func (s *StableController) EntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	return s.entryOperation(ctx, v, id, "operation")
}
func (s *StableController) EntryPending(ctx context.Context, v string) (gymentry.OperationResult, error) {
	return s.entryOperation(ctx, v, "", "pending")
}
func (s *StableController) AbandonEntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	return s.entryOperation(ctx, v, id, "abandon")
}
func (s *StableController) EntryJob(ctx context.Context, v, id string) (EntryJob, error) {
	if !canonicalManifestID(id) {
		return EntryJob{}, gymentry.ErrInvalid
	}
	var out EntryJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.entryEnabled || s.stableClosed {
			return gymentry.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.entry != nil && (t == s.active || t.published) && t.entry.JobID == id {
				out = cloneEntry(*t.entry)
				return nil
			}
		}
		if s.entryLast != nil && s.entryLast.JobID == id {
			out = cloneEntry(*s.entryLast)
			return nil
		}
		return gymentry.ErrNotFound
	})
	if e != nil {
		return EntryJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelEntryJob(ctx context.Context, v, id string) (EntryJob, error) {
	if !canonicalManifestID(id) {
		return EntryJob{}, gymentry.ErrInvalid
	}
	var out EntryJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.entryEnabled || s.stableClosed {
			return gymentry.ErrUnavailable
		}
		if s.active != nil && s.active.entry != nil && s.active.entry.JobID == id {
			s.active.cancel()
			out = cloneEntry(*s.active.entry)
			return nil
		}
		if s.queued != nil && s.queued.entry != nil && s.queued.published && s.queued.entry.JobID == id {
			s.cancelQueuedEntryLocked()
			out = cloneEntry(*s.entryLast)
			return nil
		}
		if s.entryLast != nil && s.entryLast.JobID == id {
			out = cloneEntry(*s.entryLast)
			return nil
		}
		return gymentry.ErrNotFound
	})
	if e != nil {
		return EntryJob{}, e
	}
	return out, nil
}
