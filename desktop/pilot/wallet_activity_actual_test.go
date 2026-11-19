//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
)

type activityActualReference struct {
	Name   string `json:"name"`
	Record struct {
		Raw     string `json:"raw_hex"`
		Credits []struct {
			Index  uint32 `json:"index"`
			Atoms  int64  `json:"atoms"`
			Change bool   `json:"change"`
		} `json:"credits"`
		Debits []struct {
			Index uint32 `json:"index"`
			Atoms int64  `json:"atoms"`
		} `json:"debits"`
	} `json:"record"`
	Expected struct {
		Item    WalletActivityItem     `json:"item"`
		Outputs []WalletActivityOutput `json:"outputs"`
	} `json:"expected"`
}

func activityActualReferences(t *testing.T) map[string]activityActualReference {
	t.Helper()
	raw, e := os.ReadFile("../../../slugwallet/wallet/testdata/own-activity/core-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "4b3880f7facb9286502aa6a3bdcf38db7821a96e1d1b622f4d27a898a057598e" {
		t.Fatal("independent core fixture differs")
	}
	var corpus struct {
		Transactions struct {
			Positives []activityActualReference `json:"positives"`
		} `json:"transactions"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("reference parse")
	}
	out := make(map[string]activityActualReference)
	for _, row := range corpus.Transactions.Positives {
		out[row.Name] = row
	}
	return out
}

// Concrete normal-PoW node transport, actual synchronized schema9 wallet DB,
// atomic capture and production projection on copies of the qualified Set68
// wallets. No synthetic activityRead hook, sends, addresses or mining. Original
// view admission/pipe/native lifetimes have separate controller/package tests.
func TestWalletActivityRetainedActual(t *testing.T) {
	root, binary := os.Getenv("SLUG_ACTIVITY_ACTUAL_ROOT"), os.Getenv("SLUG_ACTIVITY_ACTUAL_NODE")
	if root == "" || binary == "" {
		t.Skip("explicit owned fixture inputs required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(binary) {
		t.Fatal("absolute owned paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	references := activityActualReferences(t)
	node := phaseAActualNodes(t, ctx, binary, root, 0)
	defer node.close(t)
	rpc := prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}
	var results []map[string]any
	for _, fixture := range phaseAActualFixtures {
		func() {
			original, e := openAccountFixtureWallet(filepath.Join(root, "wallets", fixture.name), fixture.name, false)
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				if e := original.Close(); e != nil {
					t.Error(e)
				}
			}()
			if e = original.Attach(ctx, rpc); e != nil {
				t.Fatal(e)
			}
			phaseAActualWaitWallet(t, ctx, original)
			before := phaseAActualState(t, ctx, original, fixture)
			observed, e := original.captureWalletActivity(ctx, rpc)
			if e != nil {
				t.Fatal("actual bounded capture", e)
			}
			if observed.snapshot.Version != 1 || observed.snapshot.SyncedHeight != 110 || observed.snapshot.SyncedHash.String() != phaseAActualTip.Hash || observed.snapshot.HasOlder || len(observed.snapshot.Records) != 3 {
				t.Fatal("actual capture membership/tip")
			}
			identity := original.Identity()
			job := WalletActivityJob{Version: 1, JobID: strings.Repeat("a", 64), State: "running", Binding: WalletActivityBinding{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}}
			cache, e := buildWalletActivityCache(ctx, job, observed)
			if e != nil {
				t.Fatal(e)
			}
			cases := []string{"actual/" + fixture.name + "/own-confirmed", "actual/" + fixture.name + "/pool-payout", "actual-raw/" + fixture.name + "/incoming"}
			expected := make(map[string]activityActualReference)
			for _, name := range cases {
				ref, ok := references[name]
				if !ok {
					t.Fatal("missing reference", name)
				}
				expected[ref.Expected.Item.TxID] = ref
			}
			var details []WalletActivityDetail
			for _, record := range cache.records {
				ref, ok := expected[record.TxID.String()]
				if !ok {
					t.Fatal("unexpected recorded transaction")
				}
				delete(expected, record.TxID.String())
				if hex.EncodeToString(record.Raw) != ref.Record.Raw || len(record.Credits) != len(ref.Record.Credits) || len(record.Debits) != len(ref.Record.Debits) {
					t.Fatal("raw/ownership differs from independent source")
				}
				for i, c := range record.Credits {
					r := ref.Record.Credits[i]
					if c.Index != r.Index || c.Atoms != r.Atoms || c.Change != r.Change {
						t.Fatal("credit differs")
					}
				}
				for i, d := range record.Debits {
					r := ref.Record.Debits[i]
					if d.Index != r.Index || d.Atoms != r.Atoms {
						t.Fatal("debit differs")
					}
				}
				item, outputs, e := projectWalletActivity(ctx, record, *cache.job.Capture, true)
				if e != nil {
					t.Fatal(e)
				}
				want := ref.Expected.Item
				if strings.HasPrefix(ref.Name, "actual-raw/") {
					// Old evidence has no recipient first-seen timestamp. Preserve its actual
					// new observation, but never pretend the modeled zero was an old DB fact.
					if !payoutTime(record.RecordedAtMS, true) {
						t.Fatal("incoming recorded time bounds")
					}
					want.RecordedAtMS = record.RecordedAtMS
				}
				if !WalletActivityItemEqual(item, want) || !reflect.DeepEqual(outputs, ref.Expected.Outputs) {
					t.Fatalf("actual projection differs from independent source: %s", ref.Name)
				}
				details = append(details, WalletActivityDetail{Version: 1, Binding: job.Binding, JobID: job.JobID, Capture: *cache.job.Capture, Item: item, Outputs: outputs})
			}
			if len(expected) != 0 {
				t.Fatal("missing actual incoming/outgoing/payout")
			}
			after := phaseAActualState(t, ctx, original, fixture)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("read changed original wallet semantic state")
			}
			again, e := original.captureWalletActivity(ctx, rpc)
			if e != nil || !reflect.DeepEqual(again.snapshot, observed.snapshot) {
				t.Fatal("repeat capture changed stable wallet facts", e)
			}
			payoutActualWrite(t, filepath.Join(root, "activity-"+fixture.name+".json"), details)
			results = append(results, map[string]any{"fixture": fixture.name, "records": len(cache.items), "wallet_height": observed.snapshot.SyncedHeight, "wallet_tip": observed.snapshot.SyncedHash.String(), "unchanged_wallet_state": true, "incoming_timestamp_baseline": "bounded actual observation; independent old first-seen unavailable"})
		}()
	}
	node.close(t)
	payoutActualWrite(t, filepath.Join(root, "actual-public-result.json"), map[string]any{"fixtures": []string{"zero256", "incrementing256"}, "results": results, "no_mining": true, "no_delivery": true, "no_address_allocation": true, "concrete_runtime": true})
}
