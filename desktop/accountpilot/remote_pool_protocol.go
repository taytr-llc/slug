// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

// WireSnapshot mechanically converts the actual original-owner projection.
// Snapshot and its existing JSON tags/bytes remain unchanged.
func (c *RemotePoolController) WireSnapshot(ctx context.Context) (pilot.RemotePoolWireSnapshot, error) {
	if c == nil || c.daily {
		return pilot.RemotePoolWireSnapshot{}, pilot.ErrRemotePoolProtocol
	}
	s, e := c.Snapshot(ctx)
	if e != nil {
		return pilot.RemotePoolWireSnapshot{}, e
	}
	return remotePoolWireSnapshot(s), nil
}

func remotePoolWireSnapshot(s RemotePoolControllerSnapshot) pilot.RemotePoolWireSnapshot {
	return pilot.RemotePoolWireSnapshot{Version: s.Version, ConfigID: s.ConfigID, State: s.State,
		Account: s.Account, View: s.View, Remote: s.Remote, Error: s.Error}
}

var _ pilot.RemotePoolControllerAPI = (*RemotePoolController)(nil)
