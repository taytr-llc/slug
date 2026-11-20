// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
)

func payoutAuthorityResponse(h *poolDesktopHarness) poolservice.PayoutHistoryResponse {
	return poolservice.PayoutHistoryResponse{Version: 1, BindingID: h.response.BindingID, History: poolsettle.PayoutHistory{Version: 1, RecipientScript: "0014" + strings.Repeat("1", 40), Balance: h.response.Balance, SettlementObservedAtMS: h.response.ObservedAtMS, Items: []poolsettle.PayoutItem{}}}
}

func TestPoolPayoutAuthorityHeldViewAndExactReadRoute(t *testing.T) {
	h := newPoolDesktopHarness(t)
	expected := payoutAuthorityResponse(h)
	view := h.authority.account.ViewLease
	h.authority.account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return view(ctx, func(account gymaccount.Account, check func() error) error {
			// A modeled already-authorized View during a hold. This adapter
			// must not add RecoveryExport/Withdrawal checks to a read.
			account.HoldUntilMS = time.Now().Add(24 * time.Hour).UnixMilli()
			return fn(account, check)
		})
	}
	h.hook = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pool/v1/payouts" || r.URL.RawQuery != "" {
			t.Error("history used an unrelated or mutating route")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}
	got, err := h.authority.Payouts(context.Background())
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatal("held original View could not read", err)
	}
	if h.authority.SourceConfig() != h.config.Source || h.authority.TransportTrustID() != h.config.TransportTrustID {
		t.Fatal("read changed source binding")
	}
}

func TestPoolPayoutAuthorityOriginalTokenSurvivesBlockedHTTP(t *testing.T) {
	h := newPoolDesktopHarness(t)
	expected := payoutAuthorityResponse(h)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	respond := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}
	h.hook = func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		respond(w, r)
	}
	type result struct {
		value poolservice.PayoutHistoryResponse
		err   error
	}
	done := make(chan result, 1)
	go func() { value, err := h.authority.Payouts(context.Background()); done <- result{value, err} }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP not entered")
	}
	if h.sessions.active.Load() {
		t.Fatal("blocked HTTP retained account lease")
	}
	h.sessions.mu.Lock()
	h.sessions.tokens[strings.Repeat("1", 64)] = false
	h.sessions.mu.Unlock()
	h.mu.Lock()
	h.token = strings.Repeat("2", 64)
	h.mu.Unlock()
	once.Do(func() { close(release) })
	select {
	case got := <-done:
		if got.err == nil || !reflect.DeepEqual(got.value, poolservice.PayoutHistoryResponse{}) {
			t.Fatal("revoked original token published a payload", got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revoked read did not drain")
	}
	h.mu.Lock()
	if len(h.requests) != 1 || h.requests[0] != "Bearer "+strings.Repeat("1", 64) {
		t.Error("read retried a replacement credential")
	}
	h.hook = respond
	h.mu.Unlock()
	if got, err := h.authority.Payouts(context.Background()); err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatal("next explicit operation could not use authorized replacement", err)
	}
}

func TestPoolPayoutAuthorityCloseJoinsHTTP(t *testing.T) {
	h := newPoolDesktopHarness(t)
	entered := make(chan struct{})
	h.hook = func(_ http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }
	done := make(chan error, 1)
	go func() { _, err := h.authority.Payouts(context.Background()); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not enter")
	}
	if err := h.authority.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed read succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close failed to join")
	}
	if got, err := h.authority.Payouts(context.Background()); err == nil || !reflect.DeepEqual(got, poolservice.PayoutHistoryResponse{}) {
		t.Fatal("closed history authority reused")
	}
}
