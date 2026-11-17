// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestNativeManifestModeAndWrongRouteRefusal(t *testing.T) {
	old := &Gym{}
	if old.Mode() != pilot.AccountMode {
		t.Fatal("default changed")
	}
	if _, e := old.ManifestSnapshot(); !errors.Is(e, bridge.ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e := old.ManifestPortrait(1); !errors.Is(e, bridge.ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e := old.ExportManifestReceipt(1); !errors.Is(e, bridge.ErrUnavailable) {
		t.Fatal(e)
	}
	candidate := &Gym{manifestHelper: new(bridge.ManifestBridge)}
	if candidate.Mode() != pilot.ManifestMode {
		t.Fatal("candidate mode not explicit")
	}
	if _, e := candidate.Snapshot(); !errors.Is(e, bridge.ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e := candidate.ExportReceipt(1); !errors.Is(e, bridge.ErrUnavailable) {
		t.Fatal(e)
	}
}
func TestCandidateNativeDialogPinsAccountBeforeTransport(t *testing.T) {
	// The unstarted bridge would block/panic if any action reached transport.
	// Holding the native dialog gate must reject account changes and overlap.
	gym := &Gym{manifestHelper: new(bridge.ManifestBridge)}
	gym.exportMu.Lock()
	defer gym.exportMu.Unlock()
	for _, action := range []func() error{func() error { return gym.LoginFixture("incrementing256") }, gym.Logout, gym.ResetFixtureAccount} {
		if e := action(); !errors.Is(e, bridge.ErrBusy) {
			t.Fatal(e)
		}
	}
	if _, e := gym.ExportManifestReceipt(1); !errors.Is(e, bridge.ErrBusy) {
		t.Fatal(e)
	}
	if _, e := gym.ExportRecovery(); !errors.Is(e, bridge.ErrBusy) {
		t.Fatal(e)
	}
	if _, e := gym.VerifyBackup(); !errors.Is(e, bridge.ErrBusy) {
		t.Fatal(e)
	}
}
