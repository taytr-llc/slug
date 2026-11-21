// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

// Separate protocol projection; the old V1 WireSnapshot keeps refusing DAILY.
func (c *RemotePoolController) DailyWalletSnapshot(ctx context.Context) (pilot.RemotePoolWireSnapshot, error) {
	if c == nil || !c.daily || c.recovery == nil {
		return pilot.RemotePoolWireSnapshot{}, pilot.ErrDailyWalletProtocol
	}
	s, e := c.Snapshot(ctx)
	if e != nil {
		return pilot.RemotePoolWireSnapshot{}, e
	}
	out := remotePoolWireSnapshot(s)
	if pilot.ValidateDailyWalletSnapshot(out, *c.recovery) != nil {
		return pilot.RemotePoolWireSnapshot{}, pilot.ErrDailyWalletProtocol
	}
	return out, nil
}

var _ pilot.DailyWalletControllerAPI = (*RemotePoolController)(nil)
