package accountpilot

import (
	"context"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyRemotePayoutParentModeBeforeOwners(t *testing.T) {
	c := &RemotePoolController{}
	want := error(pilot.ErrRemotePoolConfig)
	if wire.TestPoW {
		want = pilot.ErrRemotePoolProfile
	}
	if _, e := c.BeginDailyPayoutRead(context.Background(), "unselected", 1, 0, 0); e != want {
		t.Fatal(e)
	}
	if _, e := c.DailyPayoutRead(context.Background(), "unselected", 1); e != want {
		t.Fatal(e)
	}
	if e := c.CancelDailyPayoutRead(context.Background(), "unselected", 1); e != want {
		t.Fatal(e)
	}
}

func TestDailyRemotePayoutParentCancellationBeforeAttachment(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	c.daily = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.BeginDailyPayoutRead(ctx, c.viewID, 1, 0, 0); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e := c.DailyPayoutRead(ctx, c.viewID, 1); e != context.Canceled {
		t.Fatal(e)
	}
	if e := c.CancelDailyPayoutRead(ctx, c.viewID, 1); e != context.Canceled {
		t.Fatal(e)
	}
	if c.attachment != nil || c.wallet != nil || c.operation || remoteControllerCheckView(c, c.viewID) != nil {
		t.Fatal("read refusal replaced or invalidated the original account")
	}
}
