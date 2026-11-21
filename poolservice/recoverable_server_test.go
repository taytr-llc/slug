//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"net/http/httptest"
	"strings"
	"testing"
)

func recoveryServerCall(s *Server, path, token string, v any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(v)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, serverRequest(recoverablePathPrefix+path, token, string(raw)))
	return w
}
func recoveryFailure(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var r ErrorResponse
	if w.Code != status || decodeDailyError(w.Body.Bytes(), &r) != nil || r.Version != 3 || r.Error.Code != code || r.Error.OutcomeUnknown != (code == "outcome_unknown") {
		t.Fatalf("failure %d %s", w.Code, w.Body.String())
	}
}
func TestRecoverableServerOriginalOwnersAndExactRequest(t *testing.T) {
	h, p := newRecoveryServerHarness(t)
	ctx := context.Background()
	if _, e := NewDailyServer(ctx, ServerConfig{Version: 2, Listen: "127.0.0.1:0"}, h.pool, h.settlement, h.sessions); e == nil {
		t.Fatal("plain daily adopted selected source")
	}
	wrong := p
	wrong.BaseSetupID = strings.Repeat("e", 64)
	if _, e := NewRecoverableDailyServer(ctx, ServerConfig{Version: 3, Listen: "127.0.0.1:0"}, h.pool, h.settlement, h.sessions, wrong); e == nil {
		t.Fatal("adopted wrong expected profile")
	}
	id := strings.Repeat("1", 64)
	absent := recoveryServerCall(h.server, "issue-lookup", serverTokenA, issueRequest{id})
	var a IssueResponse
	if absent.Code != 200 || decodeRecoverableIssue(absent.Body.Bytes(), &a) != nil || a.Result.State != "absent" {
		t.Fatalf("absent %d %s", absent.Code, absent.Body.String())
	}
	first := recoveryServerCall(h.server, "issue", serverTokenA, issueRequest{id})
	var r IssueResponse
	if first.Code != 200 || decodeRecoverableIssue(first.Body.Bytes(), &r) != nil || r.Result.State != "work" || r.Result.Work == nil || r.PublicKey != h.publicA || r.GymID != h.gymA || r.IssueProfileID != h.server.recoverable.profileID {
		t.Fatalf("issue %d %s", first.Code, first.Body.String())
	}
	// Both exact replay operations retain all public evidence and do not ask the
	// node for another work template. Authenticated token rotation preserves it.
	before := h.node.resolveCalls.Load()
	replay := recoveryServerCall(h.server, "issue", serverTokenRenewed, issueRequest{id})
	lookup := recoveryServerCall(h.server, "issue-lookup", serverTokenA, issueRequest{id})
	if replay.Code != 200 || lookup.Code != 200 || replay.Body.String() != first.Body.String() || lookup.Body.String() != first.Body.String() || h.node.resolveCalls.Load() != before {
		t.Fatal("same request was replaced or reissued")
	}
	foreign := recoveryServerCall(h.server, "issue-lookup", serverTokenB, issueRequest{id})
	if foreign.Code == 200 {
		t.Fatal("cross-account disclosure")
	}
	work := recoveryServerCall(h.server, "work", serverTokenA, workRequest{r.Result.JobID})
	var w RecoverableWorkResponse
	if work.Code != 200 || decodeDailyStrict(work.Body.Bytes(), MaxWorkResponseSize, &w) != nil || w.IssueProfileID != r.IssueProfileID || w.Version != 3 || w.Work.Job.ID != r.Result.JobID {
		t.Fatal("retained work envelope")
	}
	money := recoveryServerCall(h.server, "balance", serverTokenA, struct{}{})
	var b RecoverableBalanceResponse
	if money.Code != 200 || decodeRecoverableBalance(money.Body.Bytes(), &b) != nil || b.IssueProfileID != r.IssueProfileID || b.SettlementConfigID != r.SettlementConfigID || b.RecipientScript != r.RecipientScript {
		t.Fatal("selected money binding")
	}
	page := recoveryServerCall(h.server, "payouts", serverTokenA, poolsettle.DailyPayoutRequest{})
	var history RecoverablePayoutPage
	if page.Code != 200 || decodeRecoverablePage(page.Body.Bytes(), &history) != nil || history.IssueProfileID != r.IssueProfileID || !history.Done {
		t.Fatal("selected page")
	}
	if h.sessions.nested.Load() != 0 || h.server.pool != h.pool || h.server.settlement != h.settlement {
		t.Fatal("original owner/session boundary")
	}
}
func TestRecoverableServerRefusesBeforeEffects(t *testing.T) {
	h, _ := newRecoveryServerHarness(t)
	before := h.node.resolveCalls.Load()
	for _, path := range []string{pathPrefix + "issue", dailyPathPrefix + "issue"} {
		w := httptest.NewRecorder()
		h.server.ServeHTTP(w, serverRequest(path, serverTokenA, "{}"))
		recoveryFailure(t, w, 400, "invalid_request")
	}
	for _, raw := range []string{"{}", `{"request_id":"` + strings.Repeat("0", 64) + `"}`, `{"request_id":"` + strings.Repeat("1", 64) + `","extra":1}`} {
		w := httptest.NewRecorder()
		h.server.ServeHTTP(w, serverRequest(recoverablePathPrefix+"issue-lookup", serverTokenA, raw))
		recoveryFailure(t, w, 400, "invalid_request")
	}
	if h.node.resolveCalls.Load() != before {
		t.Fatal("invalid request reached source")
	}
	h.sessions.revoke(serverTokenA)
	w := recoveryServerCall(h.server, "issue-lookup", serverTokenA, issueRequest{strings.Repeat("1", 64)})
	recoveryFailure(t, w, 401, "unauthorized")
	h.server.active <- struct{}{}
	w = recoveryServerCall(h.server, "issue", serverTokenB, issueRequest{strings.Repeat("2", 64)})
	<-h.server.active
	recoveryFailure(t, w, 503, "busy")
}
