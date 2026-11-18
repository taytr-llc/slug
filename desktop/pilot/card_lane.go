// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func (s *StableController) beginCard(ctx context.Context, v, date, kind string) (CardJob, error) {
	if !entryDateValid(date) {
		return CardJob{}, gymcard.ErrCalendar
	}
	var task *stableTask
	var out CardJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.cardEnabled || !s.cardAttached || !s.installed || s.stableClosed {
			return gymcard.ErrUnavailable
		}
		if s.cardStore == nil {
			return errCardSetup
		}
		if s.stopping {
			return gymcard.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.card != nil {
			return gymcard.ErrBusy
		}
		ev, e := gymevent.EventForDate(s.cardPin.SourcePin.SourcePin, date)
		if e != nil {
			return e
		}
		key, e := gymcard.CardID(s.cardPin, ev)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.cardBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.boutBytes+MaxCardCacheSize) {
			return gymcard.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = CardJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.cardID, CardID: key, Saturday: date}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		task = &stableTask{card: &owned, ctx: taskctx, cancel: cancel}
		s.queued = task
		s.cardBytes = MaxCardCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && task != nil && s.queued == task && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		task.published = true
		s.cardLast = nil
		s.cardFailure = ""
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymcard.ErrCancelled
		}
		if task != nil {
			task.cancel()
			if s.queued == task {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.cardBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return CardJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainCardLocked(j CardJob) {
	if s.stableClosed {
		return
	}
	owned := cloneCardJob(j)
	s.cardLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.cardBytes = MaxCardMetadataSize + len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedCardLocked() {
	t := s.queued
	if t == nil || t.card == nil {
		return
	}
	t.cancel()
	j := *t.card
	j.State = "cancelled"
	j.Error = "card_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainCardLocked(j)
	}
}
func (s *StableController) runCardTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.card
	var e error
	if s.cardCheckpoint != nil {
		s.cardCheckpoint("before-card")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && (s.cardStore == nil || s.seedStore == nil || s.eventStore == nil || s.entryStore == nil) {
		e = gymcard.ErrUnavailable
	}
	committed := false
	if e == nil {
		r, callErr := s.cardStore.Build(t.ctx, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday)
		e = callErr

		if e == nil {
			committed = true
			j.Result = &r
			j.State = "completed"
			if _, ve := gymcard.ValidateBuildResult(s.cardPin, r); ve != nil {
				e = gymcard.ErrCorrupt
			} else if !cardJobValid(&j) {
				e = gymcard.ErrCorrupt
			}
			raw, _ := json.Marshal(j)
			if len(raw) > MaxCardJobSize {
				e = gymcard.ErrCapacity
			}
		}
	}
	if e == nil {
		if s.cardCheckpoint != nil {
			s.cardCheckpoint("before-card-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymcard.ErrUnavailable
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymcard.ErrCorrupt) || errors.Is(e, gymcard.ErrPoisoned) || errors.Is(e, gymcard.ErrClosed) || errors.Is(e, gymcard.ErrUnavailable)
	if committed && e != nil {
		e = gymcard.ErrOutcomeUnknown
	}
	if errors.Is(e, gymcard.ErrOutcomeUnknown) {
		fatal = true
	}
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = cardProtocolError(e)
		if !errors.Is(e, gymcard.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymcard.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "card_cancelled"
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
			s.cardFailure = "card_unavailable"
		}
		s.retainCardLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymcard.ErrUnavailable
	}
	return nil
}

func (s *StableController) BeginCardBuild(ctx context.Context, v, date string) (CardJob, error) {
	return s.beginCard(ctx, v, date, "build")
}
