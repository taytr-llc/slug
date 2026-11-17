//go:build desktopintegration && slugtestclock && !slugtestpow

package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"
)

type eventClockSetup struct {
	Saturday string
	CutoffMS int64
	Snapshot pilot.EventSnapshot
	Records  map[string]string
}

func eventClockWait(t *testing.T, ctx context.Context, fn func() bool) {
	t.Helper()
	for !fn() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// This .test child is the only clock-controlled process. Neither the shipped
// CLI nor the wire grammar accepts a clock. All work uses normal256MiB PoW.
func eventClockHelper(t *testing.T, root, node string) {
	fail := func(args ...any) {
		_ = os.WriteFile(filepath.Join(root, "serve-error.txt"), []byte(fmt.Sprint(args...)+"\n"+string(debug.Stack())), 0600)
		t.Fatal(args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var n atomic.Int64
	n.Store(time.Now().UnixMilli())
	clock := func() time.Time { return time.UnixMilli(n.Load()) }
	cfg := pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}
	c, e := accountpilot.NewEventsForTestClock(cfg, clock)
	if e != nil {
		fail(e)
	}
	defer func() {
		if c != nil {
			_ = c.Close()
		}
	}()
	login := func(f string) {
		t.Helper()
		if e := c.LoginFixture(f); e != nil {
			fail(e)
		}
		eventClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.Account.State == "failed" {
				fail("login", s.Account.LastError)
			}
			return s.Account.State == "signed_in" && s.View != nil
		})
	}
	logout := func() {
		t.Helper()
		if e := c.Logout(); e != nil {
			fail(e)
		}
		eventClockWait(t, ctx, func() bool { s := c.Snapshot(); return s.Account.State == "signed_out" })
	}
	login("zero256")
	v := c.StableSnapshot().View.ViewID
	up, e := c.EntryUpcoming(ctx, v)
	if e != nil {
		fail(e)
	}
	date, C := up.Event.Saturday, up.Event.CutoffMS
	if e = c.Close(); e != nil {
		fail(e)
	}
	c = nil
	n.Store(C - 60000)
	c, e = accountpilot.NewEventsForTestClock(cfg, clock)
	if e != nil {
		fail(e)
	}
	stableDone := func(j pilot.StableJob, e error) pilot.StableJob {
		t.Helper()
		if e != nil {
			fail(e)
		}
		eventClockWait(t, ctx, func() bool {
			var e error
			j, e = c.StableJob(ctx, j.View.ViewID, j.JobID)
			if e != nil {
				fail(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "done" {
			fail("stable", j.Error)
		}
		return j
	}
	entryDone := func(j pilot.EntryJob, e error) pilot.EntryJob {
		t.Helper()
		if e != nil {
			fail(e)
		}
		eventClockWait(t, ctx, func() bool {
			var e error
			j, e = c.EntryJob(ctx, j.View.ViewID, j.JobID)
			if e != nil {
				fail(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "complete" {
			fail("entry", j.Error)
		}
		return j
	}
	ids := map[string]string{}
	observed := map[string]pilot.ManifestSnapshot{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		login(fixture)
		observed[fixture] = c.Snapshot()
		progress, _ := json.Marshal(observed)
		if e = os.WriteFile(filepath.Join(root, "observed-fixtures.json"), progress, 0600); e != nil {
			fail(e)
		}
		if e = c.Start(); e != nil {
			fail(e)
		}
		eventClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.State == "failed" {
				fail("mining", s.LastError)
			}
			return s.AcceptedCount >= 1
		})
		c.Stop()
		v = c.StableSnapshot().View.ViewID
		stableDone(c.BeginStableSync(ctx, v))
		inv := stableDone(c.BeginStableInventory(ctx, v, 0, 1))
		if inv.Page == nil || len(inv.Page.Items) != 1 {
			fail("missing genuine record")
		}
		id := inv.Page.Items[0].Assignment.ProspectID
		ids[fixture] = id
		st := c.StableSnapshot()
		if st.Status == nil {
			fail("status")
		}
		p := stableDone(c.BeginStablePrepare(ctx, v, gymstable.SlotRequest{ExpectedRevision: st.Status.RosterRevision, Slot: 1, ProspectID: id}))
		a := stableDone(c.BeginStableApply(ctx, v, p.Intent.OperationID))
		u, e := c.EntryUpcoming(ctx, v)
		if e != nil || u.Event.Saturday != date {
			fail("cutoff changed", e)
		}
		ep := entryDone(c.BeginEntryPrepare(ctx, v, gymentry.Request{Version: 1, EventID: u.Event.EventID, Saturday: date, Action: "replace", ExpectedRosterRevision: a.Mutation.Receipt.Revision, ProspectIDs: []string{id}}))
		entryDone(c.BeginEntryApply(ctx, v, ep.OperationID))
		logout()
	}
	n.Store(C + 1000)
	login("zero256")
	setup := eventClockSetup{date, C, c.EventSnapshot(), ids}
	if setup.Snapshot.Pin == nil || setup.Snapshot.CoordinatorCreatedAtMS >= C {
		fail("coordinator not admitted before cutoff")
	}
	b, _ := json.Marshal(setup)
	if e = os.WriteFile(filepath.Join(root, "setup.json"), b, 0600); e != nil {
		fail(e)
	}
	if e = pilot.ServeEvents(ctx, c, os.Stdin, os.Stdout); e != nil {
		_ = os.WriteFile(filepath.Join(root, "serve-error.txt"), []byte(e.Error()), 0600)
		fail(e)
	}
	if e = c.Close(); e != nil {
		fail(e)
	}
	c = nil
	os.Exit(0)
}
func TestEventClockNativeHelperComposition(t *testing.T) {
	if root := os.Getenv("SLUG_EVENT_CLOCK_TEST_CHILD"); root != "" {
		eventClockHelper(t, root, os.Getenv("SLUG_STABLE_NODE"))
		return
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node")
	}
	base, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "event-testclock-")
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("normal-PoW tagged clock composition", root)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestEventClockNativeHelperComposition$", "-test.timeout=11m")
	cmd.Env = append(os.Environ(), "SLUG_EVENT_CLOCK_TEST_CHILD="+root)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	b.entriesEnabled, b.eventsEnabled = true, true
	processes := newManifestProcessInventory(filepath.Join(root, "data"), node, cmd.Process.Pid)
	defer func() {
		if b != nil {
			_ = b.Close()
		}
		processes.checkClosed(t)
	}()
	observed := map[string]bool{}
	observe := func() {
		var rows map[string]pilot.ManifestSnapshot
		raw, e := os.ReadFile(filepath.Join(root, "observed-fixtures.json"))
		if e != nil || json.Unmarshal(raw, &rows) != nil {
			return
		}
		for f, s := range rows {
			if !observed[f] && s.View != nil {
				processes.capture(t, f, "test-clock-setup", s.View.AccountID, s.View.AuthorityOrigin, false)
				observed[f] = true
			}
		}
	}
	eventClockWait(t, ctx, func() bool {
		observe()
		select {
		case <-b.done:
			raw, _ := os.ReadFile(filepath.Join(root, "serve-error.txt"))
			t.Fatal("helper exited", string(raw))
		default:
		}
		_, e := os.Stat(filepath.Join(root, "setup.json"))
		return e == nil
	})
	var setup eventClockSetup
	raw, e := os.ReadFile(filepath.Join(root, "setup.json"))
	if e != nil || json.Unmarshal(raw, &setup) != nil {
		t.Fatal(e)
	}
	request := func(q pilot.EventRequest) pilot.EventResponse {
		t.Helper()
		var r pilot.EventResponse
		eventClockWait(t, ctx, func() bool { r = b.EventRequest(ctx, q); return r.Error != "busy" })
		if !r.OK {
			t.Fatal(q.Command, r.Error)
		}
		return r
	}
	snap := func() pilot.EventSnapshot {
		t.Helper()
		r := request(pilot.EventRequest{Command: "event_snapshot"})
		if r.Snapshot == nil || r.Snapshot.View == nil {
			t.Fatal("snapshot")
		}
		return *r.Snapshot
	}
	capture := func(s pilot.EventSnapshot) pilot.EventJob {
		t.Helper()
		j := request(pilot.EventRequest{Command: "event_capture", ViewID: s.View.ViewID, Saturday: setup.Saturday}).Job
		if j == nil {
			t.Fatal("ACK")
		}
		eventClockWait(t, ctx, func() bool {
			j = request(pilot.EventRequest{Command: "event_job", ViewID: s.View.ViewID, JobID: j.JobID}).Job
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "completed" || j.Result == nil {
			t.Fatal("capture", j.State, j.Error)
		}
		return *j
	}
	manifestWait := func(state string) {
		t.Helper()
		eventClockWait(t, ctx, func() bool {
			r, e := b.Request(ctx, "snapshot", 0)
			if e != nil {
				t.Fatal(e)
			}
			return r.Snapshot != nil && r.Snapshot.Account.State == state
		})
	}
	first := snap()
	one := capture(first)
	if len(one.Result.Capture.Snapshot.Members) != 1 || one.Result.Capture.Snapshot.Members[0].Assignment.ProspectID != setup.Records["zero256"] || one.Result.Capture.Snapshot.Members[0].ConditionAtCutoff.AsOfMS != setup.CutoffMS {
		t.Fatal("first cutoff member")
	}
	stableBytes, _ := json.Marshal(one)
	for i := 0; i < 3; i++ {
		again := request(pilot.EventRequest{Command: "event_job", ViewID: first.View.ViewID, JobID: one.JobID})
		rb, _ := json.Marshal(again.Job)
		if string(rb) != string(stableBytes) {
			t.Fatal("terminal bytes changed")
		}
	}
	if _, e = b.Request(ctx, "logout", 0); e != nil {
		t.Fatal(e)
	}
	manifestWait("signed_out")
	if r := b.EventRequest(ctx, pilot.EventRequest{Command: "event_job", ViewID: first.View.ViewID, JobID: one.JobID}); r.OK {
		t.Fatal("logged-out cache")
	}
	if _, e = b.RequestFixture(ctx, "incrementing256"); e != nil {
		t.Fatal(e)
	}
	manifestWait("signed_in")
	second := snap()
	two := capture(second)
	if two.Result.Status.Phase != "snapshotted" {
		t.Fatal("aggregate incomplete")
	}
	record := request(pilot.EventRequest{Command: "event_record", ViewID: second.View.ViewID, Saturday: setup.Saturday}).Record
	if record == nil || len(record.Input.Participants) != 2 {
		t.Fatal("record")
	}
	if _, e = gymevent.ValidateSnapshotReceipt(*second.Pin, *record); e != nil {
		t.Fatal("checked snapshot", e)
	}
	for _, p := range record.Input.Participants {
		if len(p.Members) != 1 || p.Members[0].Eligibility != "eligible" || p.Members[0].ConditionAtCutoff.AsOfMS != setup.CutoffMS {
			t.Fatal("frozen eligibility")
		}
	}
	replay := capture(second)
	if !replay.Result.Replayed || !reflect.DeepEqual(replay.Result.Capture, two.Result.Capture) {
		t.Fatal("replay")
	}
	// A verified fixture reset changes current generation and holds recovery. The
	// captured facts remain accessible through the new original View only.
	if _, e = b.Request(ctx, "reset_fixture", 0); e != nil {
		t.Fatal(e)
	}
	manifestWait("signed_in")
	held := snap()
	if held.View.Generation <= second.View.Generation {
		t.Fatal("reset generation")
	}
	heldRecord := request(pilot.EventRequest{Command: "event_record", ViewID: held.View.ViewID, Saturday: setup.Saturday}).Record
	if !reflect.DeepEqual(heldRecord, record) {
		t.Fatal("held snapshot changed")
	}
	if r := b.EventRequest(ctx, pilot.EventRequest{Command: "event_record", ViewID: second.View.ViewID, Saturday: setup.Saturday}); r.OK {
		t.Fatal("old view disclosed")
	}
	history := request(pilot.EventRequest{Command: "event_history", ViewID: held.View.ViewID, Limit: 8}).History
	if history == nil || len(history.Events) != 1 || history.NextEventID != "" {
		t.Fatal("history")
	}
	output := map[string]any{"scope": "normal-PoW, test-clock NewEvents/ServeEvents/native pipe composition", "setup": setup, "first": one, "second": two, "record": record, "replay": replay, "held": held, "history": history, "closed_capture_executed": true, "normal_release_binary": false}
	raw, _ = json.MarshalIndent(output, "", "  ")
	if e = os.WriteFile(filepath.Join(root, "public-result.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if e = b.Close(); e != nil {
		t.Fatal(fmt.Errorf("helper close: %w", e))
	}
	b = nil
}
