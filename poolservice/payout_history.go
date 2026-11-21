// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/poolsettle"
)

// MaxPayoutResponseSize bounds the entire new HTTP success body, including LF.
// Existing worker, balance and error response limits are unchanged.
const MaxPayoutResponseSize = 131072

type PayoutHistoryResponse struct {
	Version   uint32                   `json:"version"`
	BindingID string                   `json:"binding_id"`
	History   poolsettle.PayoutHistory `json:"history"`
}

// PayoutReader is separate from Authority; workers need no history capability.
type PayoutReader interface {
	Payouts(context.Context) (PayoutHistoryResponse, error)
}

func payoutErrorCode(code string) bool {
	switch code {
	case "invalid_request", "unauthorized", "forbidden", "too_large", "rate_limited", "busy", "capacity", "unavailable":
		return true
	}
	return false
}

func payoutFailure(err error) string {
	if errors.Is(err, mininglease.ErrUnauthorized) || errors.Is(err, mininglease.ErrOutcomeUnknown) {
		return "unauthorized"
	}
	code := operationFailure(err, false)
	if !payoutErrorCode(code) {
		return "unavailable"
	}
	return code
}

func (c *Client) Payouts(ctx context.Context) (PayoutHistoryResponse, error) {
	if c != nil && c.daily != nil {
		return PayoutHistoryResponse{}, &ClientError{Code: "invalid_request"}
	}
	if err := c.ready(ctx); err != nil {
		return PayoutHistoryResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := c.post(ctx, "payouts", struct{}{}, false, MaxPayoutResponseSize)
	if err != nil {
		var remote *ClientError
		if !errors.As(err, &remote) || remote.OutcomeUnknown || !payoutErrorCode(remote.Code) {
			return PayoutHistoryResponse{}, failure(false)
		}
		return PayoutHistoryResponse{}, err
	}
	var result PayoutHistoryResponse
	if decodePayoutResponse(raw, &result) != nil || result.Version != Version1 || result.BindingID != c.binding {
		return PayoutHistoryResponse{}, failure(false)
	}
	result.History, err = poolsettle.ValidatePayoutHistory(result.History, c.gym)
	if err != nil || ctx.Err() != nil {
		return PayoutHistoryResponse{}, failure(false)
	}
	return result, nil
}

var _ PayoutReader = (*Client)(nil)
