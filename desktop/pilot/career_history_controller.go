// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"time"
)

func cloneHistoryJob(j CareerHistoryJob) CareerHistoryJob {
	b, _ := json.Marshal(j)
	var x CareerHistoryJob
	_ = json.Unmarshal(b, &x)
	return x
}
func historyFatal(e error) bool {
	return errors.Is(e, gymstable.ErrClock) || errors.Is(e, careerstate.ErrUnfenced) || errors.Is(e, gymstable.ErrCorrupt) || errors.Is(e, gymstable.ErrPoisoned) || errors.Is(e, gymstable.ErrClosed) || errors.Is(e, careerstate.ErrCorrupt) || errors.Is(e, careerstate.ErrPoisoned) || errors.Is(e, careerstate.ErrClosed)
}

// Caller owns stableMu. Known observation uncertainty must survive a later
// dependency-close report; no later healthy read repairs this family latch.
func (s *StableController) latchHistoryFailureLocked(e error) {
	if !errors.Is(s.historyFailure, gymstable.ErrOutcomeUnknown) {
		s.historyFailure = e
	}
}

// Caller owns stableMu. Invalidation cannot let a late task restore a handle.
func (s *StableController) invalidateHistoryLocked() {
	s.cancelEvidenceLocked()
	s.historyEpoch++
	s.historyHandle = nil
	s.historyLast = nil
	for _, t := range []*stableTask{s.active, s.queued} {
		if t != nil && t.history != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
			}
		}
	}
	s.mu.Lock()
	// Cancellation is not a join: an active task may still own its proof or
	// prospective capture. Keep its full charge until that exact task exits.
	if s.active != nil && s.active.history != nil {
		s.historyBytes = careerHistoryAllowance
	} else {
		s.historyBytes = 0
	}
	s.mu.Unlock()
}
func (s *StableController) historyActivationLocked() (careerstate.Activation, error) {
	if s.historyFailure != nil {
		return careerstate.Activation{}, s.historyFailure
	}
	if s.stableClosed || !s.installed || s.stableStatus == nil {
		return careerstate.Activation{}, gymstable.ErrClosed
	}
	if !s.careerEnabled || s.careerReader == nil {
		return careerstate.Activation{}, careerstate.ErrInactive
	}
	a, e := s.careerReader.Activation()
	if historyFatal(e) {
		s.latchHistoryFailureLocked(e)
		s.invalidateHistoryLocked()
	}
	return a, e
}
func (s *StableController) historyView(ctx context.Context, v string, fn func() error) error {
	if ctx == nil {
		return gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, careerMetadataTimeout)
	defer cancel()
	if v != s.view.ViewID || !canonicalManifestID(v) {
		return gymstable.ErrStaleView
	}
	lease := s.stableConfig.HistoryPublicationLease
	if lease == nil {
		lease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			return s.stableConfig.Lease(ctx, gymaccount.View, fn)
		}
	}
	e := lease(work, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.account.AccountID || a.Generation != s.view.Generation || check == nil {
			return gymstable.ErrUnauthorized
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return check()
	})
	return historyMetadataError(work, e)
}
func (s *StableController) CareerHistorySnapshot(ctx context.Context) (CareerHistorySnapshot, error) {
	blank := CareerHistorySnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}
	out := blank
	e := s.historyView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		s.expireQueuedLocked()
		a, e := s.historyActivationLocked()
		if e != nil {
			return e
		}
		v := s.viewPin()
		out = CareerHistorySnapshot{Version: 1, State: "idle", View: &v, StoreID: s.stableStatus.StoreID, Activation: &a}
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.history != nil && (t == s.active || t.published) {
				out.State = "busy"
				if t == s.active {
					out.ActiveJobID = t.history.JobID
				} else {
					out.QueuedJobID = t.history.JobID
				}
				return nil
			}
		}
		if j := s.historyLast; j != nil {
			out.State = j.State
			out.LastJobID = j.JobID
			out.Error = j.Error
			if j.State == "done" {
				out.State = "ready"
			}
		}
		return nil
	})
	if e != nil {
		code := careerHistoryProtocolError(e)
		if historyUnavailableError(code) {
			blank.Error = code
			return blank, nil
		}
		return CareerHistorySnapshot{}, e
	}
	return out, nil
}

