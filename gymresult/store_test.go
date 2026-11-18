//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"testing"
)

func fixtureCall(t *testing.T, f *resultFixture) (sourceCall, func()) {
	t.Helper()
	c, done, e := f.s.call(f.call())
	if e != nil {
		t.Fatal(e)
	}
	return c, done
}
func TestResultPrepareReplayAuthority(t *testing.T) {
	f := newResultFixture(t)
	c, done := fixtureCall(t, f)
	first, e := f.s.prepare(c)
	done()
	if e != nil {
		t.Fatal("prepare", e)
	}
	if validateActionResult(f.pin, first) != nil {
		t.Fatal("invalid prepared response")
	}
	before := recordDigest(first.Record)
	f.now.Add(100)
	c, done = fixtureCall(t, f)
	again, e := f.s.prepare(c)
	done()
	if e != nil || recordDigest(again.Record) != before {
		t.Fatal("immutable replay", e)
	}
	f.held.Store(true)
	c, done = fixtureCall(t, f)
	_, e = f.s.prepare(c)
	done()
	if !errors.Is(e, ErrForbidden) {
		t.Fatal("held strong", e)
	}
	c, done = fixtureCall(t, f)
	_, st, e := f.s.read(c, gymaccount.View, true)
	done()
	if e != nil || !st.Exists {
		t.Fatal("held view", e)
	}
	f.revoked.Store(true)
	c, done = fixtureCall(t, f)
	_, _, e = f.s.read(c, gymaccount.View, true)
	done()
	if !errors.Is(e, ErrUnauthorized) {
		t.Fatal("original revocation", e)
	}
	f.revoked.Store(false)
	f.held.Store(false)
	f.s.Close()
	f.s, e = open(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock, false)
	if e != nil {
		t.Fatal("reopen", e)
	}
	r, ok, e := f.s.record(first.Record.ResultKey)
	if e != nil || !ok || recordDigest(r) != before {
		t.Fatal("replayed bytes after reopen", e)
	}
}
func TestResultUncertainSyncPoisons(t *testing.T) {
	f := newResultFixture(t)
	f.s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { return errors.New("test sync") }
	c, done := fixtureCall(t, f)
	_, e := f.s.prepare(c)
	done()
	if !errors.Is(e, ErrOutcomeUnknown) || !f.s.poisoned {
		t.Fatal("sync uncertainty", e)
	}
	if _, e = f.s.Info(context.Background()); !errors.Is(e, ErrPoisoned) {
		t.Fatal("poison reuse", e)
	}
}
func TestResultPostSyncRevocationUnknown(t *testing.T) {
	f := newResultFixture(t)
	f.s.checkpoint = func(stage string) {
		if stage == "prepared" {
			f.revoked.Store(true)
		}
	}
	c, done := fixtureCall(t, f)
	_, e := f.s.prepare(c)
	done()
	if !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("post-sync revoke", e)
	}
	if f.s.meta.ResultCount != 1 {
		t.Fatal("retained result lost")
	}
}
func TestResultSourceOwnershipAndPreparationFence(t *testing.T) {
	f := newResultFixture(t)
	ctx := context.Background()
	var first gymbout.ResultSource
	called := 0
	e := f.bout.WithResultView(ctx, f.card, f.seed, f.event, f.entry, f.session, gymaccount.View, f.record.Source.Event.Saturday, f.record.BoutID, func(v gymbout.ResultView, check func() error) error {
		if f.inLease.Load() != 1 {
			t.Fatal("view outside original lease")
		}
		first = *v.Source
		v.Pin.SourcePin.SourcePin.SourcePin.Participants[0].StablePin.AccountID = "mutated"
		v.Source.Source.Corners[0].Member.Assignment.ProspectID = "mutated"
		called++
		return check()
	})
	if e != nil {
		t.Fatal(e)
	}
	e = f.bout.WithResultView(ctx, f.card, f.seed, f.event, f.entry, f.session, gymaccount.View, f.record.Source.Event.Saturday, f.record.BoutID, func(v gymbout.ResultView, check func() error) error {
		if v.Pin.SourcePin.SourcePin.SourcePin.Participants[0].StablePin.AccountID == "mutated" || v.Source.Source.Corners[0].Member.Assignment.ProspectID == "mutated" {
			t.Fatal("source alias")
		}
		called++
		return check()
	})
	if e != nil || called != 2 {
		t.Fatal(e)
	}
	_ = first
	commits := 0
	e = f.bout.WithResultSource(ctx, f.card, f.seed, f.event, f.entry, f.session, f.record.Source.Event.Saturday, f.record.BoutID, func(src gymbout.ResultSource) (func(gymbout.ResultView, func() error) error, error) {
		if f.inLease.Load() != 0 {
			t.Fatal("preparation retained account lease")
		}
		f.revoked.Store(true)
		return func(gymbout.ResultView, func() error) error { commits++; return nil }, nil
	})
	if !errors.Is(e, gymbout.ErrUnauthorized) || commits != 0 {
		t.Fatal("revoked final source fence", e, commits)
	}
}
func TestResultSourceLowerCloseDuringPreparation(t *testing.T) {
	f := newResultFixture(t)
	commits := 0
	e := f.bout.WithResultSource(context.Background(), f.card, f.seed, f.event, f.entry, f.session, f.record.Source.Event.Saturday, f.record.BoutID, func(src gymbout.ResultSource) (func(gymbout.ResultView, func() error) error, error) {
		if f.inLease.Load() != 0 {
			t.Fatal("lease retained")
		}
		if e := f.seed.Close(); e != nil {
			t.Fatal(e)
		}
		return func(gymbout.ResultView, func() error) error { commits++; return nil }, nil
	})
	if !errors.Is(e, gymbout.ErrUnavailable) || commits != 0 {
		t.Fatal("closed source disclosure", e, commits)
	}
}
func TestResultAbsentRefusalAndIndexTamper(t *testing.T) {
	f := newResultFixture(t)
	f.now.Add(1)
	c, done := fixtureCall(t, f)
	_, st, e := f.s.read(c, gymaccount.View, false)
	done()
	if e != nil || st.Exists {
		t.Fatal("absent", e)
	}
	f.now.Add(1)
	c, done = fixtureCall(t, f)
	_, _, e = f.s.read(c, gymaccount.View, true)
	done()
	if !errors.Is(e, ErrNotPrepared) {
		t.Fatal("missing is definite", e)
	}
	c, done = fixtureCall(t, f)
	out, e := f.s.prepare(c)
	done()
	if e != nil {
		t.Fatal(e)
	}
	k := key('r', out.Record.ResultKey)
	raw, e := f.s.db.Get(k, nil)
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, ' ')
	if e = f.s.db.Put(k, raw, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.record(out.Record.ResultKey); !errors.Is(e, ErrCorrupt) {
		t.Fatal("digest drift admitted", e)
	}
}
func TestResultPurposeDoesNotBroadenSeedView(t *testing.T) {
	f := newResultFixture(t)
	calls := 0
	e := f.entry.WithSeedView(context.Background(), f.session, gymaccount.ResultPublish, func(gymentry.SnapshotSourceInfo, func() error) error { calls++; return nil })
	if !errors.Is(e, gymentry.ErrInvalid) || calls != 0 {
		t.Fatal("old purpose broadened", e)
	}
}
func referenceRecord(t *testing.T, name string) Record {
	t.Helper()
	for _, x := range loadReference(t).Positive {
		if x.Name == name {
			v, e := ParseRecord(loadReference(t).Pin, []byte(x.Canonical))
			if e != nil {
				t.Fatal(e)
			}
			return v
		}
	}
	t.Fatal("missing reference record", name)
	return Record{}
}
func installModeledResult(t *testing.T, f *resultFixture, r Record) {
	t.Helper()
	if validateRecord(f.pin, r) != nil {
		t.Fatal("bad modeled result")
	}
	f.now.Store(max(f.now.Load(), recordMaxTime(r)) + 10)
	m := f.s.meta
	m.ResultCount = 1
	m.ClockMS = f.now.Load()
	b := new(leveldb.Batch)
	if e := put(b, key('r', r.ResultKey), r, MaxRecordSize); e != nil {
		t.Fatal(e)
	}
	if r.Publication != nil {
		if e := put(b, coinKey('u', r.Publication.Coin.Outpoint), reservation{1, r.ResultKey, r.Publication.TxID}, 256); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.s.commitLocked(b, m); e != nil {
		t.Fatal(e)
	}
	f.s.installIndex(r)
}
func TestResultAlternateCheckpointPreserved(t *testing.T) {
	f := newResultFixture(t)
	r := referenceRecord(t, "record-checkpoint59")
	var alternate chaincfg.SignedCheckpoint
	if json.Unmarshal(loadReference(t).Facts["alternate_checkpoint"], &alternate) != nil {
		t.Fatal("alternate")
	}
	if !validCheckpoint(f.pin, alternate) || equal(alternate, *r.Checkpoint) {
		t.Fatal("independent alternate fixture")
	}
	r.Checkpoint = &alternate
	installModeledResult(t, f, r)
	original := recordDigest(r)
	held := true
	f.held.Store(held)
	c := f.call()
	ordinary := referenceRecord(t, "record-checkpoint59").Checkpoint
	out, e := f.s.ObserveCheckpoint(c.ctx, c.bouts, c.cards, c.seeds, c.events, c.entries, c.session, c.date, c.id, *ordinary)
	if e != nil || recordDigest(out.Record) != original || !equal(out.Record.Checkpoint, &alternate) {
		t.Fatal("matching certificate replaced", e)
	}
}
func TestResultOwnVersusSourceConflictFreshness(t *testing.T) {
	f := newResultFixture(t)
	r := referenceRecord(t, "record-finalized60")
	installModeledResult(t, f, r)
	before := recordDigest(r)
	var seed gymseed.Record
	fixtureJSON(t, "seed_record", &seed)
	n := f.now.Add(100)
	tip := gymseed.Tip{Height: seed.Target.Height - 1, Hash: hash("modeled source conflict tip")}
	seed.Conflicts = append(seed.Conflicts, gymseed.Conflict{Kind: "chain", ObservedAtMS: n, ExpectedHeight: seed.Target.Height, ExpectedHash: seed.Target.Hash, ObservedTip: &tip})
	seed.Observation.CurrentFinality = false
	seed.Observation.ObservedAtMS = n
	seed.Observation.ChainObservedAtMS = n
	if e := f.seed.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := gymseed.MarshalRecord(f.pin.SourcePin.SourcePin.SourcePin, seed)
	if e != nil {
		t.Fatal(e)
	}
	injectRows(t, filepath.Join(f.dir, "seed"), map[string][]byte{string(key('r', seed.EventKey)): raw}, map[string]any{"clock_ms": n})
	f.seed, e = gymseed.OpenForTestClock(context.Background(), filepath.Join(f.dir, "seed"), f.pin.SourcePin.SourcePin.SourcePin, f.event, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	c, done := fixtureCall(t, f)
	got, st, e := f.s.read(c, gymaccount.View, true)
	done()
	if e != nil || recordDigest(got) != before || st.Phase != "conflict" || st.Source.Saved.Usable || !st.CurrentFinality || st.ChainObservedAtMS != r.Observation.ChainObservedAtMS {
		t.Fatal("source overlay fabricated result observation", e, st.Phase)
	}
	if validateReadResult(f.pin, ReadResult{1, got, st}) != nil {
		t.Fatal("conflict overlay codec")
	}
	c, done = fixtureCall(t, f)
	_, e = f.s.prepare(c)
	done()
	if !errors.Is(e, ErrSourceConflict) {
		t.Fatal("source conflict signing", e)
	}
}
func TestResultNilPublisherBeforeEffects(t *testing.T) {
	f := newResultFixture(t)
	before := f.s.meta.ClockMS
	f.now.Add(100)
	c := f.call()
	_, e := f.s.Publish(c.ctx, c.bouts, c.cards, c.seeds, c.events, c.entries, c.session, c.date, c.id, nil)
	if !errors.Is(e, ErrInvalid) || f.s.meta.ClockMS != before {
		t.Fatal("nil publisher observation", e)
	}
}
