// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
)

func payoutProtocolError(e error) string {
	switch {
	case errors.Is(e, ErrPayoutReplyUnknown), errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, ErrPayoutNotFound):
		return "payout_not_found"
	case errors.Is(e, ErrPayoutConflict):
		return "operation_conflict"
	case errors.Is(e, gymstable.ErrInvalid):
		return "invalid_request"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymstable.ErrUnauthorized), errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired), errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymstable.ErrForbidden):
		return "unauthorized"
	case errors.Is(e, gymstable.ErrBusy), errors.Is(e, errWalletPayoutBusy):
		return "busy"
	case errors.Is(e, gymstable.ErrCapacity):
		return "capacity"
	}
	return "payout_unavailable"
}
func payoutTaskError(e error) string {
	if errors.Is(e, gymstable.ErrCapacity) {
		return "capacity"
	}
	if errors.Is(e, gymstable.ErrBusy) {
		return "busy"
	}
	var ce *poolservice.ClientError
	if errors.As(e, &ce) {
		if ce.Code == "busy" || ce.Code == "rate_limited" {
			return "busy"
		}
		if ce.Code == "capacity" {
			return "capacity"
		}
	}
	return "payout_unavailable"
}
func handlePoolPayoutRequest(ctx context.Context, r PoolPayoutRequest, c PoolPayoutSurfaceAPI) PoolPayoutResponse {
	out := PoolPayoutResponse{ID: r.ID, Version: 1, OK: true}
	var e error
	switch r.Command {
	case "pool_payout_snapshot":
		v, x := c.PoolPayoutSnapshot(ctx)
		out.Snapshot, e = &v, x
	case "pool_payout_begin":
		v, x := c.BeginPoolPayoutHistory(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "pool_payout_job":
		v, x := c.PoolPayoutJob(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "pool_payout_cancel":
		v, x := c.CancelPoolPayoutHistory(ctx, r.ViewID, r.JobID)
		out.Job, e = &v, x
	case "pool_payout_page":
		v, x := c.PoolPayoutPage(ctx, r.ViewID, r.JobID, r.After, r.Limit)
		out.Page, e = &v, x
	case "pool_payout_wallet":
		v, x := c.ReadPoolPayoutTransaction(ctx, r.ViewID, r.JobID, r.TxID, r.OutputIndex)
		out.Observation, e = &v, x
	default:
		e = gymstable.ErrInvalid
	}
	if e != nil {
		return PoolPayoutResponse{ID: r.ID, Version: 1, Error: payoutProtocolError(e)}
	}
	return out
}
