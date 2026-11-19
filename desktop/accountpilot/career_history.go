// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"time"
)

const historyBudget = 500 * time.Millisecond

func historyParentError(ctx context.Context, e error) error {
	if errors.Is(e, context.DeadlineExceeded) && !errors.Is(e, gymstable.ErrOutcomeUnknown) {
		return gymstable.ErrBusy
	}
	return e
}
func (c *StableController) historyChild(ctx context.Context, view string) (*pilot.StableController, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.operation || c.state != "signed_in" || c.viewID != view {
		return nil, gymstable.ErrStaleView
	}
	if e := ctx.Err(); e != nil {
		return nil, historyParentError(ctx, e)
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

// No source work or process operation runs under this lease. The parent mutex
// stays held through the child's bounded cache publication, fencing reset,
// logout, expiry and child replacement; the actual account lease stays held too.
func (c *ManifestController) withHistoryPublicationLease(ctx context.Context, session gymaccount.Session, view string, child manifestChild, fn func(gymaccount.Account, func() error) error) error {
	if fn == nil || child == nil {
		return gymaccount.ErrUnauthorized
	}
	return c.store.WithSession(ctx, session.Token, gymaccount.View, func(a gymaccount.Account, recheck func() error) error {
		if e := recheck(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || c.operation || c.state != "signed_in" || c.viewID != view || c.child != child || c.session.Token != session.Token || a.ID != session.Account.ID || a.Generation != session.Account.Generation || child.AccountID() != a.ID {
			return gymaccount.ErrUnauthorized
		}
		guard := func() error {
			if e := ctx.Err(); e != nil {
				return e
			}
			return recheck()
		}
		if e := guard(); e != nil {
			return e
		}
		return fn(a, guard)
	})
}
func (c *StableController) CareerHistorySnapshot(ctx context.Context) (pilot.CareerHistorySnapshot, error) {
	if ctx == nil {
		return pilot.CareerHistorySnapshot{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, historyBudget)
	defer cancel()
	c.mu.Lock()
	view := c.viewID
	c.mu.Unlock()
	child, e := c.historyChild(work, view)
	if e != nil {
		if errors.Is(e, careerstate.ErrInactive) || errors.Is(e, gymstable.ErrStaleView) {
			return pilot.CareerHistorySnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}, nil
		}
		return pilot.CareerHistorySnapshot{}, historyParentError(work, e)
	}
	return child.CareerHistorySnapshot(work)
}
func (c *StableController) BeginCareerHistory(ctx context.Context, v, p string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, historyBudget)
	defer cancel()
	s, e := c.historyChild(work, v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.BeginCareerHistoryAdmission(work, ctx, v, p)
}
func (c *StableController) CareerHistoryJob(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, historyBudget)
	defer cancel()
	s, e := c.historyChild(work, v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.CareerHistoryJob(work, v, id)
}
func (c *StableController) CancelCareerHistory(ctx context.Context, v, id string) (pilot.CareerHistoryJob, error) {
	if ctx == nil {
		return pilot.CareerHistoryJob{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, historyBudget)
	defer cancel()
	s, e := c.historyChild(work, v)
	if e != nil {
		return pilot.CareerHistoryJob{}, e
	}
	return s.CancelCareerHistory(work, v, id)
}
func (c *StableController) CareerHistoryPage(ctx context.Context, v, id string, after, limit uint32) (pilot.CareerHistoryPageResult, error) {
	if ctx == nil {
		return pilot.CareerHistoryPageResult{}, gymstable.ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, historyBudget)
	defer cancel()
	s, e := c.historyChild(work, v)
	if e != nil {
		return pilot.CareerHistoryPageResult{}, e
	}
	return s.CareerHistoryPage(work, v, id, after, limit)
}

var _ pilot.CareerHistoryControllerAPI = (*StableController)(nil)
