// Copyright (c) 2026 The SLUG developers. ISC license.
package careerconsumer

import (
	"context"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// CaptureHistory uses the same bound trusted reader as Snapshot, but takes one
// owned State for both the full temporal prefix and selected fighter's rows.
func (g *Guard) CaptureHistory(ctx context.Context, id string, asOf int64) (*careerstate.History, error) {
	if g == nil || !g.bound || g.reader == nil || g.expected == nil {
		return nil, careerstate.ErrInactive
	}
	h, e := g.reader.CaptureHistory(ctx, id, asOf)
	if e != nil {
		return nil, e
	}
	if h.Snapshot().Activation() != *g.expected {
		return nil, careerstate.ErrCorrupt
	}
	return h, nil
}

// CheckHistory rechecks only the installed reader lifetime, activation and
// scope. It never reconstructs a Current prefix or changes a captured history.
// The consumer's operation gate/lock serializes Guard attachment and use.
func (g *Guard) CheckHistory(ctx context.Context, kind, storeID string) (careerstate.Activation, error) {
	if g == nil || !g.bound || g.reader == nil || g.expected == nil {
		return careerstate.Activation{}, careerstate.ErrInactive
	}
	if ctx == nil || ctx.Err() != nil {
		return careerstate.Activation{}, careerstate.ErrCancelled
	}
	a, e := g.verify(g.reader, kind, storeID)
	if e != nil {
		return careerstate.Activation{}, e
	}
	if ctx.Err() != nil {
		return careerstate.Activation{}, careerstate.ErrCancelled
	}
	return a, nil
}
