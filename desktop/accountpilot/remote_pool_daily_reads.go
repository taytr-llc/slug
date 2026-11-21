// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
)

// The child retains this same controller's original epoch. Its read admission
// and publication reacquire that View; no parent account lease spans HTTP.
func (c *RemotePoolController) BeginDailyPayoutRead(ctx context.Context, view string, sequence, captureSequence, beforeBatch uint64) (pilot.RemoteDailyPayoutJob, error) {
	if wire.TestPoW {
		return pilot.RemoteDailyPayoutJob{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteDailyPayoutJob{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteDailyPayoutJob{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.BeginDailyPayoutRead(work, view, sequence, captureSequence, beforeBatch)
}

func (c *RemotePoolController) DailyPayoutRead(ctx context.Context, view string, sequence uint64) (pilot.RemoteDailyPayoutRead, error) {
	if wire.TestPoW {
		return pilot.RemoteDailyPayoutRead{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteDailyPayoutRead{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteDailyPayoutRead{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.DailyPayoutRead(work, view, sequence)
}

func (c *RemotePoolController) CancelDailyPayoutRead(ctx context.Context, view string, sequence uint64) error {
	if wire.TestPoW {
		return pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.CancelDailyPayoutRead(work, view, sequence)
}
