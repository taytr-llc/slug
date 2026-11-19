// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithdrawalNativeExactAmountInput(t *testing.T) {
	for _, tc := range []struct {
		text  string
		atoms int64
	}{{"0.00000001", 1}, {"1", 100000000}, {"29.70000000", 2970000000}, {"21000000", 2100000000000000}} {
		got, e := withdrawalAtoms(tc.text)
		if e != nil || got != tc.atoms {
			t.Fatal(tc, got, e)
		}
	}
	for _, bad := range []string{"", "0", "-1", "+1", " 1", "1 ", "1e2", ".1", "1.", "01", "1,000", "0.000000001", "21000000.00000001", "9999999999999999999999999"} {
		if _, e := withdrawalAtoms(bad); e == nil {
			t.Fatal("accepted inexact/out-of-range amount", bad)
		}
	}
	if withdrawalAmount(1) != "0.00000001 SLUG" || withdrawalAmount(2970000000) != "29.70000000 SLUG" {
		t.Fatal("native review rounding")
	}
}
func nativeWithdrawalFixture() (*Gym, *nativeWithdrawalAction) {
	b := &bridge.PoolBridge{}
	g := &Gym{poolHelper: b}
	view, op, action := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	a := &nativeWithdrawalAction{helper: b, request: pilot.WithdrawalRequest{ViewID: view, OperationID: op, ActionID: action}, state: "admitted", binding: pilot.WithdrawalBinding{View: pilot.ManifestViewPin{ViewID: view}}}
	a.review = &pilot.WithdrawalReview{OperationID: op, ActionID: action, ReviewID: strings.Repeat("d", 64), ReviewedAtMS: time.Now().UnixMilli() - 1, ExpiresAtMS: time.Now().UnixMilli() + 10000}
	g.withdrawal.action = a
	return g, a
}
func TestWithdrawalNativeLostPrepareLookupUsesExactRetainedAction(t *testing.T) {
	g, a := nativeWithdrawalFixture()
	_, op, ok := g.withdrawalActionSelection(a.request.ViewID, "", a.request.ActionID)
	if !ok || op != a.request.OperationID {
		t.Fatal("exact lost response cannot resolve")
	}
	for _, r := range []pilot.WithdrawalRequest{{ViewID: "other", OperationID: "", ActionID: a.request.ActionID}, {ViewID: a.request.ViewID, OperationID: "other", ActionID: a.request.ActionID}, {ViewID: a.request.ViewID, OperationID: "", ActionID: "other"}} {
		if _, _, ok = g.withdrawalActionSelection(r.ViewID, r.OperationID, r.ActionID); ok {
			t.Fatal("foreign lookup selected latest")
		}
	}
	g.poolHelper = &bridge.PoolBridge{}
	if _, _, ok = g.withdrawalActionSelection(a.request.ViewID, "", a.request.ActionID); ok {
		t.Fatal("replacement helper inherited action")
	}
}
func TestWithdrawalNativeAffirmativeIsOneShotAndOwnerBound(t *testing.T) {
	g, a := nativeWithdrawalFixture()
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_, yes, ok := g.takeWithdrawalAnswer(a, true)
			if ok && yes {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("double native callback could send twice")
	}
	for _, mode := range []string{"expired", "early", "revoked", "replaced", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			g, a := nativeWithdrawalFixture()
			switch mode {
			case "expired":
				a.review.ExpiresAtMS = time.Now().UnixMilli()
			case "early":
				a.review.ReviewedAtMS = time.Now().UnixMilli() + 1000
			case "revoked":
				a.state = "revoked"
			case "replaced":
				g.poolHelper = &bridge.PoolBridge{}
			case "stopped":
				a.answered = true
			}
			_, yes, ok := g.takeWithdrawalAnswer(a, true)
			if ok && yes {
				t.Fatal("late or foreign native affirmative accepted")
			}
		})
	}
}
func TestWithdrawalNativeViewInvalidationRetainsRecoveryWithoutApproval(t *testing.T) {
	g, a := nativeWithdrawalFixture()
	g.observeWithdrawalAccount(g.poolHelper, "resetting", nil)
	if a.state != "revoked" || !a.answered || a.request.OperationID == "" {
		t.Fatal("reset lost durable recovery or retained approval")
	}
	if _, yes, ok := g.takeWithdrawalAnswer(a, true); yes || ok {
		t.Fatal("late affirmative after reset")
	}
}
