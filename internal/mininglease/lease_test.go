// Copyright (c) 2026 The SLUG developers. ISC license.
package mininglease

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Public secp256k1 generator; no private material is used by these tests.
const publicFixtureKey = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
const publicFixtureToken = "explicit-test-session"

type authorizerFunc func(context.Context, string, func(string, func() error) error) error

func (f authorizerFunc) WithSession(c context.Context, t string, fn func(string, func() error) error) error {
	return f(c, t, fn)
}

func TestOriginalMiningLeaseNeverRetainsTemporaryGuard(t *testing.T) {
	var mu sync.Mutex
	revoked, calls := false, 0
	a := authorizerFunc(func(ctx context.Context, token string, fn func(string, func() error) error) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if token != publicFixtureToken || revoked {
			return ErrUnauthorized
		}
		active := true
		defer func() { active = false }()
		return fn(publicFixtureKey, func() error {
			if !active {
				t.Error("expired temporary guard reused")
				return ErrUnauthorized
			}
			return ctx.Err()
		})
	})
	l, e := Capture(context.Background(), a, publicFixtureToken)
	if e != nil || l.PublicKey() != publicFixtureKey {
		t.Fatal(e)
	}
	if e = l.Check(); e != nil || calls != 2 {
		t.Fatal("fresh original credential check", e, calls)
	}
	mu.Lock()
	revoked = true
	mu.Unlock()
	wrote := false
	if e = l.Do(func(func() error) error { wrote = true; return nil }); e == nil || wrote {
		t.Fatal("revoked credential committed", e)
	}
}

func TestOriginalMiningLeaseSerializesOnlyFinalLocalCommit(t *testing.T) {
	var mu sync.Mutex
	revoked := false
	a := authorizerFunc(func(ctx context.Context, token string, fn func(string, func() error) error) error {
		mu.Lock()
		defer mu.Unlock()
		if revoked || token != publicFixtureToken {
			return ErrUnauthorized
		}
		return fn(publicFixtureKey, ctx.Err)
	})
	l, e := Capture(context.Background(), a, publicFixtureToken)
	if e != nil {
		t.Fatal(e)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { done <- l.Do(func(check func() error) error { close(entered); <-release; return check() }) }()
	<-entered
	reset := make(chan struct{})
	go func() { mu.Lock(); revoked = true; mu.Unlock(); close(reset) }()
	select {
	case <-reset:
		t.Fatal("reset crossed final local write")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	select {
	case <-reset:
	case <-time.After(time.Second):
		t.Fatal("reset did not finish after commit")
	}
	if e = l.Check(); e == nil {
		t.Fatal("old credential survived reset")
	}
}

func TestOriginalMiningLeaseRejectsBrokenAuthorizerCallbacks(t *testing.T) {
	callbackFailure := errors.New("local batch refused")
	for _, mode := range []string{"missing", "double", "swallowed", "changed-key", "nil-check", "revoked-before", "revoked-after", "outer-after"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			a := authorizerFunc(func(ctx context.Context, _ string, fn func(string, func() error) error) error {
				calls++
				if calls == 1 {
					return fn(publicFixtureKey, ctx.Err)
				}
				switch mode {
				case "missing":
					return nil
				case "double":
					_ = fn(publicFixtureKey, ctx.Err)
					_ = fn(publicFixtureKey, ctx.Err)
					return nil
				case "changed-key":
					return fn("03"+publicFixtureKey[2:], ctx.Err)
				case "nil-check":
					return fn(publicFixtureKey, nil)
				case "revoked-before":
					_ = fn(publicFixtureKey, func() error { return ErrUnauthorized })
					return nil
				case "revoked-after":
					n := 0
					_ = fn(publicFixtureKey, func() error {
						n++
						if n > 1 {
							return ErrUnauthorized
						}
						return nil
					})
					return nil
				case "outer-after":
					_ = fn(publicFixtureKey, ctx.Err)
					return errors.New("must not expose " + publicFixtureToken)
				default:
					_ = fn(publicFixtureKey, ctx.Err)
					return nil
				}
			})
			l, e := Capture(context.Background(), a, publicFixtureToken)
			if e != nil {
				t.Fatal(e)
			}
			commits := 0
			e = l.Do(func(func() error) error {
				commits++
				if mode == "swallowed" {
					return callbackFailure
				}
				return nil
			})
			if e == nil || commits > 1 || strings.Contains(e.Error(), publicFixtureToken) {
				t.Fatal("broken authority accepted or leaked", e, commits)
			}
			if mode == "swallowed" && !errors.Is(e, callbackFailure) {
				t.Fatal("swallowed callback error", e)
			}
			if (mode == "double" || mode == "revoked-after" || mode == "outer-after") && !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatal("postcallback failure not uncertain", e)
			}
		})
	}
}

func TestOriginalMiningLeaseRefusesMissingCaptureAndCancelledUse(t *testing.T) {
	for _, a := range []Authorizer{nil, authorizerFunc(func(context.Context, string, func(string, func() error) error) error { return nil })} {
		if l, e := Capture(context.Background(), a, publicFixtureToken); e == nil || l != nil {
			t.Fatal("missing capture accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := authorizerFunc(func(c context.Context, _ string, fn func(string, func() error) error) error {
		return fn(publicFixtureKey, c.Err)
	})
	l, e := Capture(ctx, a, publicFixtureToken)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	if e = l.Do(func(func() error) error { t.Fatal("cancelled operation executed"); return nil }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
