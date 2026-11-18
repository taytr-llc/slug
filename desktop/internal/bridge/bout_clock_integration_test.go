//go:build desktopintegration && slugtestclock && !slugtestpow

package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
)

type boutNativePublicCard struct {
	Version     uint32              `json:"version"`
	TaggedClock bool                `json:"tagged_clock"`
	NormalPoW   bool                `json:"normal_pow"`
	ClockMS     int64               `json:"clock_ms"`
	Saturday    string              `json:"saturday"`
	Build       gymcard.BuildResult `json:"build"`
}

func boutNativeLoad(root, name string, out any) error {
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || len(b) > 2<<20 {
		return fmt.Errorf("bounded public evidence %s: %v", name, err)
	}
	return json.Unmarshal(b, out)
}

func boutNativeSave(t *testing.T, root, name string, value any) {
	t.Helper()
	// Evidence includes the actual repeated transport results; compact JSON
	// keeps that bounded archive within the same limit used by its readers.
	b, err := json.Marshal(value)
	if err != nil || len(b)+1 > 2<<20 {
		t.Fatal("bounded native evidence", err, len(b))
	}
	f, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, we := f.Write(append(b, '\n'))
	ce := f.Close()
	if we != nil || ce != nil {
		t.Fatal(we, ce)
	}
}

func TestBoutClockNativeChild(t *testing.T) {
	root := os.Getenv("SLUG_BOUT_CLOCK_CHILD")
	if root == "" {
		return
	}
	fail := func(err error) {
		_ = os.WriteFile(filepath.Join(root, "bout-child-error.txt"), []byte(err.Error()+"\n"), 0600)
		os.Exit(2)
	}
	var p boutNativePublicCard
	if err := boutNativeLoad(root, "public-bout-prepared-card.json", &p); err != nil || p.Version != 1 || !p.TaggedClock || !p.NormalPoW || p.ClockMS <= 0 {
		fail(fmt.Errorf("public test-clock declaration: %v", err))
	}
	var conflict struct {
		ClockMS int64 `json:"clock_ms"`
	}
	if _, err := os.Stat(filepath.Join(root, "public-bout-conflict.json")); err == nil {
		if err := boutNativeLoad(root, "public-bout-conflict.json", &conflict); err != nil || conflict.ClockMS < p.ClockMS {
			fail(fmt.Errorf("public conflict clock: %v", err))
		}
		p.ClockMS = conflict.ClockMS
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c, err := accountpilot.NewBoutsForTestClock(ctx, pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: os.Getenv("SLUG_STABLE_NODE")}, func() time.Time { return time.UnixMilli(p.ClockMS) })
	if err != nil {
		fail(err)
	}
	err = pilot.ServeBouts(ctx, c, os.Stdin, os.Stdout)
	closeErr := c.Close()
	if err != nil && err != context.Canceled {
		fail(err)
	}
	if closeErr != nil {
		fail(closeErr)
	}
	os.Exit(0)
}

func TestBoutClockActualNativeRun(t *testing.T)        { boutNativeExercise(t, false) }
func TestBoutClockHistoricalNativeReplay(t *testing.T) { boutNativeExercise(t, true) }

