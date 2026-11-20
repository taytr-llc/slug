// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
)

func TestNativeRemoteCleanupProjectionPreservesUncertainty(t *testing.T) {
	for _, cause := range []error{bridge.ErrRemotePoolBridgeCleanup, &bridge.RemotePoolFailure{Code: "cleanup_incomplete"}, errors.Join(errors.New("SYNTHETIC-PRIVATE-CAUSE"), bridge.ErrRemotePoolBridgeCleanup)} {
		e := &nativeRemoteError{cause: cause}
		if e.Error() != errNativeRemoteCleanup.Error() {
			t.Fatal("cleanup projected as generic unavailable", e)
		}
		e.cause = errors.Join(cause, bridge.ErrRemotePoolOutcomeUnknown)
		if e.Error() == errNativeRemoteCleanup.Error() {
			t.Fatal("cleanup hid uncertain mutation")
		}
	}
}
func TestNativeRemoteSignalsCoverBootstrapAndHandoff(t *testing.T) {
	for _, early := range []bool{true, false} {
		t.Run(map[bool]string{true: "bootstrap", false: "running"}[early], func(t *testing.T) {
			s := newNativeRemoteSignals()
			defer s.close()
			called := make(chan bool, 1)
			quit := func() { called <- s.life.Err() != nil }
			if !early && !s.handoff(quit) {
				t.Fatal("initialized handoff refused")
			}
			// Deliver to the same owned notification channel, never signal the test
			// process or any external app. The production watcher runs unchanged.
			s.events <- syscall.SIGTERM
			select {
			case <-s.life.Done():
			case <-time.After(time.Second):
				t.Fatal("signal did not cancel original lifetime")
			}
			if early {
				if s.handoff(quit) {
					t.Fatal("canceled bootstrap admitted an app callback")
				}
				select {
				case <-called:
					t.Fatal("bootstrap invoked an uninitialized app")
				default:
				}
			} else {
				select {
				case canceled := <-called:
					if !canceled {
						t.Fatal("quit preceded original cancellation")
					}
				case <-time.After(time.Second):
					t.Fatal("signal handoff lost")
				}
			}
			s.close()
			select {
			case <-s.done:
			default:
				t.Fatal("signal watcher not joined")
			}
		})
	}
}
