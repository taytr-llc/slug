// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

func (c *StableController) FighterEvidenceSnapshot(ctx context.Context) (pilot.FighterEvidenceSnapshot, error) {
	if ctx == nil {
		return pilot.FighterEvidenceSnapshot{}, gymstable.ErrInvalid
	}
	c.mu.Lock()
	v, signed := c.viewID, c.state == "signed_in" && !c.closed && !c.operation
	c.mu.Unlock()
	if !signed {
		return pilot.BlankFighterEvidenceSnapshot(), nil
	}
	s, e := c.historyChild(ctx, v)
	if e != nil {
		return pilot.FighterEvidenceSnapshot{}, e
	}
	return s.FighterEvidenceSnapshot(ctx)
}
func (c *StableController) BeginFighterEvidence(ctx context.Context, v, hjob, hid, id string, pin recoveryfile.Pin) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.historyChild(ctx, v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.BeginFighterEvidence(ctx, v, hjob, hid, id, pin)
}
func (c *StableController) FighterEvidenceJob(ctx context.Context, v, id string) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.historyChild(ctx, v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.FighterEvidenceJob(ctx, v, id)
}
func (c *StableController) CancelFighterEvidence(ctx context.Context, v, id string) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.historyChild(ctx, v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.CancelFighterEvidence(ctx, v, id)
}
func (c *PoolController) evidenceChild(v string) (pilot.FighterEvidenceSurfaceAPI, error) {
	child, e := c.stableChild(v)
	if e != nil {
		return nil, e
	}
	s, ok := child.(pilot.FighterEvidenceSurfaceAPI)
	if !ok {
		return nil, pilot.ErrEvidenceUnavailable
	}
	return s, nil
}
func (c *PoolController) FighterEvidenceSnapshot(ctx context.Context) (pilot.FighterEvidenceSnapshot, error) {
	if ctx == nil {
		return pilot.FighterEvidenceSnapshot{}, gymstable.ErrInvalid
	}
	c.mu.Lock()
	v, signed := c.viewID, c.state == "signed_in" && !c.closed && !c.operation
	c.mu.Unlock()
	if !signed {
		return pilot.BlankFighterEvidenceSnapshot(), nil
	}
	s, e := c.evidenceChild(v)
	if e != nil {
		return pilot.FighterEvidenceSnapshot{}, e
	}
	return s.FighterEvidenceSnapshot(ctx)
}
func (c *PoolController) BeginFighterEvidence(ctx context.Context, v, hjob, hid, id string, pin recoveryfile.Pin) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.evidenceChild(v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.BeginFighterEvidence(ctx, v, hjob, hid, id, pin)
}
func (c *PoolController) FighterEvidenceJob(ctx context.Context, v, id string) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.evidenceChild(v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.FighterEvidenceJob(ctx, v, id)
}
func (c *PoolController) CancelFighterEvidence(ctx context.Context, v, id string) (pilot.FighterEvidenceJob, error) {
	if ctx == nil {
		return pilot.FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	s, e := c.evidenceChild(v)
	if e != nil {
		return pilot.FighterEvidenceJob{}, e
	}
	return s.CancelFighterEvidence(ctx, v, id)
}

var _ pilot.FighterEvidenceSurfaceAPI = (*StableController)(nil)
var _ pilot.FighterEvidenceSurfaceAPI = (*PoolController)(nil)
