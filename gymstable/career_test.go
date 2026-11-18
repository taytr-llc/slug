// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Synthetic trusted-owner wiring isolates consumer behavior. Manifest proofs are
// the retained independent normal-PoW corpus; career finality is modeled here.
func stableCareerOwner(t *testing.T, x *coreFixture) (*careerstate.State, *careerstate.Reader, *error) {
	t.Helper()
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: x.now, SourceCreatedAtMS: x.now, ActivatedAtMS: x.now}
	a.ActivationID = careerstate.ActivationID(a)
	s := &careerstate.State{Activation: a, Scope: careerstate.Scope{StableStoreIDs: [2]string{x.s.meta.StoreID, strings.Repeat("2", 64)}, CardStoreID: strings.Repeat("3", 64)}, ClockMS: x.now, Events: []careerstate.Event{}}
	var failure error
	r, e := careerstate.NewReader(a, s.Scope, func(context.Context) (careerstate.State, error) { return *s, failure })
	if e != nil {
		t.Fatal(e)
	}
	return s, r, &failure
}

func TestStableCareerOpenDetectsMissingCutoffReceipt(t *testing.T) {
	x := newCoreFixture(t, "zero256", 0)
	s, r, _ := stableCareerOwner(t, x)
	if e := x.s.BindCareer(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	cutoffRead(t, x, x.now, nil)
	it := x.s.db.NewIterator(util.BytesPrefix([]byte(careerconsumer.FencePrefix)), nil)
	if !it.Next() {
		it.Release()
		t.Fatal("missing committed receipt")
	}
	key := append([]byte{}, it.Key()...)
	it.Release()
	if e := x.s.db.Delete(key, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	x.s.Close()
	x.s = nil
	opened, e := OpenCareer(x.path, x.pin, x.source, s.Activation)
	if e == nil {
		opened.Close()
		t.Fatal("deleted career receipt accepted")
	}
	if !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}
func stableCareerEvent(s *careerstate.State, id string, at int64) careerstate.Event {
	e := careerstate.Event{Version: 1, ResultKey: strings.Repeat("4", 64), CardID: strings.Repeat("5", 64), BoutID: strings.Repeat("6", 64), SourceRecordDigest: strings.Repeat("7", 64), InputID: strings.Repeat("8", 64), ResultDigest: strings.Repeat("9", 64), FinalityDigest: strings.Repeat("a", 64), EffectiveAtMS: at, Saturday: "2026-11-21", CornerA: careerstate.Corner{ProspectID: id, AccountID: strings.Repeat("b", 64), GymID: strings.Repeat("c", 64)}, CornerB: careerstate.Corner{ProspectID: strings.Repeat("d", 64), AccountID: strings.Repeat("e", 64), GymID: strings.Repeat("f", 64)}, Winner: id, Method: "decision"}
	e.AwardID = careerstate.AwardID(s.Activation, e.ResultKey)
	return e
}
func careerReopen(t *testing.T, x *coreFixture, s *careerstate.State, r *careerstate.Reader, prepared bool) {
	t.Helper()
	if x.s != nil {
		if e := x.s.Close(); e != nil {
			t.Fatal(e)
		}
		x.s = nil
	}
	var e error
	x.s, e = openCareerWithClock(x.path, x.pin, x.source, func() time.Time { return time.UnixMilli(x.now) }, &s.Activation, prepared)
	if e != nil {
		t.Fatal(e)
	}
	if r != nil {
		if e = x.s.AttachCareer(context.Background(), r); e != nil {
			t.Fatal(e)
		}
	}
}
func TestStableCareerProjectionEligibilityAndOriginalLease(t *testing.T) {
	x := newCoreFixture(t, "zero256", 1)
	ctx := context.Background()
	id := mustSync(t, x).Items[0].Assignment.ProspectID
	s, r, failure := stableCareerOwner(t, x)
	if e := x.s.BindCareer(ctx, r); e != nil {
		t.Fatal(e)
	}
	baseline, e := x.s.CareerInventory(ctx, x.session, 0, 1)
	if e != nil || len(baseline.Items) != 1 || baseline.Items[0].Totals.XP != 0 {
		t.Fatal("initial", e)
	}
	s.Events = []careerstate.Event{stableCareerEvent(s, id, x.now)}
	p, e := x.s.CareerInventory(ctx, x.session, 0, 1)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e)
	}
	i := p.Items[0]
	if i.Totals.Wins != 1 || i.Totals.XP != 1 || i.Totals.BasePoints != 10 || i.Condition.ProXP != 1 || p.Source.Items[0].Condition.ProXP != 0 || i.Condition.LeagueAttributesTenths.Skill != baseline.Items[0].Condition.LeagueAttributesTenths.Skill+10 {
		t.Fatal("career overlay/raw source", i, p.Source.Items[0].Condition)
	}
	if p.CareerDigest == baseline.CareerDigest || p.Source.Status != baseline.Source.Status || p.Source.Items[0].Assignment != baseline.Source.Items[0].Assignment {
		t.Fatal("prefix/source binding")
	}
	_, e = x.s.ReadEntrySelection(ctx, x.session, []string{id})
	var ineligible *CareerIneligibleError
	if !errors.As(e, &ineligible) || len(ineligible.ProspectIDs) != 1 || ineligible.ProspectIDs[0] != id {
		t.Fatal("C1 bypass", e)
	}
	// Empty pages retain non-null arrays and an authenticated complete prefix.
	empty, e := x.s.CareerInventory(ctx, x.session, 1, 1)
	if e != nil || empty.Items == nil || len(empty.Items) != 0 || !careerstate.ID(empty.CareerDigest) {
		t.Fatal("empty", e)
	}
	held := x.session
	orig := held.Lease
	held.Lease = func(c context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrHeld
		}
		return orig(c, cap, fn)
	}
	if _, e = x.s.CareerInventory(ctx, held, 0, 1); e != nil {
		t.Fatal("held read", e)
	}
	stale := held
	stale.Generation++
	if _, e = x.s.CareerInventory(ctx, stale, 0, 1); !errors.Is(e, ErrStaleView) {
		t.Fatal("stale view", e)
	}
	*failure = careerstate.ErrClosed
	if _, e = x.s.CareerInventory(ctx, x.session, 0, 1); !errors.Is(e, careerstate.ErrClosed) {
		t.Fatal("unavailable became zero", e)
	}
	*failure = nil
	careerReopen(t, x, s, nil, false)
	if _, e = x.s.Inventory(ctx, x.session, 0, 1); !errors.Is(e, careerstate.ErrInactive) {
		t.Fatal("bootstrap disclosed", e)
	}
	if _, e = x.s.EntrySourceInfo(ctx); e != nil {
		t.Fatal("bootstrap metadata unavailable", e)
	}
	if e = x.s.AttachCareer(ctx, r); e != nil {
		t.Fatal(e)
	}
	again, e := x.s.CareerInventory(ctx, x.session, 0, 1)
	if e != nil || again.CareerDigest != p.CareerDigest || again.Items[0].Totals != p.Items[0].Totals {
		t.Fatal("reopen changed career", e)
	}
	x.s.Close()
	x.s = nil
	if old, e := Open(x.path, x.pin, x.source); e == nil {
		old.Close()
		t.Fatal("legacy writer opened guarded stable")
	}
}
func TestStableCareerNoonBoundaryAndSyncedPrefix(t *testing.T) {
	for _, offset := range []int64{-1, 0, 1} {
		t.Run(map[int64]string{-1: "before", 0: "equal", 1: "after"}[offset], func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 1)
			ctx := context.Background()
			id := mustSync(t, x).Items[0].Assignment.ProspectID
			s, r, _ := stableCareerOwner(t, x)
			if e := x.s.BindCareer(ctx, r); e != nil {
				t.Fatal(e)
			}
			x.now++
			cutoffAssign(t, x, 0, 1, id)
			c := x.now + 10
			x.now = c + 1
			s.ClockMS = c - 1
			called := false
			read := func() error {
				return x.s.WithCutoffSelection(ctx, x.session, c, []string{id}, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
					called = true
					return func(_ CutoffFence, check func() error) error { return check() }, nil
				})
			}
			if e := read(); !errors.Is(e, careerstate.ErrUnfenced) || called {
				t.Fatal("unfenced cutoff", e)
			}
			s.ClockMS = x.now
			s.Events = []careerstate.Event{stableCareerEvent(s, id, c+offset)}
			if offset < 0 {
				if e := read(); !errors.Is(e, ErrCareerCandidateIneligible) || called {
					t.Fatal("pre-noon career admitted", e)
				}
				return
			}
			first, _ := cutoffRead(t, x, c, []string{id})
			if first.Members[0].ConditionAtCutoff.ProXP != 0 {
				t.Fatal("post-noon XP entered snapshot")
			}
			careerReopen(t, x, s, r, false)
			again, _ := cutoffRead(t, x, c, []string{id})
			if !equalEncoded(first, again) {
				t.Fatal("restart rewrote noon")
			}
			if _, e := x.s.ReadEntrySelection(ctx, x.session, []string{id}); !errors.Is(e, ErrCareerCandidateIneligible) {
				t.Fatal("current entry omitted award", e)
			}
		})
	}
}
func TestStableCareerUnknownBindingAndCutoffDoNotDisclose(t *testing.T) {
	for _, stage := range []string{"binding", "cutoff"} {
		t.Run(stage, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 0)
			ctx := context.Background()
			s, r, _ := stableCareerOwner(t, x)
			if stage == "cutoff" {
				if e := x.s.BindCareer(ctx, r); e != nil {
					t.Fatal(e)
				}
			}
			real := x.s.writeBatch
			x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := real(b, o); e != nil {
					return e
				}
				return errors.New("lost synced ACK")
			}
			var e error
			if stage == "binding" {
				e = x.s.BindCareer(ctx, r)
			} else {
				e = x.s.WithCutoffSelection(ctx, x.session, x.now, nil, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
					return func(CutoffFence, func() error) error { t.Fatal("unknown cutoff disclosed"); return nil }, nil
				})
			}
			if !errors.Is(e, ErrOutcomeUnknown) || !x.s.poisoned {
				t.Fatal("unknown outcome", e)
			}
			careerReopen(t, x, s, nil, true)
			if e = x.s.BindCareer(ctx, r); e != nil {
				t.Fatal("exact recovery", e)
			}
			if stage == "cutoff" {
				cutoffRead(t, x, x.now, nil)
			}
		})
	}
}
