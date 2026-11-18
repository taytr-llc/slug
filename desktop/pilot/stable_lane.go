// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

type stableTask struct {
	bout         *BoutJob
	card         *CardJob
	seed         *SeedJob
	event        *EventJob
	entry        *EntryJob
	entryRequest gymentry.Request
	preview      *PreviewJob
	followLatest bool
	result       StableJob
	ctx          context.Context
	cancel       context.CancelFunc
	after        uint64
	limit, first uint32
	request      gymstable.SlotRequest
	automatic    bool
	published    bool
}

func (s *StableController) expireQueuedLocked() {
	if s.queued != nil && s.queued.ctx.Err() != nil {
		s.queued.cancel()
		if s.queued.bout != nil {
			s.cancelQueuedBoutLocked()
			return
		}
		if s.queued.card != nil {
			s.cancelQueuedCardLocked()
			return
		}
		if s.queued.seed != nil {
			s.cancelQueuedSeedLocked()
			return
		}
		if s.queued.event != nil {
			s.cancelQueuedEventLocked()
			return
		}
		if s.queued.entry != nil {
			s.cancelQueuedEntryLocked()
			return
		}
		if s.queued.preview != nil {
			s.cancelQueuedPreviewLocked()
			return
		}
		j := s.queued.result
		j.State = "cancelled"
		j.Error = "stable_cancelled"
		s.last = &j
		s.lastAcknowledged = false
		s.lastAutomatic = false
		s.queued = nil
	}
}
func (s *StableController) cancelTasksLocked() {
	if s.active != nil {
		s.active.cancel()
	}
	if s.queued != nil {
		s.queued.cancel()
		if s.queued.bout != nil {
			s.cancelQueuedBoutLocked()
			return
		}
		if s.queued.card != nil {
			s.cancelQueuedCardLocked()
			return
		}
		if s.queued.seed != nil {
			s.cancelQueuedSeedLocked()
			return
		}
		if s.queued.event != nil {
			s.cancelQueuedEventLocked()
			return
		}
		if s.queued.entry != nil {
			s.cancelQueuedEntryLocked()
			return
		}
		if s.queued.preview != nil {
			s.cancelQueuedPreviewLocked()
			return
		}
		s.queued.result.State = "cancelled"
		s.queued.result.Error = "stable_cancelled"
		j := s.queued.result
		s.last = &j
		s.queued = nil
	}
}
func (s *StableController) beginStable(ctx context.Context, view, kind, operation string, after uint64, limit, first uint32, request gymstable.SlotRequest, follow ...bool) (StableJob, error) {
	var out StableJob
	var reserved *stableTask
	err := s.shortView(ctx, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil {
			return gymstable.ErrClosed
		}
		if s.stopping {
			return gymstable.ErrBusy
		}
		s.expireQueuedLocked()
		if s.queued != nil {
			return gymstable.ErrBusy
		}
		var id [32]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		jobctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		out = StableJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: kind, State: "queued", View: s.viewPin(), StoreID: s.stableStatus.StoreID, OperationID: operation}
		reserved = &stableTask{result: out, ctx: jobctx, cancel: cancel, after: after, limit: limit, first: first, request: request}
		if len(follow) > 0 {
			reserved.followLatest = follow[0]
		}
		s.queued = reserved
		if s.active != nil && s.active.automatic {
			s.active.cancel()
		}
		s.paused = false
		return nil
	})
	s.stableMu.Lock()
	if err != nil {
		if reserved != nil {
			reserved.cancel()
			if s.queued == reserved {
				s.queued = nil
			}
		}
	} else if s.queued == reserved && !s.stableClosed {
		reserved.published = true
		s.lastAcknowledged = true
	} else {
		err = gymstable.ErrCancelled
	}
	s.stableMu.Unlock()
	if err != nil {
		return StableJob{}, err
	}
	s.startStoppedLoop()
	s.signal()
	return out, nil
}
func (s *StableController) BeginStableSync(ctx context.Context, v string) (StableJob, error) {
	return s.beginStable(ctx, v, "sync", "", 0, 0, 0, gymstable.SlotRequest{})
}
func (s *StableController) BeginStableInventory(ctx context.Context, v string, a uint64, l uint32) (StableJob, error) {
	if a > gymstable.MaxAssignments || l == 0 || l > 8 {
		return StableJob{}, gymstable.ErrInvalid
	}
	return s.beginStable(ctx, v, "inventory", "", a, l, 0, gymstable.SlotRequest{})
}
func (s *StableController) BeginStableRoster(ctx context.Context, v string, f, l uint32) (StableJob, error) {
	if f < 1 || f > 10 || l == 0 || l > 8 {
		return StableJob{}, gymstable.ErrInvalid
	}
	return s.beginStable(ctx, v, "roster", "", 0, l, f, gymstable.SlotRequest{})
}
func (s *StableController) BeginStablePrepare(ctx context.Context, v string, r gymstable.SlotRequest) (StableJob, error) {
	if _, err := gymstable.ValidateSlotRequest(r); err != nil {
		return StableJob{}, err
	}
	return s.beginStable(ctx, v, "prepare", "", 0, 0, 0, r)
}
func (s *StableController) BeginStableApply(ctx context.Context, v, id string) (StableJob, error) {
	if !canonicalManifestID(id) {
		return StableJob{}, gymstable.ErrInvalid
	}
	return s.beginStable(ctx, v, "apply", id, 0, 0, 0, gymstable.SlotRequest{})
}

