// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func (s *StableController) BeginEventCapture(ctx context.Context, v, date string) (EventJob, error) {
	if !entryDateValid(date) {
		return EventJob{}, gymevent.ErrCalendar
	}
	var task *stableTask
	var out EventJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.eventEnabled || !s.eventAttached || !s.installed || s.stableClosed {
			return gymevent.ErrUnavailable
		}
		if s.eventStore == nil {
			return errEventSetup
		}
		if s.stopping {
			return gymevent.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.event != nil {
			return gymevent.ErrBusy
		}
		ev, e := gymevent.EventForDate(s.eventPin, date)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.eventBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+MaxEventCacheSize+s.seedBytes+s.cardBytes+s.boutBytes) {
			return gymevent.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = EventJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: "capture", State: "queued", View: s.viewPin(), StoreID: s.eventID, EventID: ev.EventID, Saturday: date, ParticipantID: s.eventParticipant}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		task = &stableTask{event: &owned, ctx: taskctx, cancel: cancel}
		s.queued = task
		s.eventBytes = MaxEventCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && task != nil && s.queued == task && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		task.published = true
		s.eventLast = nil
		s.eventFailure = ""
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymevent.ErrCancelled
		}
		if task != nil {
			task.cancel()
			if s.queued == task {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.eventBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return EventJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainEventLocked(j EventJob) {
	if s.stableClosed {
		return
	}
	owned := cloneEventJob(j)
	s.eventLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.eventBytes = eventMetadataBytes + len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedEventLocked() {
	t := s.queued
	if t == nil || t.event == nil {
		return
	}
	t.cancel()
	j := *t.event
	j.State = "cancelled"
	j.Error = "event_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainEventLocked(j)
	}
}
func (s *StableController) runEventTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.event
	var e error
	if s.eventCheckpoint != nil {
		s.eventCheckpoint("before-event")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && (s.eventStore == nil || s.entryStore == nil) {
		e = gymevent.ErrUnavailable
	}
	committed := false
	if e == nil {
		var r gymevent.CaptureResult
		r, e = s.eventStore.Capture(t.ctx, s.entryStore, s.session(), j.Saturday)
		if e == nil {
			committed = true
			j.Result = &r
			j.State = "completed"
			if _, ve := gymevent.ValidateCaptureResult(s.eventPin, r); ve != nil {
				e = gymevent.ErrCorrupt
			} else if !eventJobValid(&j) {
				e = gymevent.ErrCorrupt
			}
			raw, _ := json.Marshal(j)
			if len(raw)+eventMetadataBytes > MaxEventCacheSize {
				e = gymevent.ErrCapacity
			}
		}
	}
	if e == nil {
		if s.eventCheckpoint != nil {
			s.eventCheckpoint("before-event-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymevent.ErrUnavailable
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymevent.ErrCorrupt) || errors.Is(e, gymevent.ErrPoisoned) || errors.Is(e, gymevent.ErrClosed) || errors.Is(e, gymevent.ErrUnavailable)
	if committed && e != nil {
		e = gymevent.ErrOutcomeUnknown
	}
	if errors.Is(e, gymevent.ErrOutcomeUnknown) {
		fatal = true
	}
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = eventProtocolError(e)
		if !errors.Is(e, gymevent.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymevent.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "event_cancelled"
		}
	}
	s.stableMu.Lock()
	if s.active == t {
		s.active = nil
	}
	if !s.stableClosed {
		if e == nil && (t.ctx.Err() != nil || s.stopping) {
			j.Result = nil
			j.State = "failed"
			j.Error = "outcome_unknown"
		}
		if fatal {
			s.eventFailure = "event_unavailable"
		}
		s.retainEventLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymevent.ErrUnavailable
	}
	return nil
}
