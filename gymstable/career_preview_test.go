// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Normal-PoW retained admissions are real; the trusted Career owner below is an
// explicit model. Its events test capture/lifetime, not result finality authority.
func careerPreviewFixture(t *testing.T) (*coreFixture, [2]string, *careerstate.State, *careerstate.Reader, *error) {
	t.Helper()
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 6)
	page := mustSync(t, x)
	var ids [2]string
	for n, a := range page.Items {
		for _, b := range page.Items[n+1:] {
			if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
				ids = [2]string{a.Assignment.ProspectID, b.Assignment.ProspectID}
			}
		}
	}
	if ids[0] == "" {
		t.Fatal("six verified inputs must include a same-class pair")
	}
	if ids[1] < ids[0] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	state, reader, failure := stableCareerOwner(t, x)
	if e := x.s.BindCareer(t.Context(), reader); e != nil {
		t.Fatal(e)
	}
	return x, ids, state, reader, failure
}

func careerPreviewRows(t *testing.T, s *Store) map[string]string {
	t.Helper()
	rows := map[string]string{}
	it := s.db.NewIterator(nil, nil)
	for it.Next() {
		if string(it.Key()) != "meta" {
			rows[string(it.Key())] = string(it.Value())
		}
	}
	e := it.Error()
	it.Release()
	if e != nil {
		t.Fatal(e)
	}
	return rows
}

func TestCareerPreviewCapturedPrefixDetachedAndUnrecorded(t *testing.T) {
	x, ids, state, _, _ := careerPreviewFixture(t)
	snapshots, proofs := 0, 0
	r, e := careerstate.NewReader(state.Activation, state.Scope, func(context.Context) (careerstate.State, error) {
		snapshots++
		return *state, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if e = x.s.AttachCareer(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	snapshots = 0
	x.s.proofRead = func() { proofs++ }
	rows, before := careerPreviewRows(t, x.s), x.s.meta
	p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, [2]string{ids[1], ids[0]})
	if e != nil || h == nil || proofs != 2 || snapshots != 1 || p.AsOfMS != x.now || p.Status.LastAsOfMS != x.now || p.Fighters[0].Assignment.ProspectID != ids[0] {
		t.Fatal("single common capture", proofs, snapshots, e)
	}
	expected, _ := json.Marshal(p)
	opaque, _ := json.Marshal(h)
	if string(opaque) != "{}" || h.RetainedBytes() != len(expected)+512 || (*CareerPreviewRead)(nil).RetainedBytes() != 0 {
		t.Fatal("opaque token or charge")
	}
	state.Events = []careerstate.Event{stableCareerEvent(state, ids[0], x.now)}
	p.Fighters[0].Totals.XP = 15
	p.Fighters[0].Manifest.Name.First = "changed"
	p.Status.Slots[0] = ids[1]
	for range 2 {
		got, e := x.s.ReadCareerPreviewPair(t.Context(), x.session, h)
		raw, _ := json.Marshal(got)
		if e != nil || !bytes.Equal(raw, expected) || proofs != 2 {
			t.Fatal("retained prefix/copy changed or rehashed", proofs, e)
		}
		got.Fighters[0].Totals.XP = 7
	}
	fresh, _, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, ids)
	if e != nil || fresh.CareerDigest == p.CareerDigest || fresh.Fighters[0].Totals != (fightercareer.Totals{Wins: 1, XP: 1, BasePoints: 10}) || fresh.Fighters[1].Totals != (fightercareer.Totals{}) || proofs != 4 {
		t.Fatal("fresh request failed to capture same-ms award", proofs, e)
	}
	if fresh.Fighters[0].Condition.LeagueAttributesTenths.Skill != fresh.Fighters[0].Condition.AgedAttributesTenths.Skill+10 || fresh.Fighters[0].Condition.ProXP != 1 {
		t.Fatal("XP not applied exactly once")
	}
	legacy, e := x.s.ReadPreviewPair(t.Context(), x.session, ids)
	if e != nil || legacy.Fighters[0].Condition.ProXP != 0 {
		t.Fatal("legacy Preview changed", e)
	}
	if _, e = x.s.ReadEntrySelection(t.Context(), x.session, ids[:]); !errors.Is(e, ErrCareerCandidateIneligible) {
		t.Fatal("practice weakened C1", e)
	}
	after := x.s.meta
	after.ClockMS, after.LastAsOfMS = before.ClockMS, before.LastAsOfMS
	if after != before || !reflect.DeepEqual(rows, careerPreviewRows(t, x.s)) || len(state.Events) != 1 {
		t.Fatal("practice wrote non-observation state")
	}
}

func TestCareerPreviewRetainsPairAcrossUnrelatedSourceGrowth(t *testing.T) {
	x, ids, state, _, _ := careerPreviewFixture(t)
	p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, ids)
	if e != nil {
		t.Fatal(e)
	}
	_, oldPage, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	beforeSource := x.source
	appendLiveCoreRecord(t, x)
	state.ClockMS = x.now
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	got, e := x.s.ReadCareerPreviewPair(t.Context(), x.session, h)
	if e != nil || !reflect.DeepEqual(got, p) || proofs != 0 || x.s.meta.ObservedSourceCount != p.Status.ObservedSourceCount+1 || x.source != beforeSource {
		t.Fatal("monotonic unrelated append changed capture", proofs, e)
	}
	if stale, e := x.s.ReadCareerInventory(t.Context(), x.session, oldPage); !errors.Is(e, ErrConflict) || stale.Version != 0 {
		t.Fatal("old page count equality was relaxed", e)
	}
}

