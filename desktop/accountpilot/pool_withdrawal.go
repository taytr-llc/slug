// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (c *PoolController) HandleWithdrawal(ctx context.Context, r pilot.WithdrawalRequest) pilot.WithdrawalResponse {
	if ctx == nil {
		return pilot.WithdrawalFailure(r.ID, "invalid_request")
	}
	if _, e := pilot.MarshalWithdrawalRequest(r); e != nil {
		return pilot.WithdrawalFailure(r.ID, "invalid_request")
	}
	c.mu.Lock()
	view, child, valid := c.viewID, c.child, !c.closed && !c.operation && c.state == "signed_in" && c.child != nil
	c.mu.Unlock()
	if r.Command == "withdrawal_snapshot" && !valid {
		s := pilot.BlankWithdrawalSnapshot()
		return pilot.WithdrawalResponse{ID: r.ID, Version: 1, OK: true, Snapshot: &s}
	}
	if !valid || r.Command != "withdrawal_snapshot" && r.ViewID != view {
		return pilot.WithdrawalFailure(r.ID, "stale_view")
	}
	surface, ok := child.(pilot.WithdrawalSurfaceAPI)
	if !ok {
		return pilot.WithdrawalFailure(r.ID, "withdrawal_unavailable")
	}
	// The immutable child owns original authorization. No parent or account lease
	// spans journal acquisition, wallet/signing, node work or cancellation joins.
	out := surface.HandleWithdrawal(ctx, r)
	c.mu.Lock()
	same := !c.closed && !c.operation && c.state == "signed_in" && c.viewID == view && c.child == child
	c.mu.Unlock()
	if !same {
		return pilot.WithdrawalFailure(r.ID, "stale_view")
	}
	return out
}

var _ pilot.WithdrawalSurfaceAPI = (*PoolController)(nil)
