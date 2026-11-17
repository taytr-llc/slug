// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (c *StableController) PreviewSnapshot() pilot.PreviewSnapshot {
	c.mu.Lock()
	v := c.viewID
	c.mu.Unlock()
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.PreviewSnapshot{Version: 1, State: "unavailable"}
	}
	return s.PreviewSnapshot()
}
func (c *StableController) BeginFightPreview(ctx context.Context, v, a, b string) (pilot.PreviewJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.PreviewJob{}, e
	}
	return s.BeginFightPreview(ctx, v, a, b)
}
func (c *StableController) FightPreviewJob(ctx context.Context, v, id string) (pilot.PreviewJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.PreviewJob{}, e
	}
	return s.FightPreviewJob(ctx, v, id)
}
func (c *StableController) CancelFightPreview(ctx context.Context, v, id string) (pilot.PreviewJob, error) {
	s, e := c.stableChild(v)
	if e != nil {
		return pilot.PreviewJob{}, e
	}
	return s.CancelFightPreview(ctx, v, id)
}
