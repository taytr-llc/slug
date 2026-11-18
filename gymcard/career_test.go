//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func TestCardCareerOpenDetectsMissingReceiptFromSavedCard(t *testing.T) {
	f := newCardFixture(t)
	s, r := cardCareerOwner(t, f)
	ctx := context.Background()
	if e := f.s.BindCareer(ctx, r); e != nil {
		t.Fatal(e)
	}
	if _, e := f.build(t); e != nil {
		t.Fatal(e)
	}
	it := f.s.db.NewIterator(util.BytesPrefix([]byte(careerconsumer.FencePrefix)), nil)
	if !it.Next() {
		it.Release()
		t.Fatal("missing committed receipt")
	}
	key := append([]byte{}, it.Key()...)
	it.Release()
	if e := f.s.db.Delete(key, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	f.s.Close()
	f.s = nil
	opened, e := OpenCareer(ctx, f.path, f.p, f.seed, s.Activation)
	if e == nil {
		opened.Close()
		t.Fatal("saved card bypassed missing receipt")
	}
	if !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

// Synthetic Reader timing isolates Card's sealed-prefix consumption. It does
// not claim the fixture's synthetic seed header is mined or a real Result was
// activated before this historical card. Actual composition is tested separately.
func cardCareerOwner(t *testing.T, f *cardFixture) (*careerstate.State, *careerstate.Reader) {
	t.Helper()
	c := f.x.cutoff(t)
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("1", 64), SourceCreatedAtMS: c - 3, ResultCreatedAtMS: c - 2, ActivatedAtMS: c - 1}
	a.ActivationID = careerstate.ActivationID(a)
	var stableIDs [2]string
	for i, s := range f.x.stable {
		v, e := s.EntrySourceInfo(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		stableIDs[i] = v.StoreID
	}
	s := &careerstate.State{Activation: a, Scope: careerstate.Scope{StableStoreIDs: stableIDs, CardStoreID: f.s.meta.StoreID}, ClockMS: f.x.now, Events: []careerstate.Event{}}
	r, e := careerstate.NewReader(a, s.Scope, func(context.Context) (careerstate.State, error) { return *s, nil })
	if e != nil {
		t.Fatal(e)
	}
	return s, r
}
func cardCareerEvent(s *careerstate.State, ids [2]string, at int64) careerstate.Event {
	e := careerstate.Event{Version: 1, ResultKey: strings.Repeat("4", 64), CardID: strings.Repeat("5", 64), BoutID: strings.Repeat("6", 64), SourceRecordDigest: strings.Repeat("7", 64), InputID: strings.Repeat("8", 64), ResultDigest: strings.Repeat("9", 64), FinalityDigest: strings.Repeat("a", 64), EffectiveAtMS: at, Saturday: "2026-11-21", CornerA: careerstate.Corner{ProspectID: ids[0], AccountID: strings.Repeat("b", 64), GymID: strings.Repeat("c", 64)}, CornerB: careerstate.Corner{ProspectID: ids[1], AccountID: strings.Repeat("e", 64), GymID: strings.Repeat("f", 64)}, Winner: ids[0], Method: "decision"}
	e.AwardID = careerstate.AwardID(s.Activation, e.ResultKey)
	return e
}
func TestCardCareerDirectCoreNoonBoundary(t *testing.T) {
	for _, offset := range []int64{-1, 0, 1} {
		t.Run(map[int64]string{-1: "before", 0: "equal", 1: "after"}[offset], func(t *testing.T) {
			f := newCardFixture(t)
			s, r := cardCareerOwner(t, f)
			ctx := context.Background()
			c := f.x.cutoff(t)
			if e := f.s.BindCareer(ctx, r); e != nil {
				t.Fatal(e)
			}
			s.ClockMS = c - 1
			if _, e := f.build(t); !errors.Is(e, careerstate.ErrUnfenced) {
				t.Fatal("direct unfenced booking", e)
			}
			s.ClockMS = f.x.now
			s.Events = []careerstate.Event{cardCareerEvent(s, f.x.ids, c+offset)}
			out, e := f.build(t)
			if offset < 0 {
				var bad *CareerIneligibleError
				if !errors.As(e, &bad) || len(bad.ProspectIDs) != 2 || f.s.meta.EventCount != 0 {
					t.Fatal("C1 bypass", e)
				}
				return
			}
			if e != nil || out.Replayed || f.s.meta.EventCount != 1 {
				t.Fatal("valid noon booking", e)
			}
			f.s.Close()
			f.s = nil
			if old, e := Open(ctx, f.path, f.p, f.seed); e == nil {
				old.Close()
				t.Fatal("legacy writer accepted guard")
			}
			f.s, e = openWithCareer(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(f.x.now) }, false, &s.Activation, false)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.AttachCareer(ctx, r); e != nil {
				t.Fatal(e)
			}
			// Historical replay retains the booked inputs even if later career/source
			// status differs; no re-derivation or award rollback occurs here.
			again, e := f.build(t)
			if e != nil || !again.Replayed || !equal(again.Record, out.Record) {
				t.Fatal("saved card replay", e)
			}
		})
	}
}
func TestCardCareerBindingAndCardSyncUnknownRecovery(t *testing.T) {
	for _, stage := range []string{"binding", "card"} {
		t.Run(stage, func(t *testing.T) {
			f := newCardFixture(t)
			s, r := cardCareerOwner(t, f)
			ctx := context.Background()
			if stage == "card" {
				if e := f.s.BindCareer(ctx, r); e != nil {
					t.Fatal(e)
				}
			}
			real := f.s.writeBatch
			f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := real(b, o); e != nil {
					return e
				}
				return errors.New("lost synced ACK")
			}
			var e error
			if stage == "binding" {
				e = f.s.BindCareer(ctx, r)
			} else {
				_, e = f.build(t)
			}
			if !errors.Is(e, ErrOutcomeUnknown) || !f.s.poisoned {
				t.Fatal("unknown outcome", e)
			}
			f.s.Close()
			f.s = nil
			f.s, e = openWithCareer(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(f.x.now) }, false, &s.Activation, true)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.BindCareer(ctx, r); e != nil {
				t.Fatal(e)
			}
			out, e := f.build(t)
			if e != nil || out.Replayed != (stage == "card") {
				t.Fatal("exact recovery", e, out.Replayed)
			}
		})
	}
}

func TestCardCareerEmptySelectionDoesNotInventSentinelEntrant(t *testing.T) {
	f := newCardFixture(t)
	s, r := cardCareerOwner(t, f)
	if e := f.s.BindCareer(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	// All-zero is a valid identifier encoding. A synthetic unrelated event for
	// that ID must not turn the empty-selection digest request into an entrant.
	s.Events = []careerstate.Event{cardCareerEvent(s, [2]string{strings.Repeat("0", 64), f.x.ids[1]}, f.x.cutoff(t)-1)}
	view, ids, e := f.s.careerSelection(context.Background(), Plan{}, f.x.cutoff(t))
	if e != nil || !careerstate.ID(view.Digest()) || len(ids) != 1 || ids[0] != strings.Repeat("0", 64) {
		t.Fatal("invented sentinel entrant", e)
	}
}
