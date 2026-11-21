// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
)

func dailyServerCall(s *Server, path, token string, value any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(value)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, serverRequest(dailyPathPrefix+path, token, string(raw)))
	return w
}
func dailyServerFailure(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var e ErrorResponse
	if w.Code != status || decodeDailyError(w.Body.Bytes(), &e) != nil || e.Version != Version2 || e.Error.Code != code || e.Error.OutcomeUnknown != (code == "outcome_unknown") {
		t.Fatalf("daily error: status%d body%s", w.Code, w.Body.String())
	}
}
func TestDailyServerOriginalOwnersAndMoneyBinding(t *testing.T) {
	h := newDailyServerHarness(t)
	if _, err := NewServer(ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, h.pool, h.settlement, h.sessions); err == nil {
		t.Fatal("legacy server selected daily source")
	}
	if _, err := NewDailyServer(context.Background(), ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, h.pool, h.settlement, h.sessions); err == nil {
		t.Fatal("daily server selected legacy config")
	}
	w := dailyServerCall(h.server, "issue", serverTokenA, struct{}{})
	var work DailyWorkResponse
	if w.Code != 200 || decodeDailyStrict(w.Body.Bytes(), MaxWorkResponseSize, &work) != nil || work.Version != 2 || work.BindingID != h.server.binding || work.SettlementConfigID != h.server.daily.settlement || work.RecipientScript != h.server.daily.destinations[h.gymA] || work.Work.Job.PublicKey != h.publicA {
		t.Fatalf("daily work envelope %d %s", w.Code, w.Body.String())
	}
	got := dailyServerCall(h.server, "work", serverTokenA, workRequest{work.Work.Job.ID})
	var recovered DailyWorkResponse
	if got.Code != 200 || decodeDailyStrict(got.Body.Bytes(), MaxWorkResponseSize, &recovered) != nil || !reflect.DeepEqual(work, recovered) {
		t.Fatal("original work changed")
	}
	other := dailyServerCall(h.server, "work", serverTokenB, workRequest{work.Work.Job.ID})
	if other.Code == 200 {
		t.Fatal("another gym received work")
	}
	receipt := dailyServerCall(h.server, "submit", serverTokenA, submissionRequest{work.Work.Job.ID, 0})
	var saved DailySubmissionResponse
	if receipt.Code != 200 || decodeDailyStrict(receipt.Body.Bytes(), MaxResponseSize, &saved) != nil || saved.RecipientScript != work.RecipientScript || saved.SettlementConfigID != work.SettlementConfigID || saved.Submission.JobID != work.Work.Job.ID || saved.Submission.Sequence != 1 {
		t.Fatalf("original receipt: %d %s", receipt.Code, receipt.Body.String())
	}
	lookup := dailyServerCall(h.server, "lookup", serverTokenA, submissionRequest{work.Work.Job.ID, 0})
	var duplicate DailySubmissionResponse
	if lookup.Code != 200 || decodeDailyStrict(lookup.Body.Bytes(), MaxResponseSize, &duplicate) != nil || !reflect.DeepEqual(saved, duplicate) {
		t.Fatal("exact retained receipt changed")
	}
	balance := dailyServerCall(h.server, "balance", serverTokenA, struct{}{})
	var b DailyBalanceResponse
	if balance.Code != 200 || decodeDailyBalance(balance.Body.Bytes(), &b) != nil || b.Balance.GymID != h.gymA || b.RecipientScript != work.RecipientScript || b.SettlementConfigID != work.SettlementConfigID {
		t.Fatal("balance binding")
	}
	page := dailyServerCall(h.server, "payouts", serverTokenA, poolsettle.DailyPayoutRequest{})
	var p poolsettle.DailyPayoutPage
	if page.Code != 200 || decodeDailyPage(page.Body.Bytes(), &p) != nil || p.Revision == 0 || p.HighBatch != 0 || p.BeforeBatch != 1 || !p.Done || p.NextBefore != 0 || p.Items == nil || p.RecipientScript != work.RecipientScript {
		t.Fatalf("empty capture %d %s", page.Code, page.Body.String())
	}
	if h.sessions.nested.Load() != 0 {
		t.Fatal("nested original session lease")
	}
	if h.server.pool != h.pool || h.server.settlement != h.settlement {
		t.Fatal("replaced original owners")
	}
}
func TestDailyServerRefusesBeforeEffects(t *testing.T) {
	h := newDailyServerHarness(t)
	before := h.node.resolveCalls.Load()
	// Cross-mode route and malformed canonical body precede source effects.
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, serverRequest(pathPrefix+"issue", serverTokenA, "{}"))
	dailyServerFailure(t, w, 400, "invalid_request")
	w = httptest.NewRecorder()
	h.server.ServeHTTP(w, serverRequest(dailyPathPrefix+"issue", serverTokenA, "\u00a0{}"))
	dailyServerFailure(t, w, 400, "invalid_request")
	key, _, _ := serverFixtureKey(7)
	token := strings.Repeat("7", 64)
	h.sessions.mu.Lock()
	h.sessions.keys[token] = key
	h.sessions.mu.Unlock()
	w = dailyServerCall(h.server, "issue", token, struct{}{})
	dailyServerFailure(t, w, 401, "unauthorized")
	if h.node.resolveCalls.Load() != before {
		t.Fatal("invalid/nonmember request reached node")
	}
	w = dailyServerCall(h.server, "payouts", serverTokenA, poolsettle.DailyPayoutRequest{Revision: 1, HighBatch: 5, BeforeBatch: 6})
	dailyServerFailure(t, w, 400, "invalid_request")
	// Correctly shaped but stale capture is a definite read-only409, never unknown.
	w = dailyServerCall(h.server, "payouts", serverTokenA, poolsettle.DailyPayoutRequest{Revision: 99, HighBatch: 1, BeforeBatch: 2})
	dailyServerFailure(t, w, 409, "stale_capture")
	h.sessions.revoke(serverTokenA)
	w = dailyServerCall(h.server, "balance", serverTokenA, struct{}{})
	dailyServerFailure(t, w, 401, "unauthorized")
	h.server.active <- struct{}{}
	w = dailyServerCall(h.server, "issue", serverTokenB, struct{}{})
	<-h.server.active
	dailyServerFailure(t, w, 503, "busy")
	if _, err := h.pool.AccountingPage(context.Background(), 0, 0, 1); err != nil {
		t.Fatal(err)
	}
	if poolshare.Version2 != h.source.Version {
		t.Fatal("fixture version")
	}
}
