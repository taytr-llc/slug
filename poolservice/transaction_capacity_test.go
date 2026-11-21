// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
)

func TestPoolClientTransactionCapacityBalance(t *testing.T) {
	c, _, _ := normalClient(t)
	want := BalanceResponse{
		Version: Version1, BindingID: c.binding, ObservedAtMS: 1795000000000,
		Balance: poolsettle.Balance{
			GymID:     c.gym,
			Confirmed: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"},
			Available: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"},
			PaidAtoms: 990000000, Reason: "transaction-capacity",
		},
	}
	response, calls := want, 0
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != pathPrefix+"balance" || r.Header.Get("Authorization") != "Bearer "+c.config.Token {
			t.Fatal("balance request lost its original authenticated binding")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil || string(raw) != "{}" {
			t.Fatal("balance request carried claimant-selected identity", err)
		}
		return clientResponse(t, http.StatusOK, response), nil
	})
	got, err := c.Balance(context.Background())
	if err != nil || got != want || calls != 1 {
		t.Fatal("whole-payout capacity reason or exact balance was lost", got, err)
	}
	// The one new reason must not turn the closed grammar into arbitrary text.
	response.Balance.Reason = "transaction-capacity-unrecognized"
	got, err = c.Balance(context.Background())
	clientError(t, err, "unavailable", false)
	if got != (BalanceResponse{}) || calls != 2 {
		t.Fatal("unknown reason disclosed a balance or triggered retry")
	}
}
