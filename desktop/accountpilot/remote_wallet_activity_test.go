package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
	"testing"
)

func TestDailyWalletOwnActivityParentMode(t *testing.T) {
	c := &RemotePoolController{}
	want := error(pilot.ErrRemotePoolConfig)
	if wire.TestPoW {
		want = pilot.ErrRemotePoolProfile
	}
	if _, e := c.BeginWalletActivityRead(context.Background(), "unselected", 1); e != want {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityRead(context.Background(), "unselected", 1); e != want {
		t.Fatal(e)
	}
	if e := c.CancelWalletActivityRead(context.Background(), "unselected", 1); e != want {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityPage(context.Background(), "unselected", 1, 0, 8); e != want {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityDetail(context.Background(), "unselected", 1, ""); e != want {
		t.Fatal(e)
	}
	if _, e := c.ObserveDailyPayout(context.Background(), "unselected", 1, 1, "", 0); e != want {
		t.Fatal(e)
	}
}
func TestDailyWalletOwnActivityParentCancellation(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	c.daily = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.BeginWalletActivityRead(ctx, c.viewID, 1); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityRead(ctx, c.viewID, 1); e != context.Canceled {
		t.Fatal(e)
	}
	if e := c.CancelWalletActivityRead(ctx, c.viewID, 1); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityPage(ctx, c.viewID, 1, 0, 8); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e := c.WalletActivityDetail(ctx, c.viewID, 1, ""); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e := c.ObserveDailyPayout(ctx, c.viewID, 1, 1, "", 0); e != context.Canceled {
		t.Fatal(e)
	}
	if c.attachment != nil || c.wallet != nil || c.operation || remoteControllerCheckView(c, c.viewID) != nil {
		t.Fatal("read refusal changed original owner")
	}
}
