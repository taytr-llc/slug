// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"testing"

	"github.com/taytr-llc/slug/poolservice"
)

func TestPoolDesktopTransactionCapacityBalanceProjection(t *testing.T) {
	// Existing fixture retains the original account/view lease and makes a
	// bounded HTTP call. This adds no wallet, node or real-provider claim.
	h := newPoolDesktopHarness(t)
	h.mu.Lock()
	h.response.Balance.Reason = "transaction-capacity"
	want := h.response
	h.mu.Unlock()
	got, err := h.authority.Balance(context.Background())
	if err != nil || got != want {
		t.Fatal("original-view balance lost whole-payout deferral", got, err)
	}
	snapshot := PoolSnapshot{
		GymID: got.Balance.GymID, SourceBindingID: got.BindingID,
		Balance: PoolBalanceSnapshot{State: "ready", Value: &got},
	}
	// This is the balance validator called by ValidatePoolSnapshot, not a
	// fabricated complete account snapshot or an authorization shortcut.
	if !poolBalanceValid(snapshot) || *snapshot.Balance.Value != want {
		t.Fatal("desktop balance validator refused or changed the new reason")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*poolservice.BalanceResponse)
	}{
		{"unknown-reason", func(r *poolservice.BalanceResponse) { r.Balance.Reason = "transaction-capacity-unrecognized" }},
		{"wrong-source", func(r *poolservice.BalanceResponse) { r.BindingID = "" }},
		{"wrong-gym", func(r *poolservice.BalanceResponse) { r.Balance.GymID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := got
			tc.mutate(&bad)
			projection := snapshot
			projection.Balance.Value = &bad
			if poolBalanceValid(projection) {
				t.Fatal("new reason widened the original closed/bound validator")
			}
		})
	}
	if got != want || *snapshot.Balance.Value != want {
		t.Fatal("refusal controls aliased the retained successful response")
	}
}
