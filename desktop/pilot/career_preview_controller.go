// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"time"
)

func cloneCareerPreview(j CareerPreviewJob) CareerPreviewJob {
	raw, _ := json.Marshal(j)
	var out CareerPreviewJob
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s *StableController) CareerPreviewSnapshot() CareerPreviewSnapshot {
	blank := CareerPreviewSnapshot{Version: 1, State: "unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil {
			return gymstable.ErrClosed
		}
		s.expireQueuedLocked()
		activation, err := s.careerPreviewActivationLocked()
		if err != nil {
			return err
		}
		v := s.viewPin()
		out = CareerPreviewSnapshot{Version: 1, State: "idle", View: &v, StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: careerPreviewEngineID, Activation: &activation}
		if s.active != nil && s.active.careerPreview != nil {
			out.ActiveJobID = s.active.careerPreview.JobID
			out.State = "busy"
		}
		if s.queued != nil && s.queued.careerPreview != nil && s.queued.published {
			out.QueuedJobID = s.queued.careerPreview.JobID
			out.State = "busy"
		}
		if s.careerPreviewLast != nil {
			j := s.careerPreviewLast
			out.LastJobID = j.JobID
			out.State = j.State
			out.Error = j.Error
			if j.State == "done" {
				out.State = "ready"
			}
		}
		return nil
	})
	if e != nil {
		code := careerPreviewProtocolError(e)
		if careerPreviewUnavailableError(code) {
			blank.Error = code
		}
		return blank
	}
	return out
}
func (s *StableController) BeginCareerFightPreview(ctx context.Context, v, a, b string) (CareerPreviewJob, error) {
	if !canonicalManifestID(a) || !canonicalManifestID(b) || a == b {
		return CareerPreviewJob{}, gymstable.ErrInvalid
	}
	if b < a {
		a, b = b, a
	}
	var t *stableTask
	var out CareerPreviewJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil {
			return gymstable.ErrClosed
		}
		if s.stopping {
			return gymstable.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && (s.active.preview != nil || s.active.careerPreview != nil) {
			return gymstable.ErrBusy
		}
		activation, err := s.careerPreviewActivationLocked()
		if err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.previewBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.careerBytes+max(oldBytes, careerPreviewAllowance)+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes) {
			return gymstable.ErrCapacity
		}
		var id [32]byte
		if _, e := rand.Read(id[:]); e != nil {
			return e
		}
		out = CareerPreviewJob{Version: 1, JobID: hex.EncodeToString(id[:]), State: "queued", View: s.viewPin(), StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: careerPreviewEngineID, ActivationID: activation.ActivationID, FighterAID: a, FighterBID: b}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		t = &stableTask{previewPriorBytes: oldBytes, careerPreview: &owned, ctx: taskctx, cancel: cancel}
		s.queued = t
		s.previewBytes = max(oldBytes, careerPreviewAllowance)
		return nil
	})
	s.stableMu.Lock()
	if e != nil {
		if t != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
				s.mu.Lock()
				if !s.stableClosed {
					s.previewBytes = oldBytes
				}
				s.mu.Unlock()
			}
		}
	} else if s.queued == t && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		t.published = true
		s.careerPreviewLast = nil
		s.previewLast = nil
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		e = gymstable.ErrCancelled
		if t != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
				s.mu.Lock()
				if !s.stableClosed {
					s.previewBytes = oldBytes
				}
				s.mu.Unlock()
			}
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return CareerPreviewJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainCareerPreviewLocked(j CareerPreviewJob) {
	if s.careerPreviewFailure != nil {
		s.dropCareerPreviewLocked()
		return
	}
	if s.stableClosed {
		return
	}
	owned := cloneCareerPreview(j)
	s.careerPreviewLast = &owned
	s.previewLast = nil
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.previewBytes = len(raw) + 2304
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedCareerPreviewLocked() {
	t := s.queued
	if t == nil || t.careerPreview == nil {
		return
	}
	t.cancel()
	j := *t.careerPreview
	j.State = "cancelled"
	j.Error = "preview_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainCareerPreviewLocked(j)
	} else {
		// An unpublished task owns its rollback only while it is this queue.
		s.mu.Lock()
		if !s.stableClosed {
			s.previewBytes = t.previewPriorBytes
		}
		s.mu.Unlock()
	}
}
func (s *StableController) CareerFightPreviewJob(ctx context.Context, v, id string) (CareerPreviewJob, error) {
	if !canonicalManifestID(id) {
		return CareerPreviewJob{}, gymstable.ErrInvalid
	}
	var out CareerPreviewJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		if s.careerPreviewFailure != nil {
			return s.careerPreviewFailure
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.careerPreview != nil && (t == s.active || t.published) && t.careerPreview.JobID == id {
				out = cloneCareerPreview(*t.careerPreview)
				return nil
			}
		}
		if s.careerPreviewLast != nil && s.careerPreviewLast.JobID == id {
			out = cloneCareerPreview(*s.careerPreviewLast)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if e != nil {
		return CareerPreviewJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelCareerFightPreview(ctx context.Context, v, id string) (CareerPreviewJob, error) {
	if !canonicalManifestID(id) {
		return CareerPreviewJob{}, gymstable.ErrInvalid
	}
	var out CareerPreviewJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		if s.careerPreviewFailure != nil {
			return s.careerPreviewFailure
		}
		if s.active != nil && s.active.careerPreview != nil && s.active.careerPreview.JobID == id {
			s.active.cancel()
			out = cloneCareerPreview(*s.active.careerPreview)
			return nil
		}
		if s.queued != nil && s.queued.careerPreview != nil && s.queued.published && s.queued.careerPreview.JobID == id {
			s.cancelQueuedCareerPreviewLocked()
			out = cloneCareerPreview(*s.careerPreviewLast)
			return nil
		}
		if s.careerPreviewLast != nil && s.careerPreviewLast.JobID == id {
			out = cloneCareerPreview(*s.careerPreviewLast)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if e != nil {
		return CareerPreviewJob{}, e
	}
	return out, nil
}

// runCareerPreviewTask owns laneGate. No account/store/cache lock spans either proof
// or engine call. Pair private receipts are stripped before retained publication.
func (s *StableController) runCareerPreviewTask(laneCtx context.Context, t *stableTask) error {
	stop := context.AfterFunc(laneCtx, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.careerPreview
	var e error
	var pair gymstable.CareerPreviewPair
	var handle *gymstable.CareerPreviewRead
	var result CareerPreviewResult
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil {
		pair, handle, e = s.stableStore.PrepareCareerPreviewPair(t.ctx, s.session(), [2]string{j.FighterAID, j.FighterBID})
		if e == nil && (handle == nil || handle.RetainedBytes() > MaxCareerPreviewPairSize+512) {
			e = gymstable.ErrCapacity
		}
	}
	if e == nil {
		if s.careerPreviewCheckpoint != nil {
			s.careerPreviewCheckpoint("pair-read")
		}
		if t.ctx.Err() != nil {
			e = t.ctx.Err()
		}
		result.Version = 1
		result.Pair = pair
		if e == nil {
			result.Recipe, e = careerPreviewRecipe(pair)
		}
	}
	if e == nil {
		result.Seed, e = CareerPreviewSeed(result.Recipe)
	}
	if e == nil {
		var input fighterbout.Input
		input, e = careerPreviewInput(pair, result.Seed)
		if e == nil {
			if s.careerPreviewCheckpoint != nil {
				s.careerPreviewCheckpoint("before-engine")
			}
			if t.ctx.Err() != nil {
				e = t.ctx.Err()
			} else {
				result.EngineResult, e = fighterbout.Simulate(t.ctx, input)
			}
		}
		if e == nil {
			if s.careerPreviewCheckpoint != nil {
				s.careerPreviewCheckpoint("before-validation")
			}
			if t.ctx.Err() != nil {
				e = t.ctx.Err()
			} else {
				result.EngineResultDigest, e = fighterbout.Digest(t.ctx, input, result.EngineResult)
			}
		}
	}
	if e == nil {
		if s.careerPreviewCheckpoint != nil {
			s.careerPreviewCheckpoint("before-token-read")
		}
		var checked gymstable.CareerPreviewPair
		checked, e = s.stableStore.ReadCareerPreviewPair(t.ctx, s.session(), handle)
		if e == nil {
			before, e1 := json.Marshal(pair)
			after, e2 := json.Marshal(checked)
			if e1 != nil || e2 != nil || !bytes.Equal(before, after) {
				e = gymstable.ErrCorrupt
			}
		}
	}
	// Only the public Pair survives in the result. The opaque token is no longer retained.
	handle = nil
	if e == nil {
		j.State = "done"
		j.Result = &result
		public := cloneCareerPreview(j)
		if !careerPreviewJobValid(public) {
			e = gymstable.ErrCorrupt
		} else {
			j = public
		}
	}
	if e == nil {
		if s.careerPreviewCheckpoint != nil {
			s.careerPreviewCheckpoint("before-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymstable.ErrClosed
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymstable.ErrCorrupt) || errors.Is(e, gymstable.ErrPoisoned) || errors.Is(e, gymstable.ErrClosed) || errors.Is(e, gymstable.ErrOutcomeUnknown) || errors.Is(e, careerstate.ErrClosed) || errors.Is(e, careerstate.ErrPoisoned) || errors.Is(e, careerstate.ErrCorrupt)
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = careerPreviewProtocolError(e)
		if !errors.Is(e, gymstable.ErrOutcomeUnknown) && (errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, gymstable.ErrCancelled) || errors.Is(e, careerstate.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "preview_cancelled"
		}
	}
	s.stableMu.Lock()
	ownedActive := s.active == t
	if ownedActive {
		s.active = nil
	}
	if !s.stableClosed && ownedActive {
		if e == nil && (t.ctx.Err() != nil || s.stopping) {
			j.Result = nil
			j.State = "cancelled"
			j.Error = "preview_cancelled"
			e = gymstable.ErrCancelled
		}
		// The captured Status may precede an unrelated source append. Never
		// replace the shared Stable cache with this older captured projection.

		s.retainCareerPreviewLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return e
	}
	return nil
}

// This is metadata-only reader liveness/activation, not a career snapshot.
// Caller holds stableMu and the original account View lease.
func (s *StableController) careerPreviewActivationLocked() (careerstate.Activation, error) {
	if s.careerPreviewFailure != nil {
		return careerstate.Activation{}, s.careerPreviewFailure
	}
	if !s.careerEnabled || s.careerReader == nil {
		return careerstate.Activation{}, careerstate.ErrInactive
	}
	a, e := s.careerReader.Activation()
	if errors.Is(e, careerstate.ErrClosed) || errors.Is(e, careerstate.ErrPoisoned) || errors.Is(e, careerstate.ErrCorrupt) {
		s.careerPreviewFailure = e
		s.dropCareerPreviewLocked()
	}
	return a, e
}

// A learned fatal Career owner failure invalidates this family's cached payload.
// No fresh prefix/source traversal is added to Job or portrait authorization.
func (s *StableController) dropCareerPreviewLocked() {
	s.careerPreviewLast = nil
	pending := false
	for _, t := range []*stableTask{s.active, s.queued} {
		if t != nil && (t.preview != nil || t.careerPreview != nil) {
			pending = true
			if t.careerPreview != nil {
				t.cancel()
			}
		}
	}
	if !pending && s.previewLast == nil {
		s.mu.Lock()
		s.previewBytes = 0
		s.mu.Unlock()
	}
}
