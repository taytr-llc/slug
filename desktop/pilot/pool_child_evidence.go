// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *PoolChild) FighterEvidenceSnapshot(ctx context.Context) (FighterEvidenceSnapshot, error) {
	s, e := c.readyStable()
	if e != nil {
		return BlankFighterEvidenceSnapshot(), nil
	}
	return s.FighterEvidenceSnapshot(ctx)
}
func (c *PoolChild) BeginFighterEvidence(ctx context.Context, v, hjob, hid, id string, pin recoveryfile.Pin) (FighterEvidenceJob, error) {
	return poolLaneWork(ctx, c, func(s *StableController) (FighterEvidenceJob, error) {
		c.mu.Lock()
		mining := c.run != nil || c.snapshot.State != "stopped" && c.snapshot.State != "failed"
		c.mu.Unlock()
		if mining {
			return FighterEvidenceJob{}, gymstable.ErrBusy
		}
		return s.BeginFighterEvidence(ctx, v, hjob, hid, id, pin)
	})
}
func (c *PoolChild) FighterEvidenceJob(ctx context.Context, v, id string) (FighterEvidenceJob, error) {
	s, e := c.readyStable()
	if e != nil {
		return FighterEvidenceJob{}, e
	}
	return s.FighterEvidenceJob(ctx, v, id)
}
func (c *PoolChild) CancelFighterEvidence(ctx context.Context, v, id string) (FighterEvidenceJob, error) {
	s, e := c.readyStable()
	if e != nil {
		return FighterEvidenceJob{}, e
	}
	return s.CancelFighterEvidence(ctx, v, id)
}
