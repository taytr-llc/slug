//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymresult"
	"strings"
	"testing"
	"time"
)

func TestResultOwnerExactEpochDrainsBothBorrowerFamilies(t *testing.T) {
	s, _ := resultLaneFixture(t)
	pair := func() *pilotNodes {
		return &pilotNodes{primary: &childNode{done: make(chan struct{})}, peer: &childNode{done: make(chan struct{})}}
	}
	old, current := pair(), pair()
	s.seedNodes = current
	a, ac := context.WithCancel(t.Context())
	defer ac()
	q, qc := context.WithCancel(t.Context())
	defer qc()
	s.active = &stableTask{publication: &ResultJob{JobID: strings.Repeat("a", 64)}, ctx: a, cancel: ac}
	s.queued = &stableTask{seed: &SeedJob{JobID: strings.Repeat("b", 64)}, ctx: q, cancel: qc}
	s.laneGate <- struct{}{}
	stale := make(chan struct{})
	go func() { s.unregisterSeedNodes(old); close(stale) }()
	select {
	case <-stale:
	case <-time.After(time.Second):
		t.Fatal("stale owner attempted to drain successor lane")
	}
	if a.Err() != nil || q.Err() != nil || s.seedNodes != current {
		t.Fatal("old owner cancelled successor")
	}
	done := make(chan struct{})
	go func() { s.unregisterSeedNodes(current); close(done) }()
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("Result borrow not cancelled")
	}
	select {
	case <-q.Done():
	case <-time.After(time.Second):
		t.Fatal("Seed borrow not cancelled")
	}
	select {
	case <-done:
		t.Fatal("owner failed to join lane")
	case <-time.After(10 * time.Millisecond):
	}
	<-s.laneGate
	<-done
	s.active, s.queued = nil, nil
	if s.seedNodes != nil {
		t.Fatal("registration retained")
	}
}
func TestResultNodeReservationRejectedLeaseCleansOnlyItsEpoch(t *testing.T) {
	s, _ := resultLaneFixture(t)
	s.manifest.Phase = "ready"
	orig := s.stableConfig.Lease
	s.stableConfig.ResultNodeLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if e := orig(ctx, gymaccount.View, fn); e != nil {
			return e
		}
		return gymaccount.ErrUnauthorized
	}
	if e := s.ensureResultNodes(t.Context()); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal(e)
	}
	if s.resultNodeOwner != nil || s.cancel != nil || s.operation != "" || s.seedNodes != nil {
		t.Fatal("rejected reservation leaked an owner")
	}
	// The stopped source and lane stay intact; no node call was made before the
	// final original admission check. This is not an AttachOwned process proof.
	if s.entryStore == nil || s.boutStore == nil {
		t.Fatal("reservation altered source handles")
	}
	s.stableConfig.ResultNodeLease = nil
	if e := s.ensureResultNodes(t.Context()); !errors.Is(e, gymresult.ErrUnauthorized) {
		t.Fatal("missing original lease accepted", e)
	}
}
func TestResultBorrowedMiningOwnerSurvivesJobCancellation(t *testing.T) {
	s, _ := resultLaneFixture(t)
	n := &pilotNodes{primary: &childNode{done: make(chan struct{})}, peer: &childNode{done: make(chan struct{})}}
	s.seedNodes = n
	s.mining = true
	s.stableConfig.ResultNodeLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return s.stableConfig.Lease(ctx, gymaccount.View, fn)
	}
	ctx, cancel := context.WithCancel(t.Context())
	if e := s.ensureResultNodes(ctx); e != nil {
		t.Fatal(e)
	}
	cancel()
	if s.resultNodeOwner != nil || s.seedNodes != n || !s.seedNodesReadyLocked() {
		t.Fatal("job cancellation adopted or killed mining owner")
	}
	s.seedNodes = nil
	s.mining = false
}

func TestResultLateStartupCleanupDoesNotClearSuccessor(t *testing.T) {
	s, _ := resultLaneFixture(t)
	oldLife, oldCancel := context.WithCancel(t.Context())
	defer oldCancel()
	nextLife, nextCancel := context.WithCancel(t.Context())
	defer nextCancel()
	old := &resultNodeOwner{life: oldLife, cancel: oldCancel, done: make(chan struct{})}
	next := &resultNodeOwner{life: nextLife, cancel: nextCancel, done: make(chan struct{})}
	s.resultNodeOwner = next
	s.cancel, s.done, s.operation = nextCancel, next.done, "result_nodes"
	s.finishResultNodeStartup(old, nil)
	select {
	case <-old.done:
	default:
		t.Fatal("old startup not joined")
	}
	if s.resultNodeOwner != next || s.done != next.done || s.cancel == nil || s.operation != "result_nodes" || nextLife.Err() != nil {
		t.Fatal("late startup cleared or cancelled successor")
	}
	s.resultNodeOwner = nil
	s.cancel = nil
	s.operation = ""
	close(next.done)
}
