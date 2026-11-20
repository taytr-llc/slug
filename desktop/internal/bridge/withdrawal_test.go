// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"strings"
	"testing"
)

func withdrawalBridgeFixture(t *testing.T) (*PoolBridge, pilot.WithdrawalBinding) {
	t.Helper()
	s := poolBridgeSnapshot(t)
	binding, e := pilot.WithdrawalBindingForSnapshot(s, strings.Repeat("e", 64))
	if e != nil {
		t.Fatal(e)
	}
	b := &PoolBridge{&StableBridge{poolMode: true, withdrawalPool: &s}}
	r := pilot.WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_snapshot"}
	snap := pilot.WithdrawalSnapshot{Version: 1, State: "idle", Binding: &binding, CanPrepare: true}
	if !b.correlateWithdrawal(r, pilot.WithdrawalResponse{ID: 1, Version: 1, OK: true, Snapshot: &snap}) {
		t.Fatal("original helper scope handshake refused")
	}
	return b, binding
}
func TestWithdrawalBridgeNamespaceCannotBeChosenByOperation(t *testing.T) {
	b, binding := withdrawalBridgeFixture(t)
	other := binding
	other.JournalID = strings.Repeat("f", 64)
	s := pilot.WithdrawalSnapshot{Version: 1, State: "idle", Binding: &other}
	if b.correlateWithdrawal(pilot.WithdrawalRequest{ID: 1, Command: "withdrawal_snapshot"}, pilot.WithdrawalResponse{ID: 1, Version: 1, OK: true, Snapshot: &s}) {
		t.Fatal("namespace replaced")
	}
	binding.JournalID = strings.Repeat("f", 64)
	expected, e := pilot.WithdrawalBindingForSnapshot(*b.withdrawalPool, binding.JournalID)
	if e != nil {
		t.Fatal(e)
	}
	s.Binding = &expected
	if b.correlateWithdrawal(pilot.WithdrawalRequest{ID: 1, Command: "withdrawal_snapshot"}, pilot.WithdrawalResponse{ID: 1, Version: 1, OK: true, Snapshot: &s}) {
		t.Fatal("second valid namespace replaced first")
	}
	b.clearStableView()
	if b.withdrawalBinding != nil || b.withdrawalExpected != nil {
		t.Fatal("old namespace survived view clear")
	}
}
func TestWithdrawalBridgeExactActionAndDispatchReservation(t *testing.T) {
	b, binding := withdrawalBridgeFixture(t)
	dest := b.withdrawalPool.PayoutAddress
	script, e := pilot.WithdrawalDestination(dest)
	if e != nil {
		t.Fatal(e)
	}
	r := pilot.WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_prepare", ViewID: binding.View.ViewID, OperationID: strings.Repeat("a", 64), ActionID: strings.Repeat("b", 64), Intent: &pilot.WithdrawalIntent{Destination: dest, RecipientScript: script, AmountAtoms: 100000000, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}}
	if e = b.RememberWithdrawal(r); e != nil {
		t.Fatal(e)
	}
	action := pilot.WithdrawalAction{Version: 1, Binding: binding, OperationID: r.OperationID, ActionID: r.ActionID, Kind: "prepare", State: "running"}
	bad := action
	bad.ActionID = strings.Repeat("c", 64)
	if b.correlateWithdrawal(r, pilot.WithdrawalResponse{ID: 1, Version: 1, OK: true, Action: &bad}) {
		t.Fatal("unknown action adopted")
	}
	if !b.correlateWithdrawal(r, pilot.WithdrawalResponse{ID: 1, Version: 1, OK: true, Action: &action}) {
		t.Fatal("known action refused")
	}
	// A concurrent local busy refusal cannot retire an already reserved dispatch.
	b.gate = make(chan struct{}, 1)
	b.gate <- struct{}{}
	b.failed = make(chan struct{})
	b.withdrawalExpected.dispatched = true
	out := b.WithdrawalRequest(context.Background(), r)
	if out.Error != "busy" || b.withdrawalExpected.rejected {
		t.Fatal("busy erased prior handoff")
	}
	b.withdrawalExpected.dispatched = false
	out = b.WithdrawalRequest(context.Background(), r)
	if out.Error != "busy" || !b.withdrawalExpected.rejected {
		t.Fatal("never-dispatched refusal remained unresolved")
	}
}
