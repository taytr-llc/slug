// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
)

// BeginWalletActivityRead retains this controller's original View and attachment.
func (c *RemotePoolController) BeginWalletActivityRead(ctx context.Context, view string, sequence uint64) (pilot.RemoteWalletActivityJob, error) {
	if wire.TestPoW {
		return pilot.RemoteWalletActivityJob{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteWalletActivityJob{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteWalletActivityJob{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.BeginWalletActivityRead(work, view, sequence)
}

// WalletActivityRead retains this controller's original View and attachment.
func (c *RemotePoolController) WalletActivityRead(ctx context.Context, view string, sequence uint64) (pilot.RemoteWalletActivityRead, error) {
	if wire.TestPoW {
		return pilot.RemoteWalletActivityRead{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteWalletActivityRead{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteWalletActivityRead{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.WalletActivityRead(work, view, sequence)
}

// WalletActivityPage retains this controller's original View and attachment.
func (c *RemotePoolController) WalletActivityPage(ctx context.Context, view string, sequence uint64, after, limit uint32) (pilot.RemoteWalletActivityPage, error) {
	if wire.TestPoW {
		return pilot.RemoteWalletActivityPage{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteWalletActivityPage{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteWalletActivityPage{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.WalletActivityPage(work, view, sequence, after, limit)
}

// WalletActivityDetail retains this controller's original View and attachment.
func (c *RemotePoolController) WalletActivityDetail(ctx context.Context, view string, sequence uint64, txid string) (pilot.RemoteWalletActivityDetail, error) {
	if wire.TestPoW {
		return pilot.RemoteWalletActivityDetail{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteWalletActivityDetail{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteWalletActivityDetail{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.WalletActivityDetail(work, view, sequence, txid)
}

// ObserveDailyPayout retains this controller's original View and attachment.
func (c *RemotePoolController) ObserveDailyPayout(ctx context.Context, view string, sequence, batch uint64, txid string, index uint32) (pilot.RemoteDailyPayoutObservation, error) {
	if wire.TestPoW {
		return pilot.RemoteDailyPayoutObservation{}, pilot.ErrRemotePoolProfile
	}
	if c == nil || !c.daily {
		return pilot.RemoteDailyPayoutObservation{}, pilot.ErrRemotePoolConfig
	}
	work, done, e := c.admit(ctx, view)
	if e != nil {
		return pilot.RemoteDailyPayoutObservation{}, e
	}
	defer done()
	c.mu.Lock()
	a := c.attachment
	c.mu.Unlock()
	return a.ObserveDailyPayout(work, view, sequence, batch, txid, index)
}
func (c *RemotePoolController) CancelWalletActivityRead(ctx context.Context, view string, sequence uint64) error {
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
	return a.CancelWalletActivityRead(work, view, sequence)
}
