// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
)

func (c *PoolController) withPoolResultNodeLease(ctx context.Context, session gymaccount.Session, view string, child poolChild, fn func(gymaccount.Account, func() error) error) error {
	return c.withCapability(ctx, session, view, child, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		guard := func() error {
			if e := check(); e != nil {
				return e
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.state != "signed_in" || c.operation {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := guard(); e != nil {
			return e
		}
		if e := fn(a, guard); e != nil {
			return e
		}
		return guard()
	})
}
func (c *PoolController) withPoolHistoryPublicationLease(ctx context.Context, session gymaccount.Session, view string, child poolChild, fn func(gymaccount.Account, func() error) error) error {
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
