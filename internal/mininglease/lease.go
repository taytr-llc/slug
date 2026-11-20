// Copyright (c) 2026 The SLUG developers. ISC license.

// Package mininglease captures one original mining credential for short
// authorization fences. It never retains an authorizer's temporary recheck.
package mininglease

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/btcec/v2"
)

var (
	ErrUnauthorized   = errors.New("original mining session unavailable")
	ErrOutcomeUnknown = errors.New("mining session fence outcome unknown")
)

// Authorizer must serialize revocation/reset against fn until it returns.
// recheck is valid only inside fn. This is the established session contract.
type Authorizer interface {
	WithSession(context.Context, string, func(string, func() error) error) error
}

// Lease retains the exact original token and operation context, never a token
// supplier. Do not run node, wallet, network, proof or blocking source work in Do.
// A Do callback may perform only a bounded local commit or result publication.
type Lease struct {
	ctx              context.Context
	sessions         Authorizer
	token, publicKey string
}

func validKey(key string) bool {
	if len(key) != 66 || key != strings.ToLower(key) {
		return false
	}
	raw, err := hex.DecodeString(key)
	if err != nil {
		return false
	}
	_, err = btcec.ParsePubKey(raw)
	return err == nil
}

func available(a Authorizer) bool {
	if a == nil {
		return false
	}
	v := reflect.ValueOf(a)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice, reflect.Chan:
		return !v.IsNil()
	}
	return true
}

// Capture authorizes briefly and copies only the original credential binding.
// No external work executes while the author's account lease is held.
func Capture(ctx context.Context, sessions Authorizer, token string) (*Lease, error) {
	if ctx == nil || !available(sessions) || len(token) == 0 || len(token) > 4096 {
		return nil, ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	count, failed := 0, false
	var key string
	err := sessions.WithSession(ctx, token, func(public string, check func() error) error {
		count++
		if count != 1 || !validKey(public) || check == nil || ctx.Err() != nil || check() != nil {
			failed = true
			return ErrUnauthorized
		}
		key = public
		return nil
	})
	if err != nil || failed || count != 1 || ctx.Err() != nil {
		return nil, ErrUnauthorized
	}
	return &Lease{ctx: ctx, sessions: sessions, token: token, publicKey: key}, nil
}

func (l *Lease) PublicKey() string {
	if l == nil {
		return ""
	}
	return l.publicKey
}

// Check obtains a new short lease for the original token. It grants no future
// mutation authority; a subsequent write must itself occur inside Do.
func (l *Lease) Check() error {
	return l.Do(func(check func() error) error { return check() })
}

// Do reauthorizes the original token/key and holds revocation serialization
// through fn. Missing/double callbacks and swallowed failures are refused.
// A failure after fn has begun is conservatively unknown when authorization
// or the outer authorizer fails; callers must recover the original identity.
func (l *Lease) Do(fn func(func() error) error) error {
	if l == nil || l.ctx == nil || !available(l.sessions) || !validKey(l.publicKey) || fn == nil {
		return ErrUnauthorized
	}
	if err := l.ctx.Err(); err != nil {
		return err
	}
	count, failed, entered := 0, false, false
	var operationErr error
	err := l.sessions.WithSession(l.ctx, l.token, func(key string, check func() error) error {
		count++
		if count != 1 || key != l.publicKey || check == nil {
			failed = true
			return ErrUnauthorized
		}
		guard := func() error {
			if err := l.ctx.Err(); err != nil {
				return err
			}
			if check() != nil {
				return ErrUnauthorized
			}
			return l.ctx.Err()
		}
		if e := guard(); e != nil {
			failed = true
			return e
		}
		entered = true
		operationErr = fn(guard)
		if e := guard(); e != nil {
			failed = true
			return e
		}
		return operationErr
	})
	if failed || count != 1 || l.ctx.Err() != nil || err != nil && operationErr == nil {
		if entered {
			return errors.Join(ErrOutcomeUnknown, operationErr)
		}
		return ErrUnauthorized
	}
	if operationErr != nil {
		return operationErr
	}
	if err != nil {
		return ErrOutcomeUnknown
	}
	return nil
}
