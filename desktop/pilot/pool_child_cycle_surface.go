// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *PoolChild) SeedSnapshot() SeedSnapshot {
	s, err := c.readyStable()
	if err != nil {
		return SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
	}
	return s.SeedSnapshot()
}

func (c *PoolChild) SeedStatus(a0 context.Context, a1 string, a2 string) (gymseed.Status, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymseed.Status, error) { return s.SeedStatus(a0, a1, a2) })
}

func (c *PoolChild) SeedRecord(a0 context.Context, a1 string, a2 string) (gymseed.Record, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymseed.Record, error) { return s.SeedRecord(a0, a1, a2) })
}

func (c *PoolChild) SeedHistory(a0 context.Context, a1 string, a2 string, a3 uint32) (gymseed.HistoryPage, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymseed.HistoryPage, error) { return s.SeedHistory(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginSeedPrepare(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (SeedJob, error) { return s.BeginSeedPrepare(a0, a1, a2) })
}

func (c *PoolChild) BeginSeedPublish(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (SeedJob, error) { return s.BeginSeedPublish(a0, a1, a2) })
}

func (c *PoolChild) BeginSeedReconcile(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (SeedJob, error) { return s.BeginSeedReconcile(a0, a1, a2) })
}

func (c *PoolChild) BeginSeedAttest(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (SeedJob, error) { return s.BeginSeedAttest(a0, a1, a2) })
}

func (c *PoolChild) SeedJob(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return SeedJob{}, err
	}
	return s.SeedJob(a0, a1, a2)
}

func (c *PoolChild) CancelSeedJob(a0 context.Context, a1 string, a2 string) (SeedJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return SeedJob{}, err
	}
	return s.CancelSeedJob(a0, a1, a2)
}

func (c *PoolChild) CardSnapshot() CardSnapshot {
	s, err := c.readyStable()
	if err != nil {
		return CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
	}
	return s.CardSnapshot()
}

func (c *PoolChild) CardStatus(a0 context.Context, a1 string, a2 string) (gymcard.Status, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymcard.Status, error) { return s.CardStatus(a0, a1, a2) })
}

func (c *PoolChild) CardRecord(a0 context.Context, a1 string, a2 string) (gymcard.ReadResult, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymcard.ReadResult, error) { return s.CardRecord(a0, a1, a2) })
}

func (c *PoolChild) CardHistory(a0 context.Context, a1 string, a2 string, a3 uint32) (gymcard.HistoryPage, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymcard.HistoryPage, error) { return s.CardHistory(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginCardBuild(a0 context.Context, a1 string, a2 string) (CardJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (CardJob, error) { return s.BeginCardBuild(a0, a1, a2) })
}

func (c *PoolChild) CardJob(a0 context.Context, a1 string, a2 string) (CardJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return CardJob{}, err
	}
	return s.CardJob(a0, a1, a2)
}

func (c *PoolChild) CancelCardJob(a0 context.Context, a1 string, a2 string) (CardJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return CardJob{}, err
	}
	return s.CancelCardJob(a0, a1, a2)
}

func (c *PoolChild) BoutSnapshot() BoutSnapshot {
	s, err := c.readyStable()
	if err != nil {
		return BoutSnapshot{Version: 1, State: "unavailable", Error: "bout_unavailable"}
	}
	return s.BoutSnapshot()
}

func (c *PoolChild) BoutCardStatus(a0 context.Context, a1 string, a2 string) (gymbout.CardStatus, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymbout.CardStatus, error) { return s.BoutCardStatus(a0, a1, a2) })
}

func (c *PoolChild) BoutStatus(a0 context.Context, a1 string, a2 string, a3 string) (gymbout.Status, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymbout.Status, error) { return s.BoutStatus(a0, a1, a2, a3) })
}

func (c *PoolChild) BoutRecord(a0 context.Context, a1 string, a2 string, a3 string) (gymbout.ReadResult, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymbout.ReadResult, error) { return s.BoutRecord(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginBoutRun(a0 context.Context, a1 string, a2 string, a3 string) (BoutJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (BoutJob, error) { return s.BeginBoutRun(a0, a1, a2, a3) })
}

func (c *PoolChild) BoutJob(a0 context.Context, a1 string, a2 string) (BoutJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return BoutJob{}, err
	}
	return s.BoutJob(a0, a1, a2)
}

func (c *PoolChild) CancelBoutJob(a0 context.Context, a1 string, a2 string) (BoutJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return BoutJob{}, err
	}
	return s.CancelBoutJob(a0, a1, a2)
}

