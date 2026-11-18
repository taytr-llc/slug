//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func activateFixture(t *testing.T, f *resultFixture) {
	t.Helper()
	if e := f.s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := ActivateCareerForTestClock(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.s = s
}
func careerIDs(f *resultFixture) []string {
	ids := []string{f.record.Source.Corners[0].Member.Assignment.ProspectID, f.record.Source.Corners[1].Member.Assignment.ProspectID}
	sort.Strings(ids)
	return ids
}
func TestCareerActivationRecoveryAndLegacyRefusal(t *testing.T) {
	f := newResultFixture(t)
	path := filepath.Join(f.dir, "result")
	f.s.Close()
	if e := CheckCareerActivationForTestClock(context.Background(), path, f.pin, f.bout, f.clock); e != nil {
		t.Fatal(e)
	}
	for _, stage := range []string{"career-intent-synced", "career-marker-temp-created", "career-marker-temp-synced", "career-marker-renamed", "career-metadata-synced", "career-ready"} {
		t.Run(stage, func(t *testing.T) {
			// A fresh real journal per cut uses the unchanged source incarnation.
			dst := filepath.Join(f.dir, "career-"+stage)
			s, e := create(context.Background(), dst, f.pin, f.bout, f.clock, nil)
			if e != nil {
				t.Fatal(e)
			}
			created := s.meta.CreatedAtMS
			s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			hit := false
			got, e := openCareer(ctx, dst, f.pin, f.bout, f.clock, "activate", func(x string) {
				if x == stage && !hit {
					hit = true
					cancel()
				}
			})
			if got != nil || !errors.Is(e, ErrOutcomeUnknown) || !hit {
				t.Fatal("activation uncertainty", e, hit)
			}
			old, e := OpenForTestClock(context.Background(), dst, f.pin, f.bout, f.clock)
			if old != nil {
				old.Close()
				t.Fatal("legacy writer reopened activation")
			}
			if e == nil {
				t.Fatal("legacy accepted")
			}
			resumed, e := ResumeCareerActivationForTestClock(context.Background(), dst, f.pin, f.bout, f.clock)
			if e != nil {
				t.Fatal("resume", e)
			}
			info, e := resumed.CareerInfo(context.Background())
			if e != nil || info.ResultCreatedAtMS != created {
				t.Fatal("identity", e)
			}
			resumed.Close()
			f.now.Add(1)
			opened, e := OpenCareerForTestClock(context.Background(), dst, f.pin, f.bout, f.clock)
			if e != nil {
				t.Fatal(e)
			}
			again, _ := opened.CareerInfo(context.Background())
			opened.Close()
			if again != info {
				t.Fatal("activation changed")
			}
		})
	}
	// The original untouched v1 journal remains eligible and writable.
	s, e := OpenForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.s = s
	final := referenceRecord(t, "record-finalized60")
	installModeledResult(t, f, final)
	before, _ := os.ReadFile(filepath.Join(path, markerName))
	f.s.Close()
	for _, fn := range []func() error{func() error {
		return CheckCareerActivationForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
	}, func() error {
		s, e := ActivateCareerForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
		if s != nil {
			s.Close()
		}
		return e
	}} {
		if e := fn(); !errors.Is(e, ErrCareerLegacyFinality) {
			t.Fatal("historical finalized activation", e)
		}
	}
	after, _ := os.ReadFile(filepath.Join(path, markerName))
	if !bytes.Equal(before, after) {
		t.Fatal("legacy refusal changed marker")
	}
	s, e = OpenForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal("legacy damaged", e)
	}
	f.s = s
}
func TestCareerFirstFinalityAtomicReplayAndConflict(t *testing.T) {
	f := newResultFixture(t)
	r := referenceRecord(t, "record-included59")
	installModeledResult(t, f, r)
	beforeRows := map[string][]byte{}
	it := f.s.db.NewIterator(nil, nil)
	for it.Next() {
		if string(it.Key()) != "meta" {
			beforeRows[string(it.Key())] = append([]byte(nil), it.Value()...)
		}
	}
	if e := it.Error(); e != nil {
		t.Fatal(e)
	}
	it.Release()
	oldMeta := f.s.meta
	activateFixture(t, f)
	for key, want := range beforeRows {
		got, e := f.s.db.Get([]byte(key), nil)
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("activation rewrote publication/reservation", e)
		}
	}
	normalized := f.s.meta
	normalized.Version = 1
	normalized.Career = nil
	normalized.ClockMS = oldMeta.ClockMS
	if !equal(normalized, oldMeta) {
		t.Fatal("activation changed funding/source metadata")
	}
	reader, e := f.s.CareerReader()
	if e != nil {
		t.Fatal(e)
	}
	ids := careerIDs(f)
	zero, e := reader.Snapshot(context.Background(), ids, f.now.Load(), careerstate.Current)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		v, ok := zero.Totals(id)
		if !ok || v.XP != 0 {
			t.Fatal("not zero")
		}
	}
	final := referenceRecord(t, "record-finalized60")
	m := &modelAttestation{modelResultChain: modelTip(r), cp: *final.Checkpoint}
	m.tip = final.Finality.ObservedTip
	m.hashes[m.tip.Height] = m.tip.Hash
	m.hashes[r.Inclusion.Anchor.Height] = r.Inclusion.Anchor.Hash
	c, done := fixtureCall(t, f)
	out, e := f.s.attest(c, m)
	done()
	if e != nil || out.Record.Finality == nil {
		t.Fatal("first finality", e)
	}
	effective := out.Record.Finality.ObservedAtMS
	got, e := reader.Snapshot(context.Background(), ids, effective, careerstate.Current)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		v, ok := got.Totals(id)
		if !ok || v.XP != 1 || v.Wins+v.Losses+v.Draws != 1 {
			t.Fatal("partial award", v)
		}
	}
	cut, e := reader.Snapshot(context.Background(), ids, effective, careerstate.Cutoff)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		v, _ := cut.Totals(id)
		if v.XP != 0 {
			t.Fatal("equal-time cutoff awarded")
		}
	}
	// Repeated source-authenticated save and later own conflict retain one event.
	c, done = fixtureCall(t, f)
	_, e = f.s.save(c, gymaccount.View, out.Record, f.s.meta, nil, nil, false)
	done()
	if e != nil {
		t.Fatal(e)
	}
	before := append([]careerstate.Event{}, f.s.careerIndex...)
	c, done = fixtureCall(t, f)
	// Explicitly modeled post-checkpoint conflict evidence exercises the real
	// source-authorized durable save; it is not a new chain-verification claim.
	conflicted := out.Record
	addConflict(&conflicted, Conflict{Kind: "checkpoint_chain_conflict", ObservedAtMS: effective, ExpectedHeight: conflicted.Inclusion.Anchor.Height, ExpectedHash: conflicted.Inclusion.Anchor.Hash, ObservedHash: strings.Repeat("e", 64), ObservedTip: &conflicted.Finality.ObservedTip})
	_, e = f.s.save(c, gymaccount.View, conflicted, f.s.meta, nil, nil, false)
	done()
	if e != nil {
		t.Fatal("conflict", e)
	}
	if !equal(before, f.s.careerIndex) {
		t.Fatal("conflict changed award")
	}
	f.s.Close()
	if _, e = reader.Activation(); !errors.Is(e, careerstate.ErrClosed) {
		t.Fatal("closed reader", e)
	}
	f.s, e = OpenCareerForTestClock(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal("reopen", e)
	}
	newReader, _ := f.s.CareerReader()
	again, e := newReader.Snapshot(context.Background(), ids, effective, careerstate.Current)
	if e != nil || again.Digest() != got.Digest() {
		t.Fatal("replay changed prefix", e)
	}
}
func TestCareerUnknownSyncDoesNotPublishIndex(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	r := referenceRecord(t, "record-finalized60")
	f.now.Store(recordMaxTime(r) + 20)
	reader, _ := f.s.CareerReader()
	m := f.s.meta
	m.ResultCount = 1
	b := new(leveldb.Batch)
	put(b, coinKey('u', r.Publication.Coin.Outpoint), reservation{1, r.ResultKey, r.Publication.TxID}, 256)
	actual := f.s.writeBatch
	f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if e := actual(b, o); e != nil {
			return e
		}
		return errors.New("modeled lost sync acknowledgement")
	}
	c, done := fixtureCall(t, f)
	_, e := f.s.save(c, gymaccount.View, r, m, b, nil, false)
	done()
	if !errors.Is(e, ErrOutcomeUnknown) || len(f.s.careerIndex) != 0 {
		t.Fatal("uncertain index escaped", e)
	}
	if _, e = reader.Activation(); !errors.Is(e, careerstate.ErrPoisoned) {
		t.Fatal("poison not propagated", e)
	}
	f.s.Close()
	f.s, e = OpenCareerForTestClock(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock)
	if e != nil || len(f.s.careerIndex) != 1 {
		t.Fatal("committed award recovery", e)
	}
}
func TestCareerFenceOriginalLeaseAndCutoff(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	reader, _ := f.s.CareerReader()
	ids := careerIDs(f)
	date := f.record.Source.Event.Saturday
	f.held.Store(true)
	f.now.Add(1)
	if e := f.s.FenceCareer(context.Background(), f.entry, f.session, date); e != nil {
		t.Fatal("held View fence", e)
	}
	_, e := reader.Snapshot(context.Background(), ids, f.now.Load()+1, careerstate.Cutoff)
	if !errors.Is(e, careerstate.ErrUnfenced) {
		t.Fatal("future fence", e)
	}
	f.revoked.Store(true)
	if e = f.s.FenceCareer(context.Background(), f.entry, f.session, date); e == nil {
		t.Fatal("revoked lease accepted")
	}
}

