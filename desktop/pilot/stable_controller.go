// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

// StableController owns one serial work lane. Its source lifetime is fenced by
// laneGate, never by the legacy resource mutex across an account callback.
type StableController struct {
	careerHandle                       *gymstable.CareerRead
	careerJobID                        string
	careerView                         ManifestViewPin
	careerAfter                        uint64
	careerFirst, careerLimit           uint32
	careerRoster                       bool
	careerEnabled                      bool
	careerReader                       *careerstate.Reader
	careerOpenStable                   func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error)
	resultNodeOwner                    *resultNodeOwner
	resultEnabled, resultAttached      bool
	resultStore                        *gymresult.Store
	resultPin                          gymresult.Pin
	resultID, resultFailure            string
	resultCreated, resultSourceCreated int64
	resultLast                         *ResultJob
	resultCheckpoint                   func(string)

	boutEnabled, boutAttached      bool
	boutStore                      *gymbout.Store
	boutPin                        gymbout.Pin
	boutID, boutFailure            string
	boutCreated, boutSourceCreated int64
	boutLast                       *BoutJob
	boutCheckpoint                 func(string)

	cardEnabled, cardAttached      bool
	cardStore                      *gymcard.Store
	cardPin                        gymcard.Pin
	cardID, cardFailure            string
	cardCreated, cardSourceCreated int64
	cardLast                       *CardJob
	cardCheckpoint                 func(string)

	*ManifestController
	seedEnabled, seedAttached                         bool
	seedStore                                         *gymseed.Store // immutable parent-owned handle
	seedPin                                           gymseed.Pin
	seedID, seedFailure                               string
	seedCreated                                       int64
	seedLast                                          *SeedJob
	seedNodes                                         *pilotNodes // concrete process owner, only during its run
	seedCheckpoint                                    func(string)
	eventEnabled, eventAttached                       bool
	eventStore                                        *gymevent.Store // borrowed immutable parent handle; never closed by child
	eventPin                                          gymevent.Pin
	eventID, eventParticipant, eventFailure           string
	eventCreated                                      int64
	eventLast                                         *EventJob
	eventCheckpoint                                   func(string)
	eventFactories                                    *eventStoreFactories
	entryEnabled                                      bool
	entryStore                                        *gymentry.Store // laneGate owns handle lifetime
	entryPin                                          gymentry.Pin    // immutable after preparation
	entryStoreID                                      string
	entryPending                                      string
	entryLast                                         *EntryJob
	entryCheckpoint                                   func(string)
	stableConfig                                      StableAccountConfig
	stableMu                                          sync.Mutex
	stopMu                                            sync.Mutex
	stopping                                          bool
	laneGate                                          chan struct{}
	wake                                              chan struct{}
	stableStore                                       *gymstable.Store
	source                                            *registry.ManifestStore
	ownedSource                                       bool
	stableStatus                                      *gymstable.Status
	stableState, stableError                          string
	stableClosed, installed, mining, paused, needSync bool
	active, queued                                    *stableTask
	last                                              *StableJob
	historyFailure                                    error
	historyLast                                       *CareerHistoryJob
	historyHandle                                     *gymstable.CareerHistoryRead
	historyEpoch                                      uint64
	historyCheckpoint                                 func(string)
	careerPreviewFailure                              error
	careerPreviewLast                                 *CareerPreviewJob
	careerPreviewCheckpoint                           func(string)
	previewLast                                       *PreviewJob
	previewCheckpoint                                 func(string)
	laneCheckpoint                                    func(string)
	lastAcknowledged, lastAutomatic                   bool
	loopCancel                                        context.CancelFunc
	loopDone                                          chan struct{}
	startCancel                                       context.CancelFunc
	startDone                                         chan struct{}
	stableCloseOnce                                   sync.Once
	stableCloseErr                                    error
}

func NewStableManifestAccount(config Config, account StableAccountConfig) (*StableController, error) {
	if account.Lease == nil {
		return nil, gymstable.ErrInvalid
	}
	c, err := NewManifestAccount(config, account.Manifest)
	if err != nil {
		return nil, err
	}
	s := &StableController{ManifestController: c, stableConfig: account, laneGate: make(chan struct{}, 1), wake: make(chan struct{}, 1), stableState: "unavailable"}
	c.stable = s
	c.mu.Lock()
	c.recomputeProjectionLocked()
	c.mu.Unlock()
	return s, nil
}
func (s *StableController) session() gymstable.Session {
	return gymstable.Session{Generation: s.view.Generation, ViewID: s.view.ViewID, Lease: s.stableConfig.Lease}
}

