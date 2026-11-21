// Copyright (c) 2026 The SLUG developers. ISC license.
package mininglease

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDailyCaptureOriginalCancellationAndLegacyParity(t *testing.T) {
	for _, where := range []string{"before-callback", "inside-callback", "account-check", "after-callback"} {
		for _, daily := range []bool{false, true} {
			t.Run(where+map[bool]string{false: "/legacy", true: "/daily"}[daily], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				checks := 0
				a := authorizerFunc(func(c context.Context, token string, fn func(string, func() error) error) error {
					if c != ctx || token != publicFixtureToken {
						t.Fatal("replacement original context/token")
					}
					if where == "before-callback" {
						cancel()
						return c.Err()
					}
					if where == "inside-callback" {
						cancel()
					}
					e := fn(publicFixtureKey, func() error {
						checks++
						if where == "account-check" {
							cancel()
						}
						return c.Err()
					})
					if where == "after-callback" {
						cancel()
					}
					return e
				})
				capture := Capture
				want := error(ErrUnauthorized)
				if daily {
					capture, want = CaptureDaily, context.Canceled
				}
				l, e := capture(ctx, a, publicFixtureToken)
				if l != nil || e != want || checks > 1 || (where == "before-callback" || where == "inside-callback") && checks != 0 {
					t.Fatal("original cancellation/callback bound", e, checks)
				}
			})
		}
	}
}

func TestDailyCapturePreservesObservedIndependentFailures(t *testing.T) {
	private := errors.New("private observed account failure " + publicFixtureToken)
	for _, mode := range []string{"authority", "check", "bad-key", "nil-check", "double", "missing", "swallowed", "mixed-cause"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := authorizerFunc(func(c context.Context, _ string, fn func(string, func() error) error) error {
				switch mode {
				case "authority":
					cancel()
					return private
				case "mixed-cause":
					cancel()
					return errors.Join(private, c.Err())
				case "missing":
					cancel()
					return nil
				case "bad-key":
					cancel()
					return fn("malformed", c.Err)
				case "nil-check":
					cancel()
					return fn(publicFixtureKey, nil)
				case "double":
					if e := fn(publicFixtureKey, c.Err); e != nil {
						return e
					}
					cancel()
					return fn(publicFixtureKey, c.Err)
				}
				e := fn(publicFixtureKey, func() error { cancel(); return private })
				if mode == "swallowed" {
					return nil
				}
				return e
			})
			l, e := CaptureDaily(ctx, a, publicFixtureToken)
			if l != nil || !errors.Is(e, ErrUnauthorized) || !errors.Is(e, context.Canceled) || e == context.Canceled || strings.Contains(e.Error(), publicFixtureToken) {
				t.Fatal("observed failure lost or leaked", e)
			}
			if (mode == "authority" || mode == "check" || mode == "swallowed" || mode == "mixed-cause") && !errors.Is(e, private) {
				t.Fatal("original authority cause lost")
			}
		})
	}
}

func TestDailyCaptureSuccessfulLeaseRetainsOriginalCommitFences(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	a := authorizerFunc(func(c context.Context, token string, fn func(string, func() error) error) error {
		calls++
		if c != ctx || token != publicFixtureToken {
			return ErrUnauthorized
		}
		return fn(publicFixtureKey, c.Err)
	})
	l, e := CaptureDaily(ctx, a, publicFixtureToken)
	if e != nil || l.PublicKey() != publicFixtureKey || calls != 1 {
		t.Fatal("original capture", e, calls)
	}
	if e = l.Check(); e != nil || calls != 2 {
		t.Fatal("original short account check", e, calls)
	}
	cancel()
	if e = l.Do(func(func() error) error { t.Fatal("cancelled commit executed"); return nil }); e != context.Canceled || calls != 2 {
		t.Fatal("original cancelled lease revived", e, calls)
	}
}
