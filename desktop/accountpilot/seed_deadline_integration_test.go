//go:build desktopintegration && slugtestclock && !slugtestpow

package accountpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymseed"
)

// Continue only an explicitly supplied, closed, disposable composition root.
// This is a fresh original login at the next calendar lock, with no Start,
// mining, publication, attestation, node RPC, or caller-supplied authority.
func TestSeedClockDeadlineContinuation(t *testing.T) {
	input := os.Getenv("SLUG_SEED_DEADLINE_SOURCE")
	node := os.Getenv("SLUG_STABLE_NODE")
	if input == "" || node == "" {
		t.Skip("explicit prior owned composition result and normal node")
	}
	absolute, e := filepath.EvalSymlinks(input)
	if e != nil || absolute != input {
		t.Fatal("exact real input path", e)
	}
	allowed := "/Users/taytrrip/slug/artifacts/set45"
	relative, e := filepath.Rel(allowed, input)
	if e != nil || relative == ".." || len(relative) > 3 && relative[:3] == "../" {
		t.Fatal("only disposable Set45 evidence")
	}
	root := filepath.Dir(input)
	raw, e := os.ReadFile(input)
	if e != nil || len(raw) > pilot.MaxSeedResponseSize {
		t.Fatal("bounded prior public record", e)
	}
	var prior struct {
		Date                   string
		TaggedClock, NormalPoW bool
		Snapshot               pilot.SeedSnapshot
		Record                 gymseed.Record
	}
	if json.Unmarshal(raw, &prior) != nil || !prior.TaggedClock || !prior.NormalPoW || prior.Record.Finality == nil || prior.Record.Target == nil || prior.Record.Publication == nil || len(prior.Record.Conflicts) == 0 || prior.Snapshot.Pin == nil {
		t.Fatal("incomplete original actual chain evidence")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	at := prior.Record.SourceReceipt.Input.Event.IncidentDeadlineMS
	if prior.Record.IncidentAtMS != 0 {
		t.Fatal("prior run already observed deadline")
	}
	cfg := pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}
	processLogs := func() map[string][32]byte {
		t.Helper()
		paths, e := filepath.Glob(filepath.Join(cfg.DataDir, "gyms", "*", "*-process.log"))
		if e != nil || len(paths) != 4 {
			t.Fatal("exact prior owned node log inventory", e, len(paths))
		}
		out := make(map[string][32]byte, len(paths))
		for _, path := range paths {
			info, e := os.Stat(path)
			if e != nil || info.Size() > 16<<20 {
				t.Fatal("bounded owned process log", e)
			}
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			out[path] = sha256.Sum256(raw)
		}
		return out
	}
	priorLogs := processLogs()
	c, e := NewSeedsForTestClock(cfg, func() time.Time { return time.UnixMilli(at) })
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if c != nil {
			_ = c.Close()
		}
	}()
	login := func() {
		t.Helper()
		if e := c.LoginFixture("zero256"); e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.Account.State == "failed" {
				t.Fatal("fresh deadline login", s.Account.LastError)
			}
			return s.Account.State == "signed_in" && s.View != nil
		})
	}
	login()
	w := newSeedClockWire(t, ctx, c)
	v := c.SeedSnapshot().View.ViewID
	request := func(r pilot.SeedRequest) pilot.SeedResponse {
		t.Helper()
		var out pilot.SeedResponse
		seedClockWait(t, ctx, func() bool { out = w.call(t, r); return out.Error != "busy" })
		if !out.OK {
			t.Fatal(r.Command, out.Error)
		}
		return out
	}
	before := request(pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: prior.Date}).Record
	// Record itself is an authorized deadline observer, so the incident may be
	// committed before the following Status call. Both use the same exact time.
	status := request(pilot.SeedRequest{Command: "seed_status", ViewID: v, Saturday: prior.Date}).Status
	if status == nil || status.Phase != "conflict" || !status.EscalationDue || status.IncidentAtMS != at || status.FinalizedAtMS != prior.Record.Finality.ObservedAtMS {
		t.Fatal("deadline escalation", status)
	}
	after := request(pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: prior.Date}).Record
	if before == nil || after == nil || after.IncidentAtMS != at {
		t.Fatal("durable incident")
	}
	expected := prior.Record
	expected.IncidentAtMS = at
	// Later explicit held reconciliation may have refreshed only the coherent
	// observation after the initial public result. Preserve the actual current
	// observation as obtained above; the deadline may not change it again.
	expected.Observation = before.Observation
	expected.Cursor = before.Cursor
	expected.ScanRestarts = before.ScanRestarts
	if !reflect.DeepEqual(expected, *after) || !reflect.DeepEqual(before, after) {
		t.Fatal("deadline changed retained event/publication/finality/conflict")
	}
	snap := c.Snapshot()
	if snap.State != "stopped" || c.SeedSnapshot().NodeState != "offline" || !reflect.DeepEqual(priorLogs, processLogs()) {
		t.Fatal("deadline implicitly mined or attached publisher")
	}
	w.close(t)
	c = nil
	c, e = NewSeedsForTestClock(cfg, func() time.Time { return time.UnixMilli(at + 1) })
	if e != nil {
		t.Fatal(e)
	}
	login()
	w = newSeedClockWire(t, ctx, c)
	nv := c.SeedSnapshot().View.ViewID
	reopened := request(pilot.SeedRequest{Command: "seed_record", ViewID: nv, Saturday: prior.Date}).Record
	if !reflect.DeepEqual(after, reopened) || !reflect.DeepEqual(priorLogs, processLogs()) {
		t.Fatal("reopen changed incident")
	}
	inputHash := sha256.Sum256(raw)
	out := struct {
		Scope       string
		InputSHA256 string
		DeadlineMS  int64
		Status      gymseed.Status
		Record      gymseed.Record
		Reopened    gymseed.Record
	}{"fresh authenticated scoped-clock next-lock observation and reopen; no mining/publication", hex.EncodeToString(inputHash[:]), at, *status, *after, *reopened}
	encoded, _ := json.MarshalIndent(out, "", "  ")
	path := filepath.Join(root, "public-seed-deadline.json")
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(append(encoded, '\n')); e != nil {
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
	unchanged, e := os.ReadFile(input)
	if e != nil || !bytes.Equal(raw, unchanged) {
		t.Fatal("original evidence modified")
	}
	t.Log("preserved declaration/transaction/target/finality; incident retained at", at, path)
}
