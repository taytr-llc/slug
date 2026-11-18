// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"time"
)

func (s *StableController) beginSeed(ctx context.Context, v, date, kind string) (SeedJob, error) {
	if !entryDateValid(date) {
		return SeedJob{}, gymseed.ErrCalendar
	}
	var task *stableTask
	var out SeedJob
	var oldBytes int
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.seedEnabled || !s.seedAttached || !s.installed || s.stableClosed {
			return gymseed.ErrUnavailable
		}
		if s.seedStore == nil {
			return errSeedSetup
		}
		if s.stopping {
			return gymseed.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil || s.active != nil && s.active.seed != nil {
			return gymseed.ErrBusy
		}
		ev, e := gymevent.EventForDate(s.seedPin.SourcePin, date)
		if e != nil {
			return e
		}
		key, e := gymseed.EventKey(s.seedPin, ev)
		if e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		oldBytes = s.seedBytes
		if !stableCacheFits(s.cache, s.receiptSlot, s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.cardBytes+MaxSeedCacheSize+s.boutBytes+s.resultBytes) {
			return gymseed.ErrCapacity
		}
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return e
		}
		out = SeedJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.seedID, EventKey: key, Saturday: date}
		taskctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		owned := out
		task = &stableTask{seed: &owned, ctx: taskctx, cancel: cancel}
		s.queued = task
		s.seedBytes = MaxSeedCacheSize
		return nil
	})
	s.stableMu.Lock()
	if e == nil && task != nil && s.queued == task && !s.stableClosed && !s.stopping && ctx.Err() == nil {
		task.published = true
		s.seedLast = nil
		s.seedFailure = ""
		s.paused = false
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
	} else {
		if e == nil {
			e = gymseed.ErrCancelled
		}
		if task != nil {
			task.cancel()
			if s.queued == task {
				s.queued = nil
			}
			s.mu.Lock()
			if !s.stableClosed {
				s.seedBytes = oldBytes
			}
			s.mu.Unlock()
		}
	}
	s.stableMu.Unlock()
	if e != nil {
		return SeedJob{}, e
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) retainSeedLocked(j SeedJob) {
	if s.stableClosed {
		return
	}
	owned := cloneSeedJob(j)
	s.seedLast = &owned
	raw, _ := json.Marshal(owned)
	s.mu.Lock()
	s.seedBytes = MaxSeedMetadataSize + len(raw)
	s.mu.Unlock()
}
func (s *StableController) cancelQueuedSeedLocked() {
	t := s.queued
	if t == nil || t.seed == nil {
		return
	}
	t.cancel()
	j := *t.seed
	j.State = "cancelled"
	j.Error = "seed_cancelled"
	j.Result = nil
	s.queued = nil
	if t.published {
		s.retainSeedLocked(j)
	}
}
func (s *StableController) runSeedTask(lane context.Context, t *stableTask) error {
	stop := context.AfterFunc(lane, t.cancel)
	defer stop()
	defer t.cancel()
	j := *t.seed
	var e error
	if s.seedCheckpoint != nil {
		s.seedCheckpoint("before-seed")
	}
	if t.ctx.Err() != nil {
		e = t.ctx.Err()
	} else if s.stableStore == nil {
		e = s.openPair(nil)
	}
	if e == nil && (s.seedStore == nil || s.eventStore == nil || s.entryStore == nil) {
		e = gymseed.ErrUnavailable
	}
	if e == nil && j.Kind != "prepare" {
		cap := gymaccount.View
		if j.Kind == "publish" || j.Kind == "attest" {
			cap = gymaccount.SeedPublish
		}
		e = s.stableConfig.Lease(t.ctx, cap, func(a gymaccount.Account, check func() error) error {
			if check == nil || a.ID != s.account.AccountID || a.Generation != j.View.Generation {
				return gymseed.ErrUnauthorized
			}
			if cap == gymaccount.SeedPublish && a.ID != s.seedPin.PublisherAccountID {
				return gymseed.ErrForbidden
			}
			return check()
		})
	}
	committed := false
	if e == nil {
		var r gymseed.Record
		if j.Kind == "prepare" {
			r, e = s.seedStore.Prepare(t.ctx, s.eventStore, s.entryStore, s.session(), j.Saturday)
		} else {
			var publisher *seednode.Publisher
			publisher, e = s.borrowSeedPublisher(t.ctx)
			if e == nil {
				// Close and join even when a trusted dependency panics. The
				// owning worker may recover that panic after this frame exits.
				func() {
					defer func() {
						if closeErr := publisher.Close(); e == nil && closeErr != nil {
							e = gymseed.ErrOutcomeUnknown
						}
					}()
					switch j.Kind {
					case "publish":
						r, e = s.seedStore.Publish(t.ctx, s.eventStore, s.entryStore, s.session(), publisher, j.Saturday)
					case "reconcile":
						r, e = s.seedStore.Reconcile(t.ctx, s.eventStore, s.entryStore, s.session(), publisher, j.Saturday)
					case "attest":
						r, e = s.seedStore.Attest(t.ctx, s.eventStore, s.entryStore, s.session(), publisher, j.Saturday)
					default:
						e = gymseed.ErrInvalid
					}
				}()
			}
		}

		if e == nil {
			committed = true
			j.Result = &r
			j.State = "completed"
			if _, ve := gymseed.ValidateRecord(s.seedPin, r); ve != nil {
				e = gymseed.ErrCorrupt
			} else if !seedJobValid(&j) {
				e = gymseed.ErrCorrupt
			}
			raw, _ := json.Marshal(j)
			if len(raw) > MaxSeedJobSize {
				e = gymseed.ErrCapacity
			}
		}
	}
	if e == nil {
		if s.seedCheckpoint != nil {
			s.seedCheckpoint("before-seed-publication")
		}
		e = s.shortView(t.ctx, j.View.ViewID, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.active != t {
				return gymseed.ErrUnavailable
			}
			return t.ctx.Err()
		})
	}
	fatal := errors.Is(e, gymseed.ErrCorrupt) || errors.Is(e, gymseed.ErrPoisoned) || errors.Is(e, gymseed.ErrClosed) || errors.Is(e, gymseed.ErrUnavailable)
	if committed && e != nil {
		e = gymseed.ErrOutcomeUnknown
	}
	if errors.Is(e, gymseed.ErrOutcomeUnknown) {
		fatal = true
	}
	if e != nil {
		j.Result = nil
		j.State = "failed"
		j.Error = seedProtocolError(e)
		if !errors.Is(e, gymseed.ErrOutcomeUnknown) && (t.ctx.Err() != nil || errors.Is(e, gymseed.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "seed_cancelled"
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
			s.seedFailure = "seed_unavailable"
		}
		s.retainSeedLocked(j)
	}
	s.stableMu.Unlock()
	if fatal {
		s.setStableFailure(gymstable.ErrClosed)
		return gymseed.ErrUnavailable
	}
	return nil
}

func (s *StableController) BeginSeedPrepare(ctx context.Context, v, date string) (SeedJob, error) {
	return s.beginSeed(ctx, v, date, "prepare")
}

func (s *StableController) BeginSeedPublish(ctx context.Context, v, date string) (SeedJob, error) {
	return s.beginSeed(ctx, v, date, "publish")
}

func (s *StableController) BeginSeedReconcile(ctx context.Context, v, date string) (SeedJob, error) {
	return s.beginSeed(ctx, v, date, "reconcile")
}

func (s *StableController) BeginSeedAttest(ctx context.Context, v, date string) (SeedJob, error) {
	return s.beginSeed(ctx, v, date, "attest")
}
