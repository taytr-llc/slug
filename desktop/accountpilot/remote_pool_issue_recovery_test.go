// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"testing"
)

func TestRecoverableRemoteControllerOriginalEpochAndLegacyRefusal(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	c.daily = true
	c.recovery = &pilot.RecoverableRemotePoolConfig{Version: 1}
	// The actual original signed-account owner is used here. This is an epoch
	// fence control, not a node-ready attachment or financial acceptance claim.
	view := c.viewID
	if e := remoteControllerCheckView(c, view); e != nil {
		t.Fatal(e)
	}
	if _, e := c.WireSnapshot(context.Background()); !errors.Is(e, pilot.ErrRemotePoolProtocol) {
		t.Fatal("old helper admitted selected controller", e)
	}
	if e := c.accounts.Logout(context.Background(), c.session.Token); e != nil {
		t.Fatal(e)
	}
	if e := remoteControllerCheckView(c, view); e == nil {
		t.Fatal("original login revived")
	}
}
func TestRecoverableRemoteControllerRefusesBeforeOriginalRoot(t *testing.T) {
	c, e := OpenRecoverableDailyRemotePoolController(context.Background(), "/unselected-original-root", pilot.PoolOptions{}, pilot.RecoverableRemotePoolInputs{})
	if c != nil || e == nil {
		t.Fatal("invalid selection reached original root")
	}
}