func TestCareerPreviewFinalOwnerEpochAndSelection(t *testing.T) {
	x, ids, state, reader, failure := careerPreviewFixture(t)
	_, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, ids)
	if e != nil {
		t.Fatal(e)
	}
	refuse := func(session Session, handle *CareerPreviewRead, want error) {
		t.Helper()
		p, e := x.s.ReadCareerPreviewPair(t.Context(), session, handle)
		if !errors.Is(e, want) || p.Version != 0 {
			t.Fatal("refusal disclosed capture", want, e)
		}
	}
	for _, invalid := range []*CareerPreviewRead{nil, {}, {owner: x.s}} {
		refuse(x.session, invalid, ErrInvalid)
	}
	refuse(fixtureSession(x.pin, x.session.Generation+1, x.session.ViewID), h, ErrStaleView)
	refuse(fixtureSession(x.pin, x.session.Generation, strings.Repeat("b", 64)), h, ErrStaleView)
	denied := x.session
	denied.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return ErrUnauthorized
	}
	refuse(denied, h, ErrUnauthorized)
	for _, failureValue := range []error{careerstate.ErrClosed, careerstate.ErrPoisoned, careerstate.ErrCorrupt, errors.New("opaque reader failure")} {
		*failure = failureValue
		refuse(x.session, h, failureValue)
	}
	*failure = nil
	// Fault injection isolates each final metadata/identity guard; restore each
	// value before the next check so this is not a legitimate mutation fixture.
	x.s.meta.Checkpoint--
	refuse(x.session, h, ErrConflict)
	x.s.meta.Checkpoint++
	x.s.meta.RosterRevision++
	refuse(x.session, h, ErrConflict)
	x.s.meta.RosterRevision--
	x.s.meta.ObservedSourceCount++
	refuse(x.session, h, ErrCorrupt)
	x.s.meta.ObservedSourceCount--
	x.s.source = nil
	refuse(x.session, h, ErrInvalid)
	x.s.source = x.source
	a, e := x.s.assignment(ids[0])
	if e != nil {
		t.Fatal(e)
	}
	original := a
	a.SourceRecordDigest = strings.Repeat("f", 64)
	b := new(leveldb.Batch)
	if e = encodedPut(b, seqKey(a.Sequence), a, MaxAssignmentSize); e != nil {
		t.Fatal(e)
	}
	if e = x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	refuse(x.session, h, ErrConflict)
	b.Reset()
	if e = encodedPut(b, seqKey(a.Sequence), original, MaxAssignmentSize); e != nil {
		t.Fatal(e)
	}
	if e = x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	old := x.s
	careerReopen(t, x, state, reader, false)
	refuse(x.session, h, ErrInvalid)
	if p, e := old.ReadCareerPreviewPair(t.Context(), x.session, h); !errors.Is(e, ErrClosed) || p.Version != 0 {
		t.Fatal("closed original owner", e)
	}
}

