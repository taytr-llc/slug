// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Capture one original child. Its existing account lease rechecks this exact
// child and View at admission/publication; a successor login cannot repair it.
func (c *StableController) careerPreviewChild(view string) (*pilot.StableController, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.viewID != view {
		return nil, gymstable.ErrStaleView
	}
	if c.careerOwner == nil {
		return nil, careerstate.ErrInactive
	}
	child, ok := c.child.(*pilot.StableController)
	if !ok {
		return nil, gymstable.ErrUnauthorized
	}
	return child, nil
}
func (c *StableController) CareerPreviewSnapshot() pilot.CareerPreviewSnapshot {
	c.mu.Lock()
	view := c.viewID
	c.mu.Unlock()
	child, err := c.careerPreviewChild(view)
	if err != nil {
		return pilot.CareerPreviewSnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}
	}
	return child.CareerPreviewSnapshot()
}
func (c *StableController) BeginCareerFightPreview(ctx context.Context, view, a, b string) (pilot.CareerPreviewJob, error) {
	child, err := c.careerPreviewChild(view)
	if err != nil {
		return pilot.CareerPreviewJob{}, err
	}
	return child.BeginCareerFightPreview(ctx, view, a, b)
}
func (c *StableController) CareerFightPreviewJob(ctx context.Context, view, id string) (pilot.CareerPreviewJob, error) {
	child, err := c.careerPreviewChild(view)
	if err != nil {
		return pilot.CareerPreviewJob{}, err
	}
	return child.CareerFightPreviewJob(ctx, view, id)
}
func (c *StableController) CancelCareerFightPreview(ctx context.Context, view, id string) (pilot.CareerPreviewJob, error) {
	child, err := c.careerPreviewChild(view)
	if err != nil {
		return pilot.CareerPreviewJob{}, err
	}
	return child.CancelCareerFightPreview(ctx, view, id)
}
