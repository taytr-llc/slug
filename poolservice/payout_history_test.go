// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

// Modeled transport data; no row is claimed to be an on-chain transaction.
func payoutServiceFixture(count int, gym, binding string) PayoutHistoryResponse {
	h := poolsettle.PayoutHistory{Version: 1, RecipientScript: "0014" + strings.Repeat("1", 40), Balance: poolsettle.Balance{GymID: gym, Confirmed: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, Available: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, PaidAtoms: int64(count) * poolsettle.MinimumAtoms}, Items: make([]poolsettle.PayoutItem, count)}
	for i := range h.Items {
		date := time.Date(2026, 1, 1+count-i, 12, 0, 0, 0, time.UTC)
		h.Items[i] = poolsettle.PayoutItem{Day: date.Format("2006-01-02"), CreatedAtMS: date.UnixMilli(), TxID: fmt.Sprintf("%064x", i+1), OutputIndex: 1, Atoms: poolsettle.MinimumAtoms, RecordedState: "confirmed"}
		if i == 0 {
			h.SettlementObservedAtMS = date.UnixMilli()
		}
	}
	return PayoutHistoryResponse{Version: 1, BindingID: binding, History: h}
}

func TestPayoutHTTPClosedPreflight(t *testing.T) {
	value := payoutServiceFixture(1, strings.Repeat("a", 64), strings.Repeat("b", 64))
	raw, _ := json.Marshal(value)
	for _, valid := range [][]byte{raw, append(bytes.Clone(raw), '\n'), append([]byte(" \t\r\n"), raw...)} {
		var got PayoutHistoryResponse
		if err := decodePayoutResponse(valid, &got); err != nil || !reflect.DeepEqual(got, value) {
			t.Fatal("closed canonical response", err)
		}
	}
	for name, bad := range map[string][]byte{
		"null":               []byte("null"),
		"nonascii padding":   append([]byte{0xc2, 0xa0}, raw...),
		"inner whitespace":   bytes.Replace(raw, []byte(`"version":1`), []byte(`"version": 1`), 1),
		"unknown":            bytes.Replace(raw, []byte(`"version":1`), []byte(`"extra":1,"version":1`), 1),
		"duplicate":          bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		"omitted":            bytes.Replace(raw, []byte(`"version":1,`), nil, 1),
		"escape":             bytes.Replace(raw, []byte(`"day":"2026`), []byte(`"day":"\u0032026`), 1),
		"fraction oversized": bytes.Replace(raw, []byte(`"numerator":"0"`), []byte(`"numerator":"`+strings.Repeat("1", 4097)+`"`), 1),
		"exponent":           bytes.Replace(raw, []byte(`"output_index":1`), []byte(`"output_index":1e0`), 1),
		"boolean number":     bytes.Replace(raw, []byte(`"output_index":1`), []byte(`"output_index":true`), 1),
		"null items":         bytes.Replace(raw, raw[bytes.Index(raw, []byte(`"items":`))+8:len(raw)-2], []byte("null"), 1),
		"array nesting":      []byte(`{"version":1,"binding_id":"","history":[[[[[[]]]]]]}`),
		"second value":       append(bytes.Clone(raw), []byte("{}")...),
		"frame too large":    append(bytes.Clone(raw), bytes.Repeat([]byte(" "), MaxPayoutResponseSize-len(raw)+1)...),
	} {
		t.Run(name, func(t *testing.T) {
			var got PayoutHistoryResponse
			if decodePayoutResponse(bad, &got) == nil {
				t.Fatal("malformed closed response accepted")
			}
		})
	}
	tooMany, _ := json.Marshal(payoutServiceFixture(367, value.History.Balance.GymID, value.BindingID))
	var got PayoutHistoryResponse
	if decodePayoutResponse(tooMany, &got) == nil {
		t.Fatal("367 items decoded")
	}
	boundary := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), MaxPayoutResponseSize-len(raw))...)
	if decodePayoutResponse(boundary, &got) != nil {
		t.Fatal("whole-body inclusive byte cap rejected")
	}
}

