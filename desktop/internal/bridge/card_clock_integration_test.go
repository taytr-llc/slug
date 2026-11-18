//go:build desktopintegration && slugtestclock && !slugtestpow

package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymcard"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The child reopens the explicitly retained genuine chain/journals. Only this
// tagged test process reads a public saved clock; no release CLI accepts one.
func TestCardClockRetainedNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_CARD_REPLAY_CHILD")
	if root == "" {
		return
	}
	fail := func(e error) {
		_ = os.WriteFile(filepath.Join(os.Getenv("SLUG_CARD_REPLAY_OUTPUT"), "child-error.txt"), []byte(e.Error()+"\n"), 0600)
		os.Exit(2)
	}
	raw, e := os.ReadFile(filepath.Join(root, "public-card-result.json"))
	if e != nil || len(raw) > 2<<20 {
		fail(fmt.Errorf("public clock source: %v", e))
	}
	var p struct {
		ClockMS     int64 `json:"clock_ms"`
		TaggedClock bool  `json:"tagged_clock"`
		NormalPoW   bool  `json:"normal_pow"`
	}
	if json.Unmarshal(raw, &p) != nil || !p.TaggedClock || !p.NormalPoW || p.ClockMS <= 0 {
		fail(fmt.Errorf("public test-clock declaration"))
	}
	c, e := accountpilot.NewCardsForTestClock(pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(p.ClockMS) })
	if e != nil {
		fail(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	e = pilot.ServeCards(ctx, c, os.Stdin, os.Stdout)
	_ = c.Close()
	if e != nil && e != context.Canceled {
		fail(e)
	}
	os.Exit(0)
}

func TestCardClockRetainedNativeReplay(t *testing.T) {
	root, node := os.Getenv("SLUG_CARD_REPLAY_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit retained genuine Card journal and normal node")
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil || !filepath.IsAbs(root) || real != root {
		t.Fatal("canonical owned root", e)
	}
	raw, e := os.ReadFile(filepath.Join(root, "public-card-result.json"))
	if e != nil || len(raw) > 2<<20 {
		t.Fatal(e)
	}
	baselineHash := sha256.Sum256(raw)
	var baseline struct {
		ClockMS  int64               `json:"clock_ms"`
		Saturday string              `json:"saturday"`
		Snapshot pilot.CardSnapshot  `json:"snapshot"`
		Build    gymcard.BuildResult `json:"build"`
		Read     gymcard.ReadResult  `json:"read"`
	}
	if json.Unmarshal(raw, &baseline) != nil || baseline.Build.Record.Plan.BoutCount < 1 || baseline.Read.Status.Usable {
		t.Fatal("genuine conflicted baseline")
	}
	output, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "card-native-replay-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("owned native replay evidence", output)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCardClockRetainedNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_CARD_REPLAY_CHILD="+root, "SLUG_CARD_REPLAY_OUTPUT="+output)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled = true, true, true, true
	processes := newManifestProcessInventory(filepath.Join(root, "data"), node, cmd.Process.Pid)
	defer func() {
		if b != nil {
			_ = b.Close()
		}
		processes.checkClosed(t)
	}()
	request := func(q pilot.CardRequest) pilot.CardResponse {
		t.Helper()
		var r pilot.CardResponse
		seedNativeClockWait(t, ctx, func() bool { r = b.CardRequest(ctx, q); return r.Error != "busy" })
		return r
	}
	waitAccount := func(state string) pilot.ManifestSnapshot {
		t.Helper()
		var s pilot.ManifestSnapshot
		seedNativeClockWait(t, ctx, func() bool {
			r, e := b.Request(ctx, "snapshot", 0)
			if e != nil {
				t.Fatal(e)
			}
			if r.Snapshot == nil {
				t.Fatal("snapshot")
			}
			s = *r.Snapshot
			if s.Account.State == "failed" {
				t.Fatal("account", s.Account.LastError)
			}
			return s.Account.State == state
		})
		return s
	}
	if r := request(pilot.CardRequest{Command: "card_snapshot"}); !r.OK || r.Snapshot.State != "unavailable" {
		t.Fatal("signed-out Card", r.Error)
	}
	var receipts []pilot.ManifestReceipt
	rows := []map[string]any{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		if _, e = b.RequestFixture(ctx, fixture); e != nil {
			t.Fatal(e)
		}
		account := waitAccount("signed_in")
		processes.capture(t, fixture, "retained-card-native-replay", account.View.AccountID, account.View.AuthorityOrigin, false)
		s := request(pilot.CardRequest{Command: "card_snapshot"})
		if !s.OK || s.Snapshot.Pin == nil || s.Snapshot.LastJobID != "" || !reflect.DeepEqual(s.Snapshot.Pin, baseline.Snapshot.Pin) || s.Snapshot.StoreID != baseline.Snapshot.StoreID || s.Snapshot.CreatedAtMS != baseline.Snapshot.CreatedAtMS || s.Snapshot.SourceCreatedAtMS != baseline.Snapshot.SourceCreatedAtMS {
			t.Fatal("native authority/restart", s.Error)
		}
		v := s.Snapshot.View.ViewID
		var stableBefore pilot.StableResponse
		seedNativeClockWait(t, ctx, func() bool {
			stableBefore = b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
			return stableBefore.OK && stableBefore.Snapshot != nil && stableBefore.Snapshot.Status != nil && stableBefore.Snapshot.Status.CaughtUp && stableBefore.Snapshot.ActiveJobID == "" && stableBefore.Snapshot.QueuedJobID == ""
		})
		walletBefore, e := b.Request(ctx, "snapshot", 0)
		if e != nil || walletBefore.Snapshot == nil || walletBefore.Snapshot.State != "stopped" {
			t.Fatal("stopped wallet baseline", e)
		}
		entryBefore := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_status", ViewID: v, Saturday: baseline.Saturday})
		if !entryBefore.OK || entryBefore.Status == nil {
			t.Fatal("entry baseline", entryBefore.Error)
		}
		seedBefore := b.SeedRequest(ctx, pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: baseline.Saturday})
		if !seedBefore.OK || seedBefore.Record == nil {
			t.Fatal("seed baseline", seedBefore.Error)
		}
		read := request(pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: baseline.Saturday})
		if !read.OK || read.Record.Status.Usable || !reflect.DeepEqual(read.Record.Record, baseline.Build.Record) {
			t.Fatal("native historical card", read.Error)
		}
		j := request(pilot.CardRequest{Command: "card_build", ViewID: v, Saturday: baseline.Saturday})
		if !j.OK || j.Job == nil || j.Job.State != "queued" {
			t.Fatal("native Build ACK", j.Error)
		}
		seedNativeClockWait(t, ctx, func() bool {
			j = request(pilot.CardRequest{Command: "card_job", ViewID: v, JobID: j.Job.JobID})
			if !j.OK || j.Job == nil {
				t.Fatal("native job", j.Error)
			}
			return j.Job.State != "queued" && j.Job.State != "running"
		})
		if j.Job.State != "completed" || !j.Job.Result.Replayed || j.Job.Result.Status.Usable || !reflect.DeepEqual(j.Job.Result.Record, baseline.Build.Record) {
			t.Fatal("native immutable replay", j.Job.Error)
		}
		terminal, _ := json.Marshal(j.Job)
		fresh := request(pilot.CardRequest{Command: "card_status", ViewID: v, Saturday: baseline.Saturday})
		if !fresh.OK || fresh.Status.Usable || fresh.Status.Source.Status.ConflictCount == 0 {
			t.Fatal("native current overlay", fresh.Error)
		}
		again := request(pilot.CardRequest{Command: "card_job", ViewID: v, JobID: j.Job.JobID})
		same, _ := json.Marshal(again.Job)
		if !again.OK || !bytes.Equal(same, terminal) {
			t.Fatal("native terminal changed after explicit overlay")
		}
		// Load only the current fixture's entered public records through the
		// existing authenticated Inventory + receipt path; no raw source DB read.
		for _, p := range baseline.Build.Record.Source.SourceReceipt.Input.Participants {
			if p.Owner.AccountID != account.View.AccountID {
				continue
			}
			for _, m := range p.Members {
				q := pilot.StableRequest{Command: "stable_inventory", ViewID: v, After: m.Assignment.Sequence - 1, Limit: 1}
				var sr pilot.StableResponse
				seedNativeClockWait(t, ctx, func() bool { sr = b.StableRequest(ctx, q); return sr.Error != "busy" })
				if !sr.OK || sr.Job == nil {
					t.Fatal("public source Inventory", sr.Error)
				}
				seedNativeClockWait(t, ctx, func() bool {
					sr = b.StableRequest(ctx, pilot.StableRequest{Command: "stable_job", ViewID: v, JobID: sr.Job.JobID})
					if sr.Error == "busy" {
						return false
					}
					if !sr.OK || sr.Job == nil {
						t.Fatal(sr.Error)
					}
					return sr.Job.State != "queued" && sr.Job.State != "running"
				})
				if sr.Job.State != "done" || sr.Job.Page == nil || len(sr.Job.Page.Items) != 1 || sr.Job.Page.Items[0].Assignment.ProspectID != m.Assignment.ProspectID {
					t.Fatal("public source selected identity", sr.Job.Error)
				}
				r, e := b.Request(ctx, "manifest_receipt", m.Assignment.Sequence)
				if e != nil || !r.OK || r.ManifestReceipt == nil {
					t.Fatal("public admitted receipt", e, r.Error)
				}
				receipts = append(receipts, *r.ManifestReceipt)
				if _, e = b.ReleaseManifest(ctx, v, m.Assignment.ProspectID); e != nil {
					t.Fatal(e)
				}
			}
		}
		walletAfter, e := b.Request(ctx, "snapshot", 0)
		if e != nil || walletAfter.Snapshot == nil {
			t.Fatal(e)
		}
		a, z := walletBefore.Snapshot, walletAfter.Snapshot
		if z.State != "stopped" || a.Attempts != z.Attempts || a.AcceptedCount != z.AcceptedCount || a.PayoutAddress != z.PayoutAddress || a.GymID != z.GymID || a.GymPublicKey != z.GymPublicKey || a.SpendableAtoms != z.SpendableAtoms || a.ImmatureAtoms != z.ImmatureAtoms {
			t.Fatal("Card replay/read changed mining or wallet")
		}
		stableAfter := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
		if !stableAfter.OK || stableAfter.Snapshot == nil || stableAfter.Snapshot.Status == nil {
			t.Fatal("stable after")
		}
		left, right := *stableBefore.Snapshot.Status, *stableAfter.Snapshot.Status
		left.LastAsOfMS, right.LastAsOfMS = 0, 0
		if !reflect.DeepEqual(left, right) {
			t.Fatal("Card changed assignments/roster/operations")
		}
		entryAfter := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_status", ViewID: v, Saturday: baseline.Saturday})
		if !entryAfter.OK || entryAfter.Status == nil || !reflect.DeepEqual(entryBefore.Status, entryAfter.Status) {
			t.Fatal("Card changed entry status")
		}
		seedAfter := b.SeedRequest(ctx, pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: baseline.Saturday})
		if !seedAfter.OK || !reflect.DeepEqual(seedBefore.Record, seedAfter.Record) {
			t.Fatal("Card changed retained seed evidence", seedAfter.Error)
		}
		rows = append(rows, map[string]any{"fixture": fixture, "account_held": account.Account.Held, "snapshot": s.Snapshot, "read": read.Record, "replay": j.Job, "status": fresh.Status, "stable_before": stableBefore.Snapshot, "stable_after": stableAfter.Snapshot, "entry_before": entryBefore.Status, "entry_after": entryAfter.Status, "wallet_before": walletBefore.Snapshot, "wallet_after": walletAfter.Snapshot, "seed_before": seedBefore.Record, "seed_after": seedAfter.Record})
		if _, e = b.Request(ctx, "logout", 0); e != nil {
			t.Fatal(e)
		}
		waitAccount("signed_out")
		if r := request(pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: baseline.Saturday}); r.OK {
			t.Fatal("old native View disclosed")
		}
	}
	if len(receipts) != int(baseline.Build.Record.Plan.Counts.TotalCount) {
		t.Fatal("receipt count", len(receipts))
	}
	result := map[string]any{"status": "passed", "scope": "Final-source native helper replay of preserved genuine scoped-clock journal, no new mining/publication/card matching; immutable conflicted Card plus original-view admitted public receipts.", "baseline_public_sha256": hex.EncodeToString(baselineHash[:]), "baseline_root": root, "rows": rows, "admission_receipts": receipts, "normal_pow": true, "tagged_clock": true, "new_mining": false, "new_publication": false}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	if e = os.WriteFile(filepath.Join(output, "public-result.json"), append(encoded, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	b = nil
}
