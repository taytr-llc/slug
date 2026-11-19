// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

func walletActivityProtocolError(e error) string {
	switch {
	case errors.Is(e, ErrActivityReplyUnknown), errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, ErrActivityNotFound):
		return "activity_not_found"
	case errors.Is(e, ErrActivityConflict):
		return "operation_conflict"
	case errors.Is(e, gymstable.ErrInvalid):
		return "invalid_request"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymstable.ErrUnauthorized), errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired), errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymstable.ErrForbidden):
		return "unauthorized"
	case errors.Is(e, gymstable.ErrBusy):
		return "busy"
	case errors.Is(e, gymstable.ErrCapacity):
		return "capacity"
	}
	return "activity_unavailable"
}
func handleWalletActivityRequest(ctx context.Context, r WalletActivityRequest, c WalletActivitySurfaceAPI) WalletActivityResponse {
	out := WalletActivityResponse{ID: r.ID, Version: 1, OK: true}
	var e error
	switch r.Command {
	case "wallet_activity_snapshot":
		v, x := c.WalletActivitySnapshot(ctx)
		out.Snapshot, e = &v, x
	case "wallet_activity_begin":
		v, x := c.BeginWalletActivity(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "wallet_activity_job":
		v, x := c.WalletActivityJob(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "wallet_activity_cancel":
		v, x := c.CancelWalletActivity(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "wallet_activity_page":
		v, x := c.WalletActivityPage(ctx, r.ViewID, r.JobID, r.After, r.Limit)
		out.Page, e = &v, x
	case "wallet_activity_detail":
		v, x := c.WalletActivityDetail(ctx, r.ViewID, r.JobID, r.TxID)
		out.Detail, e = &v, x
	default:
		e = gymstable.ErrInvalid
	}
	if e != nil {
		return WalletActivityResponse{ID: r.ID, Version: 1, Error: walletActivityProtocolError(e)}
	}
	return out
}
