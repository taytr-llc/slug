//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

// Retained independent fixture assignments keep their original source sequence.
// Only the public transport framing selects the new shared pin and sparse page.
func sharedTransportPage(t *testing.T) StableJob {
	t.Helper()
	view, status, items := stableTransportReference(t)
	status.Pin.Version = gymstable.SharedPinVersion2
	var err error
	status.StoreID, err = gymstable.ID(status.Pin)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Assignment.Sequence < items[j].Assignment.Sequence })
	items = []gymstable.Item{items[0], items[2]}
	asOf := max(items[0].Condition.AsOfMS, items[1].Condition.AsOfMS)
	for i := range items {
		items[i].Condition, err = fightercondition.Project(1, items[i].Manifest.DNA, items[i].Assignment.BornAtMS, asOf, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	status.Checkpoint, status.ObservedSourceCount, status.CaughtUp, status.LastAsOfMS = 10, 10, true, asOf
	page := gymstable.Page{Version: 1, Status: status, AsOfMS: asOf, After: 0, NextAfter: 10, Items: items}
	return StableJob{Version: 1, JobID: strings.Repeat("f", 64), Kind: "inventory", State: "done", View: view, StoreID: status.StoreID, Page: &page}
}

func TestSharedStableSparsePageTypedBoundaries(t *testing.T) {
	j := sharedTransportPage(t)
	if !stableJobValid(j) {
		t.Fatal("valid sparse terminal page rejected")
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &j})
	j.Page.HasMore, j.Page.NextAfter = true, 3
	if !stableJobValid(j) {
		t.Fatal("valid sparse continuation rejected")
	}
	j.Page.Items = []gymstable.Item{}
	j.Page.After, j.Page.NextAfter, j.Page.HasMore = 3, 10, false
	if !stableJobValid(j) {
		t.Fatal("foreign-only suffix rejected")
	}
	for name, change := range map[string]func(*StableJob){
		"legacy_pin": func(j *StableJob) {
			j.Page.Status.Pin.Version = 1
			j.Page.Status.StoreID, _ = gymstable.ID(j.Page.Status.Pin)
			j.StoreID = j.Page.Status.StoreID
		},
		"unknown_pin":                   func(j *StableJob) { j.Page.Status.Pin.Version = 3 },
		"duplicate_sequence":            func(j *StableJob) { j.Page.Items[1].Assignment.Sequence = j.Page.Items[0].Assignment.Sequence },
		"descending_sequence":           func(j *StableJob) { j.Page.Items[0], j.Page.Items[1] = j.Page.Items[1], j.Page.Items[0] },
		"item_at_cursor":                func(j *StableJob) { j.Page.After = 1 },
		"item_beyond_checkpoint":        func(j *StableJob) { j.Page.Items[1].Assignment.Sequence = 11 },
		"terminal_before_global_cursor": func(j *StableJob) { j.Page.NextAfter = 3 },
		"more_after_global_cursor":      func(j *StableJob) { j.Page.HasMore = true },
		"more_with_wrong_local_cursor":  func(j *StableJob) { j.Page.HasMore, j.Page.NextAfter = true, 2 },
		"empty_more":                    func(j *StableJob) { j.Page.Items = []gymstable.Item{}; j.Page.HasMore, j.Page.NextAfter = true, 1 },
		"foreign_gym":                   func(j *StableJob) { j.Page.Items[1].Assignment.GymID = strings.Repeat("e", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := sharedTransportPage(t)
			change(&bad)
			raw, err := json.Marshal(StableResponse{ID: 1, OK: true, Job: &bad})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseStableResponse(raw); err == nil {
				t.Fatal("invalid sparse relation accepted")
			}
		})
	}
}

func TestSharedStableSyncAllowsForeignOnlyProgress(t *testing.T) {
	j := sharedTransportPage(t)
	p := j.Page
	j.Kind, j.Page = "sync", nil
	j.Sync = &gymstable.SyncResult{Version: 1, Status: p.Status, AsOfMS: p.AsOfMS, Items: []gymstable.Item{}}
	if !stableJobValid(j) {
		t.Fatal("empty local sync at global cursor rejected")
	}
	// Source sequence 3 is within the last eight global records at cursor10.
	j.Sync.Items = []gymstable.Item{p.Items[1]}
	if !stableJobValid(j) {
		t.Fatal("sparse bounded sync rejected")
	}
	j.Sync.Items = []gymstable.Item{p.Items[0]}
	if stableJobValid(j) {
		t.Fatal("sync disclosed a record outside its bounded source window")
	}
}

func TestSharedCareerPagePreservesSparseSourceAndOverlay(t *testing.T) {
	r := careerTransportFixture(t)
	p := &r.Page.Source
	p.Status.Pin.Version = gymstable.SharedPinVersion2
	var err error
	p.Status.StoreID, err = gymstable.ID(p.Status.Pin)
	if err != nil {
		t.Fatal(err)
	}
	r.StoreID = p.Status.StoreID
	p.Status.Checkpoint, p.Status.ObservedSourceCount = 10, 10
	p.After, p.NextAfter, p.HasMore = 0, 10, false
	raw, err := MarshalCareerResponse(r)
	if err != nil {
		t.Fatal("valid sparse career page rejected", err)
	}
	got, err := ParseCareerResponse(raw)
	if err != nil || got.Page.Source.Items[0].Assignment != p.Items[0].Assignment || !reflect.DeepEqual(got.Page.Items, r.Page.Items) {
		t.Fatal("career roundtrip changed source identity or XP overlay", err)
	}
	p.Status.Pin.Version = 1
	p.Status.StoreID, _ = gymstable.ID(p.Status.Pin)
	r.StoreID = p.Status.StoreID
	if _, err := MarshalCareerResponse(r); err == nil {
		t.Fatal("shared pagination silently changed legacy career grammar")
	}
}