// cycle is called by either the stopped loop or the worker's synchronous hook.
// A context cancellation joins the currently executing source proof; it never
// creates a detached proof goroutine or dispatches a subsequent operation.
func (s *StableController) cycle(ctx context.Context) error {
	select {
	case s.laneGate <- struct{}{}:
	case <-ctx.Done():
		return nil
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	if s.stableClosed || s.paused {
		s.stableMu.Unlock()
		return nil
	}
	task := s.queued
	if task != nil {
		if !task.published {
			s.stableMu.Unlock()
			return nil
		}
		s.queued = nil
	} else if s.needSync && (s.last == nil || s.lastAcknowledged || s.lastAutomatic) {
		var id [32]byte
		if _, err := rand.Read(id[:]); err != nil {
			s.stableMu.Unlock()
			return err
		}
		autoctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		task = &stableTask{ctx: autoctx, cancel: cancel, automatic: true, result: StableJob{Version: 1, JobID: hex.EncodeToString(id[:]), Kind: "sync", State: "running", View: s.viewPin(), StoreID: s.stableStatus.StoreID}}
	}
	if task == nil {
		s.stableMu.Unlock()
		return nil
	}
	if task.bout != nil {
		task.bout.State = "running"
	} else if task.card != nil {
		task.card.State = "running"
	} else if task.seed != nil {
		task.seed.State = "running"
	} else if task.event != nil {
		task.event.State = "running"
	} else if task.entry != nil {
		task.entry.State = "running"
	} else if task.preview != nil {
		task.preview.State = "running"
	} else {
		task.result.State = "running"
	}
	s.active = task
	s.stableMu.Unlock()
	if task.bout != nil {
		return s.runBoutTask(ctx, task)
	}
	if task.card != nil {
		return s.runCardTask(ctx, task)
	}
	if task.seed != nil {
		return s.runSeedTask(ctx, task)
	}
	if task.event != nil {
		return s.runEventTask(ctx, task)
	}
	if task.entry != nil {
		return s.runEntryTask(ctx, task)
	}
	if task.preview != nil {
		return s.runPreviewTask(ctx, task)
	}
	result := task.result
	if s.laneCheckpoint != nil {
		s.laneCheckpoint(result.Kind)
	}
	stop := context.AfterFunc(ctx, task.cancel)
	defer stop()
	defer task.cancel()
	var err error
	var fatal error
	if task.ctx.Err() != nil {
		err = task.ctx.Err()
	} else if s.stableStore == nil {
		err = s.openPair(nil)
	}
	var items []gymstable.Item
	var status *gymstable.Status
	if err == nil {
		switch result.Kind {
		case "sync":
			var r gymstable.SyncResult
			r, err = s.stableStore.Sync(task.ctx, s.session())
			if err == nil {
				result.Sync = &r
				items = r.Items
				status = &r.Status
			}
		case "inventory":
			if task.followLatest {
				current, e := s.stableStore.Status(task.ctx, s.session())
				if e != nil {
					err = e
					break
				}
				task.after = 0
				if current.Checkpoint > 8 {
					task.after = current.Checkpoint - 8
				}
				task.limit = 8
			}
			var r gymstable.Page
			r, err = s.stableStore.Inventory(task.ctx, s.session(), task.after, task.limit)
			if err == nil {
				result.Page = &r
				items = r.Items
				status = &r.Status
			}
		case "roster":
			var r gymstable.RosterPage
			r, err = s.stableStore.Roster(task.ctx, s.session(), task.first, task.limit)
			if err == nil {
				result.Roster = &r
				status = &r.Status
				for _, v := range r.Items {
					items = append(items, v.Item)
				}
			}
		case "prepare":
			var r gymstable.Intent
			r, err = s.stableStore.PrepareSlot(task.ctx, s.session(), task.request)
			if err == nil {
				result.Intent = &r
			}
		case "apply":
			var r gymstable.MutationResult
			r, err = s.stableStore.ApplySlot(task.ctx, s.session(), result.OperationID)
			if err == nil {
				result.Mutation = &r
			}
		default:
			err = gymstable.ErrInvalid
		}
	}
	if (err == nil && (result.Kind == "prepare" || result.Kind == "apply")) || errors.Is(err, gymstable.ErrSyncRequired) || errors.Is(err, gymstable.ErrOutcomeUnknown) {
		observed, statusErr := s.stableStore.Status(task.ctx, s.session())
		if errors.Is(statusErr, gymstable.ErrPoisoned) || errors.Is(statusErr, gymstable.ErrCorrupt) {
			fatal = statusErr
		}
		if statusErr == nil {
			status = &observed
		} else if err == nil {
			err = gymstable.ErrOutcomeUnknown
		}
	}
	if err == nil && items != nil {
		err = s.publishStableItems(task.ctx, items, result.Kind != "sync" && !task.followLatest, task.followLatest)
	}
	if err == nil {
		result.State = "done"
	} else {
		result.Sync = nil
		result.Page = nil
		result.Roster = nil
		result.Intent = nil
		result.Mutation = nil
		result.Error = stableProtocolError(err)
		result.State = "failed"
		if errors.Is(err, gymstable.ErrOutcomeUnknown) || errors.Is(err, gymstable.ErrPoisoned) {
			result.State = "outcome_unknown"
			result.Error = "outcome_unknown"
		} else if task.ctx.Err() != nil || errors.Is(err, gymstable.ErrCancelled) {
			result.State = "cancelled"
			result.Error = "stable_cancelled"
		}
	}
	// Marshal/unmarshal drops private receipt fields from the public result. The
	// only retained proof copies now belong to the bounded appearance cache.
	public := cloneStableJob(result)
	if raw, e := json.Marshal(public); e != nil || len(raw) > gymstable.MaxResponseSize {
		return gymstable.ErrCorrupt
	}
	s.stableMu.Lock()
	if status != nil && (err == nil || errors.Is(err, gymstable.ErrSyncRequired)) {
		copy := *status
		s.stableStatus = &copy
		s.needSync = !copy.CaughtUp
		if copy.CaughtUp {
			s.stableState = "ready"
		} else {
			s.stableState = "syncing"
		}
		s.stableError = ""
	}
	if result.Intent != nil && s.stableStatus != nil {
		s.stableStatus.PendingOperationID = result.Intent.OperationID
		s.stableStatus.LastOperationID = result.Intent.OperationID
		s.stableStatus.OperationCount = uint32(result.Intent.Sequence)
	}
	if result.Mutation != nil && s.stableStatus != nil {
		r := result.Mutation.Receipt
		s.stableStatus.PendingOperationID = ""
		s.stableStatus.LastOperationID = r.OperationID
		s.stableStatus.RosterRevision = r.Revision
		s.stableStatus.Slots = r.Slots
	}
	if s.active == task {
		s.active = nil
	}
	if !s.stableClosed {
		s.last = &public
		s.lastAcknowledged = false
		s.lastAutomatic = task.automatic
	}
	preempted := task.automatic && task.ctx.Err() != nil && (s.queued != nil || ctx.Err() != nil || s.paused)
	if err != nil && task.automatic && !preempted {
		s.needSync = false
		s.stableState = "failed"
		s.stableError = stableProtocolError(err)
	}
	s.stableMu.Unlock()
	if fatal != nil {
		s.setStableFailure(gymstable.ErrClosed)
		return fatal
	}
	if errors.Is(err, gymstable.ErrCorrupt) || errors.Is(err, gymstable.ErrPoisoned) || errors.Is(err, gymstable.ErrClosed) {
		s.setStableFailure(gymstable.ErrClosed)
		return err
	}
	return nil
}
func (s *StableController) publishStableItems(ctx context.Context, items []gymstable.Item, selected, followLatest bool) error {
	next := make(map[uint64]*manifestCached, len(items))
	for _, item := range items {
		v, err := s.projectManifest(item.Receipt())
		if err != nil {
			return err
		}
		next[v.summary.Sequence] = v
	}
	return s.withView(ctx, func(check func() error) error {
		if err := check(); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return gymstable.ErrClosed
		}
		if !selected && s.stableProtected && !followLatest {
			return nil
		}
		if !selected && !followLatest {
			for seq, v := range s.cache {
				if _, ok := next[seq]; !ok {
					next[seq] = v
				}
			}
			for len(next) > 8 {
				low := ^uint64(0)
				for seq := range next {
					low = min(low, seq)
				}
				delete(next, low)
			}
		}
		if !manifestCacheFits(next, s.receiptSlot) || !stableCacheFits(next, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes) {
			return gymstable.ErrCapacity
		}
		s.cache = next
		for seq := range next {
			s.snapshot.AcceptedCount = max(s.snapshot.AcceptedCount, seq)
		}
		s.stableSelected = selected
		s.stableProtected = selected
		if !selected {
			n := s.snapshot.AcceptedCount
			if uint64(len(next)) != min(n, uint64(8)) {
				s.stableSelected = true
			}
			for i := uint64(0); i < min(n, uint64(8)); i++ {
				if next[n-i] == nil {
					s.stableSelected = true
				}
			}
		}
		for seq := range next {
			s.snapshot.AcceptedCount = max(s.snapshot.AcceptedCount, seq)
		}
		s.snapshot.Projection.State = "ready"
		s.snapshot.Projection.Error = ""
		s.snapshot.Projection.Revision++
		s.recomputeProjectionLocked()
		return nil
	})
}
func (s *StableController) observeStable(ctx context.Context, _ registry.ManifestRecord) error {
	s.stableMu.Lock()
	if s.stableState == "failed" {
		s.stableMu.Unlock()
		return nil
	}
	s.needSync = true
	s.stableMu.Unlock()
	return s.cycle(ctx)
}
func (s *StableController) StableJob(ctx context.Context, view, id string) (StableJob, error) {
	var out StableJob
	if !canonicalManifestID(id) {
		return out, gymstable.ErrInvalid
	}
	err := s.shortView(ctx, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.family() == "stable" && t.result.JobID == id {
				out = cloneStableJob(t.result)
				return nil
			}
		}
		if s.last != nil && s.last.JobID == id {
			out = cloneStableJob(*s.last)

			return nil
		}
		return gymstable.ErrNotFound
	})
	if err != nil {
		return StableJob{}, err
	}
	s.stableMu.Lock()
	if s.last != nil && s.last.JobID == id {
		s.lastAcknowledged = true
	}
	s.stableMu.Unlock()
	s.signal()
	return out, nil
}
func (s *StableController) CancelStableJob(ctx context.Context, view, id string) (StableJob, error) {
	var out StableJob
	err := s.shortView(ctx, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.active != nil && s.active.family() == "stable" && s.active.result.JobID == id {
			s.active.cancel()
			out = cloneStableJob(s.active.result)
			return nil
		}
		if s.queued != nil && s.queued.family() == "stable" && s.queued.result.JobID == id {
			s.queued.cancel()
			s.queued.result.State = "cancelled"
			s.queued.result.Error = "stable_cancelled"
			out = cloneStableJob(s.queued.result)
			s.last = &out
			s.queued = nil
			return nil
		}
		if s.last != nil && s.last.JobID == id {
			out = cloneStableJob(*s.last)
			return nil
		}
		return gymstable.ErrNotFound
	})
	if err != nil {
		return StableJob{}, err
	}
	return out, nil
}
func (s *StableController) stableOperation(ctx context.Context, view, id string, kind string) (StableOperation, error) {
	short, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	ctx = short
	var out StableOperation
	if kind != "pending" && !canonicalManifestID(id) {
		return out, gymstable.ErrInvalid
	}
	if err := s.shortView(ctx, view, func() error { return nil }); err != nil {
		return out, err
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return out, gymstable.ErrBusy
	}
	defer func() { <-s.laneGate }()
	if s.stableStore == nil {
		return out, gymstable.ErrBusy
	}
	var op *gymstable.Operation
	var err error
	if kind == "pending" {
		op, err = s.stableStore.Pending(short, s.session())
	} else {
		var value gymstable.Operation
		if kind == "abandon" {
			value, err = s.stableStore.Abandon(short, s.session(), id)
		} else {
			value, err = s.stableStore.Operation(short, s.session(), id)
		}
		if err == nil {
			op = &value
		}
	}
	if err != nil {
		if errors.Is(err, gymstable.ErrCancelled) && errors.Is(short.Err(), context.DeadlineExceeded) {
			return out, gymstable.ErrBusy
		}
		return out, err
	}
	fresh, statusErr := s.stableStore.Status(short, s.session())
	if statusErr != nil {
		if kind == "abandon" {
			return StableOperation{}, gymstable.ErrOutcomeUnknown
		}
		return StableOperation{}, statusErr
	}
	err = s.shortView(short, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableStatus == nil || s.stableClosed {
			return gymstable.ErrClosed
		}
		out = StableOperation{Version: 1, View: s.viewPin(), StoreID: s.stableStatus.StoreID, Operation: op}
		s.stableStatus = &fresh
		return nil
	})
	if err != nil {
		if kind == "abandon" {
			return StableOperation{}, gymstable.ErrOutcomeUnknown
		}
		return StableOperation{}, err
	}
	return out, nil
}
func (s *StableController) StableOperation(ctx context.Context, v, id string) (StableOperation, error) {
	return s.stableOperation(ctx, v, id, "operation")
}
func (s *StableController) StablePending(ctx context.Context, v string) (StableOperation, error) {
	return s.stableOperation(ctx, v, "", "pending")
}
func (s *StableController) AbandonStableOperation(ctx context.Context, v, id string) (StableOperation, error) {
	return s.stableOperation(ctx, v, id, "abandon")
}

