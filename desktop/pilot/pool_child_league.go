// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"reflect"

	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
)

// validatePoolLeagueChild binds the current participant to the complete parent
// authority. A matching participant alone cannot authorize a different source
// or a substituted second gym. Returned slices belong to this child.
func validatePoolLeagueChild(r poolChildResources) (*poolLeagueChildBinding, error) {
	if r.League == nil || r.Events == nil {
		return nil, ErrManifestInitialization
	}
	x := *r.League
	sourceID, err := poolservice.BindingID(r.Source)
	if err != nil || x.SourceBindingID != sourceID || x.SourceBindingID != r.Recipient.SourceBindingID {
		return nil, ErrManifestInitialization
	}
	p, err := gymentry.ValidatePin(x.EntryPin)
	if err != nil || p.StablePin != r.StablePin || p.StablePin.Version != gymstable.SharedPinVersion2 {
		return nil, ErrManifestInitialization
	}
	id, err := gymentry.ID(p)
	if err != nil || id != x.EntryStoreID {
		return nil, ErrManifestInitialization
	}
	event, err := gymevent.ValidatePin(x.EventPin)
	if err != nil {
		return nil, ErrManifestInitialization
	}
	eventID, err := gymevent.ID(event)
	if err != nil || eventID != x.EventStoreID || x.EventCreatedMS <= 0 {
		return nil, ErrManifestInitialization
	}
	matches := 0
	for _, participant := range event.Participants {
		if participant.StablePin.Version != gymstable.SharedPinVersion2 || participant.StablePin.Source != r.Source.AuthorityPin || participant.StablePin.AccountAuthorityID != r.StablePin.AccountAuthorityID {
			return nil, ErrManifestInitialization
		}
		if participant == p {
			matches++
		}
	}
	if matches != 1 {
		return nil, ErrManifestInitialization
	}
	x.EntryPin, x.EventPin = p, event
	return &x, nil
}

// preparePoolLeague runs before openPair or any visible publication. The parent
// has already established these stores; this path never calls solo installers.
func (s *StableController) preparePoolLeague(r poolChildResources) error {
	binding, err := validatePoolLeagueChild(r)
	if err != nil {
		return err
	}
	p, err := s.EntryPin()
	if err != nil || p != binding.EntryPin || s.sharedSource != r.Issuer || s.installed || s.stableStore != nil {
		return ErrManifestInitialization
	}
	s.entryEnabled, s.eventEnabled = true, true
	s.entryPin, s.entryStoreID = binding.EntryPin, binding.EntryStoreID
	s.sharedFactories = r.Factories
	return nil
}

// attachPoolLeague performs source/store reads outside the original account
// lease. Only bounded, owned public metadata is installed inside the lease.
func (s *StableController) attachPoolLeague(ctx context.Context, r poolChildResources) error {
	binding, err := validatePoolLeagueChild(r)
	if err != nil || s.entryStore == nil || s.entryPin != binding.EntryPin || s.entryStoreID != binding.EntryStoreID {
		return ErrManifestInitialization
	}
	info, err := r.Events.Info(ctx)
	if err != nil || info.StoreID != binding.EventStoreID || info.CreatedAtMS != binding.EventCreatedMS || !reflect.DeepEqual(info.Pin, binding.EventPin) {
		return ErrManifestInitialization
	}
	pending, err := s.entryStore.Pending(ctx, s.session())
	if err != nil {
		return err
	}
	pendingID := ""
	if pending.Operation != nil {
		pendingID = pending.Operation.Intent.OperationID
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.eventAttached {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+s.careerBytes+s.previewBytes+s.entryBytes+eventMetadataBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes) {
			return gymevent.ErrCapacity
		}
		s.entryPending = pendingID
		s.eventStore, s.eventPin = r.Events, binding.EventPin
		s.eventID, s.eventCreated = binding.EventStoreID, binding.EventCreatedMS
		s.eventParticipant = binding.EntryPin.StablePin.Identity.GymID
		s.eventAttached, s.eventBytes = true, eventMetadataBytes
		return nil
	})
}

