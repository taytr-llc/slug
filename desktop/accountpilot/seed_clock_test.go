//go:build slugtestclock && !slugtestpow

package accountpilot

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"path/filepath"
	"testing"
	"time"
)

func TestSeedFixtureClockScopesAssertionsWithoutWideningIdentity(t *testing.T) {
	at := time.Date(2026, 11, 21, 19, 59, 0, 0, time.UTC)
	c, e := NewSeedsForTestClock(pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}, func() time.Time { return at })
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for _, f := range []string{"zero256", "incrementing256"} {
		p, e := c.verifyPublicFixture(context.Background(), f, false)
		if e != nil || p.AuthenticatedAtMS != at.UnixMilli() || p.Subject != f || p.Issuer != fixtureIssuer || p.RecoveryVerified {
			t.Fatal("assertion", e)
		}
		r, e := c.verifyPublicFixture(context.Background(), f, true)
		if e != nil || r.AuthenticatedAtMS != at.UnixMilli() || !r.RecoveryVerified || len(r.RecoveryEventID) != 64 {
			t.Fatal("reset", e)
		}
	}
	if _, e = c.verifyPublicFixture(context.Background(), "unknown", false); e == nil {
		t.Fatal("unknown fixture accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = c.verifyPublicFixture(ctx, "zero256", false); e == nil {
		t.Fatal("cancel accepted")
	}
	if _, e = NewSeedsForTestClock(pilot.Config{}, nil); e == nil {
		t.Fatal("nil clock")
	}
}