func sharedDesktopLaneFixture(t *testing.T) (*StableController, []registry.ManifestRecord) {
	t.Helper()
	s, records := stableLaneFixture(t)
	if err := s.stableStore.Close(); err != nil {
		t.Fatal(err)
	}
	pin := s.stableStatus.Pin
	pin.Version = gymstable.SharedPinVersion2
	store, err := gymstable.CreateShared(filepath.Join(s.config.DataDir, "shared-stable"), pin, s.source)
	if err != nil {
		t.Fatal(err)
	}
	s.stableStore = store
	status, err := store.Status(t.Context(), s.session())
	if err != nil {
		t.Fatal(err)
	}
	s.stableStatus = &status
	return s, records
}

func sharedRecentJob(t *testing.T, s *StableController) StableJob {
	t.Helper()
	ack, err := s.BeginStableRecent(t.Context(), s.view.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cycle(t.Context()); err != nil {
		t.Fatal(err)
	}
	job, err := s.StableJob(t.Context(), s.view.ViewID, ack.JobID)
	if err != nil || job.State != "done" || job.Page == nil {
		t.Fatal("shared recent lane", job.State, job.Error, err)
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &job})
	return job
}

func TestSharedStableLaneKeepsOriginalReceiptsOutsideLegacyProjection(t *testing.T) {
	s, records := sharedDesktopLaneFixture(t)
	if err := s.cycle(t.Context()); err != nil {
		t.Fatal(err)
	}
	job := sharedRecentJob(t, s)
	if len(job.Page.Items) != 3 || job.Page.NextAfter != 3 || job.Page.HasMore {
		t.Fatal("shared local-tail page incomplete")
	}
	bySequence := map[uint64]registry.ManifestRecord{}
	for _, r := range records {
		bySequence[r.Sequence] = r
	}
	for _, item := range job.Page.Items {
		if item.Assignment.ProspectID != bySequence[item.Assignment.Sequence].Verified.ProspectID || item.Receipt().Version != 0 {
			t.Fatal("source sequence changed or private receipt entered public job")
		}
	}
	s.mu.Lock()
	legacy := s.snapshot
	cached, selected := len(s.cache), s.stableSelected
	s.mu.Unlock()
	if !selected || cached != 3 || legacy.AcceptedCount != 0 || legacy.Projection.State != "idle" || len(legacy.Projection.Items) != 0 || legacy.Projection.ThroughSequence != 0 || legacy.Projection.CompleteRecent {
		t.Fatal("shared global cursor was published as legacy fighter count/projection")
	}
	receipt, err := s.ManifestReceipt(t.Context(), 3)
	if err != nil || !reflect.DeepEqual(receipt.Record, bySequence[3]) {
		t.Fatal("private receipt lost original source identity", err)
	}
	s.view.ViewLease = func(context.Context, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if receipt, err := s.ManifestReceipt(t.Context(), 3); err == nil || receipt.Version != 0 {
		t.Fatal("revoked original view disclosed shared receipt")
	}
}

func TestSharedCareerRecentRetainsExactOpaqueSelectionAndViewFence(t *testing.T) {
	s, records := sharedDesktopLaneFixture(t)
	if err := s.cycle(t.Context()); err != nil {
		t.Fatal(err)
	}
	at := records[0].Birth.BornAtMS
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: at, SourceCreatedAtMS: at, ActivatedAtMS: at}
	a.ActivationID = careerstate.ActivationID(a)
	scope := careerstate.Scope{StableStoreIDs: [2]string{s.stableStatus.StoreID, strings.Repeat("2", 64)}, CardStoreID: strings.Repeat("3", 64)}
	reader, err := careerstate.NewReader(a, scope, func(context.Context) (careerstate.State, error) {
		return careerstate.State{Activation: a, Scope: scope, ClockMS: time.Now().UnixMilli(), Events: []careerstate.Event{}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindCareer(t.Context(), reader, gymstable.OpenSharedCareer); err != nil {
		t.Fatal(err)
	}
	job := sharedRecentJob(t, s)
	page, err := s.CareerInventory(t.Context(), s.view.ViewID, job.JobID, job.Page.After, 8)
	if err != nil || page.Source.Status.Pin.Version != gymstable.SharedPinVersion2 || page.Source.After != job.Page.After || len(page.Items) != 3 {
		t.Fatal("shared career local-tail handle unavailable", err)
	}
	for _, item := range page.Items {
		if item.Totals.XP != 0 || item.Condition.ProXP != 0 {
			t.Fatal("shared source introduced a career award")
		}
	}
	if _, err := s.CareerInventory(t.Context(), s.view.ViewID, job.JobID, job.Page.After+1, 8); !errors.Is(err, gymstable.ErrNotFound) {
		t.Fatal("opaque recent handle accepted another page selection", err)
	}
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if page, err := s.CareerInventory(t.Context(), s.view.ViewID, job.JobID, job.Page.After, 8); err == nil || page.Version != 0 {
		t.Fatal("revoked original view disclosed captured shared career page")
	}
}
