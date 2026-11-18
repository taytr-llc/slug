//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"testing"
)

func TestCardConcreteSourceLeaseAndConflict(t *testing.T) {
	x, s, _ := seedFixture(t)
	ctx := context.Background()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	prepare := func(v CardSource) (func(CardView, func() error) error, error) {
		calls++
		return func(v CardView, check func() error) error { calls++; return check() }, nil
	}
	if e = s.WithCardSource(ctx, x.s, x.entry[0], x.session[0], "2026-11-21", prepare); !errors.Is(e, ErrSourceIncomplete) || calls != 0 {
		t.Fatal("unfinalized source", e, calls)
	}
	x.now++
	r = syntheticFinality(t, s.meta.Pin, r, x.now)
	r, e = s.saveAction(ctx, x.s, x.entry[0], x.session[0], gymaccount.View, r, s.meta, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	var source CardSource
	var view CardView
	e = s.WithCardSource(ctx, x.s, x.entry[1], x.session[1], "2026-11-21", func(v CardSource) (func(CardView, func() error) error, error) {
		source = v
		return func(v CardView, check func() error) error { view = v; return check() }, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	digest, _ := CardSourceDigest(s.meta.Pin, source)
	if len(view.States) != 1 || view.States[0].SourceDigest != digest || view.ParticipantGymID != x.p.Participants[1].StablePin.Identity.GymID {
		t.Fatal("wrong source/view")
	}
	// Revocation during preparation must prevent the foreign callback.
	committed := false
	e = s.WithCardSource(ctx, x.s, x.entry[0], x.session[0], "2026-11-21", func(v CardSource) (func(CardView, func() error) error, error) {
		x.revoke[0] = true
		return func(CardView, func() error) error { committed = true; return nil }, nil
	})
	if e == nil || committed {
		t.Fatal("revoked preparation disclosed", e)
	}
	x.revoke[0] = false
	// A concrete post-callback original-session rejection remains observable.
	x.now++
	e = s.WithCardView(ctx, x.s, x.entry[0], x.session[0], []string{"2026-11-21"}, func(CardView, func() error) error { x.revoke[0] = true; return nil })
	if !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("post observation revoke", e)
	}
	x.revoke[0] = false
	// Conflict is preserved separately from immutable source identity.
	r.Conflicts = append(r.Conflicts, Conflict{Kind: "chain", ObservedAtMS: x.now, ObservedTip: &Tip{Height: 3, Hash: hash("new branch")}, ExpectedHeight: r.Target.Height, ExpectedHash: r.Target.Hash, ObservedHash: hash("replacement")})
	r.Observation.CurrentFinality = false
	if _, e = s.saveAction(ctx, x.s, x.entry[0], x.session[0], gymaccount.View, r, s.meta, nil, nil); e != nil {
		t.Fatal(e)
	}
	if e = s.WithCardSource(ctx, x.s, x.entry[0], x.session[0], "2026-11-21", prepare); !errors.Is(e, ErrConflict) {
		t.Fatal("conflicted new source", e)
	}
	e = s.WithCardView(ctx, x.s, x.entry[1], x.session[1], []string{"2026-11-21"}, func(v CardView, check func() error) error {
		if v.States[0].SourceDigest != digest || v.States[0].Status.ConflictCount != 1 {
			t.Fatal("lost historical source")
		}
		return check()
	})
	if e != nil {
		t.Fatal(e)
	}
	before := s.meta.ClockMS
	if e = s.WithCardView(ctx, nil, x.entry[0], x.session[0], []string{}, func(CardView, func() error) error { return nil }); !errors.Is(e, ErrInvalid) || before != s.meta.ClockMS {
		t.Fatal("nil source effects", e)
	}
	_ = gymevent.ErrIncomplete
}
