//go:build desktopintegration && !slugtestpow && !slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

// This runs the packaged helper through its actual typed native bridge. It
// mines only disposable isolated normal-PoW fixtures, not the public chain.
func TestStablePackagedHelperNormalPoW(t *testing.T) {
	helper, node := os.Getenv("SLUG_STABLE_HELPER"), os.Getenv("SLUG_STABLE_NODE")
	if helper == "" || node == "" {
		t.Skip("explicit frozen stable helper and normal node required")
	}
	careerEnabled := os.Getenv("SLUG_CAREER_ACCEPTANCE") == "1"
	var careerOriginal map[string]pilot.ManifestSnapshot
	if careerEnabled {
		careerOriginal = map[string]pilot.ManifestSnapshot{}
	}
	resultEnabled := os.Getenv("SLUG_RESULT_ACCEPTANCE") == "1" || careerEnabled
	boutEnabled := os.Getenv("SLUG_BOUT_ACCEPTANCE") == "1" || resultEnabled
	cardEnabled := os.Getenv("SLUG_CARD_ACCEPTANCE") == "1" || boutEnabled
	seedEnabled := os.Getenv("SLUG_SEED_ACCEPTANCE") == "1" || cardEnabled
	eventEnabled := os.Getenv("SLUG_EVENT_ACCEPTANCE") == "1" || seedEnabled
	entryEnabled := os.Getenv("SLUG_ENTRY_ACCEPTANCE") == "1" || eventEnabled
	previewEnabled := os.Getenv("SLUG_PREVIEW_ACCEPTANCE") == "1" || entryEnabled
	minimumCandidates := uint64(3)
	if previewEnabled {
		minimumCandidates = 6
	}
	root, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "stable-desktop-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("public candidate stable helper artifacts", root)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	data := filepath.Join(root, "data")
	launch := LaunchStable
	if entryEnabled {
		launch = LaunchEntries
	}
	if eventEnabled {
		launch = LaunchEvents
	}
	if seedEnabled {
		launch = LaunchSeeds
	}
	if cardEnabled {
		launch = LaunchCards
	}
	if boutEnabled {
		launch = LaunchBouts
	}
	if resultEnabled {
		launch = LaunchResults
	}
	b, e := launch(helper, data, node)
	if e != nil {
		t.Fatal(e)
	}
	processes := newManifestProcessInventory(data, node, b.cmd.Process.Pid)
	var previews *previewAcceptance
	if previewEnabled {
		previews = newPreviewAcceptance(t, ctx, b)
	}
	var entries *entryAcceptance
	if entryEnabled {
		entries = newEntryAcceptance(t, ctx, b)
		previews.restartCheck = entries.checkProcessRestart
	}
	var events *eventAcceptance
	if eventEnabled {
		events = newEventAcceptance(t, ctx, b)
		previews.restartCheck = func(restarted *StableBridge, s pilot.StableSnapshot) any {
			entry := entries.checkProcessRestart(restarted, s)
			events.checkRestart(restarted, s)
			return entry
		}
	}
	var seeds *seedAcceptance
	if seedEnabled {
		seeds = newSeedAcceptance(t, ctx, b)
		previews.restartCheck = func(restarted *StableBridge, s pilot.StableSnapshot) any {
			entry := entries.checkProcessRestart(restarted, s)
			events.checkRestart(restarted, s)
			seeds.checkRestart(restarted, s)
			return entry
		}
	}
	var results *resultAcceptance
	if resultEnabled {
		results = newResultAcceptance(t, ctx, b)
	}
	var bouts *boutAcceptance
	if boutEnabled {
		bouts = newBoutAcceptance(t, ctx, b)
	}
	var cards *cardAcceptance
	if cardEnabled {
		cards = newCardAcceptance(t, ctx, b)
		previews.restartCheck = func(restarted *StableBridge, s pilot.StableSnapshot) any {
			entry := entries.checkProcessRestart(restarted, s)
			events.checkRestart(restarted, s)
			seeds.checkRestart(restarted, s)
			cards.checkRestart(restarted, s)
			if bouts != nil {
				bouts.checkRestart(restarted, s)
				if results != nil {
					results.checkRestart(restarted, s)
				}
			}
			return entry
		}
	}
	cleanupChecked := false
	defer func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
		if !cleanupChecked {
			processes.checkClosed(t)
		}
	}()
	pause := func() {
		t.Helper()
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	request := func(command string, seq uint64) pilot.ManifestResponse {
		t.Helper()
		r, e := b.Request(ctx, command, seq)
		if e != nil {
			t.Fatal(command, e)
		}
		return r
	}
	stable := func(r pilot.StableRequest) pilot.StableResponse { t.Helper(); return b.StableRequest(ctx, r) }
	snapshot := func() pilot.StableSnapshot {
		t.Helper()
		r := stable(pilot.StableRequest{Command: "stable_snapshot"})
		if !r.OK || r.Snapshot == nil {
			t.Fatal("stable snapshot", r.Error)
		}
		return *r.Snapshot
	}
	waitManifest := func(check func(pilot.ManifestSnapshot) bool) pilot.ManifestSnapshot {
		t.Helper()
		deadline := time.Now().Add(150 * time.Second)
		for time.Now().Before(deadline) {
			s := *request("snapshot", 0).Snapshot
			if s.Account.State == "failed" || s.State == "failed" {
				t.Fatalf("helper failed account=%s worker=%s", s.Account.LastError, s.LastError)
			}
			if check(s) {
				return s
			}
			pause()
		}
		t.Fatal("manifest state transition timeout")
		return pilot.ManifestSnapshot{}
	}
	waitStable := func(check func(pilot.StableSnapshot) bool) pilot.StableSnapshot {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			s := snapshot()
			if s.State == "failed" {
				t.Fatal("stable failed", s.Error)
			}
			if check(s) {
				return s
			}
			pause()
		}
		t.Fatal("stable transition timeout")
		return pilot.StableSnapshot{}
	}
	begin := func(r pilot.StableRequest) pilot.StableJob {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			v := stable(r)
			if v.OK && v.Job != nil {
				return *v.Job
			}
			if v.Error != "busy" {
				t.Fatal(r.Command, v.Error)
			}
			pause()
		}
		t.Fatal("stable admission remained busy", r.Command)
		return pilot.StableJob{}
	}
	await := func(j pilot.StableJob) pilot.StableJob {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if j.State != "queued" && j.State != "running" {
				return j
			}
			pause()
			r := stable(pilot.StableRequest{Command: "stable_job", ViewID: j.View.ViewID, JobID: j.JobID})
			if !r.OK || r.Job == nil {
				t.Fatal("stable job query", r.Error)
			}
			j = *r.Job
		}
		t.Fatal("tracked stable job timeout", j.Kind)
		return pilot.StableJob{}
	}
	success := func(j pilot.StableJob) pilot.StableJob {
		t.Helper()
		j = await(j)
		if j.State != "done" {
			t.Fatal("tracked operation failed", j.Kind, j.State, j.Error)
		}
		return j
	}
	meta := func(command, view, operation string) pilot.StableOperation {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			r := stable(pilot.StableRequest{Command: command, ViewID: view, OperationID: operation})
			if r.OK && r.Operation != nil {
				return *r.Operation
			}
			if r.Error != "busy" {
				t.Fatal(command, r.Error)
			}
			pause()
		}
		t.Fatal("metadata request remained busy", command)
		return pilot.StableOperation{}
	}
	inventoryPage := func(view string) gymstable.Page {
		t.Helper()
		j := success(begin(pilot.StableRequest{Command: "stable_inventory", ViewID: view, After: 0, Limit: 8}))
		if j.Page == nil {
			t.Fatal("missing inventory page")
		}
		return *j.Page
	}
	prepare := func(view string, revision uint64, slot uint32, id string) gymstable.Intent {
		t.Helper()
		j := success(begin(pilot.StableRequest{Command: "stable_prepare_slot", ViewID: view, ExpectedRevision: revision, Slot: slot, ProspectID: id}))
		if j.Intent == nil {
			t.Fatal("missing intent")
		}
		return *j.Intent
	}
	apply := func(view, id string) gymstable.MutationResult {
		t.Helper()
		j := success(begin(pilot.StableRequest{Command: "stable_apply_slot", ViewID: view, OperationID: id}))
		if j.Mutation == nil {
			t.Fatal("missing mutation")
		}
		return *j.Mutation
	}
	type publicRow struct {
		Fixture  string                 `json:"fixture"`
		Receipt  pilot.ManifestReceipt  `json:"receipt"`
		Portrait pilot.ManifestPortrait `json:"portrait"`
	}
	rows := []publicRow{}
	evidence := []map[string]any{}
	initial := *request("snapshot", 0).Snapshot
	if initial.Account.State != "signed_out" {
		t.Fatal("unexpected initial account")
	}
	if s := snapshot(); s.State != "unavailable" || s.View != nil || s.Status != nil {
		t.Fatal("signed-out stable disclosed identity")
	}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		t.Log("stable login", fixture)
		if _, e = b.RequestFixture(ctx, fixture); e != nil {
			t.Fatal(e)
		}
		ready := waitManifest(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_in" })
		if ready.Attempts != 0 || ready.AcceptedCount != 0 || ready.State != "stopped" {
			t.Fatal("cold login mined")
		}
		synced := waitStable(func(s pilot.StableSnapshot) bool {
			return s.View != nil && s.Status != nil && s.Status.CaughtUp && s.ActiveJobID == "" && s.QueuedJobID == ""
		})
		if synced.Status.Checkpoint != 0 || synced.Status.RosterRevision != 0 {
			t.Fatal("unexpected new stable")
		}
		processes.capture(t, fixture, "prepared", ready.Account.ID, synced.View.AuthorityOrigin, false)
		request("start", 0)
		mined := waitManifest(func(s pilot.ManifestSnapshot) bool {
			if s.AcceptedCount < minimumCandidates || s.State != "running" {
				return false
			}
			for _, item := range s.Projection.Items {
				if item.EligibilityMode == "canonical-winning-block" && s.Projection.ThroughSequence > item.Sequence {
					return true
				}
			}
			return false
		})
		processes.capture(t, fixture, "initial-mining", mined.Account.ID, mined.View.AuthorityOrigin, true)
		live := waitStable(func(s pilot.StableSnapshot) bool { return s.Status != nil && s.Status.Checkpoint >= minimumCandidates })
		if live.View.ViewID != mined.View.ViewID {
			t.Fatal("live assignment wrong view")
		}
		stopped := *request("stop", 0).Snapshot
		if stopped.State != "stopped" {
			t.Fatal("Stop did not join worker")
		}
		current := snapshot()
		if current.View == nil {
			t.Fatal("stopped view absent")
		}
		view := current.View.ViewID
		if current.Status == nil || !current.Status.CaughtUp || current.Status.Checkpoint < stopped.AcceptedCount {
			success(begin(pilot.StableRequest{Command: "stable_sync", ViewID: view}))
		}
		quiet := waitStable(func(s pilot.StableSnapshot) bool {
			return s.Status != nil && s.Status.CaughtUp && s.ActiveJobID == "" && s.QueuedJobID == ""
		})
		page := inventoryPage(view)
		if uint64(len(page.Items)) < minimumCandidates || page.Status.Checkpoint != stopped.AcceptedCount || page.Status.StoreID != quiet.Status.StoreID {
			t.Fatal("missing automatic dedicated-gym assignments")
		}
		firstRow := len(rows)
		for _, item := range page.Items {
			if item.Assignment.GymID != page.Status.Pin.Identity.GymID || item.Condition.ProXP != 0 || item.Condition.BornAtMS != item.Assignment.BornAtMS || item.Condition.AsOfMS != page.AsOfMS {
				t.Fatal("untrusted condition/assignment")
			}
			receipt := request("manifest_receipt", item.Assignment.Sequence).ManifestReceipt
			portrait := request("manifest_portrait", item.Assignment.Sequence).Portrait
			if receipt == nil || portrait == nil || receipt.View != *quiet.View || portrait.View != *quiet.View || receipt.Record.Verified.ProspectID != item.Assignment.ProspectID || receipt.Record.Birth.BornAtMS != item.Assignment.BornAtMS || portrait.ID != item.Assignment.ProspectID {
				t.Fatal("page/receipt/portrait mismatch")
			}
			rows = append(rows, publicRow{fixture, *receipt, *portrait})
		}
		continued := false
		for _, winner := range rows[firstRow:] {
			if winner.Receipt.Record.Eligibility.Source != "canonical-winning-block" {
				continue
			}
			for _, later := range rows[firstRow:] {
				if later.Receipt.Record.Sequence > winner.Receipt.Record.Sequence && later.Receipt.Record.Context.ParentHeight > winner.Receipt.Record.Context.ParentHeight {
					continued = true
				}
			}
		}
		if !continued {
			t.Fatal("no accepted later job after canonical winner")
		}
		original := rows[firstRow]
		selected := page.Items[0].Assignment.ProspectID
		second := page.Items[1].Assignment.ProspectID
		intent := prepare(view, 0, 1, selected)
		changed := apply(view, intent.OperationID)
		if !changed.Receipt.Changed || changed.Receipt.Revision != 1 || changed.Receipt.Slots[0] != selected {
			t.Fatal("roster change missing")
		}
		operation := meta("stable_operation", view, intent.OperationID)
		if operation.Operation == nil || operation.Operation.Receipt == nil || *operation.Operation.Receipt != changed.Receipt {
			t.Fatal("durable operation differs")
		}
		replay := apply(view, intent.OperationID)
		if !replay.Replayed || replay.Receipt != changed.Receipt {
			t.Fatal("replay changed original outcome")
		}
		noopIntent := prepare(view, 1, 1, selected)
		noop := apply(view, noopIntent.OperationID)
		if noop.Receipt.Changed || noop.Receipt.Revision != 1 {
			t.Fatal("no-op changed CAS revision")
		}
		for _, bad := range []pilot.StableRequest{
			{Command: "stable_prepare_slot", ViewID: view, ExpectedRevision: 1, Slot: 2, ProspectID: selected},
			{Command: "stable_prepare_slot", ViewID: view, ExpectedRevision: 0, Slot: 1, ProspectID: ""},
		} {
			j := await(begin(bad))
			if j.State != "failed" || j.Error != "stable_conflict" {
				t.Fatal("duplicate/stale request accepted", j.State, j.Error)
			}
		}
		if p := meta("stable_pending", view, ""); p.Operation != nil {
			t.Fatal("rejected prepare left an intent")
		}
		rosterJob := success(begin(pilot.StableRequest{Command: "stable_roster", ViewID: view, FirstSlot: 1, Limit: 8}))
		if rosterJob.Roster == nil || rosterJob.Roster.Status.Slots[0] != selected || len(rosterJob.Roster.Items) != 1 {
			t.Fatal("roster projection mismatch")
		}
		// Selecting a page changes the bounded cache; reselect the original receipt
		// before native export, exactly as the app does.
		inventoryPage(view)
		request("manifest_receipt", original.Receipt.Record.Sequence)
		exportPath := filepath.Join(root, fixture+"-public-receipt.json")
		if _, e = b.ExportManifest(ctx, original.Receipt.Record.Sequence, view, original.Receipt.Record.Verified.ProspectID, exportPath); e != nil {
			t.Fatal(e)
		}
		raw, e := os.ReadFile(exportPath)
		var exported registry.ManifestRecord
		if e != nil || len(raw) > registry.MaxRecordSize+1 || json.Unmarshal(raw, &exported) != nil || !reflect.DeepEqual(exported, original.Receipt.Record) {
			t.Fatal("export differs from selected original", e)
		}
		canonical, _ := json.Marshal(original.Receipt.Record)
		if !bytes.Equal(raw, append(canonical, '\n')) {
			t.Fatal("export canonical bytes")
		}
		if previews != nil {
			previews.run(fixture, "initial", page)
		}
		if entries != nil {
			entries.initial(fixture, snapshot(), page.Items[0])
			if events != nil {
				events.observe(fixture, "initial", snapshot())
				if seeds != nil {
					seeds.observe(fixture, "initial", snapshot())
				}
				if cards != nil {
					cards.observe(fixture, "initial", snapshot())
					if bouts != nil {
						bouts.observe(fixture, "initial", snapshot())
						if results != nil {
							results.observe(fixture, "initial", snapshot())
						}
					}
				}
			}
		}
		beforeLogout := snapshot()
		request("logout", 0)
		waitManifest(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_out" })
		if s := snapshot(); s.View != nil || s.Status != nil {
			t.Fatal("logout stable disclosure")
		}
		if previews != nil {
			previews.refuseDetached(fixture)
		}
		if entries != nil {
			entries.refuseDetached()
			if events != nil {
				events.refuseDetached()
				if seeds != nil {
					seeds.refuseDetached()
				}
				if cards != nil {
					cards.refuseDetached()
					if bouts != nil {
						bouts.refuseDetached()
						if results != nil {
							results.refuseDetached()
						}
					}
				}
			}
		}
		if _, e = b.Request(ctx, "manifest_portrait", original.Portrait.Sequence); e == nil {
			t.Fatal("signed-out portrait disclosed")
		}
		if _, e = b.RequestFixture(ctx, fixture); e != nil {
			t.Fatal(e)
		}
		reopened := waitManifest(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_in" })
		attached := waitStable(func(s pilot.StableSnapshot) bool {
			return s.View != nil && s.Status != nil && s.Status.CaughtUp && s.ActiveJobID == ""
		})
		if attached.View.ViewID == view || attached.Status.StoreID != page.Status.StoreID || attached.Status.RosterRevision != 1 || attached.Status.Slots[0] != selected || attached.Status.Checkpoint != beforeLogout.Status.Checkpoint || reopened.Attempts != stopped.Attempts || reopened.AcceptedCount != stopped.AcceptedCount {
			t.Fatal("reopen replaced stable or mined")
		}
		if previews != nil {
			previews.emptyReopen(fixture, attached.View.ViewID)
		}
		oldView := view
		view = attached.View.ViewID
		if r := stable(pilot.StableRequest{Command: "stable_apply_slot", ViewID: oldView, OperationID: intent.OperationID}); r.OK || r.Error != "stale_view" {
			t.Fatal("native old view dispatch", r.Error)
		}
		historical := meta("stable_operation", view, intent.OperationID)
		if historical.View.ViewID != view || historical.Operation == nil || historical.Operation.Receipt == nil || *historical.Operation.Receipt != changed.Receipt {
			t.Fatal("historical result relabelled")
		}
		restored := inventoryPage(view)
		if restored.Items[0].Assignment != page.Items[0].Assignment {
			t.Fatal("reopen changed immutable assignment")
		}
		again := request("manifest_receipt", original.Receipt.Record.Sequence).ManifestReceipt
		image := request("manifest_portrait", original.Portrait.Sequence).Portrait
		if !reflect.DeepEqual(again.Record, original.Receipt.Record) || image.SVG != original.Portrait.SVG {
			t.Fatal("reopen changed evidence")
		}
		if previews != nil {
			previews.run(fixture, "reopened", restored)
		}
		if entries != nil {
			entries.reopened(fixture, attached)
			if events != nil {
				events.observe(fixture, "reopened", attached)
				if seeds != nil {
					seeds.observe(fixture, "reopened", attached)
				}
				if cards != nil {
					cards.observe(fixture, "reopened", attached)
					if bouts != nil {
						bouts.observe(fixture, "reopened", attached)
						if results != nil {
							results.observe(fixture, "reopened", attached)
						}
					}
				}
			}
		}
		pending := prepare(view, 1, 2, second)
		request("reset_fixture", 0)
		held := waitManifest(func(s pilot.ManifestSnapshot) bool {
			return s.Account.State == "signed_in" && s.Account.Held && s.Account.Generation > reopened.Account.Generation
		})
		heldStable := waitStable(func(s pilot.StableSnapshot) bool {
			return s.View != nil && s.Status != nil && s.Status.CaughtUp && s.ActiveJobID == ""
		})
		heldView := heldStable.View.ViewID
		if held.Account.CanExport || heldStable.Status.StoreID != page.Status.StoreID || heldStable.Status.Slots[0] != selected || heldStable.Status.RosterRevision != 1 || held.PayoutAddress != reopened.PayoutAddress || held.GymID != reopened.GymID {
			t.Fatal("held reset changed identity/roster or restrictions")
		}
		if previews != nil {
			previews.emptyReopen(fixture, heldView)
		}
		oldPending := meta("stable_pending", heldView, "")
		if oldPending.Operation == nil || oldPending.Operation.Intent != pending {
			t.Fatal("pending reset history lost")
		}
		refused := await(begin(pilot.StableRequest{Command: "stable_apply_slot", ViewID: heldView, OperationID: pending.OperationID}))
		if refused.State != "failed" || refused.Error != "stale_view" {
			t.Fatal("old intent executed after reset", refused.State, refused.Error)
		}
		oldOutcome := meta("stable_operation", heldView, intent.OperationID)
		if oldOutcome.Operation == nil || oldOutcome.Operation.Receipt == nil || *oldOutcome.Operation.Receipt != changed.Receipt {
			t.Fatal("reset lost immutable outcome")
		}
		abandoned := meta("stable_abandon_operation", heldView, pending.OperationID)
		if abandoned.Operation == nil || abandoned.Operation.State != "abandoned" {
			t.Fatal("could not retire old pending view")
		}
		allowed := prepare(heldView, 1, 2, second)
		heldChange := apply(heldView, allowed.OperationID)
		if !heldChange.Receipt.Changed || heldChange.Receipt.Revision != 2 || heldChange.Receipt.Slots[0] != selected || heldChange.Receipt.Slots[1] != second {
			t.Fatal("authorized held roster change refused")
		}
		deniedPath := filepath.Join(root, fixture+"-denied-recovery.json")
		if _, e = b.RequestPath(ctx, "export_recovery", deniedPath); e == nil {
			t.Fatal("held recovery exported")
		}
		if _, e = os.Lstat(deniedPath); !os.IsNotExist(e) {
			t.Fatal("denied recovery left a file", e)
		}
		processes.capture(t, fixture, "reset-prepared", held.Account.ID, heldStable.View.AuthorityOrigin, false)
		if previews != nil {
			previews.run(fixture, "held", page)
		}
		if entries != nil {
			entries.held(fixture, snapshot(), page.Items[0])
			if events != nil {
				events.observe(fixture, "held", snapshot())
				if seeds != nil {
					seeds.observe(fixture, "held", snapshot())
				}
				if cards != nil {
					cards.observe(fixture, "held", snapshot())
					if bouts != nil {
						bouts.observe(fixture, "held", snapshot())
						if results != nil {
							results.observe(fixture, "held", snapshot())
						}
					}
				}
			}
		}
		fixedHeldPage := inventoryPage(heldView)
		request("manifest_receipt", original.Receipt.Record.Sequence)
		request("start", 0)
		heldMined := waitManifest(func(s pilot.ManifestSnapshot) bool {
			return s.AcceptedCount > held.AcceptedCount && s.State == "running"
		})
		processes.capture(t, fixture, "held-mining", heldMined.Account.ID, heldMined.View.AuthorityOrigin, true)
		waitStable(func(s pilot.StableSnapshot) bool {
			return s.Status != nil && s.Status.Checkpoint > heldStable.Status.Checkpoint
		})
		protected := request("manifest_receipt", original.Receipt.Record.Sequence).ManifestReceipt
		if protected == nil || !reflect.DeepEqual(protected.Record, original.Receipt.Record) {
			t.Fatal("arrival displaced the protected page/export evidence")
		}
		request("stop", 0)
		recent := success(begin(pilot.StableRequest{Command: "stable_recent", ViewID: heldView}))
		if recent.Page == nil || recent.Page.After != max(recent.Page.Status.Checkpoint, uint64(8))-8 || recent.Page.NextAfter != recent.Page.Status.Checkpoint || recent.Page.HasMore {
			t.Fatal("explicit Recent did not select the latest bounded interval")
		}

		// Publish the newly arrived Recent receipts too, so independent verification
		// can cover every displayed condition/binding, not just the initial page.
		for _, item := range recent.Page.Items {
			known := false
			for _, old := range rows {
				if old.Fixture == fixture && old.Receipt.Record.Verified.ProspectID == item.Assignment.ProspectID {
					known = true
					break
				}
			}
			if known {
				continue
			}
			receipt := request("manifest_receipt", item.Assignment.Sequence).ManifestReceipt
			portrait := request("manifest_portrait", item.Assignment.Sequence).Portrait
			if receipt == nil || portrait == nil || receipt.Record.Verified.ProspectID != item.Assignment.ProspectID || portrait.ID != item.Assignment.ProspectID || portrait.PortraitDigest != item.Assignment.PortraitDigest {
				t.Fatal("new Recent arrival not available for portrait/receipt")
			}
			rows = append(rows, publicRow{fixture, *receipt, *portrait})
		}
		finalStable := snapshot()
		if finalStable.Status == nil || finalStable.Status.RosterRevision != 2 || finalStable.Status.Slots != heldChange.Receipt.Slots {
			t.Fatal("mining changed roster")
		}
		evidence = append(evidence, map[string]any{"fixture": fixture, "initial": synced, "inventory": page, "roster": rosterJob.Roster, "first_mutation": changed, "replay": replay, "noop": noop, "reopened": attached, "retired_old_intent": abandoned, "held_mutation": heldChange, "fixed_held_page": fixedHeldPage, "recent_after_arrival": recent.Page, "final": finalStable, "no_login_mining": true, "current_held_roster_allowed": true, "old_view_intent_refused": true, "recovery_denied_path_absent": true})
		if careerEnabled {
			original := request("snapshot", 0).Snapshot
			if original == nil || original.View == nil || finalStable.View == nil || *original.View != *finalStable.View {
				t.Fatal("pre-activation counter snapshot lost original View")
			}
			careerOriginal[fixture] = *original
		}
		request("logout", 0)
		waitManifest(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_out" })
	}
	if e = b.Close(); e != nil {
		t.Fatal("helper did not close", e)
	}
	cleanupChecked = true
	processes.checkClosed(t)
	if len(processes.nodes) < 8 {
		t.Error("missing owned node evidence for both fixtures/phases")
	}
	if b.cmd.ProcessState == nil || !b.cmd.ProcessState.Exited() || b.cmd.ProcessState.ExitCode() != 0 {
		t.Error("helper exit was not clean")
	}
	var previewRestart map[string]any
	if previews != nil {
		previewRestart = previews.checkProcessRestart(helper, data, node)
	}
	var careerEvidence map[string]any
	if careerEnabled {
		careerEvidence = runCareerPackageAcceptance(t, ctx, helper, data, node, results, careerOriginal)
	}
	if t.Failed() {
		t.Fatal("refusing success evidence after failure")
	}
	result := map[string]any{"status": "passed", "fixture_only": true, "public_chain_mined": false, "gui_exercised": false, "helper_exit": b.cmd.ProcessState.ExitCode(), "helper_pid": b.cmd.Process.Pid, "cleanup": processes.evidence(), "exported_records_equal_selected": true, "denied_recovery_files_absent": true, "rows": rows, "stable": evidence}
	if previews != nil {
		result["previews"] = previews.evidence
		result["preview_restart"] = previewRestart
		result["preview_unrecorded"] = true
	}
	if entries != nil {
		result["entries"] = entries.evidence
		result["entries_intake_only"] = true
	}
	if events != nil {
		result["events"] = events.evidence
		result["event_restart"] = events.restartEvidence
		result["event_real_clock_closed_coverage"] = false
		result["event_seed_authority"] = "unavailable"
	}
	if seeds != nil {
		result["seeds"] = seeds.evidence
		result["seed_restart"] = seeds.restartEvidence
		result["seed_real_clock_closed_coverage"] = false
		result["seed_authority"] = "candidate_fixture_v1"
	}
	if careerEnabled {
		result["careers"] = careerEvidence
		result["career_original_baseline"] = careerOriginal
	}
	if results != nil {
		result["results"] = results.evidence
		result["result_restart"] = results.restartEvidence
		result["result_real_clock_closed_coverage"] = false
		result["result_scope"] = "candidate_result_publication"
		result["result_authority"] = "candidate_fixture_v1"
	}
	if bouts != nil {
		result["bouts"] = bouts.evidence
		result["bout_restart"] = bouts.restartEvidence
		result["bout_real_clock_closed_coverage"] = false
		result["bout_scope"] = "candidate_saved_transcript"
	}
	if cards != nil {
		result["cards"] = cards.evidence
		result["card_restart"] = cards.restartEvidence
		result["card_real_clock_closed_coverage"] = false
		result["card_scope"] = "two_fixture_candidate"
	}
	raw, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "public-result.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}
