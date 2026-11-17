// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func (s *StableController) beginEntry(ctx context.Context, view, kind, date, op string, request gymentry.Request) (EntryJob, error) {
	var t *stableTask
	var out EntryJob
	var oldBytes int
	e := s.shortView(ctx, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.entryEnabled || !s.installed || s.stableClosed || s.entryStoreID == "" {
			return gymentry.ErrUnavailable
		}
		if s.stopping {
			return gymentry.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.entry != nil {
			return gymentry.ErrBusy
		}
		ev, e := gymentry.EventForDate(s.entryPin, date)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.entryBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+MaxEntryCacheSize) {
			return gymentry.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = EntryJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.entryStoreID, EventID: ev.EventID, Saturday: date, OperationID: op}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		t = &stableTask{entry: &owned, entryRequest: request, ctx: taskctx, cancel: cancel}
		s.queued = t
		s.entryBytes = MaxEntryCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && t != nil && s.queued == t && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		t.published = true
		s.entryLast = nil
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymentry.ErrCancelled
		}
		if t != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.entryBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return EntryJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) BeginEntryPrepare(ctx context.Context, v string, r gymentry.Request) (EntryJob, error) {
	owned, e := gymentry.ValidateRequest(r)
	if e != nil {
		return EntryJob{}, e
	}
	return s.beginEntry(ctx, v, "prepare", r.Saturday, "", owned)
}
func (s *StableController) BeginEntryCheck(ctx context.Context, v, date string) (EntryJob, error) {
	if !entryDateValid(date) {
		return EntryJob{}, gymentry.ErrCalendar
	}
	return s.beginEntry(ctx, v, "check", date, "", gymentry.Request{})
}
func (s *StableController) BeginEntryApply(ctx context.Context, v, id string) (EntryJob, error) {
	op, e := s.EntryOperation(ctx, v, id)
	if e != nil {
		return EntryJob{}, e
	}
	if op.Operation == nil {
		return EntryJob{}, gymentry.ErrNotFound
	}
	i := op.Operation.Intent
	if i.ViewID != v || i.Generation != s.view.Generation {
		return EntryJob{}, gymentry.ErrStaleView
	}
	return s.beginEntry(ctx, v, "apply", i.Request.Saturday, id, gymentry.Request{})
}
func (s *StableController) retainEntryLocked(j EntryJob) {
	if s.stableClosed {
		return
	}
	owned := cloneEntry(j)
	s.entryLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.entryBytes = len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedEntryLocked() {
	t := s.queued
	if t == nil || t.entry == nil {
		return
	}
	t.cancel()
	j := *t.entry
	j.State = "cancelled"
	j.Error = "entry_cancelled"
	s.queued = nil
	if t.published {
		s.retainEntryLocked(j)
	}
}
func (s *StableController) runEntryTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.entry
	var e error
	var pending *gymentry.OperationResult
	if s.entryCheckpoint != nil {
		s.entryCheckpoint("before-entry")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && s.entryStore == nil {
		e = gymentry.ErrUnavailable
	}
	if e == nil {
		switch j.Kind {
		case "prepare":
			var x gymentry.Intent
			x, e = s.entryStore.Prepare(t.ctx, s.session(), t.entryRequest)
			if e == nil {
				j.Intent = &x
				j.OperationID = x.OperationID
			}
		case "apply":
			var x gymentry.MutationResult
			x, e = s.entryStore.Apply(t.ctx, s.session(), j.OperationID)
			if e == nil {
				j.Mutation = &x
			}
		case "check":
			var x gymentry.CheckResult
			x, e = s.entryStore.Check(t.ctx, s.session(), j.Saturday)
			if e == nil {
				j.Check = &x
			}
		default:
			e = gymentry.ErrInvalid
		}
	}
	committed := e == nil && (j.Kind == "prepare" || j.Kind == "apply")
	if e == nil && (j.Kind == "prepare" || j.Kind == "apply") {
		x, pe := s.entryStore.Pending(t.ctx, s.session())
		if pe != nil {
			e = gymentry.ErrOutcomeUnknown
		} else {
			pending = &x
		}
	}
	if e == nil {
		j.State = "complete"
		public := cloneEntry(j)
		if !entryJobValid(&public) {
			e = gymentry.ErrCorrupt
		} else {
			j = public
		}
	}
	if e == nil {
		if s.entryCheckpoint != nil {
			s.entryCheckpoint("before-entry-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymentry.ErrUnavailable
			}
			return t.ctx.Err()
		})
		if e != nil && committed {
			e = gymentry.ErrOutcomeUnknown
		}
	}
	fatal := errors.Is(e, gymentry.ErrCorrupt) || errors.Is(e, gymentry.ErrPoisoned) || errors.Is(e, gymentry.ErrUnavailable)
	// A successful core mutation remains durable even when its public wrapper
	// or final publication fails. Keep fatal dependency handling, but never
	// describe that known commit as a definite failed mutation.
	if committed && e != nil {
		e = gymentry.ErrOutcomeUnknown
	}
	if errors.Is(e, gymentry.ErrOutcomeUnknown) {
		fatal = true
	}
	if e != nil {
		j.Intent = nil
		j.Mutation = nil
		j.Check = nil
		j.State = "failed"
		j.Error = entryProtocolError(e)
		if !errors.Is(e, gymentry.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymentry.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "entry_cancelled"
		}
	}
	s.stableMu.Lock()
	if s.active == t {
		s.active = nil
	}
	if !s.stableClosed {
		if e == nil && (t.ctx.Err() != nil || s.stopping) {
			j.Intent = nil
			j.Mutation = nil
			j.Check = nil
			j.State = "failed"
			j.Error = "entry_cancelled"
			if committed {
				j.Error = "outcome_unknown"
			} else {
				j.State = "cancelled"
			}
		}
		if pending != nil {
			s.entryPending = ""
			if pending.Operation != nil {
				s.entryPending = pending.Operation.Intent.OperationID
			}
		}
		s.retainEntryLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymentry.ErrUnavailable
	}
	return nil
}
