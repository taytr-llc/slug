//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
)

func poolCycleChildFixture(t *testing.T) (*PoolChild, *poolDesktopHarness) {
	t.Helper()
	c, h, events := poolLeagueChildFixture(t)
	s, r := c.stable, c.resources
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sp, err := gymseed.CandidatePin(r.League.EventPin)
	if err != nil {
		t.Fatal(err)
	}
	seeds, err := gymseed.Create(t.Context(), filepath.Join(root, "seeds"), sp, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = seeds.Close() })
	cp, _ := gymcard.CandidatePin(sp)
	cards, err := gymcard.Create(t.Context(), filepath.Join(root, "cards"), cp, seeds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cards.Close() })
	bp, _ := gymbout.CandidatePin(cp)
	bouts, err := gymbout.Create(t.Context(), filepath.Join(root, "bouts"), bp, cards)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bouts.Close() })
	rp, _ := gymresult.CandidatePin(bp)
	resultsPath := filepath.Join(root, "results")
	results, err := gymresult.Create(t.Context(), resultsPath, rp, bouts)
	if err != nil {
		t.Fatal(err)
	}
	if err = results.Close(); err != nil {
		t.Fatal(err)
	}
	if err = gymresult.CheckCareerActivation(t.Context(), resultsPath, rp, bouts); err != nil {
		t.Fatal(err)
	}
	results, err = gymresult.ActivateCareer(t.Context(), resultsPath, rp, bouts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(); _ = results.Close() })
	reader, err := results.CareerReader()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.stableStore.BindCareer(t.Context(), reader); err != nil {
		t.Fatal(err)
	}
	if err = cards.BindCareer(t.Context(), reader); err != nil {
		t.Fatal(err)
	}
	si, _ := seeds.Info(t.Context())
	ci, _ := cards.Info(t.Context())
	bi, _ := bouts.Info(t.Context())
	ri, _ := results.Info(t.Context())
	activation, err := results.CareerInfo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	r.Pipeline = &poolPipelineChildBinding{SeedPin: sp, SeedStoreID: si.StoreID, SeedCreatedMS: si.CreatedAtMS, SeedSourceCreatedMS: r.League.EventCreatedMS, CardPin: cp, CardStoreID: ci.StoreID, CardCreatedMS: ci.CreatedAtMS, CardSourceCreatedMS: ci.SourceCreatedAtMS, BoutPin: bp, BoutStoreID: bi.StoreID, BoutCreatedMS: bi.CreatedAtMS, BoutSourceCreatedMS: bi.SourceCreatedAtMS, ResultPin: rp, ResultStoreID: ri.StoreID, ResultCreatedMS: ri.CreatedAtMS, ResultSourceCreatedMS: ri.SourceCreatedAtMS, Career: activation}
	r.Seeds, r.Cards, r.Bouts, r.Results, r.CareerReader = seeds, cards, bouts, results, reader
	r.Life = c.ctx
	r.Publisher = &seednode.SharedPoolOwnerBinding{Source: r.League.EventPin, EventStoreID: r.League.EventStoreID, EventCreatedAtMS: r.League.EventCreatedMS, Pool: r.Source, PoolBindingID: r.League.SourceBindingID, Issuer: r.Issuer, Events: events, Life: r.Life}
	c.resources = r
	if err = s.closePair(); err != nil {
		t.Fatal(err)
	}
	s.installed = false
	if err = s.preparePoolPipeline(r); err != nil {
		t.Fatal(err)
	}
	if err = s.openPair(nil); err != nil {
		t.Fatal(err)
	}
	status, err := s.stableStore.Status(t.Context(), s.session())
	if err != nil {
		t.Fatal(err)
	}
	s.stableStatus, s.installed = &status, true
	if err = s.attachPoolPipeline(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	s.stableConfig.ResultNodeLease = h.account.ViewLease
	return c, h
}

