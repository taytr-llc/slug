//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func replayEvidenceRows(t *testing.T, s *Store) map[string]string {
	t.Helper()
	out := map[string]string{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k := string(it.Key())
		if k != "meta" && k != "work" {
			out[k] = string(it.Value())
		}
	}
	if e := it.Error(); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestOwnershipAbandonExactWorkRetainsEvidenceAndReplaysFresh(t *testing.T) {
	ctx := context.Background()
	s, source, node, path := cutoffGenesisFixture(t)
	r, e := s.BeginReplay(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	id := r.WorkID()
	if !hexValue(id, 32, true) {
		t.Fatal("missing preknown work identity")
	}
	progress, e := s.InspectReplay(ctx)
	if e != nil || progress.WorkID != id || progress.Ready || progress.Head != nil {
		t.Fatal(progress, e)
	}
	if e = s.AbandonReplay(ctx, id); !errors.Is(e, ErrBusy) {
		t.Fatal("abandoned active replay", e)
	}
	// Two phase transitions and one normal header retain actual public evidence.
	for n := 0; n < 3; n++ {
		if _, e = r.Step(ctx); e != nil {
			t.Fatal(e)
		}
	}
	r.Close()
	before := replayEvidenceRows(t, s)
	if s.meta.Counts.Headers != 1 {
		t.Fatal("normal header not retained")
	}
	next, _, e := s.RecordChainObservation(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginReplay(ctx, next, node); !errors.Is(e, ErrStale) {
		t.Fatal("changed target silently resumed", e)
	}
	if e = s.AbandonReplay(ctx, strings.Repeat("71", 32)); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong exact work", e)
	}
	release, e := s.project(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AbandonReplay(ctx, id); !errors.Is(e, ErrBusy) {
		t.Fatal("did not fence projection", e)
	}
	release()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = s.AbandonReplay(cancelled, id); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	generation := s.generation
	if e = s.AbandonReplay(ctx, id); e != nil {
		t.Fatal(e)
	}
	if s.generation != generation+1 || s.work != nil || s.meta.WorkID != "" || s.ready {
		t.Fatal("abandon authority state")
	}
	if !reflect.DeepEqual(before, replayEvidenceRows(t, s)) {
		t.Fatal("abandon changed immutable evidence")
	}
	if _, e = s.InspectReplay(ctx); !errors.Is(e, ErrNotFound) {
		t.Fatal("stale progress", e)
	}
	pin, identity := s.pin, s.identity
	s.Close()
	s, e = Open(path, pin, identity)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("abandon promoted old head", e)
	}
	r, e = s.BeginReplay(ctx, next, node)
	if e != nil {
		t.Fatal(e)
	}
	if r.WorkID() == id {
		t.Fatal("fresh target reused abandoned identity")
	}
	completed := boundedReplayFinish(t, r)
	if !completed.Ready || completed.Head.PublicationSequence != 1 || completed.WorkID == id {
		t.Fatal("new target incomplete")
	}
}
