// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

// Child methods retain the original account/view through final publication;
// parent selection never holds a lock across source or node work.
func (c *PoolController) SeedSnapshot() pilot.SeedSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
	}
	return s.SeedSnapshot()
}
func (c *PoolController) SeedStatus(ctx context.Context, v, date string) (gymseed.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.Status{}, e
	}
	return s.SeedStatus(ctx, v, date)
}
func (c *PoolController) SeedRecord(ctx context.Context, v, date string) (gymseed.Record, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.Record{}, e
	}
	return s.SeedRecord(ctx, v, date)
}
func (c *PoolController) SeedHistory(ctx context.Context, v, after string, limit uint32) (gymseed.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymseed.HistoryPage{}, e
	}
	return s.SeedHistory(ctx, v, after, limit)
}
func (c *PoolController) BeginSeedPrepare(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedPrepare(ctx, v, date)
}
func (c *PoolController) BeginSeedPublish(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedPublish(ctx, v, date)
}
func (c *PoolController) BeginSeedReconcile(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedReconcile(ctx, v, date)
}
func (c *PoolController) BeginSeedAttest(ctx context.Context, v, date string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.BeginSeedAttest(ctx, v, date)
}
func (c *PoolController) SeedJob(ctx context.Context, v, id string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.SeedJob(ctx, v, id)
}
func (c *PoolController) CancelSeedJob(ctx context.Context, v, id string) (pilot.SeedJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.SeedJob{}, e
	}
	return s.CancelSeedJob(ctx, v, id)
}

func (c *PoolController) CardSnapshot() pilot.CardSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
	}
	return s.CardSnapshot()
}
func (c *PoolController) CardStatus(ctx context.Context, v, date string) (gymcard.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.Status{}, e
	}
	return s.CardStatus(ctx, v, date)
}
func (c *PoolController) CardRecord(ctx context.Context, v, date string) (gymcard.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.ReadResult{}, e
	}
	return s.CardRecord(ctx, v, date)
}
func (c *PoolController) CardHistory(ctx context.Context, v, after string, limit uint32) (gymcard.HistoryPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymcard.HistoryPage{}, e
	}
	return s.CardHistory(ctx, v, after, limit)
}
func (c *PoolController) BeginCardBuild(ctx context.Context, v, date string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.BeginCardBuild(ctx, v, date)
}
func (c *PoolController) CardJob(ctx context.Context, v, id string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.CardJob(ctx, v, id)
}
func (c *PoolController) CancelCardJob(ctx context.Context, v, id string) (pilot.CardJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CardJob{}, e
	}
	return s.CancelCardJob(ctx, v, id)
}

func (c *PoolController) BoutSnapshot() pilot.BoutSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutSnapshot{Version: 1, State: "unavailable", Error: "bout_unavailable"}
	}
	return s.BoutSnapshot()
}
func (c *PoolController) BoutStatus(ctx context.Context, v, date, bid string) (gymbout.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.Status{}, e
	}
	return s.BoutStatus(ctx, v, date, bid)
}
func (c *PoolController) BoutRecord(ctx context.Context, v, date, bid string) (gymbout.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.ReadResult{}, e
	}
	return s.BoutRecord(ctx, v, date, bid)
}
func (c *PoolController) BoutCardStatus(ctx context.Context, v, date string) (gymbout.CardStatus, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymbout.CardStatus{}, e
	}
	return s.BoutCardStatus(ctx, v, date)
}
func (c *PoolController) BeginBoutRun(ctx context.Context, v, date, bid string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.BeginBoutRun(ctx, v, date, bid)
}
func (c *PoolController) BoutJob(ctx context.Context, v, id string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.BoutJob(ctx, v, id)
}
func (c *PoolController) CancelBoutJob(ctx context.Context, v, id string) (pilot.BoutJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.BoutJob{}, e
	}
	return s.CancelBoutJob(ctx, v, id)
}

func (c *PoolController) ResultSnapshot() pilot.ResultSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultSnapshot{Version: 1, State: "unavailable", Error: "result_unavailable", NodeState: "offline", NodeOwner: "none"}
	}
	return s.ResultSnapshot()
}
func (c *PoolController) ResultStatus(ctx context.Context, v, date, bid string) (gymresult.Status, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymresult.Status{}, e
	}
	return s.ResultStatus(ctx, v, date, bid)
}
func (c *PoolController) ResultRecord(ctx context.Context, v, date, bid string) (gymresult.ReadResult, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymresult.ReadResult{}, e
	}
	return s.ResultRecord(ctx, v, date, bid)
}
func (c *PoolController) BeginResultPublish(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultPublish(ctx, v, date, bid)
}
func (c *PoolController) BeginResultReconcile(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultReconcile(ctx, v, date, bid)
}
func (c *PoolController) BeginResultAttest(ctx context.Context, v, date, bid string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultAttest(ctx, v, date, bid)
}
func (c *PoolController) ResultJob(ctx context.Context, v, id string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.ResultJob(ctx, v, id)
}
func (c *PoolController) CancelResultJob(ctx context.Context, v, id string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.CancelResultJob(ctx, v, id)
}
func (c *PoolController) BeginResultExport(ctx context.Context, v, date, bid, path string) (pilot.ResultJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ResultJob{}, e
	}
	return s.BeginResultExport(ctx, v, date, bid, path)
}

func (c *PoolController) CareerInventory(ctx context.Context, v, jobID string, after uint64, limit uint32) (gymstable.CareerPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymstable.CareerPage{}, e
	}
	return s.CareerInventory(ctx, v, jobID, after, limit)
}
func (c *PoolController) CareerRoster(ctx context.Context, v, jobID string, first, limit uint32) (gymstable.CareerRosterPage, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return gymstable.CareerRosterPage{}, e
	}
	return s.CareerRoster(ctx, v, jobID, first, limit)
}

func (c *PoolController) CareerView(v string) (pilot.ManifestViewPin, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.ManifestViewPin{}, e
	}
	return s.CareerView(v)
}

func (c *PoolController) CareerHistorySnapshot(ctx context.Context) (pilot.CareerHistorySnapshot, error) {
	if ctx == nil {
		return pilot.CareerHistorySnapshot{}, gymstable.ErrInvalid
	}
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CareerHistorySnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}, nil
	}
	return s.CareerHistorySnapshot(ctx)
}
func (c *PoolController) BeginCareerHistory(ctx context.Context, v, p string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.BeginCareerHistory(ctx, v, p)
}
func (c *PoolController) CareerHistoryJob(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.CareerHistoryJob(ctx, v, id)
}
func (c *PoolController) CancelCareerHistory(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.CancelCareerHistory(ctx, v, id)
}
func (c *PoolController) CareerHistoryPage(ctx context.Context, v, id string, after, limit uint32) (pilot.CareerHistoryPageResult, error) {
	if ctx == nil {
		return pilot.CareerHistoryPageResult{}, gymstable.ErrInvalid
	}
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.CareerHistoryPageResult{}, e
	}
	return s.CareerHistoryPage(ctx, v, id, after, limit)
}
