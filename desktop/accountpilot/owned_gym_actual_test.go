//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recovery"
)

func ownedActualID(label string) string {
	v := sha256.Sum256([]byte("SLUG Set73 whole-mode public fixture action\x00" + label))
	return hex.EncodeToString(v[:])
}
func ownedActualJoined(t *testing.T, ctx context.Context, c *OwnedGymController) {
	t.Helper()
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		t.Fatal("missing original parent operation")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("parent join", ctx.Err())
	}
}
func ownedActualLogin(t *testing.T, ctx context.Context, c *OwnedGymController, fixture string) pilot.OwnedGymSnapshot {
	t.Helper()
	if e := c.LoginFixture(fixture); e != nil {
		t.Fatal("login admission", e)
	}
	ownedActualJoined(t, ctx, c)
	s := c.Snapshot()
	scope, e := c.Scope(ctx, fixture)
	if e != nil || s.State != "signed_in" || s.View == nil || s.Child == nil || scope.Source != c.init.Source || pilot.ValidateOwnedGymScope(scope) != nil || pilot.ValidateOwnedGymSnapshot(s, c.init.Source, scope.EventPin) != nil {
		t.Fatal("original owned login", s.State, s.Error, e)
	}
	return s
}
func ownedActualLogout(t *testing.T, ctx context.Context, c *OwnedGymController, view string) {
	t.Helper()
	if e := c.Logout(ctx, view); e != nil {
		t.Fatal("original logout", e)
	}
	ownedActualJoined(t, ctx, c)
	if s := c.Snapshot(); s.State != "signed_out" || s.View != nil || s.Child != nil {
		t.Fatal("logout retained visible owner", s.State)
	}
}
func ownedActualRetry[T any](t *testing.T, ctx context.Context, fn func() (T, error)) T {
	t.Helper()
	deadline, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	for {
		v, e := fn()
		if e == nil {
			return v
		}
		if ownedActualReadCategory(e) != "busy" {
			t.Fatal("definite operation failure; no blind retry", e)
		}
		select {
		case <-deadline.Done():
			t.Fatal("bounded busy retry", deadline.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Only explicit read-side contention/catch-up leaves are retryable. Walk every
// joined error so a Busy wrapper cannot hide corruption or source mismatch.
func ownedActualReadCategory(e error) string {
	if e == nil {
		return "ready"
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		if len(many.Unwrap()) == 0 {
			return "definite"
		}
		for _, child := range many.Unwrap() {
			if child != nil && ownedActualReadCategory(child) == "definite" {
				return "definite"
			}
		}
		return "transient_join"
	}
	if one, ok := e.(interface{ Unwrap() error }); ok {
		if child := one.Unwrap(); child != nil {
			return ownedActualReadCategory(child)
		}
	}
	switch e {
	case gymstable.ErrBusy, gymentry.ErrBusy, pilot.ErrPoolRuntimeBusy, fighterownership.ErrBusy, registry.ErrBusy:
		return "busy"
	case fighterownership.ErrUnavailable:
		return "public_evidence_unavailable"
	case fighterownership.ErrStale, gymstable.ErrSyncRequired, gymentry.ErrSyncRequired:
		return "catch_up"
	default:
		return "definite"
	}
}

func ownedActualCapture(t *testing.T, ctx context.Context, c *OwnedGymController, s pilot.OwnedGymSnapshot, label string, min int) pilot.OwnedStableCapture {
	t.Helper()
	var got pilot.OwnedStableCapture
	var last error
	lastCategory := "not_polled"
	attempt := 0
	poolActualWait(t, ctx, "complete owned capture", func(ctx context.Context) bool {
		attempt++
		got, last = c.CaptureStable(ctx, s.View.ViewID, ownedActualID(label+"/fresh/"+strconv.Itoa(attempt)))
		lastCategory = ownedActualReadCategory(last)
		if lastCategory == "definite" {
			t.Fatal("complete owned capture definite failure", last)
		}
		if last == nil && len(got.Page.Items) < min {
			lastCategory = "complete_capture_below_expected_count"
		}
		return last == nil && len(got.Page.Items) >= min
	}, func() any {
		return struct {
			Category string
			Error    error
			Items    int
		}{lastCategory, last, len(got.Page.Items)}
	})
	if pilot.ValidateOwnedStableCapture(got, c.init.Source, *s.View) != nil {
		t.Fatal("invalid original capture")
	}
	for _, item := range got.Page.Items {
		if item.Ownership.OwnerPublicKey != s.View.GymPublicKey || item.Birth.ProspectID != item.Ownership.ProspectID {
			t.Fatal("foreign owner disclosed")
		}
	}
	return got
}
func ownedActualEntry(t *testing.T, ctx context.Context, c *OwnedGymController, s pilot.OwnedGymSnapshot, capture pilot.OwnedStableCapture) gymentry.OwnedReceipt {
	t.Helper()
	if len(capture.Page.Items) == 0 {
		t.Fatal("missing actual fighter")
	}
	item := capture.Page.Items[0]
	slot := ownedActualRetry(t, ctx, func() (gymstable.OwnedIntent, error) {
		return c.PrepareSlot(ctx, s.View.ViewID, gymstable.OwnedSlotRequest{ExpectedRevision: capture.Page.Status.RosterRevision, Slot: 1, ProspectID: item.Birth.ProspectID, ExpectedStateID: item.Ownership.StateID, Head: capture.Page.Head})
	})
	applied := ownedActualRetry(t, ctx, func() (gymstable.OwnedMutationResult, error) {
		return c.ApplySlot(ctx, s.View.ViewID, slot.OperationID)
	})
	if applied.Receipt.Slots[0] != item.Birth.ProspectID {
		t.Fatal("original roster mutation mismatch")
	}
	upcoming := ownedActualRetry(t, ctx, func() (gymentry.OwnedEventStatus, error) { return c.EntryUpcoming(ctx, s.View.ViewID) })
	if upcoming.Event.CutoffMS-time.Now().UnixMilli() < 10*60*1000 {
		t.Fatal("baseline too close to real noon; requires an isolated fresh later attempt")
	}
	request := gymentry.Request{Version: 1, EventID: upcoming.Event.EventID, Saturday: upcoming.Event.Saturday, Action: "replace", ExpectedRevision: upcoming.Revision, ExpectedRosterRevision: applied.Receipt.Revision, ProspectIDs: []string{item.Birth.ProspectID}}
	intent := ownedActualRetry(t, ctx, func() (gymentry.OwnedIntent, error) { return c.PrepareEntry(ctx, s.View.ViewID, request) })
	result := ownedActualRetry(t, ctx, func() (gymentry.OwnedMutationResult, error) {
		return c.ApplyEntry(ctx, s.View.ViewID, intent.OperationID)
	})
	if len(result.Receipt.Members) != 1 || result.Receipt.Members[0].Birth != item.Birth || result.Receipt.OperationID != intent.OperationID {
		t.Fatal("entry changed historical birth")
	}
	return result.Receipt
}
func ownedActualVerify(t *testing.T, ctx context.Context, c *OwnedGymController, root, path, view string) {
	t.Helper()
	selection, e := recoveryfile.Capture(path, []string{root}, recoveryfile.ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if closeErr := selection.Close(); closeErr != nil {
			t.Error("retained native selection cleanup", closeErr)
		}
	}()
	ownedActualRetry(t, ctx, func() (struct{}, error) {
		return struct{}{}, c.VerifyBackup(ctx, view, selection.Pin())
	})
	e = selection.Close()
	if e != nil {
		t.Fatal("actual backup admission", e)
	}
	poolActualWait(t, ctx, "actual owned backup verify", func(context.Context) bool {
		v := c.Snapshot()
		if v.View == nil || v.View.ViewID != view || v.Child == nil {
			t.Fatal("backup lost original view")
		}
		b := v.Child.Backup
		if b.State == "failed" || b.State == "cancelled" {
			t.Fatal("backup failed", b.Error)
		}
		return b.State == "succeeded"
	})
}
func ownedActualExport(t *testing.T, ctx context.Context, c *OwnedGymController, root, path string, s pilot.OwnedGymSnapshot) {
	t.Helper()
	selection, e := recoveryfile.Capture(path, []string{root}, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if closeErr := selection.Close(); closeErr != nil {
			t.Error("retained native selection cleanup", closeErr)
		}
	}()
	ownedActualRetry(t, ctx, func() (struct{}, error) {
		return struct{}{}, c.ExportRecovery(ctx, s.View.ViewID, selection.Pin())
	})
	e = selection.Close()
	if e != nil {
		t.Fatal("actual owned export", e)
	}
	st, e := os.Stat(path)
	if e != nil || st.Mode().Perm() != 0600 || st.Size() > recovery.MaxExportSize {
		t.Fatal("private file framing")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("fixture export read")
	}
	bundle, e := recovery.ParseExport(raw)
	clear(raw)
	if e != nil {
		t.Fatal("fixture export parse")
	}
	bundle.Mnemonic = ""
	m := bundle.Manifest
	if m.RootPublicKey != s.View.RootPublicKey || m.GymPublicKey != s.View.GymPublicKey || m.GymID != s.View.GymID || len(m.Accounts) != 4 {
		t.Fatal("export public identity")
	}
	for _, a := range m.Accounts {
		want := uint32(0)
		if a.Purpose == 84 {
			want = 1
		}
		if a.Coin != 0 || a.Account != 0 || a.NextExternal != want || a.NextInternal != 0 {
			t.Fatal("baseline export omitted exact pool-recipient allocation")
		}
	}
	ownedActualVerify(t, ctx, c, root, path, s.View.ViewID)
}

// This first actual gate deliberately precedes the 100+60-block funded transfer
// continuation. It qualifies the shipped concrete constructor/child seams, not
// transfer finality, modeled noon, UI consent or a complete league award cycle.
func TestOwnedGymActualControllerBaseline(t *testing.T) {
	artifacts, binary := os.Getenv("SLUG_OWNED_GYM_ACTUAL_ROOT"), os.Getenv("SLUG_OWNED_GYM_ACTUAL_NODE")
	if artifacts == "" || binary == "" {
		t.Skip("explicit fresh fixture artifact root and normal node required")
	}
	if !filepath.IsAbs(artifacts) || !filepath.IsAbs(binary) {
		t.Fatal("absolute fixture paths required")
	}
	root := filepath.Join(artifacts, "owned-gym")
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("fresh exclusive owned root required")
	}
	files := filepath.Join(artifacts, "selected-files")
	if e := os.Mkdir(files, 0700); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cfg := pilot.Config{DataDir: root, NodeBinary: binary}
	c, e := NewOwnedGymPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("fresh whole-mode constructor", e)
	}
	defer func() {
		if c != nil {
			if e := c.Close(); e != nil {
				t.Error("actual complete owner cleanup", e)
			}
		}
	}()
	init := c.init
	if s := c.Snapshot(); s.Mode != pilot.OwnedGymMode || s.State != "signed_out" || s.Source != init.Source {
		t.Fatal("explicit mode startup")
	}
	rpc := poolActualNode(t, root)
	defer rpc.client.CloseIdleConnections()
	var height uint32
	if e = rpc.call(ctx, "getblockcount", nil, &height); e != nil || height != 0 {
		t.Fatal("constructor mined", height, e)
	}
	scopes := map[string]pilot.OwnedGymScope{}
	captures := map[string]pilot.OwnedStableCapture{}
	entries := map[string]gymentry.OwnedReceipt{}
	exports := map[string]string{}
	priorView := ""
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := ownedActualLogin(t, ctx, c, fixture)
		scope, e := c.Scope(ctx, fixture)
		if e != nil {
			t.Fatal(e)
		}
		scopes[fixture] = scope
		if s.Child.State != "stopped" || s.Child.Attempts != 0 {
			t.Fatal("login started miner")
		}
		if priorView != "" {
			if e = c.Start(ctx, priorView); e == nil {
				t.Fatal("old fixture view retargeted Start")
			}
		}
		before := ownedActualRetry(t, ctx, func() (pilot.OwnedFixtureCounters, error) {
			return c.runtime.OwnedFixtureCountersForTest(ctx, c.init.Source)
		})
		ownedActualRetry(t, ctx, func() (struct{}, error) {
			return struct{}{}, c.Start(ctx, s.View.ViewID)
		})
		var lastCounterErr error
		lastCounterCategory := "not_polled"
		var lastCounters pilot.OwnedFixtureCounters
		poolActualWait(t, ctx, "actual retained owned admission", func(ctx context.Context) bool {
			lastCounters, lastCounterErr = c.runtime.OwnedFixtureCountersForTest(ctx, c.init.Source)
			lastCounterCategory = ownedActualReadCategory(lastCounterErr)
			if lastCounterCategory == "definite" {
				t.Fatal("actual admission counter definite failure", lastCounterErr)
			}
			if lastCounterErr == nil && lastCounters.Records <= before.Records {
				lastCounterCategory = "awaiting_new_retained_admission"
			}
			return lastCounterErr == nil && lastCounters.Records > before.Records
		}, func() any {
			return struct {
				Category string
				Error    error
				Counters pilot.OwnedFixtureCounters
			}{lastCounterCategory, lastCounterErr, lastCounters}
		})
		ownedActualRetry(t, ctx, func() (struct{}, error) {
			return struct{}{}, c.Stop(ctx, s.View.ViewID)
		})
		// A fresh capture observes the final complete source after stopped work.
		capture := ownedActualCapture(t, ctx, c, s, "stopped/"+fixture, 1)
		captures[fixture] = capture
		entries[fixture] = ownedActualEntry(t, ctx, c, s, capture)
		path := filepath.Join(files, fixture+".json")
		exports[fixture] = path
		ownedActualExport(t, ctx, c, root, path, s)
		priorView = s.View.ViewID
		ownedActualLogout(t, ctx, c, priorView)
		t.Log("actual owned mining/roster/entry/export/Verify joined", fixture)
	}
	if scopes["zero256"].Source != scopes["incrementing256"].Source || scopes["zero256"].StableStoreID == scopes["incrementing256"].StableStoreID {
		t.Fatal("shared source/isolated owned store mismatch")
	}
	if e = c.Close(); e != nil {
		t.Fatal("close before exact reopen", e)
	}
	c = nil
	c, e = NewOwnedGymPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("existing owned-mode reopen", e)
	}
	if !reflect.DeepEqual(c.init, init) {
		t.Fatal("reopen repinned original namespace")
	}
	s := ownedActualLogin(t, ctx, c, "zero256")
	scope, e := c.Scope(ctx, "zero256")
	if e != nil || !reflect.DeepEqual(scope, scopes["zero256"]) {
		t.Fatal("reopen changed original scope", e)
	}
	capture := ownedActualCapture(t, ctx, c, s, "reopened/zero256", 1)
	old := captures["zero256"].Page.Items[0]
	found := false
	for _, item := range capture.Page.Items {
		if item.Birth.ProspectID == old.Birth.ProspectID {
			found = true
			if item.Birth != old.Birth || !reflect.DeepEqual(item.Admission, old.Admission) {
				t.Fatal("reopen rewrote birth/admission")
			}
		}
	}
	if !found {
		t.Fatal("reopen lost owned fighter")
	}
	retained := ownedActualRetry(t, ctx, func() (gymentry.OwnedOperation, error) {
		return c.EntryOperation(ctx, s.View.ViewID, entries["zero256"].OperationID)
	})
	if retained.Receipt == nil || !reflect.DeepEqual(*retained.Receipt, entries["zero256"]) {
		t.Fatal("reopen rewrote committed entry")
	}
	oldView := s.View.ViewID
	if e = c.ResetFixtureAccount(ctx, oldView); e != nil {
		t.Fatal("signed fixture reset admission", e)
	}
	ownedActualJoined(t, ctx, c)
	held := c.Snapshot()
	if held.State != "signed_in" || held.View == nil || held.Child == nil || !held.Account.Held || held.View.ViewID == oldView || held.View.GymPublicKey != s.View.GymPublicKey || held.View.RootPublicKey != s.View.RootPublicKey {
		t.Fatal("reset changed identity or lost hold", held.State, held.Error)
	}
	if _, e = c.CaptureStable(ctx, oldView, ownedActualID("stale-after-reset")); e == nil {
		t.Fatal("old view disclosed owned set")
	}
	if e = c.accounts.WithSession(ctx, c.session.Token, gymaccount.FighterTransfer, func(gymaccount.Account, func() error) error { return nil }); !errors.Is(e, gymaccount.ErrHeld) {
		t.Fatal("held original transfer authority", e)
	}
	rejected := filepath.Join(files, "held-refused.json")
	selection, e := recoveryfile.Capture(rejected, []string{root}, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	e = c.ExportRecovery(ctx, held.View.ViewID, selection.Pin())
	closeErr := selection.Close()
	if e == nil || closeErr != nil {
		t.Fatal("held export allowed or destination cleanup failed", e, closeErr)
	}
	if _, e = os.Lstat(rejected); !os.IsNotExist(e) {
		t.Fatal("held export created a file")
	}
	ownedActualCapture(t, ctx, c, held, "held-view", 1)
	ownedActualVerify(t, ctx, c, root, exports["zero256"], held.View.ViewID)
	if e = c.Close(); e != nil {
		t.Fatal("final actual cleanup", e)
	}
	c = nil
	t.Log("owned controller baseline complete; transfer/noon and native package acceptance remain separate")
}
