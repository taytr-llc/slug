// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestDailyRemoteControllerKeepsOriginalEpochAndRefusesLegacyWire(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	c.daily = true // Same original signed-account fixture; no wallet/node invented.
	view := c.viewID
	if e := remoteControllerCheckView(c, view); e != nil {
		t.Fatal(e)
	}
	if _, e := c.WireSnapshot(context.Background()); !errors.Is(e, pilot.ErrRemotePoolProtocol) {
		t.Fatal("legacy wire accepted daily owner", e)
	}
	if e := c.accounts.Logout(context.Background(), c.session.Token); e != nil {
		t.Fatal(e)
	}
	if e := remoteControllerCheckView(c, view); e == nil {
		t.Fatal("daily selector revived original login")
	}
}
func TestDailyRemoteControllerModeRefusesBeforeRoot(t *testing.T) {
	for _, daily := range []bool{false, true} {
		open := OpenRemotePoolController
		if daily {
			open = OpenDailyRemotePoolController
		}
		c, e := open(context.Background(), "/invalid-unselected-root", pilot.PoolOptions{}, pilot.RemotePoolInputs{})
		if c != nil || e == nil {
			t.Fatal("invalid mode/root reached owner")
		}
	}
}
