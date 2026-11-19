// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"time"
)

func (s *StableController) beginResult(ctx context.Context, v, date, bid, kind, path string) (ResultJob, error) {
	if !entryDateValid(date) {
		return ResultJob{}, gymresult.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return ResultJob{}, gymresult.ErrInvalid
	}
	var task *stableTask
	var out ResultJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.resultEnabled || !s.resultAttached || !s.installed || s.stableClosed {
			return gymresult.ErrUnavailable
		}
		if s.resultStore == nil {
			return errResultSetup
		}
		if s.stopping {
			return gymresult.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.publication != nil {
			return gymresult.ErrBusy
		}
		ev, e := gymevent.EventForDate(s.resultPin.SourcePin.SourcePin.SourcePin.SourcePin, date)
		if e != nil {
			return e
		}
		key, e := gymcard.CardID(s.resultPin.SourcePin.SourcePin, ev)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.resultBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+MaxResultCacheSize) {
			return gymresult.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = ResultJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.resultID, CardID: key, Saturday: date, BoutID: bid, ResultKey: resultKeyFor(s.resultPin, key, bid)}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		task = &stableTask{publication: &owned, resultPath: path, ctx: taskctx, cancel: cancel}
		s.queued = task
		s.resultBytes = MaxResultCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && task != nil && s.queued == task && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		task.published = true
		s.resultLast = nil
		s.resultFailure = ""
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymresult.ErrCancelled
		}
		if task != nil {
			task.cancel()
			if s.queued == task {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.resultBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return ResultJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainResultLocked(j ResultJob) {
	if s.stableClosed {
		return
	}
	owned := cloneResultJob(j)
	s.resultLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.resultBytes = resultRetainedMetadataBytes + len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedResultLocked() {
	t := s.queued
	if t == nil || t.publication == nil {
		return
	}
	t.cancel()
	j := *t.publication
	j.State = "cancelled"
	j.Error = "result_cancelled"
	j.Result = nil
	j.Export = nil
	s.queued = nil
	if t.published {
		s.retainResultLocked(j)
	}
}

func (s *StableController) BeginResultPublish(ctx context.Context, v, date, bid string) (ResultJob, error) {
	return s.beginResult(ctx, v, date, bid, "publish", "")
}
func (s *StableController) BeginResultReconcile(ctx context.Context, v, date, bid string) (ResultJob, error) {
	return s.beginResult(ctx, v, date, bid, "reconcile", "")
}
func (s *StableController) BeginResultAttest(ctx context.Context, v, date, bid string) (ResultJob, error) {
	return s.beginResult(ctx, v, date, bid, "attest", "")
}
func (s *StableController) BeginResultExport(ctx context.Context, v, date, bid, path string) (ResultJob, error) {
	if !resultPathValid(path) {
		return ResultJob{}, gymresult.ErrInvalid
	}
	return s.beginResult(ctx, v, date, bid, "export", path)
}

func (s *StableController) runResultTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.publication
	var e error
	committed := false
	if s.resultCheckpoint != nil {
		s.resultCheckpoint("before-result")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && (s.resultStore == nil || s.boutStore == nil || s.cardStore == nil || s.seedStore == nil || s.eventStore == nil || s.entryStore == nil) {
		e = gymresult.ErrUnavailable
	}
	if e == nil {
		// Source/record refusal precedes any node startup. Publish alone may create
		// the signed statement; a definite later refusal does not erase its receipt.
		var prior gymresult.ReadResult
		prior, e = s.resultStore.Read(t.ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID)
		if errors.Is(e, gymresult.ErrNotFound) && j.Kind == "publish" {
			_, e = s.resultStore.Prepare(t.ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID)
		} else if errors.Is(e, gymresult.ErrNotFound) {
			e = gymresult.ErrNotPrepared
		}
		if e == nil && stableOne(j.Kind, "publish", "attest") {
			// Prepare itself enforces strong authorization. Existing records must pass
			// the same original ResultPublish lease before node readiness effects.
			e = s.stableConfig.Lease(t.ctx, gymaccount.ResultPublish, func(a gymaccount.Account, check func() error) error {
				if a.ID != s.resultPin.PublisherAccountID || a.Generation != j.View.Generation || check == nil {
					return gymresult.ErrForbidden
				}
				return check()
			})
			if e == nil && prior.Version != 0 && !prior.Status.Source.Saved.Usable {
				e = gymresult.ErrSourceConflict
			}
		}
		if e == nil && j.Kind == "export" && prior.Record.Inclusion == nil {
			e = gymresult.ErrProofUnavailable
		}
	}
	if e == nil {
		e = s.ensureResultNodes(t.ctx)
	}
	if e == nil {
		var p *seednode.ResultPublisher
		p, e = s.borrowResultPublisher(t.ctx)
		if e == nil {
			// Closing this bounded borrow never closes the owner's processes.
			func() {
				defer func() { e = errors.Join(e, p.Close()) }()
				var r gymresult.ActionResult
				switch j.Kind {
				case "publish":
					r, e = s.resultStore.Publish(t.ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID, p)
				case "reconcile":
					r, e = s.resultStore.Reconcile(t.ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID, p)
				case "attest":
					r, e = s.resultStore.Attest(t.ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), j.Saturday, j.BoutID, p)
				case "export":
					var v gymresult.ProofSummary
					v, e = s.exportResultProof(t.ctx, j.Saturday, j.BoutID, t.resultPath, p)
					if e == nil {
						j.Export = &v
					}
				default:
					e = gymresult.ErrInvalid
				}
				if e == nil {
					committed = true
					j.State = "completed"
					if j.Kind != "export" {
						j.Result = &r
					}
					if !resultJobValid(&j) {
						e = gymresult.ErrCorrupt
					}
					raw, _ := json.Marshal(j)
					if len(raw) > MaxResultJobSize {
						e = gymresult.ErrCapacity
					}
				}
			}()
		}
	}
	if e == nil {
		if s.resultCheckpoint != nil {
			s.resultCheckpoint("before-result-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymresult.ErrUnavailable
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymresult.ErrCorrupt) || errors.Is(e, gymresult.ErrPoisoned) || errors.Is(e, gymresult.ErrClosed) || errors.Is(e, gymresult.ErrUnavailable)
	if committed && e != nil {
		e = gymresult.ErrOutcomeUnknown
	}
	if e != nil {
		j.Result = nil
		j.Export = nil
		j.State = "failed"
		j.Error = resultProtocolError(e)
		if !errors.Is(e, gymresult.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymresult.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "result_cancelled"
		}
	}
	s.stableMu.Lock()
	if s.active == t {
		s.active = nil
	}
	if !s.stableClosed {
		if e == nil && (t.ctx.Err() != nil || s.stopping) {
			j.Result = nil
			j.Export = nil
			j.State = "failed"
			j.Error = "outcome_unknown"
		}
		if fatal {
			s.resultFailure = "result_unavailable"
		}
		s.retainResultLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymresult.ErrUnavailable
	}
	return nil
}
