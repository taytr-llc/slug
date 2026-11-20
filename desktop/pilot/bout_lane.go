// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func (s *StableController) beginBout(ctx context.Context, v, date, bid, kind string) (BoutJob, error) {
	if !entryDateValid(date) {
		return BoutJob{}, gymbout.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return BoutJob{}, gymbout.ErrInvalid
	}
	var task *stableTask
	var out BoutJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.boutEnabled || !s.boutAttached || !s.installed || s.stableClosed {
			return gymbout.ErrUnavailable
		}
		if s.boutStore == nil {
			return errBoutSetup
		}
		if s.stopping || s.evidenceBusyLocked() {
			return gymbout.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.bout != nil {
			return gymbout.ErrBusy
		}
		ev, e := gymevent.EventForDate(s.boutPin.SourcePin.SourcePin.SourcePin, date)
		if e != nil {
			return e
		}
		key, e := gymcard.CardID(s.boutPin.SourcePin, ev)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.boutBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+MaxBoutCacheSize+s.resultBytes) {
			return gymbout.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = BoutJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.boutID, CardID: key, Saturday: date, BoutID: bid}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		task = &stableTask{bout: &owned, ctx: taskctx, cancel: cancel}
		s.queued = task
		s.boutBytes = MaxBoutCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && task != nil && s.queued == task && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		task.published = true
		s.boutLast = nil
		s.boutFailure = ""
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymbout.ErrCancelled
		}
		if task != nil {
			task.cancel()
			if s.queued == task {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.boutBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return BoutJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainBoutLocked(j BoutJob) {
	if s.stableClosed {
		return
	}
	owned := cloneBoutJob(j)
	s.boutLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.boutBytes = boutRetainedMetadataBytes + len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedBoutLocked() {
	t := s.queued
	if t == nil || t.bout == nil {
		return
	}
	t.cancel()
	j := *t.bout
	j.State = "cancelled"
	j.Error = "bout_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainBoutLocked(j)
	}
}
func (s *StableController) runBoutTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.bout
	var e error
	if s.boutCheckpoint != nil {
		s.boutCheckpoint("before-bout")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && (s.boutStore == nil || s.seedStore == nil || s.eventStore == nil || s.entryStore == nil) {
		e = gymbout.ErrUnavailable
	}
	committed := false
	if e == nil {
		r, callErr := s.boutStore.Run(t.ctx, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID)
		e = callErr

		if e == nil {
			committed = true
			j.Result = &r
			j.State = "completed"
			if !boutJobValid(&j) {
				e = gymbout.ErrCorrupt
			}
			raw, _ := json.Marshal(j)
			if len(raw) > MaxBoutJobSize {
				e = gymbout.ErrCapacity
			}
		}
	}
	if e == nil {
		if s.boutCheckpoint != nil {
			s.boutCheckpoint("before-bout-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymbout.ErrUnavailable
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymbout.ErrCorrupt) || errors.Is(e, gymbout.ErrPoisoned) || errors.Is(e, gymbout.ErrClosed) || errors.Is(e, gymbout.ErrUnavailable)
	if committed && e != nil {
		e = gymbout.ErrOutcomeUnknown
	}
	if errors.Is(e, gymbout.ErrOutcomeUnknown) {
		fatal = true
	}
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = boutProtocolError(e)
		if !errors.Is(e, gymbout.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymbout.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "bout_cancelled"
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
			s.boutFailure = "bout_unavailable"
		}
		s.retainBoutLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymbout.ErrUnavailable
	}
	return nil
}

func (s *StableController) BeginBoutRun(ctx context.Context, v, date, bid string) (BoutJob, error) {
	return s.beginBout(ctx, v, date, bid, "run")
}
