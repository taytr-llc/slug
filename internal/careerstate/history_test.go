// Copyright (c) 2026 The SLUG developers. ISC license.
package careerstate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func historyModel(t *testing.T) (*State, *Reader, *int, *error) {
	t.Helper()
	a := Activation{Version: 1, PolicyID: PolicyID(), ResultStoreID: strings.Repeat("1", 64), ResultCreatedAtMS: 2, SourceCreatedAtMS: 1, ActivatedAtMS: 3}
	a.ActivationID = ActivationID(a)
	scope := Scope{StableStoreIDs: [2]string{strings.Repeat("2", 64), strings.Repeat("3", 64)}, CardStoreID: strings.Repeat("4", 64)}
	state := &State{Activation: a, Scope: scope, ClockMS: 8, Events: []Event{}}
	calls := 0
	var failure error
	r, e := NewReader(a, scope, func(context.Context) (State, error) { calls++; return *state, failure })
	if e != nil {
		t.Fatal(e)
	}
	return state, r, &calls, &failure
}
func historyEvent(a Activation, pid string, at int64) Event {
	e := Event{Version: 1, ResultKey: strings.Repeat("6", 64), CardID: strings.Repeat("7", 64), BoutID: strings.Repeat("8", 64), SourceRecordDigest: strings.Repeat("9", 64), InputID: strings.Repeat("a", 64), ResultDigest: strings.Repeat("b", 64), FinalityDigest: strings.Repeat("c", 64), EffectiveAtMS: at, Saturday: "2026-11-21", CornerA: Corner{pid, strings.Repeat("d", 64), strings.Repeat("e", 64)}, CornerB: Corner{strings.Repeat("6", 64), strings.Repeat("e", 64), strings.Repeat("f", 64)}, Winner: pid, Method: "decision"}
	e.AwardID = AwardID(a, e.ResultKey)
	return e
}
func TestHistoryOneStateOwnedAndLifetime(t *testing.T) {
	state, r, calls, failure := historyModel(t)
	pid := strings.Repeat("5", 64)
	state.Events = []Event{historyEvent(state.Activation, pid, 8)}
	h, e := r.CaptureHistory(t.Context(), pid, 100)
	if e != nil || *calls != 1 || h.PrefixCount() != 1 || h.EventCount() != 1 {
		t.Fatal("one owned state", *calls, e)
	}
	rows, e := h.Rows(0, 8)
	if e != nil || len(rows) != 1 || rows[0].TotalsAfter.XP != 1 {
		t.Fatal(e)
	}
	expected, _ := json.Marshal(rows)
	state.Events[0].Method = "ko"
	rows[0].Event.Method = "mutation"
	rows[0].TotalsAfter.XP = 15
	again, e := h.Rows(0, 8)
	b, _ := json.Marshal(again)
	if e != nil || string(b) != string(expected) || *calls != 1 {
		t.Fatal("aliased state/row or reentered owner", e)
	}
	if cap(h.rows) != len(h.rows) || len(h.offsets) != 2 {
		t.Fatal("unaccounted retained buffer capacity")
	}
	for _, fail := range []error{ErrClosed, ErrPoisoned, ErrCorrupt, errors.New("opaque")} {
		*failure = fail
		next, e := r.CaptureHistory(t.Context(), pid, 100)
		if e != fail || next != nil {
			t.Fatal("owner failure changed", e)
		}
	}
	*failure = nil
	state.Scope.CardStoreID = strings.Repeat("0", 64)
	if h, e := r.CaptureHistory(t.Context(), pid, 100); e != ErrCorrupt || h != nil {
		t.Fatal("owner incarnation retarget", e)
	}
}
func TestHistoryRejectsWholeIndexAndEarlyCancellation(t *testing.T) {
	state, r, calls, _ := historyModel(t)
	pid := strings.Repeat("5", 64)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if h, e := r.CaptureHistory(ctx, pid, 100); e != ErrCancelled || h != nil || *calls != 0 {
		t.Fatal("cancel entered owner", e)
	}
	// An unrelated event later than asOf still has to belong to valid owned
	// metadata. It cannot be hidden by selection or prefix filtering.
	event := historyEvent(state.Activation, strings.Repeat("0", 64), 9)
	state.Events = []Event{event}
	if h, e := r.CaptureHistory(t.Context(), pid, 4); e != ErrCorrupt || h != nil {
		t.Fatal("future event outside owner clock", e)
	}
	state.ClockMS = 9
	h, e := r.CaptureHistory(t.Context(), pid, 4)
	if e != nil || h.EventCount() != 0 || h.PrefixCount() != 0 {
		t.Fatal("valid future event must be omitted", e)
	}
	if rows, e := h.Rows(0, 8); e != nil || rows == nil || len(rows) != 0 {
		t.Fatal("empty page", e)
	}
	for _, b := range [][2]uint32{{1, 8}, {0, 0}, {0, 9}, {^uint32(0), 1}} {
		if rows, e := h.Rows(b[0], b[1]); e != ErrInvalid || rows != nil {
			t.Fatal("invalid page", e)
		}
	}
	if rows, e := (&History{}).Rows(0, 1); e != ErrInvalid || rows != nil {
		t.Fatal("forged empty handle", e)
	}
	var nilReader *Reader
	if h, e := nilReader.CaptureHistory(t.Context(), pid, 4); e != ErrInactive || h != nil {
		t.Fatal(e)
	}
}
