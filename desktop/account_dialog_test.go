// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"errors"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
)

func TestNativeDialogPreventsAccountChangeBeforeTransport(t *testing.T) {
	gym := &Gym{}
	// A held export gate models a native dialog open while the bridge is free
	// for snapshots. No helper exists: reaching transport would panic.
	gym.exportMu.Lock()
	defer gym.exportMu.Unlock()
	for name, action := range map[string]func() error{
		"login":  func() error { return gym.LoginFixture("incrementing256") },
		"logout": gym.Logout,
		"reset":  gym.ResetFixtureAccount,
	} {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, bridge.ErrBusy) {
				t.Fatalf("account changed during native dialog: %v", err)
			}
		})
	}
}