func boutNativeExercise(t *testing.T, historical bool) {
	t.Helper()
	root, node := os.Getenv("SLUG_BOUT_CLOCK_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit prepared owned-chain candidate card and normal node")
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(root) || real != root {
		t.Fatal("canonical owned root", err)
	}
	if _, err := os.Stat(filepath.Join(root, "SET47-OWNED-COPY.json")); err != nil {
		t.Fatal("owned copy marker", err)
	}
	var prepared, old boutNativePublicCard
	if err := boutNativeLoad(root, "public-bout-prepared-card.json", &prepared); err != nil || prepared.Build.Record.Plan.BoutCount != 1 || !prepared.Build.Status.Usable {
		t.Fatal("usable later candidate card", err)
	}
	if err := boutNativeLoad(root, "public-card-result.json", &old); err != nil || old.Build.Record.Plan.BoutCount != 1 || old.Saturday == prepared.Saturday {
		t.Fatal("retained original card", err)
	}
	var expected gymbout.Record
	if historical {
		var first struct {
			Record gymbout.Record `json:"record"`
		}
		if err := boutNativeLoad(root, "public-bout-native-first.json", &first); err != nil {
			t.Fatal(err)
		}
		expected = first.Record
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoutClockNativeChild$")
	cmd.Env = append(os.Environ(), "SLUG_BOUT_CLOCK_CHILD="+root)
	b, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled, b.boutsEnabled = true, true, true, true, true
	processes := newManifestProcessInventory(filepath.Join(root, "data"), node, cmd.Process.Pid)
	defer func() {
		if b != nil {
			_ = b.Close()
		}
		processes.checkClosed(t)
	}()
	request := func(q pilot.BoutRequest) pilot.BoutResponse {
		t.Helper()
		var r pilot.BoutResponse
		seedNativeClockWait(t, ctx, func() bool { r = b.BoutRequest(ctx, q); return r.Error != "busy" })
		return r
	}
	account := func(state string) pilot.ManifestSnapshot {
		t.Helper()
		var s pilot.ManifestSnapshot
		seedNativeClockWait(t, ctx, func() bool {
			r, e := b.Request(ctx, "snapshot", 0)
			if e != nil || r.Snapshot == nil {
				t.Fatal("account snapshot", e)
			}
			s = *r.Snapshot
			if s.Account.State == "failed" {
				t.Fatal("account failed", s.Account.LastError)
			}
			return s.Account.State == state && (state != "signed_in" || s.View != nil)
		})
		return s
	}
	run := func(view, date, id string) pilot.BoutJob {
		t.Helper()
		r := request(pilot.BoutRequest{Command: "bout_run", ViewID: view, Saturday: date, BoutID: id})
		if !r.OK || r.Job == nil || r.Job.State != "queued" {
			t.Fatal("native Run ACK", r.Error)
		}
		j := *r.Job
		seedNativeClockWait(t, ctx, func() bool {
			r = request(pilot.BoutRequest{Command: "bout_job", ViewID: view, JobID: j.JobID})
			if !r.OK || r.Job == nil {
				t.Fatal("native Run poll", r.Error)
			}
			j = *r.Job
			return j.State != "queued" && j.State != "running"
		})
		return j
	}
	if r := request(pilot.BoutRequest{Command: "bout_snapshot"}); !r.OK || r.Snapshot == nil || r.Snapshot.State != "unavailable" {
		t.Fatal("signed-out Bout", r.Error)
	}
	rows := []map[string]any{}
	id := prepared.Build.Record.Plan.Bouts[0].BoutID
	for n, fixture := range []string{"zero256", "incrementing256"} {
		if _, err := b.RequestFixture(ctx, fixture); err != nil {
			t.Fatal(err)
		}
		s := account("signed_in")
		processes.capture(t, fixture, "booked-bout-native", s.View.AccountID, s.View.AuthorityOrigin, false)
		v := s.View.ViewID
		bootstrap := request(pilot.BoutRequest{Command: "bout_snapshot"})
		if !bootstrap.OK || bootstrap.Snapshot == nil || bootstrap.Snapshot.Pin == nil || bootstrap.Snapshot.View == nil || bootstrap.Snapshot.State != "ready" || bootstrap.Snapshot.View.ViewID != v || bootstrap.Snapshot.Pin.SourceStoreID != prepared.Build.Record.StoreID {
			t.Fatal("native Bout authority bootstrap", bootstrap.Error)
		}
		initialBootstrap := bootstrap.Snapshot
		oldRun := run(v, old.Saturday, old.Build.Record.Plan.Bouts[0].BoutID)
		if oldRun.State != "failed" || oldRun.Error != "bout_source_conflict" || oldRun.Result != nil {
			t.Fatal("old unsaved conflict refused", oldRun.State, oldRun.Error)
		}
		if n == 0 && !historical {
			generation := s.Account.Generation
			if _, err := b.Request(ctx, "reset_fixture", 0); err != nil {
				t.Fatal(err)
			}
			seedNativeClockWait(t, ctx, func() bool {
				s = account("signed_in")
				return s.Account.Held && s.Account.Generation > generation
			})
			if r := request(pilot.BoutRequest{Command: "bout_status", ViewID: v, Saturday: prepared.Saturday, BoutID: id}); r.OK {
				t.Fatal("reset-revoked view disclosed")
			}
			v = s.View.ViewID
			refreshed := request(pilot.BoutRequest{Command: "bout_snapshot"})
			if !refreshed.OK || refreshed.Snapshot == nil || refreshed.Snapshot.View == nil || refreshed.Snapshot.View.ViewID != v || refreshed.Snapshot.StoreID != bootstrap.Snapshot.StoreID || refreshed.Snapshot.CreatedAtMS != bootstrap.Snapshot.CreatedAtMS || refreshed.Snapshot.SourceCreatedAtMS != bootstrap.Snapshot.SourceCreatedAtMS {
				t.Fatal("held original authority bootstrap", refreshed.Error)
			}
			bootstrap = refreshed
		}
		before, err := b.Request(ctx, "snapshot", 0)
		if err != nil || before.Snapshot == nil || before.Snapshot.State != "stopped" {
			t.Fatal("stopped pre-run wallet", err)
		}
		j := run(v, prepared.Saturday, id)
		if j.State != "completed" || j.Result == nil || j.Result.Status.Saved.Usable == historical {
			t.Fatal("saved bout", j.State, j.Error)
		}
		if n == 0 && !historical {
			if j.Result.Replayed {
				t.Fatal("first native Run was already saved")
			}
			expected = j.Result.Record
		} else if !j.Result.Replayed || !reflect.DeepEqual(j.Result.Record, expected) {
			t.Fatal("immutable authorized replay")
		}
		again := run(v, prepared.Saturday, id)
		if again.State != "completed" || again.Result == nil || !again.Result.Replayed || !reflect.DeepEqual(again.Result.Record, expected) {
			t.Fatal("repeated Run changed transcript", again.Error)
		}
		terminal, _ := json.Marshal(again)
		read := request(pilot.BoutRequest{Command: "bout_record", ViewID: v, Saturday: prepared.Saturday, BoutID: id})
		if !read.OK || read.Record == nil || !reflect.DeepEqual(read.Record.Record, expected) || read.Record.Status.Saved.Usable == historical {
			t.Fatal("native selected record", read.Error)
		}
		status := request(pilot.BoutRequest{Command: "bout_card_status", ViewID: v, Saturday: prepared.Saturday})
		if !status.OK || status.CardStatus == nil || len(status.CardStatus.Bouts) != 1 || !status.CardStatus.Bouts[0].Exists || status.CardStatus.Bouts[0].Usable == historical {
			t.Fatal("native common card metadata", status.Error)
		}
		portraits := []any{}
		for _, fighter := range []string{expected.Input.A.ProspectID, expected.Input.B.ProspectID} {
			image := b.BoutPortrait(ctx, v, prepared.Saturday, id, fighter)
			if !image.OK || image.Portrait == nil || image.Status == nil || image.Portrait.SVG == "" || image.Status.Saved.Usable == historical {
				t.Fatal("native saved portrait", image.Error)
			}
			cached := b.BoutPortrait(ctx, v, prepared.Saturday, id, fighter)
			if !cached.OK || cached.Portrait == nil || cached.Status == nil || cached.Portrait.SVG != image.Portrait.SVG || cached.Status.Saved.RecordDigest != image.Status.Saved.RecordDigest || cached.Status.Saved.Usable == historical {
				t.Fatal("cached portrait fresh saved status", cached.Error)
			}
			portraits = append(portraits, image, cached)
		}
		last := request(pilot.BoutRequest{Command: "bout_job", ViewID: v, JobID: again.JobID})
		lastBytes, _ := json.Marshal(last.Job)
		if !last.OK || !bytes.Equal(terminal, lastBytes) {
			t.Fatal("terminal changed after explicit fresh status")
		}
		after, err := b.Request(ctx, "snapshot", 0)
		if err != nil || after.Snapshot == nil {
			t.Fatal(err)
		}
		a, z := before.Snapshot, after.Snapshot
		if z.State != "stopped" || a.Attempts != z.Attempts || a.AcceptedCount != z.AcceptedCount || a.PayoutAddress != z.PayoutAddress || a.GymID != z.GymID || a.SpendableAtoms != z.SpendableAtoms || a.ImmatureAtoms != z.ImmatureAtoms {
			t.Fatal("bout operation changed mining or wallet")
		}
		rows = append(rows, map[string]any{"fixture": fixture, "held": s.Account.Held, "initial_bootstrap": initialBootstrap, "snapshot": bootstrap.Snapshot, "old_conflict_refusal": oldRun, "first_job": j, "replay": again, "read": read.Record, "card_status": status.CardStatus, "portraits": portraits, "wallet_before": before.Snapshot, "wallet_after": after.Snapshot})
		if _, err := b.Request(ctx, "logout", 0); err != nil {
			t.Fatal(err)
		}
		account("signed_out")
		if r := request(pilot.BoutRequest{Command: "bout_record", ViewID: v, Saturday: prepared.Saturday, BoutID: id}); r.OK {
			t.Fatal("signed-out old View disclosed")
		}
		if r := b.BoutPortrait(ctx, v, prepared.Saturday, id, expected.Input.A.ProspectID); r.OK || r.Portrait != nil || r.Status != nil {
			t.Fatal("signed-out portrait disclosed")
		}
	}
	name := "public-bout-native-first.json"
	if historical {
		name = "public-bout-native-historical.json"
	}
	boutNativeSave(t, root, name, map[string]any{"status": "passed", "normal_pow": true, "tagged_clock": true, "historical_conflict": historical, "record": expected, "rows": rows, "scope": "Actual native private-pipe saved candidate bout; no professional result or award authority. Portrait bytes checked through native APIs, not visually rendered."})
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	b = nil
}