func (s *StableController) shortView(ctx context.Context, view string, fn func() error) error {
	return s.viewWithBudget(ctx, view, 250*time.Millisecond, fn)
}
func (s *StableController) viewWithBudget(ctx context.Context, view string, budget time.Duration, fn func() error) error {
	if view != s.view.ViewID || !canonicalManifestID(view) {
		return gymstable.ErrStaleView
	}
	short, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	err := s.stableConfig.Lease(short, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.account.AccountID || a.Generation != s.view.Generation || check == nil {
			return gymstable.ErrUnauthorized
		}
		if err := check(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		return check()
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return gymstable.ErrBusy
	}
	if errors.Is(err, gymaccount.ErrUnauthorized) || errors.Is(err, gymaccount.ErrExpired) || errors.Is(err, gymaccount.ErrForbidden) {
		return gymstable.ErrUnauthorized
	}
	return err
}
func cloneStableJob(j StableJob) StableJob {
	// Bounded public DTO only: private Item receipts have already been stripped.
	raw, _ := json.Marshal(j)
	var owned StableJob
	_ = json.Unmarshal(raw, &owned)
	return owned
}
func (s *StableController) StableSnapshot() StableSnapshot {
	blank := StableSnapshot{Version: 1, State: "unavailable"}
	out := blank
	err := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil {
			return gymstable.ErrClosed
		}
		s.expireQueuedLocked()
		status := *s.stableStatus
		view := s.viewPin()
		out = StableSnapshot{Version: 1, State: s.stableState, Error: s.stableError, View: &view, Status: &status, PendingOperationID: status.PendingOperationID}
		if s.active != nil && s.active.family() == "stable" {
			out.ActiveJobID = s.active.result.JobID
		}
		if s.queued != nil && s.queued.family() == "stable" {
			out.QueuedJobID = s.queued.result.JobID
		}
		if s.last != nil {
			out.LastJobID = s.last.JobID
		}
		return nil
	})
	if err != nil {
		return blank
	}
	return out
}
func (s *StableController) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// openPair/closePair require laneGate and never hold a controller mutex while
// invoking a source, stable store, or captured account lease.
func (s *StableController) openPair(source *registry.ManifestStore) error {
	if s.stableStore != nil {
		return gymstable.ErrBusy
	}
	pin, err := s.StablePin()
	if err != nil {
		return err
	}
	owned := source == nil
	if owned {
		source, err = registry.OpenManifestAuthority(filepath.Join(s.config.DataDir, "manifest-authority"), pin.Source, true)
		if err != nil {
			return err
		}
	}
	store, err := s.openStableStore(filepath.Join(s.config.DataDir, "stable"), pin, source)
	if err != nil {
		if owned {
			_ = source.Close()
		}
		return err
	}
	s.stableStore, s.source, s.ownedSource = store, source, owned
	if s.entryEnabled {
		entry, e := s.openEntryStore(context.Background(), filepath.Join(s.config.DataDir, "entries"), s.entryPin, store)
		if e != nil {
			_ = s.closePair()
			return e
		}
		s.entryStore = entry
	}
	return nil
}
func (s *StableController) closePair() error {
	var err error
	if s.entryStore != nil {
		err = s.entryStore.Close()
		s.entryStore = nil
	}
	if s.stableStore != nil {
		err = errors.Join(err, s.stableStore.Close())
		s.stableStore = nil
	}
	if s.ownedSource && s.source != nil {
		err = errors.Join(err, s.source.Close())
	}
	s.source = nil
	s.ownedSource = false
	if err != nil {
		s.noteShutdown("stable storage close failed")
	}
	return err
}
func (s *StableController) AttachStable(ctx context.Context) error {
	phase, err := s.installPhase()
	if err != nil || phase != "ready" {
		return ErrManifestInitialization
	}
	select {
	case s.laneGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.laneGate }()
	if err = s.openPair(nil); err != nil {
		return err
	}
	status, err := s.stableStore.Status(ctx, s.session())
	if err != nil {
		_ = s.closePair()
		return err
	}
	s.stableMu.Lock()
	if s.stableClosed {
		s.stableMu.Unlock()
		_ = s.closePair()
		return gymstable.ErrClosed
	}
	if s.entryEnabled {
		s.stableMu.Unlock()
		pending, e := s.entryStore.Pending(ctx, s.session())
		if e != nil {
			_ = s.closePair()
			return e
		}
		s.stableMu.Lock()
		if s.stableClosed {
			s.stableMu.Unlock()
			_ = s.closePair()
			return gymstable.ErrClosed
		}
		s.entryPending = ""
		if pending.Operation != nil {
			s.entryPending = pending.Operation.Intent.OperationID
		}
	}
	s.installed = true
	s.stableStatus = &status
	s.stableState = "syncing"
	s.needSync = true
	s.paused = false
	s.stableMu.Unlock()
	s.startStoppedLoop()
	return nil
}
func (s *StableController) startStoppedLoop() {
	s.stableMu.Lock()
	defer s.stableMu.Unlock()
	if s.stableClosed || s.mining || s.paused || s.loopCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.loopCancel, s.loopDone = cancel, done
	go func() {
		defer func() {
			s.stableMu.Lock()
			close(done)
			if s.loopDone == done {
				s.loopCancel = nil
			}
			s.stableMu.Unlock()
		}()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := s.cycle(ctx); err != nil {
				s.setStableFailure(err)
				return
			}
			s.stableMu.Lock()
			more := s.queued != nil && s.queued.published || s.queued == nil && s.needSync && !s.paused && (s.last == nil || s.lastAcknowledged || s.lastAutomatic)
			s.stableMu.Unlock()
			if more {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
		}
	}()
}
func (s *StableController) stopStoppedLoop() {
	s.stableMu.Lock()
	cancel, done := s.loopCancel, s.loopDone
	s.stableMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.stableMu.Lock()
	if s.loopDone == done {
		s.loopCancel, s.loopDone = nil, nil
	}
	s.stableMu.Unlock()
}
func (s *StableController) Start() error {
	s.stableMu.Lock()
	if s.stableClosed || !s.installed {
		s.stableMu.Unlock()
		return gymstable.ErrClosed
	}
	if s.stopping {
		s.stableMu.Unlock()
		return gymstable.ErrBusy
	}
	if s.mining {
		s.stableMu.Unlock()
		return nil
	}
	s.mining = true
	s.paused = false
	s.cancelTasksLocked()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.startCancel, s.startDone = cancel, done
	s.stableMu.Unlock()
	go func() {
		defer close(done)
		s.stopStoppedLoop()
		s.stopResultNodes()
		s.laneGate <- struct{}{}
		err := s.closePair()
		<-s.laneGate
		s.stableMu.Lock()
		if ctx.Err() == nil && !s.stableClosed && s.mining && err == nil {
			err = s.ManifestController.Start()
		}
		if err != nil {
			s.mining = false
			s.stableState, s.stableError = "failed", "stable_unavailable"
		}
		s.stableMu.Unlock()
	}()
	return nil
}
func (s *StableController) Stop() {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	s.stableMu.Lock()
	s.stopping = true
	defer func() { s.stableMu.Lock(); s.stopping = false; s.stableMu.Unlock() }()
	s.paused = true
	s.mining = false
	s.needSync = false
	s.cancelTasksLocked()
	cancel, done := s.startCancel, s.startDone
	s.stableState = "paused"
	s.stableMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.stopStoppedLoop()
	s.ManifestController.Stop()
	s.laneGate <- struct{}{}
	s.stableMu.Lock()
	keep := !s.stableClosed && s.installed
	s.stableMu.Unlock()
	_ = s.closePair()
	if keep && s.stableStore == nil {
		if err := s.openPair(nil); err != nil {
			s.setStableFailure(err)
		}
	}
	<-s.laneGate
}
func (s *StableController) Close() error {
	s.stableCloseOnce.Do(func() {
		s.stableMu.Lock()
		s.stableClosed = true
		s.careerHandle = nil
		s.careerJobID = ""
		s.stableStatus = nil
		s.last = nil
		s.previewLast = nil
		s.careerPreviewLast = nil
		s.historyLast = nil
		s.historyHandle = nil
		s.entryLast = nil
		s.resultLast = nil
		s.boutLast = nil
		s.boutFailure = ""
		s.cardLast = nil
		s.cardFailure = ""
		s.seedLast = nil
		s.seedFailure = ""
		s.eventLast = nil
		s.eventFailure = ""
		s.entryPending = ""
		s.mu.Lock()
		s.previewBytes = 0
		s.careerBytes = 0
		s.historyBytes = 0
		s.entryBytes = 0
		s.eventBytes = 0
		s.seedBytes = 0
		s.cardBytes = 0
		s.resultBytes = 0
		s.boutBytes = 0
		s.mu.Unlock()
		s.stableMu.Unlock()
		s.Stop()
		s.stableCloseErr = s.ManifestController.Close()
	})
	return s.stableCloseErr
}
func (s *StableController) openLive(source *registry.ManifestStore) error {
	s.laneGate <- struct{}{}
	defer func() { <-s.laneGate }()
	if err := s.openPair(source); err != nil {
		return err
	}
	s.stableMu.Lock()
	s.needSync = true
	s.stableState = "syncing"
	s.stableMu.Unlock()
	return nil
}
func (s *StableController) closeLive() {
	s.stableMu.Lock()
	s.cancelTasksLocked()
	s.mining = false
	s.stableMu.Unlock()
	s.laneGate <- struct{}{}
	_ = s.closePair()
	<-s.laneGate
}
func (s *StableController) setStableFailure(err error) {
	s.stableMu.Lock()
	s.invalidateHistoryLocked()
	if historyFatal(err) || errors.Is(err, gymstable.ErrOutcomeUnknown) {
		s.latchHistoryFailureLocked(err)
	}
	s.clearCareerReadLocked()
	s.needSync = false
	s.stableState = "failed"
	s.stableError = stableProtocolError(err)
	s.stableMu.Unlock()
}
