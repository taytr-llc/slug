// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymstable"
	"time"
)

func clonePreview(j PreviewJob) PreviewJob {
	raw, _ := json.Marshal(j)
	var out PreviewJob
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s *StableController) PreviewSnapshot() PreviewSnapshot {
	blank := PreviewSnapshot{Version: 1, State: "unavailable"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil {
			return gymstable.ErrClosed
		}
		s.expireQueuedLocked()
		v := s.viewPin()
		out = PreviewSnapshot{Version: 1, State: "idle", View: &v, StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: previewEngineID}
		if s.active != nil && s.active.preview != nil {
			out.ActiveJobID = s.active.preview.JobID
			out.State = "busy"
		}
		if s.queued != nil && s.queued.preview != nil && s.queued.published {
			out.QueuedJobID = s.queued.preview.JobID
			out.State = "busy"
		}
		if s.previewLast != nil {
			j := s.previewLast
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
		return blank
	}
	return out
}
func (s *StableController) BeginFightPreview(ctx context.Context, v, a, b string) (PreviewJob, error) {
	if !canonicalManifestID(a) || !canonicalManifestID(b) || a == b {
		return PreviewJob{}, gymstable.ErrInvalid
	}
	if b < a {
		a, b = b, a
	}
	var t *stableTask
	var out PreviewJob
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
		if s.queued != nil || s.active != nil && s.active.preview != nil {
			return gymstable.ErrBusy
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.previewBytes
		if !stableCacheFits(s.cache, s.receiptSlot, max(oldBytes, previewAllowance)+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes) {
			return gymstable.ErrCapacity
		}
		var id [32]byte
		if _, e := rand.Read(id[:]); e != nil {
			return e
		}
		out = PreviewJob{Version: 1, JobID: hex.EncodeToString(id[:]), State: "queued", View: s.viewPin(), StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: previewEngineID, FighterAID: a, FighterBID: b}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		t = &stableTask{preview: &owned, ctx: taskctx, cancel: cancel}
		s.queued = t
		s.previewBytes = max(oldBytes, previewAllowance)
		return nil
	})
	s.stableMu.Lock()
	if e != nil {
		if t != nil {
			t.cancel()
			if s.queued == t {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.previewBytes = oldBytes
			}
			s.mu.Unlock()
		}
	} else if s.queued == t && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		t.published = true
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
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.previewBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return PreviewJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainPreviewLocked(j PreviewJob) {
	if s.stableClosed {
		return
	}
	owned := clonePreview(j)
	s.previewLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.previewBytes = len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedPreviewLocked() {
	t := s.queued
	if t == nil || t.preview == nil {
		return
	}
	t.cancel()
	j := *t.preview
	j.State = "cancelled"
	j.Error = "preview_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainPreviewLocked(j)
	}
}
func (s *StableController) FightPreviewJob(ctx context.Context, v, id string) (PreviewJob, error) {
	if !canonicalManifestID(id) {
		return PreviewJob{}, gymstable.ErrInvalid
	}
	var out PreviewJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.preview != nil && (t == s.active || t.published) && t.preview.JobID == id {
				out = clonePreview(*t.preview)
				return nil
			}
		}
		if s.previewLast != nil && s.previewLast.JobID == id {
			out = clonePreview(*s.previewLast)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if e != nil {
		return PreviewJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelFightPreview(ctx context.Context, v, id string) (PreviewJob, error) {
	if !canonicalManifestID(id) {
		return PreviewJob{}, gymstable.ErrInvalid
	}
	var out PreviewJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		if s.active != nil && s.active.preview != nil && s.active.preview.JobID == id {
			s.active.cancel()
			out = clonePreview(*s.active.preview)
			return nil
		}
		if s.queued != nil && s.queued.preview != nil && s.queued.published && s.queued.preview.JobID == id {
			s.cancelQueuedPreviewLocked()
			out = clonePreview(*s.previewLast)
			return nil
		}
		if s.previewLast != nil && s.previewLast.JobID == id {
			out = clonePreview(*s.previewLast)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if e != nil {
		return PreviewJob{}, e
	}
	return out, nil
}

// runPreviewTask owns laneGate. No account/store/cache lock spans either proof
// or engine call. Pair private receipts are stripped before retained publication.
func (s *StableController) runPreviewTask(laneCtx context.Context, t *stableTask) error {
	stop := context.AfterFunc(laneCtx, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.preview
	var e error
	var pair gymstable.PreviewPair
	var result PreviewResult
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil {
		pair, e = s.stableStore.ReadPreviewPair(t.ctx, s.session(), [2]string{j.FighterAID, j.FighterBID})
	}
	if e == nil {
		if s.previewCheckpoint != nil {
			s.previewCheckpoint("pair-read")
		}
		if t.ctx.Err() != nil {
			e = t.ctx.Err()
		}
		result.Version = 1
		result.Pair = pair
		if e == nil {
			result.Recipe, e = previewRecipe(pair)
		}
	}
	if e == nil {
		result.Seed, e = PreviewSeed(result.Recipe)
	}
	if e == nil {
		var input fighterbout.Input
		input, e = previewInput(pair, result.Seed)
		if e == nil {
			if s.previewCheckpoint != nil {
				s.previewCheckpoint("before-engine")
			}
			if t.ctx.Err() != nil {
				e = t.ctx.Err()
			} else {
				result.EngineResult, e = fighterbout.Simulate(t.ctx, input)
			}
		}
		if e == nil {
			if s.previewCheckpoint != nil {
				s.previewCheckpoint("before-validation")
			}
			if t.ctx.Err() != nil {
				e = t.ctx.Err()
			} else {
				result.EngineResultDigest, e = fighterbout.Digest(t.ctx, input, result.EngineResult)
			}
		}
	}
	if e == nil {
		j.State = "done"
		j.Result = &result
		public := clonePreview(j)
		if !previewJobValid(public) {
			e = gymstable.ErrCorrupt
		} else {
			j = public
		}
	}
	if e == nil {
		if s.previewCheckpoint != nil {
			s.previewCheckpoint("before-publication")
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
	fatal := errors.Is(e, gymstable.ErrCorrupt) || errors.Is(e, gymstable.ErrPoisoned) || errors.Is(e, gymstable.ErrClosed) || errors.Is(e, gymstable.ErrOutcomeUnknown)
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = previewProtocolError(e)
		if !errors.Is(e, gymstable.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymstable.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "preview_cancelled"
		}
	}
	s.stableMu.Lock()
	if s.active == t {
		s.active = nil
	}
	if !s.stableClosed {
		if e == nil && (t.ctx.Err() != nil || s.stopping) {
			j.Result = nil
			j.State = "cancelled"
			j.Error = "preview_cancelled"
			e = gymstable.ErrCancelled
		}
		if e == nil {
			status := pair.Status
			s.stableStatus = &status
			// A preview observes backlog; it does not request assignment catch-up.
			// Preserve work independently scheduled by an accepted receipt.
			if !status.CaughtUp && !s.needSync {
				s.stableState = "paused"
			}
		}
		s.retainPreviewLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return e
	}
	return nil
}
