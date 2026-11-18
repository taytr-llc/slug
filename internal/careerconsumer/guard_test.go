// Copyright (c) 2026 The SLUG developers. ISC license.
package careerconsumer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/internal/careerstate"
)

// This synthetic owner tests the consumer capability boundary, not publication,
// signatures, PoW or hash vectors. Those have separate independently derived tests.
func owner(t *testing.T) (*careerstate.State, *careerstate.Reader) {
	t.Helper()
	a := careerstate.Activation{Version: 1, PolicyID: careerstate.PolicyID(), ResultStoreID: strings.Repeat("1", 64), SourceCreatedAtMS: 10, ResultCreatedAtMS: 20, ActivatedAtMS: 30}
	a.ActivationID = careerstate.ActivationID(a)
	s := &careerstate.State{Activation: a, Scope: careerstate.Scope{StableStoreIDs: [2]string{strings.Repeat("2", 64), strings.Repeat("3", 64)}, CardStoreID: strings.Repeat("4", 64)}, ClockMS: 100, Events: []careerstate.Event{}}
	r, e := careerstate.NewReader(a, s.Scope, func(context.Context) (careerstate.State, error) { return *s, nil })
	if e != nil {
		t.Fatal(e)
	}
	return s, r
}
func TestGuardBindingDurabilityScopeAndLegacyBarrier(t *testing.T) {
	s, r := owner(t)
	g, _ := New(nil, false)
	if _, _, e := g.PrepareBind(r, "stable", strings.Repeat("5", 64)); !errors.Is(e, careerstate.ErrInvalid) {
		t.Fatal(e)
	}
	if g.Required() {
		t.Fatal("foreign binding changed guard")
	}
	raw, install, e := g.PrepareBind(r, "stable", s.Scope.StableStoreIDs[0])
	if e != nil || len(raw) == 0 || g.Required() {
		t.Fatal("prepared binding visible", e)
	}
	install()
	if !g.Required() || g.Ready() != nil {
		t.Fatal("synced binding not usable")
	}
	if again, _, e := g.PrepareBind(r, "stable", s.Scope.StableStoreIDs[0]); e != nil || again != nil {
		t.Fatal("binding replay writes", e)
	}
	legacy, _ := New(nil, false)
	if handled, e := legacy.Audit([]byte(GuardKey), raw); !handled || !errors.Is(e, careerstate.ErrCorrupt) {
		t.Fatal("old writer accepted guard", e)
	}
	ready, _ := New(&s.Activation, false)
	if e := ready.Audited(100); !errors.Is(e, careerstate.ErrCorrupt) {
		t.Fatal("missing ready guard", e)
	}
	prepared, _ := New(&s.Activation, true)
	if e := prepared.Audited(100); e != nil {
		t.Fatal(e)
	}
	if e := prepared.Ready(); !errors.Is(e, careerstate.ErrInactive) {
		t.Fatal("prepared path enabled operations", e)
	}
	if _, e := ready.Audit([]byte(GuardKey), raw); e != nil {
		t.Fatal(e)
	}
	if e := ready.Audited(100); e != nil {
		t.Fatal(e)
	}
	if e := ready.Ready(); !errors.Is(e, careerstate.ErrInactive) {
		t.Fatal("missing reader enabled operations", e)
	}
	if e := ready.Attach(r, "card", s.Scope.CardStoreID); e != nil {
		t.Fatal(e)
	}
	s.ClockMS = 0
	if _, e := ready.Snapshot(context.Background(), []string{strings.Repeat("a", 64)}, 100, careerstate.Current); !errors.Is(e, careerstate.ErrCorrupt) {
		t.Fatal("owner corruption became zero", e)
	}
}
func TestGuardCutoffReceiptStrictScopeAuditAndCapacity(t *testing.T) {
	s, r := owner(t)
	g, _ := New(nil, false)
	_, install, e := g.PrepareBind(r, "stable", s.Scope.StableStoreIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	install()
	ids := []string{strings.Repeat("a", 64)}
	current, e := g.Snapshot(context.Background(), ids, 100, careerstate.Current)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, _, _, e = g.PrepareFence(current, ids); !errors.Is(e, careerstate.ErrInvalid) {
		t.Fatal("current snapshot authorized cutoff", e)
	}
	cutoff, e := g.Snapshot(context.Background(), ids, 100, careerstate.Cutoff)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, _, _, e = g.PrepareFence(cutoff, []string{strings.Repeat("b", 64)}); !errors.Is(e, careerstate.ErrInvalid) {
		t.Fatal("different requested IDs accepted", e)
	}
	k, raw, guard, save, e := g.PrepareFence(cutoff, ids)
	if e != nil || len(g.fences) != 0 {
		t.Fatal("unsynced fence installed", e)
	}
	save()
	if _, again, _, _, e := g.PrepareFence(cutoff, ids); e != nil || again != nil {
		t.Fatal("repeat cutoff writes", e)
	}
	for _, tc := range []struct {
		name  string
		key   string
		value []byte
		clock int64
	}{
		{"valid", k, raw, 100},
		{"deleted", "", nil, 100},
		{"future", k, raw, 99},
		{"wrong-key", FencePrefix + strings.Repeat("0", 64), raw, 100},
		{"noncanonical", k, append([]byte(" "), raw...), 100},
		{"duplicate-field", k, append([]byte(`{"version":1,`), raw[1:]...), 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := New(&s.Activation, false)
			_, e := a.Audit([]byte(GuardKey), guard)
			if e == nil && tc.name != "deleted" {
				_, e = a.Audit([]byte(tc.key), tc.value)
			}
			if e == nil {
				e = a.Audited(tc.clock)
			}
			if (e == nil) != (tc.name == "valid") {
				t.Fatal(e)
			}
		})
	}
	// Capacity is a refusal, not deletion of the oldest immutable receipt.
	for n := 1; n < MaxFences; n++ {
		s.ClockMS++
		v, e := g.Snapshot(context.Background(), ids, s.ClockMS, careerstate.Cutoff)
		if e != nil {
			t.Fatal(e)
		}
		_, _, _, put, e := g.PrepareFence(v, ids)
		if e != nil {
			t.Fatal(e)
		}
		put()
	}
	s.ClockMS++
	v, _ := g.Snapshot(context.Background(), ids, s.ClockMS, careerstate.Cutoff)
	if _, _, _, _, e := g.PrepareFence(v, ids); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	if _, raw, _, _, e := g.PrepareFence(cutoff, ids); e != nil || raw != nil {
		t.Fatal("capacity evicted old fence", e)
	}
}
