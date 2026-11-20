//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

// The tagged clock affects calendar/store observations only. Mining, jobs,
// accounts, sessions, reset holds and settlement retain their real clocks.
// These are relational actual-runtime checks, not new expected hash vectors.
func TestPoolLeagueActualMiningEntryAndNoon(t *testing.T) {
	node, artifacts := os.Getenv("SLUG_POOL_NODE"), os.Getenv("SLUG_POOL_DESKTOP_ARTIFACTS")
	if node == "" || artifacts == "" {
		t.Skip("explicit normal node and owned fixture artifacts required")
	}
	if !filepath.IsAbs(node) || !filepath.IsAbs(artifacts) {
		t.Fatal("absolute fixture paths required")
	}
	root, err := os.MkdirTemp(artifacts, "pool-league-")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	var calendarMS atomic.Int64
	clock := func() time.Time {
		if ms := calendarMS.Load(); ms != 0 {
			return time.UnixMilli(ms)
		}
		return time.Now()
	}
	config := pilot.Config{DataDir: root, NodeBinary: node}
	c, err := NewPoolForTestClock(ctx, config, clock)
	if err != nil {
		t.Fatal("shared league construction", err)
	}
	t.Cleanup(func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error("shared league shutdown", err)
			}
		}
	})
	rpc := poolActualNode(t, root)
	var initialHeight uint32
	if err = rpc.call(ctx, "getblockcount", nil, &initialHeight); err != nil || initialHeight != 0 {
		t.Fatal("league setup mined", initialHeight, err)
	}
	markerPath := filepath.Join(root, "shared", "SLUG-POOL-RUNTIME.json")
	markerBefore, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("shared league attached at genesis; explicit A/B/A mining follows", root)
	chosen := make(map[string]gymstable.Item)
	pins := make(map[string]gymstable.Pin)
	var sourceID string
	var firstA, firstB, lastA uint64
	for round, fixture := range []string{"zero256", "incrementing256", "zero256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		if sourceID == "" {
			sourceID = s.SourceBindingID
		}
		if s.SourceBindingID != sourceID {
			t.Fatal("login changed shared source")
		}
		before := s.AssignmentCount
		if err = c.Start(); err != nil {
			t.Fatal("explicit mining start", err)
		}
		poolActualWait(t, ctx, "new own shared admission", func(context.Context) bool {
			v := c.Snapshot()
			if v.State == "failed" {
				t.Fatal("shared miner failed", v.LastError)
			}
			return v.AssignmentCount > before
		})
		c.Stop()
		page := poolActualRecent(t, ctx, c)
		if len(page.Items) == 0 || page.Status.Pin.Version != gymstable.SharedPinVersion2 {
			t.Fatal("own mined inventory missing")
		}
		for _, item := range page.Items {
			if item.Assignment.GymID != s.GymID {
				t.Fatal("foreign assignment in own inventory")
			}
		}
		item := page.Items[len(page.Items)-1]
		chosen[fixture], pins[fixture] = item, page.Status.Pin
		switch round {
		case 0:
			firstA = item.Assignment.Sequence
		case 1:
			firstB = item.Assignment.Sequence
		case 2:
			lastA = item.Assignment.Sequence
		}
		t.Logf("explicit %s round: selected source sequence %d; local assignments %d", fixture, item.Assignment.Sequence, c.Snapshot().AssignmentCount)
		poolActualLogout(t, ctx, c)
	}
	if !(firstA < firstB && firstB < lastA) {
		t.Fatal("actual shared receipts did not interleave", firstA, firstB, lastA)
	}
	if pins["zero256"].Source != pins["incrementing256"].Source {
		t.Fatal("gyms did not share one source")
	}
	// Independent calendar arithmetic: the first Saturday noon Pacific at least
	// ten minutes ahead avoids a real-clock test crossing its own intake cutoff.
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(pacific)
	var cutoff time.Time
	for d := 0; d <= 7; d++ {
		candidate := time.Date(now.Year(), now.Month(), now.Day()+d, 12, 0, 0, 0, pacific)
		if candidate.Weekday() == time.Saturday && candidate.After(now.Add(10*time.Minute)) {
			cutoff = candidate
			break
		}
	}
	if cutoff.IsZero() {
		t.Fatal("independent Saturday cutoff unavailable")
	}
	date := cutoff.Format("2006-01-02")
	entries := make(map[string]gymentry.Receipt)
	entryBytes := make(map[string][]byte)
	var eventPin gymevent.Pin
	var eventStore string
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		_ = poolActualRecent(t, ctx, c)
		poolLeagueRoster(t, ctx, c, chosen[fixture].Assignment.ProspectID)
		var status gymentry.EventStatus
		poolLeagueRead(t, ctx, "actual entry status", func() error {
			var err error
			status, err = c.EntryStatus(ctx, s.View.ViewID, date)
			return err
		})
		if status.Event.CutoffMS != cutoff.UnixMilli() || status.Phase != "accepting" {
			t.Fatal("actual entry calendar", status.Phase)
		}
		stable := c.StableSnapshot()
		request := gymentry.Request{Version: 1, EventID: status.Event.EventID, Saturday: date, Action: "replace", ExpectedRevision: status.Revision, ExpectedRosterRevision: stable.Status.RosterRevision, ProspectIDs: []string{chosen[fixture].Assignment.ProspectID}}
		prepared, err := c.BeginEntryPrepare(ctx, s.View.ViewID, request)
		if err != nil {
			t.Fatal("entry prepare", err)
		}
		prepared = poolLeagueEntryJob(t, ctx, c, s.View.ViewID, prepared.JobID)
		if prepared.Intent == nil || prepared.Intent.ViewID != s.View.ViewID || !reflect.DeepEqual(prepared.Intent.Request, request) {
			t.Fatal("prepared a different entry")
		}
		applied, err := c.BeginEntryApply(ctx, s.View.ViewID, prepared.OperationID)
		if err != nil {
			t.Fatal("entry apply", err)
		}
		applied = poolLeagueEntryJob(t, ctx, c, s.View.ViewID, applied.JobID)
		if applied.Mutation == nil || len(applied.Mutation.Receipt.Members) != 1 || applied.Mutation.Receipt.Members[0].Assignment != chosen[fixture].Assignment {
			t.Fatal("entry lost original shared assignment")
		}
		entries[fixture] = applied.Mutation.Receipt
		entryBytes[fixture] = poolLeagueJSON(t, applied.Mutation.Receipt)
		observed := c.EventSnapshot()
		if observed.Pin == nil || observed.View == nil || observed.View.ViewID != s.View.ViewID || observed.State != "ready" || observed.CoordinatorCreatedAtMS >= cutoff.UnixMilli() {
			t.Fatal("shared event authority unavailable before noon")
		}
		if eventStore == "" {
			eventStore, eventPin = observed.StoreID, *observed.Pin
		} else if observed.StoreID != eventStore || !reflect.DeepEqual(*observed.Pin, eventPin) {
			t.Fatal("event authority changed between gyms")
		}
		if fixture == "incrementing256" {
			old := s
			if err = c.ResetFixtureAccount(); err != nil {
				t.Fatal("reset before capture", err)
			}
			poolActualWait(t, ctx, "held replacement session", func(context.Context) bool {
				v := c.Snapshot()
				if v.Account.State == "failed" {
					t.Fatal("reset failed")
				}
				return v.Account.State == "signed_in" && v.Account.Generation > old.Account.Generation
			})
			held := c.Snapshot()
			if !held.Account.Held || held.Account.CanExport || held.GymID != old.GymID || held.SourceBindingID != sourceID {
				t.Fatal("reset lost shared gym or hold")
			}
			if _, err = c.EntryStatus(ctx, old.View.ViewID, date); err == nil {
				t.Fatal("old reset view read entries")
			}
			if _, err = c.EventStatus(ctx, old.View.ViewID, date); err == nil {
				t.Fatal("old reset view read event")
			}
			request.ExpectedRevision = entries[fixture].Revision
			if _, err = c.BeginEntryPrepare(ctx, old.View.ViewID, request); !errors.Is(err, gymstable.ErrStaleView) {
				t.Fatal("reset did not reject the old view before entry mutation", err)
			}
		}
		poolActualLogout(t, ctx, c)
	}
	for _, participant := range eventPin.Participants {
		if participant.StablePin.Version != gymstable.SharedPinVersion2 || participant.StablePin.Source != pins["zero256"].Source {
			t.Fatal("event participant not pinned to shared source")
		}
	}
	calendarMS.Store(cutoff.UnixMilli() + 1000)
	captures := make(map[string]gymevent.ParticipantCapture)
	for n, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		if fixture == "zero256" {
			poolLeagueRoster(t, ctx, c, "")
		} // After noon, before capture.
		job, err := c.BeginEventCapture(ctx, s.View.ViewID, date)
		if err != nil {
			t.Fatal("noon capture dispatch", err)
		}
		job = poolLeagueEventJob(t, ctx, c, s.View.ViewID, job.JobID)
		if job.Result == nil || job.Result.Replayed || len(job.Result.Capture.Snapshot.Members) != 1 {
			t.Fatal("missing actual noon member")
		}
		capture := job.Result.Capture
		member := capture.Snapshot.Members[0]
		want, pin := chosen[fixture].Assignment, pins[fixture]
		condition := member.ConditionAtCutoff
		if member.Assignment != want || member.Slot != 1 || member.Eligibility != "eligible" || condition.AsOfMS != cutoff.UnixMilli() || condition.BornAtMS != want.BornAtMS || condition.Seed != want.WorkHash || condition.ProXP != 0 || condition.AgeWeeks != (cutoff.UnixMilli()-want.BornAtMS)/(7*24*60*60*1000) {
			t.Fatal("noon snapshot changed original assignment, slot or condition")
		}
		if capture.Snapshot.Owner.AccountID != pin.AccountID || capture.Snapshot.Owner.AccountAuthorityID != pin.AccountAuthorityID || capture.Snapshot.Owner.GymID != pin.Identity.GymID || capture.Snapshot.EntryStoreID != entries[fixture].StoreID {
			t.Fatal("noon owner/entry pin differs")
		}
		if len(job.Result.Status.CapturedParticipants) != n+1 || (n == 0 && job.Result.Status.Phase != "capturing") || (n == 1 && job.Result.Status.Phase != "snapshotted") {
			t.Fatal("incorrect partial/completed event progress")
		}
		captures[fixture] = capture
		poolActualLogout(t, ctx, c)
	}
	calendarMS.Store(cutoff.UnixMilli() + 2000)
	var saved gymevent.SnapshotReceipt
	var savedBytes []byte
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		job, err := c.BeginEventCapture(ctx, s.View.ViewID, date)
		if err != nil {
			t.Fatal("capture retry", err)
		}
		job = poolLeagueEventJob(t, ctx, c, s.View.ViewID, job.JobID)
		if job.Result == nil || !job.Result.Replayed || !reflect.DeepEqual(job.Result.Capture, captures[fixture]) {
			t.Fatal("capture retry rewrote its noon evidence")
		}
		var record gymevent.SnapshotReceipt
		poolLeagueRead(t, ctx, "completed snapshot readable", func() error {
			var err error
			record, err = c.EventRecord(ctx, s.View.ViewID, date)
			return err
		})
		if savedBytes == nil {
			saved, savedBytes = record, poolLeagueJSON(t, record)
		} else if !bytes.Equal(savedBytes, poolLeagueJSON(t, record)) {
			t.Fatal("gyms saw different snapshots")
		}
		if len(record.Input.Participants) != 2 || record.Input.Event.StoreID != eventStore || record.Input.Event.CutoffMS != cutoff.UnixMilli() || record.CompletedAtMS != cutoff.UnixMilli()+1000 {
			t.Fatal("snapshot completion or identity changed")
		}
		if len(record.Captures) != len(record.Input.Participants) {
			t.Fatal("snapshot capture evidence count changed")
		}
		for i, participant := range record.Input.Participants {
			var expected *gymevent.ParticipantCapture
			for _, capture := range captures {
				if capture.Snapshot.ParticipantID == participant.ParticipantID {
					copy := capture
					expected = &copy
					break
				}
			}
			if expected == nil || !reflect.DeepEqual(participant, expected.Snapshot) || !reflect.DeepEqual(record.Captures[i], expected.Evidence) {
				t.Fatal("completed record differs from independently checked participant capture")
			}
		}
		poolActualLogout(t, ctx, c)
	}
	var retainedHeight uint32
	if err = rpc.call(ctx, "getblockcount", nil, &retainedHeight); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal("close first league runtime", err)
	}
	c = nil
	calendarMS.Store(cutoff.UnixMilli() + 3000)
	c, err = NewPoolForTestClock(ctx, config, clock)
	if err != nil {
		t.Fatal("reopen retained league runtime", err)
	}
	rpc = poolActualNode(t, root)
	var reopenedHeight uint32
	if err = rpc.call(ctx, "getblockcount", nil, &reopenedHeight); err != nil || reopenedHeight != retainedHeight {
		t.Fatal("reopen changed chain or mined", reopenedHeight, retainedHeight, err)
	}
	markerAfter, err := os.ReadFile(markerPath)
	if err != nil || !bytes.Equal(markerBefore, markerAfter) {
		t.Fatal("league changed parent runtime identity")
	}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		if s.SourceBindingID != sourceID || s.State != "stopped" || s.Attempts != 0 {
			t.Fatal("reopen replaced source or started mining")
		}
		var record gymevent.SnapshotReceipt
		poolLeagueRead(t, ctx, "retained snapshot readable after restart", func() error {
			var err error
			record, err = c.EventRecord(ctx, s.View.ViewID, date)
			return err
		})
		if !bytes.Equal(savedBytes, poolLeagueJSON(t, record)) {
			t.Fatal("reopen changed original noon record")
		}
		var op gymentry.OperationResult
		poolLeagueRead(t, ctx, "retained entry readable after restart", func() error {
			var err error
			op, err = c.EntryOperation(ctx, s.View.ViewID, entries[fixture].OperationID)
			return err
		})
		if op.Operation == nil || op.Operation.Receipt == nil || !bytes.Equal(entryBytes[fixture], poolLeagueJSON(t, *op.Operation.Receipt)) {
			t.Fatal("reopen changed original committed entry")
		}
		poolActualLogout(t, ctx, c)
	}
	report := struct {
		Scope       string                      `json:"scope"`
		Source      string                      `json:"source_binding_id"`
		Interleaved []uint64                    `json:"interleaved_sequences"`
		Entries     map[string]gymentry.Receipt `json:"entries"`
		Pin         gymevent.Pin                `json:"event_pin"`
		Record      gymevent.SnapshotReceipt    `json:"record"`
		Height      uint32                      `json:"retained_height"`
	}{"normal-PoW actual shared mining and wallet runtime; tagged calendar/store clock only; no production ownership, seed finality or awards", sourceID, []uint64{firstA, firstB, lastA}, entries, eventPin, saved, retainedHeight}
	if err = os.WriteFile(filepath.Join(root, "public-league-result.json"), append(poolLeagueJSON(t, report), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual shared league complete: original sequences %d/%d/%d, Saturday %s, immutable snapshot %s, retained height %d", firstA, firstB, lastA, date, saved.SnapshotDigest, retainedHeight)
}

// Read APIs deliberately fail fast while the background stable lane is held.
// Retry only their typed busy responses within the existing test deadline;
// mutations and all other errors keep their original fail-fast assertions.
func poolLeagueRead(t *testing.T, ctx context.Context, label string, read func() error) {
	t.Helper()
	poolActualWait(t, ctx, label, func(context.Context) bool {
		err := read()
		if errors.Is(err, gymstable.ErrBusy) || errors.Is(err, gymentry.ErrBusy) || errors.Is(err, gymevent.ErrBusy) {
			return false
		}
		if err != nil {
			t.Fatal(label, err)
		}
		return true
	})
}

func poolLeagueJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func poolLeagueStableJob(t *testing.T, ctx context.Context, c *PoolController, view, id string) pilot.StableJob {
	t.Helper()
	var job pilot.StableJob
	poolActualWait(t, ctx, "shared stable job", func(context.Context) bool {
		var err error
		job, err = c.StableJob(ctx, view, id)
		if errors.Is(err, gymstable.ErrBusy) {
			return false
		}
		if err != nil {
			t.Fatal("stable job", err)
		}
		if job.State == "failed" || job.State == "cancelled" || job.State == "outcome_unknown" {
			t.Fatal("stable job outcome", job.State, job.Error)
		}
		return job.State == "done"
	})
	return job
}
func poolLeagueRoster(t *testing.T, ctx context.Context, c *PoolController, prospectID string) {
	t.Helper()
	account := c.Snapshot()
	if account.View == nil {
		t.Fatal("view unavailable before roster change")
	}
	view := account.View.ViewID
	// Recent is a bounded read of the consumed local tail, not evidence that
	// every later shared-source row has been verified. Explicitly finish sync
	// before a roster mutation; all miners are already stopped in this phase.
	var status gymstable.Status
	for {
		var job pilot.StableJob
		poolActualWait(t, ctx, "shared roster sync dispatch", func(context.Context) bool {
			var err error
			job, err = c.BeginStableSync(ctx, view)
			if errors.Is(err, gymstable.ErrBusy) {
				return false
			}
			if err != nil {
				t.Fatal("roster sync dispatch", err)
			}
			return true
		})
		job = poolLeagueStableJob(t, ctx, c, view, job.JobID)
		if job.Sync == nil {
			t.Fatal("roster sync result missing")
		}
		status = job.Sync.Status
		if status.CaughtUp {
			break
		}
	}
	prepared, err := c.BeginStablePrepare(ctx, view, gymstable.SlotRequest{ExpectedRevision: status.RosterRevision, Slot: 1, ProspectID: prospectID})
	if err != nil {
		t.Fatal("roster prepare", err)
	}
	prepared = poolLeagueStableJob(t, ctx, c, view, prepared.JobID)
	if prepared.Intent == nil {
		t.Fatal("roster intent missing")
	}
	applied, err := c.BeginStableApply(ctx, view, prepared.Intent.OperationID)
	if err != nil {
		t.Fatal("roster apply", err)
	}
	applied = poolLeagueStableJob(t, ctx, c, view, applied.JobID)
	if applied.Mutation == nil || applied.Mutation.Receipt.Slots[0] != prospectID {
		t.Fatal("roster mutation differs")
	}
}
func poolLeagueEntryJob(t *testing.T, ctx context.Context, c *PoolController, view, id string) pilot.EntryJob {
	t.Helper()
	var job pilot.EntryJob
	poolActualWait(t, ctx, "shared entry job", func(context.Context) bool {
		var err error
		job, err = c.EntryJob(ctx, view, id)
		if errors.Is(err, gymstable.ErrBusy) || errors.Is(err, gymentry.ErrBusy) {
			return false
		}
		if err != nil {
			t.Fatal("entry job", err)
		}
		if job.State == "failed" || job.State == "cancelled" || job.State == "outcome_unknown" {
			t.Fatal("entry job outcome", job.State, job.Error)
		}
		return job.State == "complete"
	})
	return job
}
func poolLeagueEventJob(t *testing.T, ctx context.Context, c *PoolController, view, id string) pilot.EventJob {
	t.Helper()
	var job pilot.EventJob
	poolActualWait(t, ctx, "shared event job", func(context.Context) bool {
		var err error
		job, err = c.EventJob(ctx, view, id)
		if errors.Is(err, gymstable.ErrBusy) || errors.Is(err, gymevent.ErrBusy) {
			return false
		}
		if err != nil {
			t.Fatal("event job", err)
		}
		if job.State == "failed" || job.State == "cancelled" || job.State == "outcome_unknown" {
			t.Fatal("event job outcome", job.State, job.Error)
		}
		return job.State == "completed"
	})
	return job
}
