// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
)

// The original-view child owns each entry/event operation and final publication.
// Capture its identity under the parent mutex, then release it before invoking
// any lane, source, journal or account callback.
func (c *PoolController) EntrySnapshot() pilot.EntrySnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntrySnapshot{Version: 1, State: "unavailable"}
	}
	return s.EntrySnapshot()
}
func (c *PoolController) EntryUpcoming(ctx context.Context, v string) (gymentry.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return s.EntryUpcoming(ctx, v)
}
func (c *PoolController) EntryStatus(ctx context.Context, v, date string) (gymentry.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.EventStatus{}, e
	}
	return s.EntryStatus(ctx, v, date)
}
func (c *PoolController) EntryHistory(ctx context.Context, v, after string, limit uint32) (gymentry.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.HistoryPage{}, e
	}
	return s.EntryHistory(ctx, v, after, limit)
}
func (c *PoolController) BeginEntryPrepare(ctx context.Context, v string, r gymentry.Request) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryPrepare(ctx, v, r)
}
func (c *PoolController) BeginEntryApply(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryApply(ctx, v, id)
}
func (c *PoolController) BeginEntryCheck(ctx context.Context, v, date string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.BeginEntryCheck(ctx, v, date)
}
func (c *PoolController) EntryJob(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.EntryJob(ctx, v, id)
}
func (c *PoolController) CancelEntryJob(ctx context.Context, v, id string) (pilot.EntryJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EntryJob{}, e
	}
	return s.CancelEntryJob(ctx, v, id)
}
func (c *PoolController) EntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.EntryOperation(ctx, v, id)
}
func (c *PoolController) EntryPending(ctx context.Context, v string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.EntryPending(ctx, v)
}
func (c *PoolController) AbandonEntryOperation(ctx context.Context, v, id string) (gymentry.OperationResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymentry.OperationResult{}, e
	}
	return s.AbandonEntryOperation(ctx, v, id)
}

func (c *PoolController) EventSnapshot() pilot.EventSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
	}
	return s.EventSnapshot()
}
func (c *PoolController) EventStatus(ctx context.Context, v, date string) (gymevent.EventStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.EventStatus{}, e
	}
	return s.EventStatus(ctx, v, date)
}
func (c *PoolController) EventRecord(ctx context.Context, v, date string) (gymevent.SnapshotReceipt, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.SnapshotReceipt{}, e
	}
	return s.EventRecord(ctx, v, date)
}
func (c *PoolController) EventHistory(ctx context.Context, v, after string, limit uint32) (gymevent.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymevent.HistoryPage{}, e
	}
	return s.EventHistory(ctx, v, after, limit)
}
func (c *PoolController) BeginEventCapture(ctx context.Context, v, date string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.BeginEventCapture(ctx, v, date)
}
func (c *PoolController) EventJob(ctx context.Context, v, id string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.EventJob(ctx, v, id)
}
func (c *PoolController) CancelEventJob(ctx context.Context, v, id string) (pilot.EventJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.EventJob{}, e
	}
	return s.CancelEventJob(ctx, v, id)
}