// BeginCareerHistoryAdmission is parent wiring for one enclosing metadata budget.
// taskParent is the original caller, never the ephemeral admission subcontext.
func (s *StableController) BeginCareerHistory(ctx context.Context, v, pid string) (CareerHistoryJob, error) {
	return s.BeginCareerHistoryAdmission(ctx, ctx, v, pid)
}
func (s *StableController) historyPublish(ctx context.Context, v string, fn func(func() error) error) error {
	if ctx == nil || v != s.view.ViewID || !canonicalManifestID(v) {
		return gymstable.ErrStaleView
	}
	work, cancel := context.WithTimeout(ctx, careerMetadataTimeout)
	defer cancel()
	lease := s.stableConfig.HistoryPublicationLease
	if lease == nil {
		lease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			return s.stableConfig.Lease(ctx, gymaccount.View, fn)
		}
	}
	e := lease(work, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.account.AccountID || a.Generation != s.view.Generation || check == nil {
			return gymstable.ErrUnauthorized
		}
		if e := check(); e != nil {
			return e
		}
		return fn(check)
	})
	return historyMetadataError(work, e)
}
func (s *StableController) BeginCareerHistoryAdmission(admission, taskParent context.Context, v, pid string) (CareerHistoryJob, error) {
	if admission == nil || taskParent == nil || !canonicalManifestID(pid) {
		return CareerHistoryJob{}, gymstable.ErrInvalid
	}
	// This entire method, including the second account publication, has one budget.
	work, cancelAdmission := context.WithTimeout(admission, careerMetadataTimeout)
	defer cancelAdmission()
	var t *stableTask
	var out CareerHistoryJob
	e := s.historyView(work, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		s.expireQueuedLocked()
		a, e := s.historyActivationLocked()
		if e != nil {
			return e
		}
		if s.stopping || s.evidenceBusyLocked() || s.queued != nil || s.active != nil && s.active.history != nil {
			return gymstable.ErrBusy
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, careerHistoryAllowance+s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes) {
			return gymstable.ErrCapacity
		}
		var id [32]byte
		if _, e := rand.Read(id[:]); e != nil {
			return e
		}
		out = CareerHistoryJob{Version: 1, JobID: hex.EncodeToString(id[:]), State: "queued", View: s.viewPin(), StoreID: s.stableStatus.StoreID, ActivationID: a.ActivationID, ProspectID: pid}
		taskctx, cancel := context.WithTimeout(taskParent, 30*time.Second)
		owned := out
		t = &stableTask{history: &owned, historyPriorBytes: s.historyBytes, historyEpoch: s.historyEpoch, ctx: taskctx, cancel: cancel}
		s.queued = t
		s.historyBytes = careerHistoryAllowance
		return nil
	})
	published := false
	if e == nil {
		e = s.historyPublish(work, v, func(check func() error) error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if t == nil || s.queued != t || s.stableClosed || s.stopping || s.historyEpoch != t.historyEpoch {
				return gymstable.ErrCancelled
			}
			if s.historyCheckpoint != nil {
				s.historyCheckpoint("before-admission-publication")
			}
			if e := t.ctx.Err(); e != nil {
				return e
			}
			if e := check(); e != nil {
				return e
			}
			t.published = true
			published = true
			s.historyHandle = nil
			s.historyLast = nil
			s.paused = false
			if s.active != nil && s.active.automatic {
				s.active.cancel()
			}
			return nil
		})
	}
	if !published {
		s.stableMu.Lock()
		if t != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
				s.mu.Lock()
				if !s.stableClosed && s.historyEpoch == t.historyEpoch {
					s.historyBytes = t.historyPriorBytes
				}
				s.mu.Unlock()
			}
		}
		s.stableMu.Unlock()
		if e == nil {
			e = gymstable.ErrCancelled
		}
		return CareerHistoryJob{}, historyMetadataError(work, e)
	}
	// Publication was admitted while the original lease was held. Lost outer ACK
	// cannot retract it or permit automatic resubmission.
	s.startStoppedLoop()
	s.signal()
	if e != nil {
		return CareerHistoryJob{}, errHistoryReplyUnknown
	}
	return out, nil
}
func (s *StableController) retainHistoryLocked(j CareerHistoryJob, h *gymstable.CareerHistoryRead) {
	owned := cloneHistoryJob(j)
	s.historyLast = &owned
	s.historyHandle = h
	s.mu.Lock()
	if h != nil {
		s.historyBytes = careerHistoryAllowance
	} else {
		b, _ := json.Marshal(j)
		s.historyBytes = len(b) + 16*1024
	}
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedHistoryLocked() {
	t := s.queued
	if t == nil || t.history == nil {
		return
	}
	t.cancel()
	s.queued = nil
	if t.published && s.historyEpoch == t.historyEpoch && !s.stableClosed && !s.stopping {
		j := *t.history
		j.State = "cancelled"
		j.Error = "history_cancelled"
		j.Capture = nil
		s.retainHistoryLocked(j, nil)
	} else {
		s.mu.Lock()
		if !s.stableClosed && s.historyEpoch == t.historyEpoch {
			s.historyBytes = t.historyPriorBytes
		}
		s.mu.Unlock()
	}
}
func (s *StableController) historyLookup(ctx context.Context, v, id string, cancel bool) (CareerHistoryJob, error) {
	if !canonicalManifestID(id) {
		return CareerHistoryJob{}, gymstable.ErrInvalid
	}
	var out CareerHistoryJob
	e := s.historyView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		s.expireQueuedLocked()
		if _, e := s.historyActivationLocked(); e != nil {
			return e
		}
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.history != nil && (t == s.active || t.published) && t.history.JobID == id {
				if cancel {
					if t == s.queued {
						s.cancelQueuedHistoryLocked()
						out = cloneHistoryJob(*s.historyLast)
						return nil
					}
					t.cancel()
				}
				out = cloneHistoryJob(*t.history)
				return nil
			}
		}
		if s.historyLast != nil && s.historyLast.JobID == id {
			out = cloneHistoryJob(*s.historyLast)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if e != nil {
		return CareerHistoryJob{}, e
	}
	return out, nil
}
func (s *StableController) CareerHistoryJob(ctx context.Context, v, id string) (CareerHistoryJob, error) {
	return s.historyLookup(ctx, v, id, false)
}
func (s *StableController) CancelCareerHistory(ctx context.Context, v, id string) (CareerHistoryJob, error) {
	return s.historyLookup(ctx, v, id, true)
}
func (s *StableController) CareerHistoryPage(ctx context.Context, v, id string, after, limit uint32) (CareerHistoryPageResult, error) {
	if ctx == nil || !canonicalManifestID(id) || after > 256 || limit == 0 || limit > 8 {
		return CareerHistoryPageResult{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, careerMetadataTimeout)
	defer cancel()
	if e := s.historyView(work, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		_, e := s.historyActivationLocked()
		return e
	}); e != nil {
		return CareerHistoryPageResult{}, e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return CareerHistoryPageResult{}, gymstable.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	_, e := s.historyActivationLocked()
	h := s.historyHandle
	store := s.stableStore
	epoch := s.historyEpoch
	selected := s.historyLast != nil && s.historyLast.JobID == id && s.historyLast.State == "done" && s.historyLast.View == s.viewPin() && !s.stopping
	var expected *gymstable.CareerHistoryCapture
	if selected {
		expected = s.historyLast.Capture
	}
	s.stableMu.Unlock()
	if e != nil {
		return CareerHistoryPageResult{}, e
	}
	if !selected || h == nil || store == nil {
		return CareerHistoryPageResult{}, gymstable.ErrNotFound
	}
	p, e := store.ReadCareerHistory(work, s.session(), h, after, limit)
	if e != nil {
		if historyFatal(e) || errors.Is(e, gymstable.ErrOutcomeUnknown) {
			s.stableMu.Lock()
			s.latchHistoryFailureLocked(e)
			s.invalidateHistoryLocked()
			s.stableMu.Unlock()
		}
		return CareerHistoryPageResult{}, historyMetadataError(work, e)
	}
	out := CareerHistoryPageResult{View: s.viewPin(), StoreID: p.Capture.Status.StoreID, JobID: id, Page: p}
	if expected == nil || p.Capture != *expected || ValidateCareerHistoryPageResult(out) != nil || len(p.Items) != min(int(limit), int(p.Capture.EventCount-after)) {
		s.stableMu.Lock()
		s.latchHistoryFailureLocked(gymstable.ErrCorrupt)
		s.invalidateHistoryLocked()
		s.stableMu.Unlock()
		return CareerHistoryPageResult{}, gymstable.ErrCorrupt
	}
	e = s.historyView(work, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if _, e := s.historyActivationLocked(); e != nil {
			return e
		}
		if s.stopping || s.historyEpoch != epoch || s.historyHandle != h || s.historyLast == nil || s.historyLast.JobID != id {
			return gymstable.ErrNotFound
		}
		return nil
	})
	if e != nil {
		return CareerHistoryPageResult{}, e
	}
	return out, nil
}
func (s *StableController) runHistoryTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.history
	var h *gymstable.CareerHistoryRead
	var cap gymstable.CareerHistoryCapture
	var e error
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil {
		if s.historyCheckpoint != nil {
			s.historyCheckpoint("before-capture")
		}
		cap, h, e = s.stableStore.PrepareCareerHistory(t.ctx, s.session(), j.ProspectID)
	}
	if e == nil && (h == nil || h.RetainedBytes() > careerHistoryCaptureAllowance || !historyCaptureValid(cap, j.View) || cap.Status.StoreID != j.StoreID || cap.Activation.ActivationID != j.ActivationID || cap.Assignment.ProspectID != j.ProspectID) {
		e = gymstable.ErrCorrupt
	}
	if e == nil {
		if s.historyCheckpoint != nil {
			s.historyCheckpoint("captured")
		}
		var p gymstable.CareerHistoryPage
		p, e = s.stableStore.ReadCareerHistory(t.ctx, s.session(), h, 0, 1)
		if e == nil && p.Capture != cap {
			e = gymstable.ErrCorrupt
		}
	}
	if e == nil {
		j.State = "done"
		j.Capture = &cap
		if ValidateCareerHistoryJob(j) != nil {
			e = gymstable.ErrCorrupt
		}
	}
	if e == nil {
		if s.historyCheckpoint != nil {
			s.historyCheckpoint("before-publication")
		}
	}
	// Failures are prospective terminal metadata too. Only the actual active task
	// may publish, under the original parent/account lease and final guard.
	if e != nil {
		j.Capture = nil
		h = nil
		j.State = "failed"
		j.Error = careerHistoryProtocolError(e)
		if j.Error == "busy" {
			j.Error = "history_cancelled"
		}
		if j.Error == "history_cancelled" {
			j.State = "cancelled"
		}
	}
	// A fatal Stable/source failure is learned even if the separate Result
	// reader still reports a live activation. It cannot be repaired by Begin.
	if historyFatal(e) {
		s.stableMu.Lock()
		if s.active == t {
			s.latchHistoryFailureLocked(e)
			s.invalidateHistoryLocked()
		}
		s.stableMu.Unlock()
	}
	committed := false
	pubctx := t.ctx
	// A cancelled task still needs a fresh original-authority lease to expose its
	// joined cancelled terminal. The lane/owner context remains lifetime-bound.
	if t.ctx.Err() != nil {
		pubctx = lane
	}
	pe := s.historyPublish(pubctx, j.View.ViewID, func(check func() error) error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		if s.stopping || s.active != t || s.historyEpoch != t.historyEpoch {
			return gymstable.ErrCancelled
		}
		if _, x := s.historyActivationLocked(); x != nil {
			return x
		}
		if e == nil && t.ctx.Err() != nil {
			j.State = "cancelled"
			j.Error = "history_cancelled"
			j.Capture = nil
			h = nil
		}
		if s.historyCheckpoint != nil {
			s.historyCheckpoint("terminal-publication")
		}
		if x := check(); x != nil {
			return x
		}
		s.active = nil
		s.retainHistoryLocked(j, h)
		committed = true
		return nil
	})
	if !committed {
		s.stableMu.Lock()
		if s.active == t {
			if historyFatal(pe) || historyFatal(e) || errors.Is(e, gymstable.ErrOutcomeUnknown) {
				failure := pe
				if historyFatal(e) {
					failure = e
				}
				// Keep a known observation uncertainty above a later dependency
				// closure. Original-account checks still precede every poll.
				if errors.Is(e, gymstable.ErrOutcomeUnknown) {
					failure = errors.Join(e, pe)
				}
				s.latchHistoryFailureLocked(failure)
				s.invalidateHistoryLocked()
			}
			s.active = nil
			// This exact task has now joined. It alone may release a discarded
			// reservation; a successor or retained terminal is never touched.
			if s.historyHandle == nil && s.historyLast == nil {
				s.mu.Lock()
				s.historyBytes = 0
				s.mu.Unlock()
			}
		}
		s.stableMu.Unlock()
	}
	// Once committed, later cancellation or a wrapper return cannot rewrite the
	// immutable terminal that was published under the exact original lease.
	// No shared Status overwrite: captured time/count may be older than other work.
	return nil
}
