// Copyright (c) 2026 The SLUG developers. ISC license.
package mininglease

import (
	"context"
	"errors"
)

// CaptureDaily has the same original-token/key authority as Capture. It keeps
// exact original-context cancellation distinct from an observed failed account
// check, malformed callback, or swallowed failure. It does no extra account
// check after cancellation. Legacy Capture and all Lease commit semantics stay
// unchanged; this entrypoint grants no authority on any error.
func CaptureDaily(ctx context.Context, sessions Authorizer, token string) (*Lease, error) {
	if ctx == nil || !available(sessions) || len(token) == 0 || len(token) > 4096 {
		return nil, ErrUnauthorized
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	var key string
	var callbackErr error
	count := 0
	e := sessions.WithSession(ctx, token, func(public string, check func() error) error {
		count++
		if count != 1 || !validKey(public) || check == nil {
			callbackErr = errors.Join(callbackErr, ErrUnauthorized)
			return ErrUnauthorized
		}
		if e := ctx.Err(); e != nil {
			callbackErr = e
			return e
		}
		if e := check(); e != nil {
			callbackErr = e
			return e
		}
		key = public
		return nil
	})
	cause := ctx.Err()
	var shapeErr error
	if count > 1 || count == 0 && e == nil || callbackErr != nil && e == nil {
		shapeErr = ErrUnauthorized
	}
	if e == nil && callbackErr == nil && shapeErr == nil && cause == nil {
		return &Lease{ctx: ctx, sessions: sessions, token: token, publicKey: key}, nil
	}
	// Only the exact context sent to this original authorizer can supply the
	// pure cancellation result. A mixed/joined authority error stays a refusal.
	if cause != nil && shapeErr == nil && (e == nil || e == cause) && (callbackErr == nil || callbackErr == cause) {
		return nil, cause
	}
	return nil, &dailyCaptureFailure{cause: errors.Join(ErrUnauthorized, e, callbackErr, shapeErr, cause)}
}

// Do not expose an authorizer's private diagnostic in the public error string.
// The original observed causes remain inspectable for error classification.
type dailyCaptureFailure struct{ cause error }

func (*dailyCaptureFailure) Error() string   { return "original daily mining session unavailable" }
func (e *dailyCaptureFailure) Unwrap() error { return e.cause }
