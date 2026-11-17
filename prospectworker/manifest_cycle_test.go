//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestManifestCycleHookPrecedesDispatchAndIsLockFree(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "pending"}[pending], func(t *testing.T) {
			f, c := mwFixture(t)
			box, _ := mwBox(t, c)
			if pending {
				mwFound(t, box, f)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, transport, signatures, hashes := 0, 0, 0, 0
			a := &mwAuthority{c: c, f: f, issue: func() { transport++ }, submit: func() (prospectservice.ManifestAdmissionResult, error) { transport++; return mwResult(f), nil }}
			n := &mwNode{f: f, call: func(string) { transport++ }}
			var w *ManifestWorker
			hook := func(context.Context) error {
				calls++
				_ = w.Status()
				if _, e := box.List(0, 1); e != nil {
					t.Fatal(e)
				}
				cancel()
				return nil
			}
			var e error
			w, e = NewManifestWithHooks(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				signatures++
				return f.Auth, nil
			}, box, nil, hook)
			if e != nil {
				t.Fatal(e)
			}
			w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
			w.hash = func(h *wire.BlockHeader) chainhash.Hash { hashes++; return h.BlockHash() }
			if e = w.Run(ctx); e != nil || calls != 1 || transport != 0 || signatures != 0 || hashes != 0 {
				t.Fatal(e, calls, transport, signatures, hashes)
			}
		})
	}
}
func TestManifestCycleHookFailureAndActualDrain(t *testing.T) {
	for _, mode := range []string{"error", "panic", "cancel-before", "cancel-during", "cancel-return"} {
		t.Run(mode, func(t *testing.T) {
			f, c := mwFixture(t)
			box, _ := mwBox(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, transport := 0, 0
			entered, release := make(chan struct{}), make(chan struct{})
			a := &mwAuthority{c: c, f: f, issue: func() { transport++ }, submit: func() (prospectservice.ManifestAdmissionResult, error) { transport++; return mwResult(f), nil }}
			w, e := NewManifestWithHooks(c, a, &mwNode{f: f, call: func(string) { transport++ }}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				transport++
				return f.Auth, nil
			}, box, nil, func(ctx context.Context) error {
				calls++
				switch mode {
				case "cancel-return":
					cancel()
					return ctx.Err()
				case "error":
					return errors.New("private hook error")
				case "panic":
					panic("private hook panic")
				case "cancel-during":
					close(entered)
					<-release
					if ctx.Err() == nil {
						t.Error("hook context was not canceled")
					}
				}
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			if mode == "cancel-before" {
				cancel()
			}
			done := make(chan error, 1)
			go func() { done <- w.Run(ctx) }()
			if mode == "cancel-during" {
				<-entered
				cancel()
				select {
				case <-done:
					t.Fatal("Run returned without draining hook")
				case <-time.After(20 * time.Millisecond):
				}
				close(release)
			}
			e = <-done
			if mode == "error" || mode == "panic" {
				if !errors.Is(e, ErrManifestCycleHook) || e.Error() != ErrManifestCycleHook.Error() || w.Status().State != "failed" || w.Status().LastError != "cycle_unavailable" {
					t.Fatal(e, w.Status())
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if transport != 0 || mode == "cancel-before" && calls != 0 || mode != "cancel-before" && calls != 1 {
				t.Fatal(calls, transport)
			}
		})
	}
}
func TestManifestCycleNilPreservesOldDispatch(t *testing.T) {
	for _, constructor := range []string{"old", "nil-hook"} {
		t.Run(constructor, func(t *testing.T) {
			f, c := mwFixture(t)
			box, _ := mwBox(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			issued := 0
			a := &mwAuthority{c: c, f: f, issue: func() { issued++; cancel() }}
			signer := func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				t.Error("signed after cancellation")
				return f.Auth, nil
			}
			var w *ManifestWorker
			var e error
			if constructor == "old" {
				w, e = NewManifest(c, a, &mwNode{f: f}, signer, box)
			} else {
				w, e = NewManifestWithHooks(c, a, &mwNode{f: f}, signer, box, nil, nil)
			}
			if e != nil {
				t.Fatal(e)
			}
			w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
			if e = w.Run(ctx); e != nil || issued != 1 || w.Status().LastError == "cycle_unavailable" {
				t.Fatal(e, issued, w.Status())
			}
		})
	}
}
