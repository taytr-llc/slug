//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
)

// Actual isolated chain membership and depth authorize every certificate here.
// The tagged clock only advances store/calendar observations. Fixture carrier
// funding is explicit, independent of scalar3 pool earnings, and normal PoW.
func TestPoolEventCycleActualFinalityAndCareers(t *testing.T) {
	node, artifacts := os.Getenv("SLUG_POOL_NODE"), os.Getenv("SLUG_POOL_DESKTOP_ARTIFACTS")
	if node == "" || artifacts == "" {
		t.Skip("explicit normal node and owned fixture artifacts required")
	}
	if !filepath.IsAbs(node) || !filepath.IsAbs(artifacts) {
		t.Fatal("absolute fixture paths required")
	}
	root, err := os.MkdirTemp(artifacts, "pool-cycle-")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	var calendar atomic.Int64
	clock := func() time.Time {
		if ms := calendar.Load(); ms != 0 {
			return time.UnixMilli(ms)
		}
		return time.Now()
	}
	config := pilot.Config{DataDir: root, NodeBinary: node}
	c, err := NewPoolForTestClock(ctx, config, clock)
	if err != nil {
		t.Fatal("shared cycle setup", err)
	}
	t.Cleanup(func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error("cycle close", err)
			}
		}
	})
	rpc := seedClockOwnedRPC(t, filepath.Join(root, "shared"))
	var height uint32
	if err = rpc.call(ctx, "getblockcount", nil, &height); err != nil || height != 0 {
		t.Fatal("setup mined", height, err)
	}
	t.Log("shared cycle initialized before lock, at genesis", root)
	fixtures := []string{"zero256", "incrementing256"}
	inventory := map[string][]gymstable.Item{}
	selected := map[string]gymstable.Item{}
	var sourceID string
	// Bounded actual prospect search. No fabricated DNA, selected nonce or
	// changed rating enters C1 matching; the first eligible observed pair wins.
	for round := 0; round < 4 && len(selected) == 0; round++ {
		for _, fixture := range fixtures {
			s := poolActualLogin(t, ctx, c, fixture)
			if sourceID == "" {
				sourceID = s.SourceBindingID
			}
			if s.SourceBindingID != sourceID {
				t.Fatal("source changed")
			}
			before := s.AssignmentCount
			if err = c.Start(); err != nil {
				t.Fatal(err)
			}
			poolActualWait(t, ctx, "genuine matching pool", func(context.Context) bool {
				v := c.Snapshot()
				if v.State == "failed" {
					t.Fatal("miner", v.LastError)
				}
				return v.AssignmentCount >= before+8
			})
			c.Stop()
			inventory[fixture] = poolActualRecent(t, ctx, c).Items
			poolActualLogout(t, ctx, c)
		}
		for _, a := range inventory[fixtures[0]] {
			for _, b := range inventory[fixtures[1]] {
				x, y := a.Condition.RatingHundredths, b.Condition.RatingHundredths
				if len(selected) == 0 && a.Manifest.Physical.Class == b.Manifest.Physical.Class && x*2 >= y && y*2 >= x {
					selected[fixtures[0]], selected[fixtures[1]] = a, b
				}
			}
		}
	}
	if len(selected) != 2 {
		t.Fatal("bounded actual population had no C1 pair; retained for diagnosis")
	}
	t.Log("genuine opposing pair selected", selected[fixtures[0]].Assignment.ProspectID, selected[fixtures[1]].Assignment.ProspectID)
	// These public fixture hash160 values are independently frozen in the
	// existing seed/result publication contract. Pool funding is scalar3.
	seedHash, _ := hex.DecodeString("751e76e8199196d454941c45d1b3a323f1433bd6")
	resultHash, _ := hex.DecodeString("06afd46bcdfd22ef94ac122aa11f241244a37ecc")
	seedHeight, seedBlock := seedClockMine(t, ctx, rpc, seedHash)
	resultHeight, resultBlock := seedClockMine(t, ctx, rpc, resultHash)
	seedCoin := poolCycleCoinbase(t, ctx, rpc, seedHeight, seedBlock, "0014"+hex.EncodeToString(seedHash))
	resultCoin := poolCycleCoinbase(t, ctx, rpc, resultHeight, resultBlock, "0014"+hex.EncodeToString(resultHash))
	poolCycleAdvance(t, ctx, rpc, resultHeight+100)

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(pacific)
	var cutoff time.Time
	for n := 0; n <= 7; n++ {
		x := time.Date(now.Year(), now.Month(), now.Day()+n, 12, 0, 0, 0, pacific)
		if x.Weekday() == time.Saturday && x.After(now.Add(10*time.Minute)) {
			cutoff = x
			break
		}
	}
	if cutoff.IsZero() {
		t.Fatal("independent next lock")
	}
	date, nextDate := cutoff.Format("2006-01-02"), cutoff.AddDate(0, 0, 7).Format("2006-01-02")
	entries := map[string]gymentry.Receipt{}
	for _, fixture := range fixtures {
		s := poolActualLogin(t, ctx, c, fixture)
		poolLeagueRoster(t, ctx, c, selected[fixture].Assignment.ProspectID)
		entries[fixture] = poolCycleEntry(t, ctx, c, s.View.ViewID, date, selected[fixture].Assignment.ProspectID)
		// Book next week's intake while still zero-record. A later effective
		// award must make this fighter ineligible at that cutoff despite booking.
		poolCycleEntry(t, ctx, c, s.View.ViewID, nextDate, selected[fixture].Assignment.ProspectID)
		poolCycleAssertCareer(t, ctx, c, s.View.ViewID, selected[fixture].Assignment.ProspectID, fightercareer.Totals{})
		poolActualLogout(t, ctx, c)
	}
	calendar.Store(cutoff.UnixMilli() + 1000)
	for _, fixture := range fixtures {
		s := poolActualLogin(t, ctx, c, fixture)
		job, err := c.BeginEventCapture(ctx, s.View.ViewID, date)
		if err != nil {
			t.Fatal("capture", err)
		}
		job = poolLeagueEventJob(t, ctx, c, s.View.ViewID, job.JobID)
		if job.Result == nil || len(job.Result.Capture.Snapshot.Members) != 1 || job.Result.Capture.Snapshot.Members[0].ConditionAtCutoff.ProXP != 0 {
			t.Fatal("original noon must be XP0")
		}
		poolActualLogout(t, ctx, c)
	}
	s := poolActualLogin(t, ctx, c, "zero256")
	view := s.View.ViewID
	var noon gymevent.SnapshotReceipt
	poolCycleRead(t, ctx, "original noon", func() error { var e error; noon, e = c.EventRecord(ctx, view, date); return e })
	seed := poolCycleSeed(t, ctx, c, view, date, "prepare")
	poolCycleSeedSuccess(t, seed)
	// Reconcile bounded stationary funding pages until the original matured
	// scalar1 output is available; each action retains the eight-header budget.
	for n := 0; n < 80; n++ {
		seed = poolCycleSeed(t, ctx, c, view, date, "reconcile")
		poolCycleSeedSuccess(t, seed)
		if seed.Result != nil && seed.Result.Cursor.CompleteToHeight >= int32(resultHeight+100) {
			break
		}
		if n == 79 {
			t.Fatal("bounded seed scan exhausted")
		}
	}
	seed = poolCycleSeed(t, ctx, c, view, date, "publish")
	poolCycleSeedSuccess(t, seed)
	if seed.Result == nil || seed.Result.Publication == nil {
		t.Fatal("seed transaction missing")
	}
	seedTx := seed.Result.Publication.RawTx
	poolCyclePublication(t, seedTx, seed.Result.Publication.Coin, seedCoin)
	seedClockMine(t, ctx, rpc, bytes.Repeat([]byte{0x45}, 20))
	seedClockMine(t, ctx, rpc, bytes.Repeat([]byte{0x45}, 20))
	seedRefreshBlocks := 0
	for n := 0; n < 80; n++ {
		seed = poolCycleSeed(t, ctx, c, view, date, "reconcile")
		poolCycleSeedSuccess(t, seed)
		if seed.Result.Target != nil {
			break
		}
		if n == 79 {
			t.Fatal("fixed seed target absent")
		}
		if seed.Result.Anchor != nil {
			// A cached template can omit the freshly published carrier. Only
			// its observed inclusion height identifies the required successor.
			poolCycleAdvance(t, ctx, rpc, seed.Result.Anchor.Height+1)
		} else {
			poolCycleRefreshAfterScan(t, ctx, rpc, "seed", seed.Result.Cursor.CompleteToHeight, &seedRefreshBlocks)
		}
	}
	target := *seed.Result.Target
	earlySeed := poolCycleSeed(t, ctx, c, view, date, "attest")
	if earlySeed.State != "failed" || earlySeed.Error != "seed_not_mature" {
		t.Fatal("seed early finality", earlySeed.State, earlySeed.Error)
	}
	poolCycleAdvance(t, ctx, rpc, target.Height+60)
	seed = poolCycleSeed(t, ctx, c, view, date, "attest")
	poolCycleSeedSuccess(t, seed)
	if seed.Result.Finality == nil || seed.Result.Finality.TargetDescendants < 60 || seed.Result.Target.Hash != target.Hash || seed.Result.Publication.RawTx != seedTx {
		t.Fatal("fixed seed finality changed")
	}
	seedFinal := *seed.Result.Finality
	cardJob, err := c.BeginCardBuild(ctx, view, date)
	if err != nil {
		t.Fatal("build card", err)
	}
	cardJob = poolCycleCardJob(t, ctx, c, view, cardJob.JobID)
	if cardJob.Result == nil {
		t.Fatal("card missing")
	}
	card := cardJob.Result.Record
	counts := card.Plan.Counts
	if counts.TotalCount != 2 || counts.EligibleCount != 2 || counts.PairedCount != 2 || counts.UnmatchedCount != 0 || card.Plan.BoutCount != 1 || len(card.Plan.Bouts) != 1 {
		t.Fatal("C1 pair/unmatched accounting", counts)
	}
	t.Logf("C1 candidate card: entered=%d paired=%d unmatched=%d reasons=%+v", counts.TotalCount, counts.PairedCount, counts.UnmatchedCount, counts.Reasons)
	boutID := card.Plan.Bouts[0].BoutID
	boutJob, err := c.BeginBoutRun(ctx, view, date, boutID)
	if err != nil {
		t.Fatal("run bout", err)
	}
	boutJob = poolCycleBoutJob(t, ctx, c, view, boutJob.JobID)
	if boutJob.Result == nil {
		t.Fatal("bout missing")
	}
	bout := boutJob.Result.Record
	expected := poolCycleAwards(t, bout, card.Plan.Bouts[0])
	poolActualLogout(t, ctx, c)
	for _, fixture := range fixtures {
		s = poolActualLogin(t, ctx, c, fixture)
		poolCycleAssertCareer(t, ctx, c, s.View.ViewID, selected[fixture].Assignment.ProspectID, fightercareer.Totals{})
		poolActualLogout(t, ctx, c)
	}
	s = poolActualLogin(t, ctx, c, "zero256")
	view = s.View.ViewID
	// Publish prepares the result statement before requiring its funding scan.
	result := poolCycleResult(t, ctx, c, view, date, boutID, "publish")
	if result.State != "failed" || result.Error != "result_scan_incomplete" {
		t.Fatal("result prepare/publish", result.State, result.Error)
	}
	for n := 0; n < 100; n++ {
		result = poolCycleResult(t, ctx, c, view, date, boutID, "reconcile")
		poolCycleResultSuccess(t, result)
		if result.Result.Status.FundingState == "available" || result.Result.Record.Publication != nil {
			break
		}
		if n == 99 {
			t.Fatal("bounded result funding scan exhausted", result.Result.Status.FundingState)
		}
	}
	result = poolCycleResult(t, ctx, c, view, date, boutID, "publish")
	poolCycleResultSuccess(t, result)
	if result.Result.Record.Publication == nil {
		t.Fatal("result transaction missing")
	}
	resultTx := result.Result.Record.Publication.RawTx
	poolCyclePublication(t, resultTx, result.Result.Record.Publication.Coin, resultCoin)
	seedClockMine(t, ctx, rpc, bytes.Repeat([]byte{0x45}, 20))
	resultRefreshBlocks := 0
	for n := 0; n < 100; n++ {
		result = poolCycleResult(t, ctx, c, view, date, boutID, "reconcile")
		poolCycleResultSuccess(t, result)
		if result.Result.Record.Inclusion != nil {
			break
		}
		if n == 99 {
			t.Fatal("result inclusion absent")
		}
		poolCycleRefreshAfterScan(t, ctx, rpc, "result", result.Result.Record.Cursor.CompleteToHeight, &resultRefreshBlocks)
	}
	anchor := result.Result.Record.Inclusion.Anchor
	earlyResult := poolCycleResult(t, ctx, c, view, date, boutID, "attest")
	if earlyResult.State != "failed" || earlyResult.Error != "result_not_mature" {
		t.Fatal("result early finality", earlyResult.State, earlyResult.Error)
	}
	poolActualLogout(t, ctx, c)
	for _, fixture := range fixtures {
		s = poolActualLogin(t, ctx, c, fixture)
		poolCycleAssertCareer(t, ctx, c, s.View.ViewID, selected[fixture].Assignment.ProspectID, fightercareer.Totals{})
		poolActualLogout(t, ctx, c)
	}
	s = poolActualLogin(t, ctx, c, "zero256")
	view = s.View.ViewID
	poolCycleAdvance(t, ctx, rpc, anchor.Height+60)
	calendar.Store(cutoff.UnixMilli() + 2000)
	result = poolCycleResult(t, ctx, c, view, date, boutID, "attest")
	poolCycleResultSuccess(t, result)
	if result.Result.Record.Finality == nil || result.Result.Record.Finality.Descendants < 60 || result.Result.Record.Publication.RawTx != resultTx {
		t.Fatal("own result finality missing")
	}
	finality := *result.Result.Record.Finality
	for _, kind := range []string{"publish", "attest"} {
		again := poolCycleResult(t, ctx, c, view, date, boutID, kind)
		poolCycleResultSuccess(t, again)
		if again.Result.Record.Publication.RawTx != resultTx || !reflect.DeepEqual(*again.Result.Record.Finality, finality) {
			t.Fatal("retry changed result evidence")
		}
	}
	poolActualLogout(t, ctx, c)
	for _, fixture := range fixtures {
		s = poolActualLogin(t, ctx, c, fixture)
		poolCycleAssertCareer(t, ctx, c, s.View.ViewID, selected[fixture].Assignment.ProspectID, expected[selected[fixture].Assignment.ProspectID])
		poolActualLogout(t, ctx, c)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	calendar.Store(cutoff.UnixMilli() + 3000)
	c, err = NewPoolForTestClock(ctx, config, clock)
	if err != nil {
		t.Fatal("career-bound restart", err)
	}
	entryRefusals := map[string]pilot.EntryJob{}
	cutoffRefusals := map[string]pilot.EventJob{}
	for _, fixture := range fixtures {
		s = poolActualLogin(t, ctx, c, fixture)
		view = s.View.ViewID
		if s.SourceBindingID != sourceID || s.State != "stopped" || s.Attempts != 0 {
			t.Fatal("restart changed source or mined")
		}
		var rr gymresult.ReadResult
		poolCycleRead(t, ctx, "retained result", func() error { var e error; rr, e = c.ResultRecord(ctx, view, date, boutID); return e })
		if rr.Record.Publication.RawTx != resultTx || !reflect.DeepEqual(*rr.Record.Finality, finality) {
			t.Fatal("restart changed finality")
		}
		var cr gymcard.ReadResult
		poolCycleRead(t, ctx, "retained card", func() error { var e error; cr, e = c.CardRecord(ctx, view, date); return e })
		if !reflect.DeepEqual(cr.Record, card) {
			t.Fatal("restart changed card")
		}
		var br gymbout.ReadResult
		poolCycleRead(t, ctx, "retained bout", func() error { var e error; br, e = c.BoutRecord(ctx, view, date, boutID); return e })
		if !reflect.DeepEqual(br.Record, bout) {
			t.Fatal("restart changed bout")
		}
		var er gymevent.SnapshotReceipt
		poolCycleRead(t, ctx, "retained noon", func() error { var e error; er, e = c.EventRecord(ctx, view, date); return e })
		if !reflect.DeepEqual(er, noon) {
			t.Fatal("award rewrote original noon")
		}
		poolCycleAssertCareer(t, ctx, c, view, selected[fixture].Assignment.ProspectID, expected[selected[fixture].Assignment.ProspectID])
		var next gymentry.EventStatus
		poolCycleRead(t, ctx, "next entry before C1 refusal", func() error { var e error; next, e = c.EntryStatus(ctx, view, nextDate); return e })
		request := gymentry.Request{Version: 1, EventID: next.Event.EventID, Saturday: nextDate, Action: "replace", ExpectedRevision: next.Revision, ExpectedRosterRevision: entries[fixture].RosterRevision, ProspectIDs: []string{selected[fixture].Assignment.ProspectID}}
		prepared := poolCycleDispatch(t, ctx, "C1 awarded entrant intent", func(ctx context.Context) (pilot.EntryJob, error) { return c.BeginEntryPrepare(ctx, view, request) })
		prepared = poolLeagueEntryJob(t, ctx, c, view, prepared.JobID)
		if prepared.Intent == nil || prepared.OperationID == "" {
			t.Fatal("C1 refusal intent missing")
		}
		refused := poolCycleDispatch(t, ctx, "C1 awarded entrant apply", func(ctx context.Context) (pilot.EntryJob, error) {
			return c.BeginEntryApply(ctx, view, prepared.OperationID)
		})
		refused = poolCycleAwait(t, ctx, "C1 awarded entrant result", func(ctx context.Context) (pilot.EntryJob, error) { return c.EntryJob(ctx, view, refused.JobID) })
		if refused.State != "failed" || refused.Error != "career_candidate_ineligible" || !reflect.DeepEqual(refused.IneligibleProspectIDs, request.ProspectIDs) {
			t.Fatal("awarded fighter reentered C1", refused.State, refused.Error)
		}
		entryRefusals[fixture] = refused
		abandoned := poolCycleDispatch(t, ctx, "abandon refused C1 intent", func(ctx context.Context) (gymentry.OperationResult, error) {
			return c.AbandonEntryOperation(ctx, view, prepared.OperationID)
		})
		if abandoned.Operation == nil || abandoned.Operation.State != "abandoned" || abandoned.Operation.Intent.OperationID != prepared.OperationID || abandoned.Operation.Receipt != nil {
			t.Fatal("refused C1 intent was not explicitly abandoned")
		}
		var after gymentry.EventStatus
		poolCycleRead(t, ctx, "unchanged entry after C1 refusal", func() error { var e error; after, e = c.EntryStatus(ctx, view, nextDate); return e })
		if after.Revision != next.Revision || !reflect.DeepEqual(after.SavedIDs, next.SavedIDs) || after.PendingOperationID != "" {
			t.Fatal("C1 refusal changed saved intake or retained pending intent")
		}
		poolActualLogout(t, ctx, c)
	}
	calendar.Store(cutoff.AddDate(0, 0, 7).UnixMilli() + 1000)
	for _, fixture := range fixtures {
		s = poolActualLogin(t, ctx, c, fixture)
		view = s.View.ViewID
		j := poolCycleDispatch(t, ctx, "later C1 cutoff capture", func(ctx context.Context) (pilot.EventJob, error) { return c.BeginEventCapture(ctx, view, nextDate) })
		j = poolCycleAwait(t, ctx, "later C1 cutoff result", func(ctx context.Context) (pilot.EventJob, error) { return c.EventJob(ctx, view, j.JobID) })
		if j.State != "failed" || j.Error != "career_candidate_ineligible" || j.Result != nil || !reflect.DeepEqual(j.IneligibleProspectIDs, []string{selected[fixture].Assignment.ProspectID}) {
			t.Fatal("later C1 noon admitted an awarded pre-enrolled fighter", j.State, j.Error)
		}
		cutoffRefusals[fixture] = j
		poolCycleAssertCareer(t, ctx, c, view, selected[fixture].Assignment.ProspectID, expected[selected[fixture].Assignment.ProspectID])
		poolActualLogout(t, ctx, c)
	}
	report := map[string]any{"scope": "normal-PoW isolated shared event cycle; real canonical depth; published fixture keys; tagged calendar only", "source_binding_id": sourceID, "entries": entries, "noon": noon, "seed_funding": seedCoin, "result_funding": resultCoin, "seed_finality": seedFinal, "card": card, "bout": bout, "result_finality": finality, "seed_raw_tx": seedTx, "result_raw_tx": resultTx, "expected_awards": expected, "c1_entry_refusals": entryRefusals, "c1_later_cutoff_refusals": cutoffRefusals}
	if err = os.WriteFile(filepath.Join(root, "public-cycle-result.json"), append(poolLeagueJSON(t, report), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("shared cycle complete; original fighters, fixed seed, one C1 bout, own result finality, exactly-once awards and restart", root)
}

func poolCycleBusy(err error) bool {
	return errors.Is(err, gymstable.ErrBusy) || errors.Is(err, gymentry.ErrBusy) || errors.Is(err, gymevent.ErrBusy) || errors.Is(err, gymseed.ErrBusy) || errors.Is(err, gymcard.ErrBusy) || errors.Is(err, gymbout.ErrBusy) || errors.Is(err, gymresult.ErrBusy)
}
func poolCycleRead(t *testing.T, ctx context.Context, label string, read func() error) {
	t.Helper()
	poolActualWait(t, ctx, label, func(context.Context) bool {
		err := read()
		if poolCycleBusy(err) {
			return false
		}
		if err != nil {
			t.Fatal(label, err)
		}
		return true
	})
}
func poolCycleDispatch[T any](t *testing.T, ctx context.Context, label string, dispatch func(context.Context) (T, error)) T {
	t.Helper()
	var out T
	poolActualWait(t, ctx, label, func(context.Context) bool {
		var err error
		out, err = dispatch(ctx)
		if poolCycleBusy(err) {
			return false
		}
		if err != nil {
			t.Fatal(label, err)
		}
		return true
	})
	return out
}
func poolCycleAwait[T any](t *testing.T, ctx context.Context, label string, read func(context.Context) (T, error)) T {
	t.Helper()
	var out T
	poolActualWait(t, ctx, label, func(readCtx context.Context) bool {
		var err error
		out, err = read(readCtx)
		if poolCycleBusy(err) {
			return false
		}
		if err != nil {
			t.Fatal(label, err)
		}
		var state string
		switch job := any(out).(type) {
		case pilot.SeedJob:
			state = job.State
		case pilot.CardJob:
			state = job.State
		case pilot.BoutJob:
			state = job.State
		case pilot.ResultJob:
			state = job.State
		case pilot.StableJob:
			state = job.State
		case pilot.EntryJob:
			state = job.State
		case pilot.EventJob:
			state = job.State
		default:
			t.Fatal("unsupported test job")
		}
		return state != "queued" && state != "running"
	})
	return out
}
func poolCycleSeed(t *testing.T, ctx context.Context, c *PoolController, v, date, kind string) pilot.SeedJob {
	t.Helper()
	dispatch := map[string]func(context.Context, string, string) (pilot.SeedJob, error){"prepare": c.BeginSeedPrepare, "publish": c.BeginSeedPublish, "reconcile": c.BeginSeedReconcile, "attest": c.BeginSeedAttest}[kind]
	if dispatch == nil {
		t.Fatal("unknown seed action")
	}
	j := poolCycleDispatch(t, ctx, "seed "+kind, func(ctx context.Context) (pilot.SeedJob, error) { return dispatch(ctx, v, date) })
	return poolCycleAwait(t, ctx, "seed "+kind, func(ctx context.Context) (pilot.SeedJob, error) { return c.SeedJob(ctx, v, j.JobID) })
}
func poolCycleSeedSuccess(t *testing.T, j pilot.SeedJob) {
	t.Helper()
	if j.State != "completed" || j.Result == nil {
		t.Fatal("seed action", j.Kind, j.State, j.Error)
	}
}
func poolCycleResult(t *testing.T, ctx context.Context, c *PoolController, v, date, bout, kind string) pilot.ResultJob {
	t.Helper()
	dispatch := map[string]func(context.Context, string, string, string) (pilot.ResultJob, error){"publish": c.BeginResultPublish, "reconcile": c.BeginResultReconcile, "attest": c.BeginResultAttest}[kind]
	if dispatch == nil {
		t.Fatal("unknown result action")
	}
	j := poolCycleDispatch(t, ctx, "result "+kind, func(ctx context.Context) (pilot.ResultJob, error) { return dispatch(ctx, v, date, bout) })
	return poolCycleAwait(t, ctx, "result "+kind, func(ctx context.Context) (pilot.ResultJob, error) { return c.ResultJob(ctx, v, j.JobID) })
}
func poolCycleResultSuccess(t *testing.T, j pilot.ResultJob) {
	t.Helper()
	if j.State != "completed" || j.Result == nil {
		t.Fatal("result action", j.Kind, j.State, j.Error)
	}
}
func poolCycleCardJob(t *testing.T, ctx context.Context, c *PoolController, v, id string) pilot.CardJob {
	t.Helper()
	j := poolCycleAwait(t, ctx, "card build", func(ctx context.Context) (pilot.CardJob, error) { return c.CardJob(ctx, v, id) })
	if j.State != "completed" {
		t.Fatal("card outcome", j.State, j.Error)
	}
	return j
}
func poolCycleBoutJob(t *testing.T, ctx context.Context, c *PoolController, v, id string) pilot.BoutJob {
	t.Helper()
	j := poolCycleAwait(t, ctx, "bout run", func(ctx context.Context) (pilot.BoutJob, error) { return c.BoutJob(ctx, v, id) })
	if j.State != "completed" {
		t.Fatal("bout outcome", j.State, j.Error)
	}
	return j
}
func poolCycleEntry(t *testing.T, ctx context.Context, c *PoolController, v, date, id string) gymentry.Receipt {
	t.Helper()
	var status gymentry.EventStatus
	poolCycleRead(t, ctx, "entry status", func() error { var err error; status, err = c.EntryStatus(ctx, v, date); return err })
	var snapshot pilot.StableSnapshot
	poolActualWait(t, ctx, "entry roster metadata", func(context.Context) bool {
		snapshot = c.StableSnapshot()
		return snapshot.View != nil && snapshot.View.ViewID == v && snapshot.Status != nil
	})
	request := gymentry.Request{Version: 1, EventID: status.Event.EventID, Saturday: date, Action: "replace", ExpectedRevision: status.Revision, ExpectedRosterRevision: snapshot.Status.RosterRevision, ProspectIDs: []string{id}}
	j := poolCycleDispatch(t, ctx, "entry prepare", func(ctx context.Context) (pilot.EntryJob, error) { return c.BeginEntryPrepare(ctx, v, request) })
	j = poolLeagueEntryJob(t, ctx, c, v, j.JobID)
	if j.Intent == nil {
		t.Fatal("entry intent missing")
	}
	j = poolCycleDispatch(t, ctx, "entry apply", func(ctx context.Context) (pilot.EntryJob, error) { return c.BeginEntryApply(ctx, v, j.OperationID) })
	j = poolLeagueEntryJob(t, ctx, c, v, j.JobID)
	if j.Mutation == nil || len(j.Mutation.Receipt.Members) != 1 || j.Mutation.Receipt.Members[0].Assignment.ProspectID != id {
		t.Fatal("exact entry missing")
	}
	return j.Mutation.Receipt
}
func poolCycleAssertCareer(t *testing.T, ctx context.Context, c *PoolController, v, id string, want fightercareer.Totals) {
	t.Helper()
	j := poolCycleDispatch(t, ctx, "career roster", func(ctx context.Context) (pilot.StableJob, error) { return c.BeginStableRoster(ctx, v, 1, 8) })
	j = poolLeagueStableJob(t, ctx, c, v, j.JobID)
	var page gymstable.CareerRosterPage
	poolCycleRead(t, ctx, "career page", func() error { var err error; page, err = c.CareerRoster(ctx, v, j.JobID, 1, 8); return err })
	if page.Source.Status.Pin.Version != gymstable.SharedPinVersion2 || len(page.Items) != 1 || page.Items[0].ProspectID != id || page.Items[0].Totals != want || page.Items[0].Condition.ProXP != int(want.XP) {
		t.Fatal("career award mismatch", page.Items, want)
	}
}
func poolCycleAdvance(t *testing.T, ctx context.Context, rpc *seedClockRPC, target uint32) {
	t.Helper()
	if target >= 400 {
		t.Fatal("bounded cycle chain limit")
	}
	for n := 0; n < 400; n++ {
		var height uint32
		if err := rpc.call(ctx, "getblockcount", nil, &height); err != nil {
			t.Fatal(err)
		}
		if height >= target {
			return
		}
		seedClockMine(t, ctx, rpc, bytes.Repeat([]byte{0x45}, 20))
	}
	t.Fatal("bounded cycle advance exhausted")
}

// Mine only after the successful bounded reconciliation has caught up with
// the actual tip and still has no carrier inclusion. Each explicit block
// invalidates the previous-tip template; none substitutes an observed seed.
func poolCycleRefreshAfterScan(t *testing.T, ctx context.Context, rpc *seedClockRPC, label string, complete int32, blocks *int) {
	t.Helper()
	var height uint32
	if err := rpc.call(ctx, "getblockcount", nil, &height); err != nil {
		t.Fatal(label, "current tip", err)
	}
	if int64(complete) < int64(height) {
		return
	}
	if int64(complete) != int64(height) || *blocks >= 8 {
		t.Fatal(label, "bounded inclusion/template refresh exhausted", complete, height, *blocks)
	}
	poolCycleAdvance(t, ctx, rpc, height+1)
	*blocks++
}

func poolCycleCoinbase(t *testing.T, ctx context.Context, rpc *seedClockRPC, height uint32, hash, script string) seednode.FundingCoin {
	t.Helper()
	var encoded string
	if err := rpc.call(ctx, "getblock", []any{hash, 0}, &encoded); err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(encoded)
	var block wire.MsgBlock
	if err != nil || block.Deserialize(bytes.NewReader(raw)) != nil || len(block.Transactions) == 0 {
		t.Fatal("funding block decode")
	}
	tx := block.Transactions[0]
	for index, out := range tx.TxOut {
		if hex.EncodeToString(out.PkScript) == script {
			if out.Value != 1000000000 {
				t.Fatal("fixture subsidy changed")
			}
			return seednode.FundingCoin{Outpoint: seednode.Outpoint{TxID: tx.TxHash().String(), Index: uint32(index)}, ValueSatoshis: out.Value, Script: script, BlockHeight: height, BlockHash: hash, Coinbase: true}
		}
	}
	t.Fatal("explicit funding output absent")
	return seednode.FundingCoin{}
}
func poolCyclePublication(t *testing.T, encoded string, got, want seednode.FundingCoin) {
	t.Helper()
	if got != want {
		t.Fatal("publication used another funding source", got, want)
	}
	raw, err := hex.DecodeString(encoded)
	var tx wire.MsgTx
	if err != nil || tx.Deserialize(bytes.NewReader(raw)) != nil || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 {
		t.Fatal("publication shape")
	}
	input := tx.TxIn[0]
	if input.PreviousOutPoint.Hash.String() != want.Outpoint.TxID || input.PreviousOutPoint.Index != want.Outpoint.Index || len(input.Witness) != 2 {
		t.Fatal("publication input/signature shape")
	}
	if tx.TxOut[0].Value != 0 || len(tx.TxOut[0].PkScript) == 0 || tx.TxOut[0].PkScript[0] != 0x6a || hex.EncodeToString(tx.TxOut[1].PkScript) != want.Script || tx.TxOut[1].Value != want.ValueSatoshis-1000 {
		t.Fatal("publication payload/change/fee")
	}
}
func poolCycleAwards(t *testing.T, b gymbout.Record, pair gymcard.Bout) map[string]fightercareer.Totals {
	t.Helper()
	a, bv := fightercareer.Totals{XP: 1}, fightercareer.Totals{XP: 1}
	// Independent D78/D42 expectations from the saved outcome: standard bouts
	// grant one XP to each, draws four points each, decision wins10, finishes14.
	// FIGHTER_BOUT.md saves a full winner ID, or an empty string for draws.
	// Corner labels belong only to the separate pure career arithmetic API.
	switch b.Result.Winner {
	case "":
		if b.Result.Method != "decision_draw" && b.Result.Method != "double_corner_stoppage" {
			t.Fatal("draw method", b.Result.Method)
		}
		a.Draws, bv.Draws, a.BasePoints, bv.BasePoints = 1, 1, 4, 4
	case pair.CornerA, pair.CornerB:
		points := uint32(14)
		switch b.Result.Method {
		case "decision":
			points = 10
		case "ko", "tko_referee", "tko_three_knockdowns", "tko_corner":
		default:
			t.Fatal("win method", b.Result.Method)
		}
		if b.Result.Winner == pair.CornerA {
			a.Wins, a.BasePoints, bv.Losses = 1, points, 1
		} else {
			bv.Wins, bv.BasePoints, a.Losses = 1, points, 1
		}
	default:
		t.Fatal("unexpected saved winner", b.Result.Winner)
	}
	return map[string]fightercareer.Totals{pair.CornerA: a, pair.CornerB: bv}
}
