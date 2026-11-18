// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerReadFixture(t *testing.T) (*coreFixture, *careerstate.State, *careerstate.Reader, *error) {
	t.Helper()
	x := newCoreFixture(t, "zero256", 1)
	mustSync(t, x)
	s, r, failure := stableCareerOwner(t, x)
	if e := x.s.BindCareer(t.Context(), r); e != nil {
		t.Fatal(e)
	}
	return x, s, r, failure
}

func TestCareerReadDetachedCapturedPrefixWithoutProofWork(t *testing.T) {
	x, state, _, _ := careerReadFixture(t)
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	p, handle, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 1)
	if e != nil || handle == nil || proofs != 1 || p.Source.Items[0].Receipt().Version == 0 {
		t.Fatal("verified preparation/private portrait evidence", proofs, e)
	}
	expected, _ := json.Marshal(p)
	if handle.RetainedBytes() != len(expected)+careerReadOverhead || handle.RetainedBytes() > MaxCareerPageSize+careerReadOverhead {
		t.Fatal("uncharged composite")
	}
	publicHandle, _ := json.Marshal(handle)
	if string(publicHandle) != "{}" || (*CareerRead)(nil).RetainedBytes() != 0 {
		t.Fatal("opaque handle exposed state")
	}
	id := p.Items[0].ProspectID
	// Public mutations cannot change the handle, and a new first finality in
	// the same millisecond cannot rewrite this already captured Current prefix.
	p.Items[0].Totals.XP = 15
	p.Source.Items[0].Assignment.ProspectID = strings.Repeat("f", 64)
	state.Events = []careerstate.Event{stableCareerEvent(state, id, x.now)}
	for n := 0; n < 2; n++ {
		got, e := x.s.ReadCareerInventory(t.Context(), x.session, handle)
		raw, _ := json.Marshal(got)
		if e != nil || !bytes.Equal(raw, expected) || proofs != 1 || got.Source.Items[0].Receipt().Version != 0 {
			t.Fatal("captured copy changed, repeated proof work or retained private receipt", proofs, e)
		}
		got.Items[0].Totals.XP = 7
		got.Source.Status.Slots[0] = id
	}
	fresh, _, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 1)
	if e != nil || fresh.Items[0].Totals.XP != 1 || fresh.CareerDigest == handleDigest(t, handle) || proofs != 2 {
		t.Fatal("new job failed to observe new finality", proofs, e)
	}
}

func handleDigest(t *testing.T, h *CareerRead) string {
	t.Helper()
	var p CareerPage
	if e := json.Unmarshal(h.raw, &p); e != nil {
		t.Fatal(e)
	}
	return p.CareerDigest
}

func TestCareerReadOriginalEpochAndOwnerLiveness(t *testing.T) {
	x, state, reader, failure := careerReadFixture(t)
	_, h, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, session := range []Session{
		fixtureSession(x.pin, x.session.Generation+1, x.session.ViewID),
		fixtureSession(x.pin, x.session.Generation, strings.Repeat("b", 64)),
	} {
		if p, e := x.s.ReadCareerInventory(t.Context(), session, h); !errors.Is(e, ErrStaleView) || p.Version != 0 {
			t.Fatal("new authenticated epoch reused old handle", e)
		}
	}
	denied := x.session
	denied.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if p, e := x.s.ReadCareerInventory(t.Context(), denied, h); e == nil || p.Version != 0 {
		t.Fatal("revoked original session disclosed")
	}
	held := x.session
	held.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrHeld
		}
		return x.session.Lease(ctx, cap, fn)
	}
	if _, e = x.s.ReadCareerInventory(t.Context(), held, h); e != nil {
		t.Fatal("held read refused", e)
	}
	*failure = careerstate.ErrClosed
	if p, e := x.s.ReadCareerInventory(t.Context(), x.session, h); !errors.Is(e, careerstate.ErrClosed) || p.Version != 0 {
		t.Fatal("closed borrowed owner disclosed", e)
	}
	*failure = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if p, e := x.s.ReadCareerInventory(ctx, x.session, h); e == nil || p.Version != 0 {
		t.Fatal("cancelled read disclosed")
	}
	for _, invalid := range []*CareerRead{nil, {}, {owner: x.s}} {
		if p, e := x.s.ReadCareerInventory(t.Context(), x.session, invalid); !errors.Is(e, ErrInvalid) || p.Version != 0 {
			t.Fatal("empty public handle accepted", e)
		}
	}
	old := x.s
	careerReopen(t, x, state, reader, false)
	if p, e := x.s.ReadCareerInventory(t.Context(), x.session, h); !errors.Is(e, ErrInvalid) || p.Version != 0 {
		t.Fatal("new store incarnation accepted old handle", e)
	}
	if p, e := old.ReadCareerInventory(t.Context(), x.session, h); !errors.Is(e, ErrClosed) || p.Version != 0 {
		t.Fatal("closed original owner disclosed", e)
	}
}

func TestCareerReadRosterAndChangedSelectionRefuse(t *testing.T) {
	x, _, _, _ := careerReadFixture(t)
	id := x.records[0].Verified.ProspectID
	intent, e := x.s.PrepareSlot(t.Context(), x.session, SlotRequest{Slot: 1, ProspectID: id})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.ApplySlot(t.Context(), x.session, intent.OperationID); e != nil {
		t.Fatal(e)
	}
	roster, rh, e := x.s.PrepareCareerRoster(t.Context(), x.session, 1, 8)
	if e != nil || len(roster.Items) != 1 || roster.Source.Items[0].Item.Receipt().Version == 0 {
		t.Fatal("original roster preparation", e)
	}
	_, ih, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 8)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.ReadCareerInventory(t.Context(), x.session, rh); !errors.Is(e, ErrInvalid) {
		t.Fatal("roster handle used for inventory", e)
	}
	if _, e = x.s.ReadCareerRoster(t.Context(), x.session, ih); !errors.Is(e, ErrInvalid) {
		t.Fatal("inventory handle used for roster", e)
	}
	got, e := x.s.ReadCareerRoster(t.Context(), x.session, rh)
	a, _ := json.Marshal(roster)
	b, _ := json.Marshal(got)
	if e != nil || !bytes.Equal(a, b) || got.Source.Items[0].Item.Receipt().Version != 0 {
		t.Fatal("detached public roster", e)
	}
	clear, e := x.s.PrepareSlot(t.Context(), x.session, SlotRequest{ExpectedRevision: 1, Slot: 1})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.ApplySlot(t.Context(), x.session, clear.OperationID); e != nil {
		t.Fatal(e)
	}
	if p, e := x.s.ReadCareerInventory(t.Context(), x.session, ih); !errors.Is(e, ErrConflict) || p.Version != 0 {
		t.Fatal("changed roster disclosed old inventory selection", e)
	}
	if p, e := x.s.ReadCareerRoster(t.Context(), x.session, rh); !errors.Is(e, ErrConflict) || p.Version != 0 {
		t.Fatal("changed roster disclosed old roster selection", e)
	}
}

func TestCareerReadClosedProofSourceRefusesWithoutPayload(t *testing.T) {
	x, _, _, _ := careerReadFixture(t)
	_, h, e := x.s.PrepareCareerInventory(t.Context(), x.session, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.source.Close(); e != nil {
		t.Fatal(e)
	}
	x.source = nil
	if p, e := x.s.ReadCareerInventory(t.Context(), x.session, h); e == nil || p.Version != 0 {
		t.Fatal("closed proof authority disclosed cached page", e)
	}
}