// Public last-job allowance is reserved even when it is currently empty.
// 64KiB manifests + 512KiB receipts + 256KiB SVG + 64KiB export
// The 128KiB closed response allowance plus 8KiB of metadata is reserved
// inside the shared 1MiB cache ceiling, including later lazy SVG/export writes.
func stableCacheFits(cache map[uint64]*manifestCached, slot *ManifestReceipt, extra ...int) bool {
	total := gymstable.MaxResponseSize + 8192
	for _, n := range extra {
		if n < 0 || n > MaxManifestCacheBytes {
			return false
		}
		total += n
	}
	for _, v := range cache {
		total += v.manifestBytes + v.receiptBytes + len(v.svg)
	}
	if slot != nil {
		raw, err := json.Marshal(slot.Record)
		if err != nil {
			return false
		}
		total += len(raw)
	}
	return total <= MaxManifestCacheBytes
}

func (s *StableController) BeginStableRecent(ctx context.Context, v string) (StableJob, error) {
	return s.beginStable(ctx, v, "inventory", "", 0, 8, 0, gymstable.SlotRequest{}, true)
}

// Legacy cold/refresh methods cannot escape the single stable proof lane.
func (s *StableController) LoadManifestProspects(context.Context) error {
	return gymstable.ErrForbidden
}
func (s *StableController) RefreshManifestProspects(ctx context.Context) error {
	_, err := s.BeginStableRecent(ctx, s.view.ViewID)
	return err
}

func (t *stableTask) family() string {
	if t.bout != nil {
		return "bout"
	}
	if t.card != nil {
		return "card"
	}
	if t.seed != nil {
		return "seed"
	}
	if t.event != nil {
		return "event"
	}
	if t.entry != nil {
		return "entry"
	}
	if t.preview != nil {
		return "preview"
	}
	return "stable"
}
