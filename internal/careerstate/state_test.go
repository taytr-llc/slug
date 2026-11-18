// Copyright (c) 2026 The SLUG developers. ISC license.
package careerstate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// These controls test handle lifetime/ownership, not independently expected math.
func TestReaderPinsOwnerAndReturnsOwnedContext(t *testing.T) {
	a := Activation{Version: 1, PolicyID: PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: 2, SourceCreatedAtMS: 1, ActivatedAtMS: 3}
	a.ActivationID = ActivationID(a)
	scope := Scope{StableStoreIDs: [2]string{strings.Repeat("2", 64), strings.Repeat("3", 64)}, CardStoreID: strings.Repeat("4", 64)}
	state := State{Activation: a, Scope: scope, ClockMS: 5, Events: []Event{}}
	var ownerError error
	r, e := NewReader(a, scope, func(context.Context) (State, error) { return state, ownerError })
	if e != nil {
		t.Fatal(e)
	}
	s, e := r.Snapshot(context.Background(), []string{strings.Repeat("5", 64)}, 5, Cutoff)
	if e != nil {
		t.Fatal(e)
	}
	if s.Mode() != Cutoff || len(s.ProspectIDs()) != 1 {
		t.Fatal("snapshot context missing")
	}
	ids := s.ProspectIDs()
	ids[0] = "changed"
	if s.ProspectIDs()[0] != strings.Repeat("5", 64) {
		t.Fatal("selection alias")
	}
	v, ok := s.Totals(strings.Repeat("5", 64))
	if !ok || v.XP != 0 {
		t.Fatal("authorized zero")
	}
	if _, ok = s.Totals(strings.Repeat("6", 64)); ok {
		t.Fatal("unrequested identity exposed")
	}
	returned, _ := r.Activation()
	returned.ResultStoreID = strings.Repeat("e", 64)
	got, _ := r.Activation()
	if got != a {
		t.Fatal("alias")
	}
	state.Activation.ActivatedAtMS++
	state.Activation.ActivationID = ActivationID(state.Activation)
	if _, e = r.Activation(); !errors.Is(e, ErrCorrupt) {
		t.Fatal("retargeted owner", e)
	}
	state.Activation = a
	state.Scope.CardStoreID = strings.Repeat("7", 64)
	if _, e = r.Scope(); !errors.Is(e, ErrCorrupt) {
		t.Fatal("retargeted source", e)
	}
	state.Scope = scope
	for _, err := range []error{ErrClosed, ErrPoisoned, ErrCancelled, ErrCorrupt} {
		ownerError = err
		if _, e = r.Snapshot(context.Background(), []string{strings.Repeat("5", 64)}, 5, Current); !errors.Is(e, err) {
			t.Fatal("owner error changed", e)
		}
	}
}
func TestReaderShortCallerAndInvalidHandle(t *testing.T) {
	var r *Reader
	if _, e := r.Activation(); !errors.Is(e, ErrInactive) {
		t.Fatal(e)
	}
	if _, e := r.Snapshot(context.Background(), nil, 0, Current); !errors.Is(e, ErrInactive) {
		t.Fatal(e)
	}
	a := Activation{Version: 1, PolicyID: PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: 2, SourceCreatedAtMS: 1, ActivatedAtMS: 3}
	a.ActivationID = ActivationID(a)
	scope := Scope{StableStoreIDs: [2]string{strings.Repeat("2", 64), strings.Repeat("3", 64)}, CardStoreID: strings.Repeat("4", 64)}
	calls := 0
	r, e := NewReader(a, scope, func(context.Context) (State, error) {
		calls++
		return State{Activation: a, Scope: scope, ClockMS: 3, Events: []Event{}}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = r.Snapshot(ctx, []string{strings.Repeat("5", 64)}, 3, Current); !errors.Is(e, ErrCancelled) || calls != 0 {
		t.Fatal("cancelled caller entered owner", e, calls)
	}
}

func TestReaderRejectsFutureOwnerEventWithoutClosingCurrentAsOf(t *testing.T) {
	a := Activation{Version: 1, PolicyID: PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: 2, SourceCreatedAtMS: 1, ActivatedAtMS: 3}
	a.ActivationID = ActivationID(a)
	scope := Scope{StableStoreIDs: [2]string{strings.Repeat("2", 64), strings.Repeat("3", 64)}, CardStoreID: strings.Repeat("4", 64)}
	state := State{Activation: a, Scope: scope, ClockMS: 5, Events: []Event{}}
	r, e := NewReader(a, scope, func(context.Context) (State, error) { return state, nil })
	if e != nil {
		t.Fatal(e)
	}
	ids := []string{strings.Repeat("5", 64)}
	if _, e = r.Snapshot(context.Background(), ids, 100, Current); e != nil {
		t.Fatal("Current acquired a cutoff restriction", e)
	}
	event := Event{Version: 1, ResultKey: strings.Repeat("6", 64), CardID: strings.Repeat("7", 64), BoutID: strings.Repeat("8", 64), SourceRecordDigest: strings.Repeat("9", 64), InputID: strings.Repeat("a", 64), ResultDigest: strings.Repeat("b", 64), FinalityDigest: strings.Repeat("c", 64), EffectiveAtMS: 6, Saturday: "2026-11-21", CornerA: Corner{strings.Repeat("5", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)}, CornerB: Corner{strings.Repeat("6", 64), strings.Repeat("e", 64), strings.Repeat("f", 64)}, Winner: "", Method: "decision_draw"}
	event.AwardID = AwardID(a, event.ResultKey)
	state.Events = []Event{event}
	if _, e = r.Snapshot(context.Background(), ids, 100, Current); !errors.Is(e, ErrCorrupt) {
		t.Fatal("future event accepted from owner", e)
	}
	state.ClockMS = 6
	if _, e = r.Snapshot(context.Background(), ids, 100, Current); e != nil {
		t.Fatal("coherent owner refused", e)
	}
}