// poolLaneWork admits only brief dispatches or already-bounded metadata work.
// It never waits for Stop/backup, and no account lease is held while acquiring
// this lifecycle gate. The core lane retains its own original-view fences.
func poolLaneWork[T any](ctx context.Context, c *PoolChild, fn func(*StableController) (T, error)) (T, error) {
	var zero T
	if ctx == nil || ctx.Err() != nil {
		return zero, gymstable.ErrCancelled
	}
	if !c.stopMu.TryLock() {
		return zero, gymstable.ErrBusy
	}
	defer c.stopMu.Unlock()
	c.mu.Lock()
	s := c.stable
	closed, busy := c.closed || !c.prepared, c.backup != nil || c.backupCloseErr != nil
	c.mu.Unlock()
	if closed || s == nil {
		return zero, gymstable.ErrClosed
	}
	if busy {
		return zero, gymstable.ErrBusy
	}
	return fn(s)
}

func (c *PoolChild) EntrySnapshot() EntrySnapshot {
	s, err := c.readyStable()
	if err != nil {
		return EntrySnapshot{Version: 1, State: "unavailable"}
	}
	return s.EntrySnapshot()
}
func (c *PoolChild) EntryUpcoming(ctx context.Context, v string) (gymentry.EventStatus, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.EventStatus, error) { return s.EntryUpcoming(ctx, v) })
}
func (c *PoolChild) EntryStatus(ctx context.Context, v, date string) (gymentry.EventStatus, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.EventStatus, error) { return s.EntryStatus(ctx, v, date) })
}
func (c *PoolChild) EntryHistory(ctx context.Context, v, after string, limit uint32) (gymentry.HistoryPage, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.HistoryPage, error) { return s.EntryHistory(ctx, v, after, limit) })
}
func (c *PoolChild) BeginEntryPrepare(ctx context.Context, v string, request gymentry.Request) (EntryJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (EntryJob, error) { return s.BeginEntryPrepare(ctx, v, request) })
}
func (c *PoolChild) BeginEntryApply(ctx context.Context, v, id string) (EntryJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (EntryJob, error) { return s.BeginEntryApply(ctx, v, id) })
}
func (c *PoolChild) BeginEntryCheck(ctx context.Context, v, date string) (EntryJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (EntryJob, error) { return s.BeginEntryCheck(ctx, v, date) })
}
func (c *PoolChild) EntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.OperationResult, error) { return s.EntryOperation(ctx, v, id) })
}
func (c *PoolChild) EntryPending(ctx context.Context, v string) (gymentry.OperationResult, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.OperationResult, error) { return s.EntryPending(ctx, v) })
}
func (c *PoolChild) AbandonEntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymentry.OperationResult, error) {
		return s.AbandonEntryOperation(ctx, v, id)
	})
}
func (c *PoolChild) EntryJob(ctx context.Context, v, id string) (EntryJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return EntryJob{}, err
	}
	return s.EntryJob(ctx, v, id)
}
func (c *PoolChild) CancelEntryJob(ctx context.Context, v, id string) (EntryJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return EntryJob{}, err
	}
	return s.CancelEntryJob(ctx, v, id)
}
func (c *PoolChild) EventSnapshot() EventSnapshot {
	s, err := c.readyStable()
	if err != nil {
		return EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
	}
	return s.EventSnapshot()
}
func (c *PoolChild) EventStatus(ctx context.Context, v, date string) (gymevent.EventStatus, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymevent.EventStatus, error) { return s.EventStatus(ctx, v, date) })
}
func (c *PoolChild) EventRecord(ctx context.Context, v, date string) (gymevent.SnapshotReceipt, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymevent.SnapshotReceipt, error) { return s.EventRecord(ctx, v, date) })
}
func (c *PoolChild) EventHistory(ctx context.Context, v, after string, limit uint32) (gymevent.HistoryPage, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (gymevent.HistoryPage, error) { return s.EventHistory(ctx, v, after, limit) })
}
func (c *PoolChild) BeginEventCapture(ctx context.Context, v, date string) (EventJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (EventJob, error) { return s.BeginEventCapture(ctx, v, date) })
}
func (c *PoolChild) EventJob(ctx context.Context, v, id string) (EventJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return EventJob{}, err
	}
	return s.EventJob(ctx, v, id)
}
func (c *PoolChild) CancelEventJob(ctx context.Context, v, id string) (EventJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return EventJob{}, err
	}
	return s.CancelEventJob(ctx, v, id)
}