func (c *PoolChild) ResultSnapshot() ResultSnapshot {
	s, err := c.readyStable()
	if err != nil {
		return ResultSnapshot{Version: 1, State: "unavailable", Error: "result_unavailable", NodeState: "offline", NodeOwner: "none"}
	}
	return s.ResultSnapshot()
}

func (c *PoolChild) ResultStatus(a0 context.Context, a1 string, a2 string, a3 string) (gymresult.Status, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymresult.Status, error) { return s.ResultStatus(a0, a1, a2, a3) })
}

func (c *PoolChild) ResultRecord(a0 context.Context, a1 string, a2 string, a3 string) (gymresult.ReadResult, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymresult.ReadResult, error) { return s.ResultRecord(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginResultPublish(a0 context.Context, a1 string, a2 string, a3 string) (ResultJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (ResultJob, error) { return s.BeginResultPublish(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginResultReconcile(a0 context.Context, a1 string, a2 string, a3 string) (ResultJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (ResultJob, error) { return s.BeginResultReconcile(a0, a1, a2, a3) })
}

func (c *PoolChild) BeginResultAttest(a0 context.Context, a1 string, a2 string, a3 string) (ResultJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (ResultJob, error) { return s.BeginResultAttest(a0, a1, a2, a3) })
}

func (c *PoolChild) ResultJob(a0 context.Context, a1 string, a2 string) (ResultJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return ResultJob{}, err
	}
	return s.ResultJob(a0, a1, a2)
}

func (c *PoolChild) CancelResultJob(a0 context.Context, a1 string, a2 string) (ResultJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return ResultJob{}, err
	}
	return s.CancelResultJob(a0, a1, a2)
}

func (c *PoolChild) BeginResultExport(a0 context.Context, a1 string, a2 string, a3 string, a4 string) (ResultJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (ResultJob, error) { return s.BeginResultExport(a0, a1, a2, a3, a4) })
}

func (c *PoolChild) CareerView(a0 string) (ManifestViewPin, error) {
	s, err := c.readyStable()
	if err != nil {
		return ManifestViewPin{}, err
	}
	var out ManifestViewPin
	err = s.shortView(context.Background(), a0, func() error {
		var e error
		out, e = s.CareerView(a0)
		return e
	})
	if err != nil {
		return ManifestViewPin{}, err
	}
	return out, nil
}

func (c *PoolChild) CareerInventory(a0 context.Context, a1 string, a2 string, a3 uint64, a4 uint32) (gymstable.CareerPage, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymstable.CareerPage, error) { return s.CareerInventory(a0, a1, a2, a3, a4) })
}

func (c *PoolChild) CareerRoster(a0 context.Context, a1 string, a2 string, a3 uint32, a4 uint32) (gymstable.CareerRosterPage, error) {
	return poolLaneWork(a0, c, func(s *StableController) (gymstable.CareerRosterPage, error) {
		return s.CareerRoster(a0, a1, a2, a3, a4)
	})
}

func (c *PoolChild) CareerHistorySnapshot(a0 context.Context) (CareerHistorySnapshot, error) {
	return poolLaneWork(a0, c, func(s *StableController) (CareerHistorySnapshot, error) { return s.CareerHistorySnapshot(a0) })
}

func (c *PoolChild) BeginCareerHistory(a0 context.Context, a1 string, a2 string) (CareerHistoryJob, error) {
	return poolLaneWork(a0, c, func(s *StableController) (CareerHistoryJob, error) { return s.BeginCareerHistory(a0, a1, a2) })
}

func (c *PoolChild) CareerHistoryJob(a0 context.Context, a1 string, a2 string) (CareerHistoryJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return CareerHistoryJob{}, err
	}
	return s.CareerHistoryJob(a0, a1, a2)
}

func (c *PoolChild) CancelCareerHistory(a0 context.Context, a1 string, a2 string) (CareerHistoryJob, error) {
	s, err := c.readyStable()
	if err != nil {
		return CareerHistoryJob{}, err
	}
	return s.CancelCareerHistory(a0, a1, a2)
}

func (c *PoolChild) CareerHistoryPage(a0 context.Context, a1 string, a2 string, a3 uint32, a4 uint32) (CareerHistoryPageResult, error) {
	return poolLaneWork(a0, c, func(s *StableController) (CareerHistoryPageResult, error) {
		return s.CareerHistoryPage(a0, a1, a2, a3, a4)
	})
}