func TestCareerUnknownTemporaryIsRetained(t *testing.T) {
	f := newResultFixture(t)
	path := filepath.Join(f.dir, "result")
	f.s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	_, e := openCareer(ctx, path, f.pin, f.bout, f.clock, "activate", func(stage string) {
		if stage == "career-intent-synced" {
			cancel()
		}
	})
	if !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	temp := filepath.Join(path, careerTempName)
	unknown := []byte("unrelated file; do not delete")
	if e = os.WriteFile(temp, unknown, 0600); e != nil {
		t.Fatal(e)
	}
	if s, e := ResumeCareerActivationForTestClock(context.Background(), path, f.pin, f.bout, f.clock); s != nil || !errors.Is(e, ErrCorrupt) {
		if s != nil {
			s.Close()
		}
		t.Fatal("unknown temporary admitted", e)
	}
	got, _ := os.ReadFile(temp)
	if !bytes.Equal(got, unknown) {
		t.Fatal("unknown file removed")
	}
	// A known exact interrupted prefix is recoverable without adopting its data.
	if e = os.WriteFile(temp, []byte(`{"application":`), 0600); e != nil {
		t.Fatal(e)
	}
	f.s, e = ResumeCareerActivationForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal("valid interrupted prefix", e)
	}
}
func TestCareerFenceShortCallerAndFutureCutoff(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	date := f.record.Source.Event.Saturday
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	session := f.session
	actual := session.Lease
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		calls++
		return actual(ctx, cap, fn)
	}
	if e := f.s.FenceCareer(ctx, f.entry, session, date); !errors.Is(e, ErrCancelled) || calls != 0 {
		t.Fatal("short caller extended", e, calls)
	}
	day, e := time.Parse("2006-01-02", date)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.FenceCareer(context.Background(), f.entry, session, day.AddDate(0, 0, 7).Format("2006-01-02")); !errors.Is(e, ErrCareerNotDue) || calls != 0 {
		t.Fatal("future cutoff observed source", e, calls)
	}
}
