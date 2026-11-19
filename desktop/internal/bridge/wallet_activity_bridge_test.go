// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"strings"
	"testing"
)

func activityBridgeFixture(t *testing.T) (*PoolBridge, pilot.WalletActivityJob, pilot.WalletActivityItem) {
	t.Helper()
	binding, e := pilot.WalletActivityBindingForSnapshot(poolBridgeSnapshot(t))
	if e != nil {
		t.Fatal(e)
	}
	b := &PoolBridge{&StableBridge{poolMode: true, activityBinding: &binding}}
	id := strings.Repeat("a", 64)
	if e = b.RememberWalletActivity(binding.View.ViewID, id); e != nil {
		t.Fatal(e)
	}
	cap := pilot.WalletActivityCapture{Version: 1, CapturedAtMS: 1, WalletHeight: 1, NodeHeight: 1, WalletTip: strings.Repeat("b", 64), NodeTip: strings.Repeat("b", 64), Count: 2}
	j := pilot.WalletActivityJob{Version: 1, JobID: id, State: "done", Binding: binding, Capture: &cap}
	item := pilot.WalletActivityItem{TxID: strings.Repeat("c", 64), RawSHA256: strings.Repeat("d", 64), Block: &pilot.WalletActivityBlock{Height: 1, Hash: strings.Repeat("b", 64)}, Confirmations: 1, ReceivedAtoms: 1, NetAtoms: 1, InputCount: 1, OutputCount: 1, OwnedOutputCount: 1}
	r := pilot.WalletActivityRequest{ID: 1, Version: 1, Command: "wallet_activity_job", ViewID: binding.View.ViewID, JobID: id}
	if !b.correlateWalletActivity(r, pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Job: &j}) {
		t.Fatal("capture refused")
	}
	return b, j, item
}
func TestWalletActivityBridgeSelectionCopiesAndRefresh(t *testing.T) {
	b, j, item := activityBridgeFixture(t)
	r := pilot.WalletActivityRequest{ID: 1, Version: 1, Command: "wallet_activity_page", ViewID: j.Binding.View.ViewID, JobID: j.JobID, Limit: 1}
	p := pilot.WalletActivityPage{Version: 1, Binding: j.Binding, JobID: j.JobID, Capture: *j.Capture, Limit: 1, Next: 1, HasMore: true, Items: []pilot.WalletActivityItem{item}}
	if !b.correlateWalletActivity(r, pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Page: &p}) {
		t.Fatal("page refused")
	}
	p.Items[0].Block.Height = 0
	if b.activityRows.Items[0].Block.Height != 1 {
		t.Fatal("retained nested block aliases caller")
	}
	r.Command = "wallet_activity_detail"
	r.Limit = 0
	r.TxID = item.TxID
	item = pilot.CloneWalletActivityItem(b.activityRows.Items[0])
	d := pilot.WalletActivityDetail{Version: 1, Binding: j.Binding, JobID: j.JobID, Capture: *j.Capture, Item: item, Outputs: []pilot.WalletActivityOutput{{Index: 0, Atoms: 1, Owned: true}}}
	if !b.correlateWalletActivity(r, pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Detail: &d}) {
		t.Fatal("selected owned no-address output refused")
	}
	d.Item.RawSHA256 = strings.Repeat("e", 64)
	if b.correlateWalletActivity(r, pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Detail: &d}) {
		t.Fatal("changed selected bytes accepted")
	}
	newer := strings.Repeat("f", 64)
	if e := b.RememberWalletActivity(r.ViewID, newer); e != nil {
		t.Fatal(e)
	}
	unknown := *j.Capture
	unknown.CapturedAtMS++
	next := pilot.WalletActivityJob{Version: 1, JobID: newer, Binding: j.Binding, State: "done", Capture: &unknown}
	r.Command = "wallet_activity_job"
	r.JobID = newer
	r.TxID = ""
	if !b.correlateWalletActivity(r, pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Job: &next}) || b.activityRows != nil || b.activityCapture.job.JobID != newer {
		t.Fatal("refresh did not invalidate old selection")
	}
	r.JobID = j.JobID
	if b.activityRequestBoundLocked(r) {
		t.Fatal("old capture remains queryable after replacement")
	}
	b.clearStableView()
	if b.activityBinding != nil || b.activityCapture != nil || b.activityRows != nil || b.activityAttempt != nil {
		t.Fatal("view clear retained activity")
	}
}
func TestWalletActivityBridgeExactMissingAndDispatchRetirement(t *testing.T) {
	for _, dispatched := range []bool{false, true} {
		t.Run(map[bool]string{false: "never-dispatched", true: "uncertain-dispatched"}[dispatched], func(t *testing.T) {
			b, j, _ := activityBridgeFixture(t)
			id := strings.Repeat("e", 64)
			if e := b.RememberWalletActivity(j.Binding.View.ViewID, id); e != nil {
				t.Fatal(e)
			}
			expected := b.activityAttempt
			expected.dispatched = dispatched
			b.gate = make(chan struct{}, 1)
			b.gate <- struct{}{}
			r := pilot.WalletActivityRequest{Command: "wallet_activity_begin", ViewID: j.Binding.View.ViewID, JobID: id}
			got := b.WalletActivityRequest(context.Background(), r)
			<-b.gate
			if got.Error != "busy" || expected.rejected == dispatched || b.activityCapture.job.JobID != j.JobID {
				t.Fatal("local busy changed wrong intent")
			}
			next := strings.Repeat("f", 64)
			e := b.RememberWalletActivity(r.ViewID, next)
			if !dispatched {
				if e != nil {
					t.Fatal(e)
				}
				return
			}
			if !errors.Is(e, ErrBusy) {
				t.Fatal("uncertain intent was retired", e)
			}
			r.ID = 1
			r.Version = 1
			r.Command = "wallet_activity_job"
			wrong := r
			wrong.JobID = next
			b.rejectWalletActivityAttempt(wrong, pilot.WalletActivityResponse{ID: 1, Version: 1, Error: "activity_not_found"}, expected)
			if expected.rejected {
				t.Fatal("foreign missing retired intent")
			}
			b.rejectWalletActivityAttempt(r, pilot.WalletActivityResponse{ID: 1, Version: 1, Error: "activity_not_found"}, expected)
			if !expected.rejected || b.activityCapture.job.JobID != j.JobID {
				t.Fatal("exact missing lost old capture or retained intent")
			}
			if e = b.RememberWalletActivity(r.ViewID, next); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestWalletActivityBridgePageNeighbors(t *testing.T) {
	_, j, item := activityBridgeFixture(t)
	first := pilot.WalletActivityPage{JobID: j.JobID, Capture: *j.Capture, After: 0, Next: 1, Items: []pilot.WalletActivityItem{item}}
	next := pilot.CloneWalletActivityPage(first)
	next.After, next.Next = 1, 2
	next.Items[0].TxID = strings.Repeat("e", 64)
	if !activityNeighborPages(&first, &next) {
		t.Fatal("ordered adjacent page refused")
	}
	next.Items[0].TxID = strings.Repeat("a", 64)
	if activityNeighborPages(&first, &next) {
		t.Fatal("reordered adjacent page accepted")
	}
	repeat := pilot.CloneWalletActivityPage(first)
	repeat.Items[0].RawSHA256 = strings.Repeat("e", 64)
	if activityNeighborPages(&first, &repeat) {
		t.Fatal("changed overlapping row accepted")
	}
}
