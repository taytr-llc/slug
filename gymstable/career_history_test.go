// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Normal-PoW source receipts are genuine retained fixtures. Career owner/event
// wiring here is an explicit model, not a claim of new chain finality.
func historyFixture(t *testing.T) (*coreFixture, string, *careerstate.State, *careerstate.Reader, *error) {
	t.Helper()
	x := newCoreFixture(t, "zero256", 1)
	id := mustSync(t, x).Items[0].Assignment.ProspectID
	state, r, failure := stableCareerOwner(t, x)
	if e := x.s.BindCareer(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	return x, id, state, r, failure
}

// A deterministic cancellation boundary: the first sampled result is nil,
// then cancellation happens before the following uncancellable source call.
// This is an ordering control, not a scheduler or latency measurement.
type historyCancelAfterCheck struct {
	context.Context
	cancel  context.CancelFunc
	checked bool
}

func (c *historyCancelAfterCheck) Err() error {
	e := c.Context.Err()
	if !c.checked {
		c.checked = true
		c.cancel()
	}
	return e
}
func TestCareerHistoryLearnedSourceFailurePrecedesCancellation(t *testing.T) {
	for _, kind := range []string{"closed", "identity", "regression"} {
		t.Run(kind, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 0)
			want := ErrCorrupt
			switch kind {
			case "closed":
				if e := x.source.Close(); e != nil {
					t.Fatal(e)
				}
				want = ErrClosed
			case "identity":
				x.s.meta.Pin.Source.AuthorityID = strings.Repeat("a", 64)
			case "regression":
				x.s.meta.ObservedSourceCount = 1
			}
			for _, history := range []bool{false, true} {
				base, cancel := context.WithCancel(t.Context())
				ctx := &historyCancelAfterCheck{Context: base, cancel: cancel}
				var e error
				if history {
					_, e = x.s.sourceInfoMode(ctx, true)
				} else {
					_, e = x.s.sourceInfo(ctx)
				}
				expected := ErrCancelled
				if history {
					expected = want
				}
				if e != expected {
					t.Fatal("metadata error ordering", history, e, expected)
				}
				cancel()
			}
		})
	}
	// Close the real source, then cancel during the post-proof observation.
	// The returned source error is already known at this exact boundary.
	x := newCoreFixture(t, "zero256", 1)
	id := mustSync(t, x).Items[0].Assignment.ProspectID
	if e := x.source.Close(); e != nil {
		t.Fatal(e)
	}
	for _, history := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		x.s.now = func() time.Time { cancel(); return time.UnixMilli(x.now) }
		var e error
		if history {
			_, e = x.s.readSourceMode(ctx, 1, id, true)
		} else {
			_, e = x.s.readSource(ctx, 1, id)
		}
		want := ErrCancelled
		if history {
			want = ErrCorrupt
		}
		if e != want {
			t.Fatal("proof error ordering", history, e, want)
		}
		cancel()
	}
}
func historyFixtureEvent(x *coreFixture, state *careerstate.State, id string) careerstate.Event {
	e := stableCareerEvent(state, id, x.now)
	e.CornerA.AccountID = x.pin.AccountID
	e.CornerA.GymID = x.pin.Identity.GymID
	return e
}
func TestCareerHistoryTrustedCaptureDetachedAndFrozen(t *testing.T) {
	x, id, state, _, _ := historyFixture(t)
	calls, proofs := 0, 0
	r, e := careerstate.NewReader(state.Activation, state.Scope, func(context.Context) (careerstate.State, error) { calls++; return *state, nil })
	if e != nil {
		t.Fatal(e)
	}
	if e = x.s.AttachCareer(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	calls = 0
	inLease := false
	session := x.session
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		inLease = true
		defer func() { inLease = false }()
		return x.session.Lease(ctx, cap, fn)
	}
	x.s.proofRead = func() {
		proofs++
		if inLease {
			t.Fatal("proof under account lease")
		}
		if !x.s.mu.TryLock() {
			t.Fatal("proof under Stable mutex")
		}
		x.s.mu.Unlock()
	}
	before := careerPreviewRows(t, x.s)
	c, h, e := x.s.PrepareCareerHistory(t.Context(), session, id)
	if e != nil || h == nil || c.EventCount != 0 || calls != 1 || proofs != 1 {
		t.Fatal("one proof and one owned State", calls, proofs, e)
	}
	if h.RetainedBytes() != len(h.header)+h.history.RetainedBytes()+4096 || h.RetainedBytes() > MaxCareerHistoryRetainedBytes {
		t.Fatal("retained accounting")
	}
	opaque, _ := json.Marshal(h)
	if string(opaque) != "{}" || (*CareerHistoryRead)(nil).RetainedBytes() != 0 {
		t.Fatal("public token importer")
	}
	state.Events = []careerstate.Event{historyFixtureEvent(x, state, id)}
	c.Status.Slots[0] = id
	c.Totals.XP = 15
	for range 2 {
		p, e := x.s.ReadCareerHistory(t.Context(), session, h, 0, 8)
		if e != nil || p.Capture.EventCount != 0 || p.Items == nil || len(p.Items) != 0 || p.Capture.Totals.XP != 0 || proofs != 1 {
			t.Fatal("same-ms award rebased old history", e)
		}
		p.Capture.Status.Slots[0] = id
	}
	if calls != 5 {
		t.Fatal("paging reconstructed Current instead of only two narrow getters", calls)
	}
	newCapture, newHandle, e := x.s.PrepareCareerHistory(t.Context(), session, id)
	if e != nil || newCapture.EventCount != 1 || newCapture.Totals.XP != 1 || newCapture.HistoryID == c.HistoryID {
		t.Fatal("new explicit capture failed to see award", e)
	}
	p, e := x.s.ReadCareerHistory(t.Context(), session, newHandle, 0, 8)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(e)
	}
	expected, _ := json.Marshal(p)
	p.Items[0].Event.Method = "changed"
	p.Items[0].TotalsAfter.XP = 15
	again, e := x.s.ReadCareerHistory(t.Context(), session, newHandle, 0, 8)
	raw, _ := json.Marshal(again)
	if e != nil || string(raw) != string(expected) {
		t.Fatal("public row alias", e)
	}
	if !reflect.DeepEqual(careerPreviewRows(t, x.s), before) {
		t.Fatal("history wrote new rows or fences")
	}
}
func TestCareerHistoryOriginalOwnerEpochAndBounds(t *testing.T) {
	x, id, state, reader, failure := historyFixture(t)
	state.Events = []careerstate.Event{historyFixtureEvent(x, state, id)}
	_, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id)
	if e != nil {
		t.Fatal(e)
	}
	refuse := func(session Session, h *CareerHistoryRead, after, limit uint32, want error) {
		t.Helper()
		p, e := x.s.ReadCareerHistory(t.Context(), session, h, after, limit)
		if !errors.Is(e, want) || p.Version != 0 || p.Items != nil {
			t.Fatal("refusal leaked page", want, e)
		}
	}
	for _, bad := range []*CareerHistoryRead{nil, {}, {owner: x.s}} {
		refuse(x.session, bad, 0, 8, ErrInvalid)
	}
	for _, bounds := range [][2]uint32{{0, 0}, {0, 9}, {2, 8}, {^uint32(0), 8}} {
		refuse(x.session, h, bounds[0], bounds[1], ErrInvalid)
	}
	refuse(fixtureSession(x.pin, x.session.Generation+1, x.session.ViewID), h, 0, 8, ErrStaleView)
	refuse(fixtureSession(x.pin, x.session.Generation, strings.Repeat("b", 64)), h, 0, 8, ErrStaleView)
	denied := x.session
	denied.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return ErrUnauthorized
	}
	refuse(denied, h, 0, 8, ErrUnauthorized)
	for _, f := range []error{careerstate.ErrClosed, careerstate.ErrPoisoned, careerstate.ErrCorrupt, errors.New("opaque owner error")} {
		*failure = f
		refuse(x.session, h, 0, 8, f)
	}
	*failure = nil
	for _, change := range []func(){func() { x.s.meta.Checkpoint-- }, func() { x.s.meta.RosterRevision++ }} {
		old := x.s.meta
		change()
		refuse(x.session, h, 0, 8, ErrConflict)
		x.s.meta = old
	}
	x.s.meta.ObservedSourceCount++
	refuse(x.session, h, 0, 8, ErrCorrupt)
	x.s.meta.ObservedSourceCount--
	x.s.source = nil
	refuse(x.session, h, 0, 8, ErrInvalid)
	x.s.source = x.source
	a, e := x.s.assignment(id)
	if e != nil {
		t.Fatal(e)
	}
	altered := a
	altered.SourceRecordDigest = strings.Repeat("f", 64)
	for _, v := range []Assignment{altered, a} {
		b := new(leveldb.Batch)
		if e = encodedPut(b, seqKey(a.Sequence), v, MaxAssignmentSize); e != nil {
			t.Fatal(e)
		}
		if e = x.s.db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
			t.Fatal(e)
		}
		if v == altered {
			refuse(x.session, h, 0, 8, ErrConflict)
		}
	}
	old := x.s
	careerReopen(t, x, state, reader, false)
	refuse(x.session, h, 0, 8, ErrInvalid)
	if p, e := old.ReadCareerHistory(t.Context(), x.session, h, 0, 8); !errors.Is(e, ErrClosed) || p.Version != 0 {
		t.Fatal("closed old owner", e)
	}
}
func TestCareerHistoryAllowsGrowthHeldRetiredOffRoster(t *testing.T) {
	x, id, state, _, _ := historyFixture(t)
	state.Events = []careerstate.Event{historyFixtureEvent(x, state, id)}
	x.now += 100 * 604800000
	state.ClockMS = x.now
	held := x.session
	held.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if c != gymaccount.View {
			return gymaccount.ErrHeld
		}
		return x.session.Lease(ctx, c, fn)
	}
	c, h, e := x.s.PrepareCareerHistory(t.Context(), held, id)
	if e != nil || c.Status.Slots != [10]string{} || c.Totals.Wins != 1 {
		t.Fatal("retired held off-roster history", e)
	}
	_, oldPage, e := x.s.PrepareCareerInventory(t.Context(), held, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	appendLiveCoreRecord(t, x)
	state.ClockMS = x.now
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	p, e := x.s.ReadCareerHistory(t.Context(), held, h, 0, 8)
	if e != nil || p.Capture != c || proofs != 0 {
		t.Fatal("unrelated growth changed captured bytes", e)
	}
	if _, e = x.s.ReadCareerInventory(t.Context(), held, oldPage); !errors.Is(e, ErrConflict) {
		t.Fatal("old page equality changed", e)
	}
}
func TestCareerHistoryPreparationRefusalsAndPostSyncErrors(t *testing.T) {
	x := newCoreFixture(t, "zero256", 1)
	id := mustSync(t, x).Items[0].Assignment.ProspectID
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	if c, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id); !errors.Is(e, careerstate.ErrInactive) || c.Version != 0 || h != nil || proofs != 0 {
		t.Fatal("unbound fallback", e)
	}
	_, r, _ := stableCareerOwner(t, x)
	if e := x.s.BindCareer(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	if c, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, strings.Repeat("a", 64)); e == nil || c.Version != 0 || h != nil || proofs != 0 {
		t.Fatal("unconsumed proof", e)
	}
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "known-sync-cancel", true: "unknown-sync-cancel"}[uncertain], func(t *testing.T) {
			x, id, _, _, _ := historyFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writes := 0
			old := x.s.writeBatch
			x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				writes++
				if e := old(b, o); e != nil {
					return e
				}
				cancel()
				if uncertain {
					return errors.New("lost sync acknowledgement")
				}
				return nil
			}
			c, h, e := x.s.PrepareCareerHistory(ctx, x.session, id)
			want := ErrCancelled
			if uncertain {
				want = ErrOutcomeUnknown
			}
			if !errors.Is(e, want) || c.Version != 0 || h != nil || writes != 1 || x.s.poisoned != uncertain {
				t.Fatal("sync precedence or disclosure", writes, e)
			}
		})
	}
}
func TestCareerHistoryCloseJoinsProof(t *testing.T) {
	x, id, _, _, _ := historyFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	proofs := 0
	x.s.proofRead = func() { proofs++; close(entered); <-release }
	result := make(chan error, 1)
	go func() {
		c, h, e := x.s.PrepareCareerHistory(context.Background(), x.session, id)
		if c.Version != 0 || h != nil {
			e = errors.New("closed capture leaked")
		}
		result <- e
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- x.s.Close() }()
	<-x.s.life.Done()
	select {
	case <-closed:
		t.Fatal("Close returned before proof joined")
	default:
	}
	close(release)
	if e := <-result; !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if e := <-closed; e != nil || proofs != 1 {
		t.Fatal("close join", proofs, e)
	}
}

func TestCareerHistorySourceChangesDuringProof(t *testing.T) {
	for _, closeSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "growth", true: "closure"}[closeSource], func(t *testing.T) {
			x, id, state, _, _ := historyFixture(t)
			proofs := 0
			x.s.proofRead = func() {
				proofs++
				if closeSource {
					if e := x.source.Close(); e != nil {
						t.Fatal(e)
					}
				} else {
					appendLiveCoreRecord(t, x)
					state.ClockMS = x.now
				}
			}
			c, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id)
			if e == nil || c.Version != 0 || h != nil || proofs != 1 {
				t.Fatal("source changed across proof but capture escaped", proofs, e)
			}
			if !closeSource && !errors.Is(e, ErrConflict) {
				t.Fatal("changed sampled count", e)
			}
		})
	}
}
