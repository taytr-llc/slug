//go:build desktopintegration && slugtestclock && !slugtestpow

package accountpilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

// Resume only an explicitly named disposable journal whose prior test stopped
// after publication. No import, funding, transaction signing or Publish call.
func TestSeedClockResumeOwnedPublication(t *testing.T) {
	root, node := os.Getenv("SLUG_SEED_RESUME_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit preserved owned journal and normal node")
	}
	resolved, e := filepath.EvalSymlinks(root)
	if e != nil || resolved != root || !strings.HasPrefix(root, "/Users/taytrrip/slug/artifacts/set45/seed-testclock-") {
		t.Fatal("exact owned disposable journal", e)
	}
	// This continuation is deliberately tied to the preserved November21 case.
	// The authenticated source event is checked against this time after login.
	date := "2026-11-21"
	instant, e := time.Parse(time.RFC3339, "2026-11-21T20:00:01Z")
	if e != nil {
		t.Fatal(e)
	}
	var now atomic.Int64
	now.Store(instant.UnixMilli())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cfg := pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}
	c, e := NewSeedsForTestClock(cfg, func() time.Time { return time.UnixMilli(now.Load()) })
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.LoginFixture("zero256"); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool {
		x := c.Snapshot()
		if x.Account.State == "failed" {
			t.Fatal("resume login", x.Account.LastError)
		}
		return x.Account.State == "signed_in" && x.View != nil
	})
	snap := c.SeedSnapshot()
	if snap.Pin == nil || snap.NodeState != "offline" || snap.LastJobID != "" {
		t.Fatal("resume identity/cache")
	}
	ev, e := gymevent.EventForDate(snap.Pin.SourcePin, date)
	if e != nil || ev.CutoffMS+1000 != now.Load() {
		t.Fatal("exact preserved calendar", e)
	}
	w := newSeedClockWire(t, ctx, c)
	v := snap.View.ViewID
	var got pilot.SeedResponse
	seedClockWait(t, ctx, func() bool {
		got = w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: date})
		return got.Error != "busy"
	})
	if !got.OK || got.Record == nil || got.Record.Publication == nil || got.Record.Publication.DispatchCount != 1 || got.Record.Finality != nil || len(got.Record.Conflicts) != 0 {
		t.Fatal("preserved publication must be definite and unfinalized", got.Error)
	}
	retained := *got.Record
	immutable, _ := json.Marshal(retained.Declaration)
	seedClockWriteExclusive(t, filepath.Join(root, "public-seed-resume-before.json"), retained)
	t.Log("resuming exact durable transaction without Publish", root, retained.Publication.TxID, retained.Cursor.Height)
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool {
		if x := c.Snapshot(); x.State == "failed" {
			t.Fatal("resume Start", x.LastError)
		}
		return c.SeedSnapshot().NodeState == "ready"
	})
	seedClockFinish(t, c, w, ctx, root, cfg, &now, date, v, snap, immutable, retained.Publication.RawTx, retained.Publication.TxID, retained.Publication.Coin.BlockHash)
}

func seedClockWriteExclusive(t *testing.T, path string, v any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		t.Fatal(e)
	}
	if e = f.Sync(); e != nil {
		f.Close()
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}

// Capture the exact historical branch through the retained first Finality tip
// before any test reorg. The independent checker owns PoW/Merkle/link proof.
func seedClockExportChain(t *testing.T, ctx context.Context, rpc *seedClockRPC, root string, tip gymseed.Tip) {
	t.Helper()
	if tip.Height > 255 {
		t.Fatal("bounded public branch")
	}
	type row struct {
		Height   uint32 `json:"height"`
		Hash     string `json:"hash"`
		BlockHex string `json:"block_hex"`
	}
	rows := make([]row, tip.Height+1)
	hash := tip.Hash
	total := 0
	for height := int(tip.Height); height >= 0; height-- {
		var body string
		if e := rpc.call(ctx, "getblock", []any{hash, 0}, &body); e != nil {
			t.Fatal("public historical block", height, e)
		}
		raw, e := hex.DecodeString(body)
		if e != nil || len(raw) < 81 || len(raw) > 4_000_000 || body != hex.EncodeToString(raw) {
			t.Fatal("bounded public body", height, e)
		}
		total += len(raw)
		if total > 16<<20 {
			t.Fatal("public branch byte bound")
		}
		rows[height] = row{uint32(height), hash, body}
		previous := append([]byte(nil), raw[4:36]...)
		for i, j := 0, len(previous)-1; i < j; i, j = i+1, j-1 {
			previous[i], previous[j] = previous[j], previous[i]
		}
		hash = hex.EncodeToString(previous)
	}
	if hash != strings.Repeat("0", 64) {
		t.Fatal("branch did not reach genesis")
	}
	seedClockWriteExclusive(t, filepath.Join(root, "public-chain-evidence.json"), rows)
	t.Log("exact pre-reorg public branch", len(rows), "blocks", total, "bytes", tip.Hash)
}