func TestCareerPreviewPrepareRefusalsAndFinalLease(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 6)
	mustSync(t, x)
	ids := [2]string{x.records[0].Verified.ProspectID, x.records[1].Verified.ProspectID}
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	if p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, ids); !errors.Is(e, careerstate.ErrInactive) || p.Version != 0 || h != nil || proofs != 0 {
		t.Fatal("legacy unbound fallback", e)
	}
	_, reader, _ := stableCareerOwner(t, x)
	if e := x.s.BindCareer(t.Context(), reader); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][2]string{{ids[0], ids[0]}, {strings.ToUpper(ids[0]), ids[1]}, {ids[0], strings.Repeat("f", 64)}} {
		p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, bad)
		if e == nil || p.Version != 0 || h != nil || proofs != 0 {
			t.Fatal("cheap refusal", proofs, e)
		}
	}
	if p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), x.session, ids); !errors.Is(e, ErrClassMismatch) || p.Version != 0 || h != nil || proofs != 2 {
		t.Fatal("class mismatch", proofs, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	proofs = 0
	x.s.proofRead = func() { proofs++; cancel() }
	if p, h, e := x.s.PrepareCareerPreviewPair(ctx, x.session, ids); !errors.Is(e, ErrCancelled) || p.Version != 0 || h != nil || proofs != 1 {
		t.Fatal("cancel before second proof", proofs, e)
	}
	// Same-class fixture makes final publication reachable, then revokes the
	// original lease at each later boundary without substituting a new login.
	y, sameClass, _, _, _ := careerPreviewFixture(t)
	for _, boundary := range []int{3, 4} {
		calls, proofs := 0, 0
		y.s.proofRead = func() { proofs++ }
		session := y.session
		session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			calls++
			if calls == boundary {
				return ErrUnauthorized
			}
			return y.session.Lease(ctx, cap, fn)
		}
		p, h, e := y.s.PrepareCareerPreviewPair(t.Context(), session, sameClass)
		if !errors.Is(e, ErrUnauthorized) || p.Version != 0 || h != nil || proofs != boundary-2 {
			t.Fatal("original lease", boundary, proofs, e)
		}
	}
}

func TestCareerPreviewRetiredHeldOffRoster(t *testing.T) {
	x, ids, state, _, _ := careerPreviewFixture(t)
	state.Events = []careerstate.Event{stableCareerEvent(state, ids[0], x.now)}
	x.now += 100 * 604800000
	state.ClockMS = x.now
	held := x.session
	held.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrHeld
		}
		return x.session.Lease(ctx, cap, fn)
	}
	p, h, e := x.s.PrepareCareerPreviewPair(t.Context(), held, ids)
	if e != nil || h == nil || p.Status.Slots != [10]string{} {
		t.Fatal("held off-roster practice", e)
	}
	for _, f := range p.Fighters {
		if f.Condition.Phase != "retired" || f.Condition.AsOfMS != x.now {
			t.Fatal("retired practice condition")
		}
	}
	if _, e = x.s.ReadCareerPreviewPair(t.Context(), held, h); e != nil {
		t.Fatal("held replay", e)
	}
}

func TestCareerPreviewObservationFailureDisclosesNothing(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "known-sync-then-cancel", true: "lost-sync-and-cancel"}[uncertain], func(t *testing.T) {
			x, ids, _, _, _ := careerPreviewFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writes := 0
			original := x.s.writeBatch
			x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				writes++
				if e := original(b, o); e != nil {
					return e
				}
				cancel()
				if uncertain {
					return errors.New("lost acknowledgement after real sync")
				}
				return nil
			}
			p, h, e := x.s.PrepareCareerPreviewPair(ctx, x.session, ids)
			want := ErrCancelled
			if uncertain {
				want = ErrOutcomeUnknown
			}
			if !errors.Is(e, want) || p.Version != 0 || h != nil || writes != 1 || x.s.poisoned != uncertain {
				t.Fatal("observation boundary", writes, e)
			}
		})
	}
}

func TestCareerPreviewCloseJoinsOwnedRead(t *testing.T) {
	x, ids, _, _, _ := careerPreviewFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	proofs := 0
	x.s.proofRead = func() { proofs++; close(entered); <-release }
	result := make(chan error, 1)
	go func() {
		p, h, e := x.s.PrepareCareerPreviewPair(context.Background(), x.session, ids)
		if p.Version != 0 || h != nil {
			e = errors.New("closed operation disclosed pair or token")
		}
		result <- e
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- x.s.Close() }()
	<-x.s.life.Done()
	select {
	case <-closed:
		t.Fatal("Close returned before owned proof path joined")
	default:
	}
	close(release)
	if e := <-result; !errors.Is(e, ErrClosed) {
		t.Fatal("owner closure lost to cancellation", e)
	}
	if e := <-closed; e != nil || proofs != 1 {
		t.Fatal("closure/join", proofs, e)
	}
}