func TestPoolCycleChildExactAuthorityAndBorrowedLifetime(t *testing.T) {
	c, h := poolCycleChildFixture(t)
	r := c.resources
	for name, change := range map[string]func(*poolChildResources){
		"missing reader":    func(v *poolChildResources) { v.CareerReader = nil },
		"missing publisher": func(v *poolChildResources) { v.Publisher = nil },
		"result identity":   func(v *poolChildResources) { v.Pipeline.ResultStoreID = strings.Repeat("a", 64) },
		"career activation": func(v *poolChildResources) { v.Pipeline.Career.ResultStoreID = strings.Repeat("b", 64) },
		"source creation":   func(v *poolChildResources) { v.Pipeline.CardSourceCreatedMS++ },
		"publisher source":  func(v *poolChildResources) { v.Publisher.PoolBindingID = strings.Repeat("c", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			v := r
			x := *r.Pipeline
			v.Pipeline = &x
			p := *r.Publisher
			v.Publisher = &p
			change(&v)
			if _, err := validatePoolPipelineChild(v); err == nil {
				t.Fatal("substituted authority accepted")
			}
		})
	}
	seed, card, bout, result := c.SeedSnapshot(), c.CardSnapshot(), c.BoutSnapshot(), c.ResultSnapshot()
	if seed.StoreID != r.Pipeline.SeedStoreID || card.StoreID != r.Pipeline.CardStoreID || bout.StoreID != r.Pipeline.BoutStoreID || result.StoreID != r.Pipeline.ResultStoreID {
		t.Fatal("wrong pipeline snapshots")
	}
	seed.Pin.SourcePin.Participants[0].StablePin.AccountID = strings.Repeat("a", 64)
	if reflect.DeepEqual(seed.Pin, c.SeedSnapshot().Pin) {
		t.Fatal("snapshot aliases retained source")
	}
	// Metadata publishes under a lease, but construction and source audits do not.
	if h.sessions.active.Load() {
		t.Fatal("account lease remained held")
	}
	h.mu.Lock()
	h.view = false
	h.mu.Unlock()
	if _, err := c.CareerView(c.account.Manifest.ViewID); err == nil {
		t.Fatal("revoked view disclosed career authority")
	}
	if c.ResultSnapshot().View != nil {
		t.Fatal("revoked view disclosed result authority")
	}
	h.mu.Lock()
	h.view = true
	h.mu.Unlock()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func() error{
		"seed": func() error { _, e := r.Seeds.Info(t.Context()); return e }, "card": func() error { _, e := r.Cards.Info(t.Context()); return e }, "bout": func() error { _, e := r.Bouts.Info(t.Context()); return e }, "result": func() error { _, e := r.Results.Info(t.Context()); return e },
	} {
		if err := read(); err != nil {
			t.Fatal("child closed borrowed", name, err)
		}
	}
	if c.ResultSnapshot().View != nil || c.SeedSnapshot().View != nil {
		t.Fatal("closed view disclosed pipeline")
	}
}

func TestPoolCycleChildCareerReopenRejectsOtherActivation(t *testing.T) {
	c, _ := poolCycleChildFixture(t)
	s := c.stable
	if err := s.closePair(); err != nil {
		t.Fatal(err)
	}
	if err := s.openPair(nil); err != nil {
		t.Fatal("exact career reopen", err)
	}
	if _, err := s.stableStore.CareerInventory(t.Context(), s.session(), 0, 8); err != nil {
		t.Fatal("reader not reattached", err)
	}
	if err := s.closePair(); err != nil {
		t.Fatal(err)
	}
	// A ready opener must never call the installer. A different activation is
	// rejected by the durable original guard, even with the same stable/source.
	a := c.resources.Pipeline.Career
	a.ActivatedAtMS++
	if x, err := c.resources.Factories.openSharedCareer(filepath.Join(s.config.DataDir, "stable"), *s.sharedPin, s.sharedSource, a); err == nil {
		_ = x.Close()
		t.Fatal("changed activation opened ready guard")
	}
	if x, err := gymstable.OpenShared(filepath.Join(s.config.DataDir, "stable"), *s.sharedPin, s.sharedSource); err == nil {
		_ = x.Close()
		t.Fatal("legacy open bypassed career guard")
	}
}

func TestPoolCycleChildBackupGateAndNoNodeSpawn(t *testing.T) {
	c, _ := poolCycleChildFixture(t)
	view := c.account.Manifest.ViewID
	op, err := c.beginPoolBackup(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, begin := range []func() error{
		func() error { _, e := c.BeginSeedPrepare(t.Context(), view, "2026-11-28"); return e },
		func() error { _, e := c.BeginCardBuild(t.Context(), view, "2026-11-28"); return e },
		func() error {
			_, e := c.BeginBoutRun(t.Context(), view, "2026-11-28", strings.Repeat("a", 64))
			return e
		},
		func() error {
			_, e := c.BeginResultPublish(t.Context(), view, "2026-11-28", strings.Repeat("a", 64))
			return e
		},
		func() error { _, e := c.BeginCareerHistory(t.Context(), view, strings.Repeat("a", 64)); return e },
	} {
		if e := begin(); !errors.Is(e, gymstable.ErrBusy) {
			t.Fatal("backup allowed new lane work", e)
		}
	}
	op.cancel()
	c.finishPoolBackup(op, "")
	s := c.stable
	// The runtime is offline in this fixture. Explicit readiness must return
	// node-offline without creating a child node-owner epoch or process pair.
	s.account.AccountID = s.resultPin.PublisherAccountID
	if e := s.ensureResultNodes(t.Context()); !errors.Is(e, gymresult.ErrNodeOffline) {
		t.Fatal("shared readiness", e)
	}
	if s.resultNodeOwner != nil || s.seedNodes != nil || s.cancel != nil {
		t.Fatal("shared action spawned solo ownership")
	}
	// Cached liveness can report the parent owner, but never blesses it for an
	// actual adapter borrow. Constructor ownership checks remain separate.
	s.sharedPublisher.Primary = seednode.OwnedProcess{Cmd: &exec.Cmd{}, Done: make(chan struct{})}
	s.sharedPublisher.Peer = seednode.OwnedProcess{Cmd: &exec.Cmd{}, Done: make(chan struct{})}
	got := c.ResultSnapshot()
	if got.NodeState != "online" || got.NodeOwner != "shared_pool" || !resultSnapshotValid(got) {
		t.Fatal("shared owner snapshot", got.NodeState, got.NodeOwner)
	}
}