func TestPayoutHTTPClientBindingCapAndReadErrors(t *testing.T) {
	c, _, _ := normalClient(t)
	value := payoutServiceFixture(366, c.gym, c.binding)
	raw, _ := json.Marshal(value)
	if len(raw)+1 <= MaxResponseSize || len(raw)+1 > MaxPayoutResponseSize {
		t.Fatal("modeled large capture does not exercise route-only cap")
	}
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != pathPrefix+"payouts" {
			t.Fatal("unexpected history route")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" {
			t.Fatal("history request supplied selectors")
		}
		return clientResponse(t, 200, value), nil
	})
	got, err := c.Payouts(context.Background())
	if err != nil || !reflect.DeepEqual(got, value) {
		t.Fatal("large owned capture", err)
	}
	got.History.Items[0].Atoms++
	if value.History.Items[0].Atoms != poolsettle.MinimumAtoms {
		t.Fatal("response alias")
	}
	for name, mutate := range map[string]func(*PayoutHistoryResponse){
		"source": func(v *PayoutHistoryResponse) { v.BindingID = strings.Repeat("0", 64) },
		"gym":    func(v *PayoutHistoryResponse) { v.History.Balance.GymID = strings.Repeat("0", 64) },
		"paid":   func(v *PayoutHistoryResponse) { v.History.Balance.PaidAtoms++ },
		"state":  func(v *PayoutHistoryResponse) { v.History.Items[0].RecordedState = "final" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := payoutServiceFixture(1, c.gym, c.binding)
			mutate(&bad)
			c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) { return clientResponse(t, 200, bad), nil })
			result, err := c.Payouts(context.Background())
			clientError(t, err, "unavailable", false)
			if !reflect.DeepEqual(result, PayoutHistoryResponse{}) {
				t.Fatal("invalid result retained payload")
			}
		})
	}
	for _, code := range []string{"invalid_request", "unauthorized", "forbidden", "too_large", "rate_limited", "busy", "capacity", "unavailable", "not_found", "expired", "rejected", "outcome_unknown"} {
		c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
			return clientResponse(t, errorStatus(code), ErrorResponse{Version1, APIError{code, code == "outcome_unknown"}}), nil
		})
		_, err := c.Payouts(context.Background())
		want := code
		if !payoutErrorCode(code) {
			want = "unavailable"
		}
		clientError(t, err, want, false)
	}
	for _, padding := range []string{"\u00a0", " "} {
		c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(padding + `{"version":1,"error":{"code":"unauthorized","outcome_unknown":false}}`))}, nil
		})
		_, err := c.Payouts(context.Background())
		want := "unauthorized"
		if padding != " " {
			want = "unavailable"
		}
		clientError(t, err, want, false)
	}
	// The larger family cap is never inherited by the old Balance operation.
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) { return clientResponse(t, 200, value), nil })
	_, err = c.Balance(context.Background())
	clientError(t, err, "unavailable", false)
}

func TestPayoutHTTPServerOwnEmptyCaptureAndClosedRequests(t *testing.T) {
	h := newServerHarness(t)
	for _, tc := range []struct{ token, gym, other string }{{serverTokenA, h.gymA, h.gymB}, {serverTokenB, h.gymB, h.gymA}} {
		w := serverCall(h.server, "payouts", tc.token, struct{}{})
		var result PayoutHistoryResponse
		if w.Code != 200 || decodePayoutResponse(w.Body.Bytes(), &result) != nil || result.History.Balance.GymID != tc.gym || len(result.History.Items) != 0 || result.History.RecipientScript == "" || bytes.Contains(w.Body.Bytes(), []byte(tc.other)) || !bytes.HasSuffix(w.Body.Bytes(), []byte("\n")) || bytes.Count(w.Body.Bytes(), []byte("\n")) != 1 {
			t.Fatal("own initial projection", w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"gym_id":"other"}`, `{"txid":"other"}`, "null", "{ }", "{}{}", "\u00a0{}"} {
		w := httptest.NewRecorder()
		h.server.ServeHTTP(w, serverRequest(pathPrefix+"payouts", serverTokenA, body))
		serverFailure(t, w, 400, "invalid_request")
	}
	w := serverCall(h.server, "payouts", strings.Repeat("0", 64), struct{}{})
	serverFailure(t, w, 401, "unauthorized")
	h.server.active <- struct{}{}
	w = serverCall(h.server, "payouts", serverTokenB, struct{}{})
	<-h.server.active
	serverFailure(t, w, 503, "busy")
	if h.node.resolveCalls.Load() != 0 || h.sessions.nested.Load() != 0 {
		t.Fatal("history issued work or nested account leases")
	}
}

func TestPayoutHTTPServerFinalOriginalCredentialFence(t *testing.T) {
	for _, failAt := range []int{3, 4} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			h := newServerHarness(t)
			calls := 0
			h.server.sessions = serverSessionFunc(func(ctx context.Context, token string, fn func(string, func() error) error) error {
				calls++
				if token != serverTokenA {
					t.Fatal("credential replaced")
				}
				if calls == failAt {
					return errors.New("original credential revoked")
				}
				return fn(h.publicA, ctx.Err)
			})
			w := serverCall(h.server, "payouts", serverTokenA, struct{}{})
			serverFailure(t, w, 401, "unauthorized")
			if bytes.Contains(w.Body.Bytes(), []byte(h.gymA)) {
				t.Fatal("revoked read exposed payload")
			}
		})
	}
}

func TestPayoutHTTPProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	var client *Client
	if value, err := client.Payouts(nil); err == nil || !reflect.DeepEqual(value, PayoutHistoryResponse{}) {
		t.Fatal("cheap payout client reached transport")
	}
	server := &Server{}
	w := serverCall(server, "payouts", serverTokenA, struct{}{})
	serverFailure(t, w, 503, "unavailable")
}

func TestPayoutHTTPServerCancelledAuthenticationIsUnavailable(t *testing.T) {
	h := newServerHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	h.server.sessions = serverSessionFunc(func(c context.Context, token string, fn func(string, func() error) error) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return fn(h.publicA, c.Err)
	})
	r := serverRequest(pathPrefix+"payouts", serverTokenA, "{}")
	// Keep the original literal host; the local-address check is optional
	// outside a real listener, while cancellation must reach core capture.
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, r)
	serverFailure(t, w, 503, "unavailable")
	if bytes.Contains(w.Body.Bytes(), []byte(h.gymA)) {
		t.Fatal("cancelled read disclosed a gym")
	}
}
