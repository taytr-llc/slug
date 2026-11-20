// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
)

// These are consent/correlation cases, not transaction or signature vectors.
func TestOwnedNativeConsentIsOneUseAndSeparate(t *testing.T) {
	for _, delivery := range []bool{false, true} {
		a := &nativeOwnedTransfer{state: "dialog", review: &pilot.OwnedTransferReview{ReviewID: "sign-review"}, delivery: &pilot.OwnedTransferDeliveryReview{ReviewID: "broadcast-review"}}
		request, send := a.takeAnswer(true, true, delivery)
		command, review, state := "transfer_confirm", "sign-review", "confirming"
		if delivery {
			command, review, state = "transfer_deliver", "broadcast-review", "delivering"
		}
		if !send || request.Command != command || request.ReviewID != review || a.state != state {
			t.Fatal("wrong consent capability", request, a.state)
		}
		if _, send = a.takeAnswer(true, true, delivery); send {
			t.Fatal("duplicate callback dispatched twice")
		}
		if _, send = a.takeAnswer(true, true, !delivery); send {
			t.Fatal("one consent authorized the other operation")
		}
	}
}
func TestOwnedNativeConsentCancellationAndRevocation(t *testing.T) {
	for _, current := range []bool{false, true} {
		a := &nativeOwnedTransfer{state: "dialog", review: &pilot.OwnedTransferReview{ReviewID: "sign-review"}, delivery: &pilot.OwnedTransferDeliveryReview{ReviewID: "broadcast-review"}}
		if _, send := a.takeAnswer(current, false, false); send {
			t.Fatal("cancel dispatched")
		}
		expected := "cancelled"
		if !current {
			expected = "revoked"
		}
		if !a.answered || a.state != expected || a.review != nil || a.delivery != nil {
			t.Fatal("consent retained after cancellation/revocation")
		}
		if _, send := a.takeAnswer(true, true, false); send {
			t.Fatal("late callback resurrected consent")
		}
	}
	for _, state := range []string{"preparing", "review", "complete", "unknown", "failed", "cancelled", "revoked"} {
		a := &nativeOwnedTransfer{state: state, review: &pilot.OwnedTransferReview{ReviewID: "sign-review"}}
		if _, send := a.takeAnswer(true, true, false); send {
			t.Fatal("dispatched outside native dialog", state)
		}
	}
	for _, delivery := range []bool{false, true} {
		a := &nativeOwnedTransfer{state: "dialog"}
		if _, send := a.takeAnswer(true, true, delivery); send {
			t.Fatal("missing retained review dispatched")
		}
		if a.state != "failed" || !a.answered {
			t.Fatal("missing capability left dialog live")
		}
	}
}
func TestOwnedNativeReviewShowsExactRecipientAndCosts(t *testing.T) {
	r := fightercarrier.Review{SenderGym: "original sender", RecipientGym: "exact recipient", TotalFeeAtoms: 20000, CommitFeeAtoms: 10000, SpendFeeAtoms: 10000, ReturnAddress: "original return address", ReturnBranch: 1, ReturnIndex: 7, CommitChangeAtoms: 300000, ReturnedAtoms: 490000}
	text := ownedTransferMessage("full fighter identifier", r)
	for _, want := range []string{"full fighter identifier", r.SenderGym, r.RecipientGym, r.ReturnAddress, "0.00020000", "0.00010000", "0.00300000", "0.00490000", "branch 1, index 7"} {
		if !strings.Contains(text, want) {
			t.Fatal("review omitted exact field", want)
		}
	}
}
