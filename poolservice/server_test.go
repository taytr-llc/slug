// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

func serverRequest(path, token, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Host = "127.0.0.1:9311"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	return r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9311}))
}
func serverCall(s *Server, path, token string, value any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(value)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, serverRequest(pathPrefix+path, token, string(raw)))
	return w
}
func serverFailure(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var e ErrorResponse
	if json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Version != Version1 || e.Error.Code != code || e.Error.OutcomeUnknown != (code == "outcome_unknown") {
		t.Fatalf("invalid error envelope: %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing private response headers")
	}
}
func serverIssue(t *testing.T, h *serverHarness, token string) poolshare.MiningWork {
	t.Helper()
	w := serverCall(h.server, "issue", token, struct{}{})
	if w.Code != 200 {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	var result WorkResponse
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Version != Version1 || result.BindingID == "" {
		t.Fatal("invalid issued-work envelope")
	}
	return result.Work
}

func TestServerRejectsHTTPShapeAndOperatorRoutes(t *testing.T) {
	h := newServerHarness(t)
	cases := []struct {
		name   string
		mutate func(*http.Request)
		status int
		code   string
	}{
		{"remote-host", func(r *http.Request) { r.Host = "evil.example:9311" }, 403, "forbidden"},
		{"wrong-local-port", func(r *http.Request) { r.Host = "127.0.0.1:9312" }, 403, "forbidden"},
		{"origin", func(r *http.Request) { r.Header.Set("Origin", "https://example.invalid") }, 403, "forbidden"},
		{"empty-origin", func(r *http.Request) { r.Header["Origin"] = []string{""} }, 403, "forbidden"},
		{"get", func(r *http.Request) { r.Method = "GET" }, 400, "invalid_request"},
		{"query", func(r *http.Request) { r.URL.RawQuery = "gym=other" }, 400, "invalid_request"},
		{"escaped-route", func(r *http.Request) { r.URL.RawPath = "/pool/v1/%62alance" }, 400, "invalid_request"},
		{"encoding", func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, 400, "invalid_request"},
		{"wrong-content-type", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 400, "invalid_request"},
		{"duplicate-content-type", func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 400, "invalid_request"},
		{"operator-payout", func(r *http.Request) { r.URL.Path = pathPrefix + "run_daily" }, 400, "invalid_request"},
		{"accounting-export", func(r *http.Request) { r.URL.Path = pathPrefix + "accounting" }, 400, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := serverRequest(pathPrefix+"balance", serverTokenA, "{}")
			tc.mutate(r)
			w := httptest.NewRecorder()
			h.server.ServeHTTP(w, r)
			serverFailure(t, w, tc.status, tc.code)
		})
	}
	if h.node.resolveCalls.Load() != 0 {
		t.Fatal("malformed request reached work issuance")
	}
}

func TestServerStrictBodiesAndAuthentication(t *testing.T) {
	h := newServerHarness(t)
	for _, body := range []string{"null", "[]", "{\"gym_id\":\"other\"}", "{}{}", "{ \"unexpected\": 1 }", "{\"x\":1,\"x\":2}"} {
		w := httptest.NewRecorder()
		h.server.ServeHTTP(w, serverRequest(pathPrefix+"balance", serverTokenA, body))
		serverFailure(t, w, 400, "invalid_request")
	}
	oversized := serverRequest(pathPrefix+"balance", serverTokenB, strings.Repeat("x", MaxRequestSize+1))
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, oversized)
	serverFailure(t, w, 413, "too_large")
	w = serverCall(h.server, "balance", strings.Repeat("0", 64), struct{}{})
	serverFailure(t, w, 401, "unauthorized")
	r := serverRequest(pathPrefix+"balance", serverTokenB, "{}")
	r.Header.Add("Authorization", "Bearer "+serverTokenA)
	w = httptest.NewRecorder()
	h.server.ServeHTTP(w, r)
	serverFailure(t, w, 401, "unauthorized")
	if h.sessions.nested.Load() != 0 {
		t.Fatal("service nested an account lease around a journal call")
	}
}

type serverSessionFunc func(context.Context, string, func(string, func() error) error) error

func (f serverSessionFunc) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return f(ctx, token, fn)
}

func TestServerRejectsRepeatedOrSwallowedAuthCallback(t *testing.T) {
	h := newServerHarness(t)
	h.server.sessions = serverSessionFunc(func(ctx context.Context, _ string, fn func(string, func() error) error) error {
		_ = fn("invalid", ctx.Err)
		_ = fn(h.publicA, ctx.Err)
		return nil
	})
	w := serverCall(h.server, "balance", serverTokenA, struct{}{})
	serverFailure(t, w, 401, "unauthorized")
	h.server.sessions = serverSessionFunc(func(_ context.Context, _ string, fn func(string, func() error) error) error {
		_ = fn(h.publicA, func() error { return errors.New("fixture revoked") })
		return nil
	})
	w = serverCall(h.server, "balance", serverTokenA, struct{}{})
	serverFailure(t, w, 401, "unauthorized")
}

func TestServerOwnWorkReceiptAndBalanceIsolation(t *testing.T) {
	h := newServerHarness(t)
	work := serverIssue(t, h, serverTokenA)
	if work.Job.PublicKey != h.publicA || work.Job.GymID != h.gymA {
		t.Fatal("session did not bind issued work")
	}
	w := serverCall(h.server, "work", serverTokenA, workRequest{JobID: work.Job.ID})
	if w.Code != 200 {
		t.Fatalf("own work: %s", w.Body.String())
	}
	var recovered WorkResponse
	if json.Unmarshal(w.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(recovered.Work, work) {
		t.Fatal("work recovery changed original issuance")
	}
	w = serverCall(h.server, "work", serverTokenB, workRequest{JobID: work.Job.ID})
	serverFailure(t, w, 401, "unauthorized")
	prepared, e := prospectwork.PrepareManifestWork(work.Job, work.Template, h.source.PayoutAddress, 0, h.source.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	check, e := prepared.CheckNonce(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	w = serverCall(h.server, "submit", serverTokenA, submissionRequest{JobID: work.Job.ID, Nonce: check.Nonce})
	if w.Code != 200 {
		t.Fatalf("own submission: %d %s", w.Code, w.Body.String())
	}
	var accepted SubmissionResponse
	if json.Unmarshal(w.Body.Bytes(), &accepted) != nil || accepted.Submission.AcceptedSequence != 1 || accepted.Submission.WorkHash != check.WorkHash {
		t.Fatalf("unbound receipt: %s", w.Body.String())
	}
	w = serverCall(h.server, "lookup", serverTokenB, submissionRequest{JobID: work.Job.ID, Nonce: check.Nonce})
	serverFailure(t, w, 401, "unauthorized")
	w = serverCall(h.server, "lookup", serverTokenA, submissionRequest{JobID: work.Job.ID, Nonce: check.Nonce})
	if w.Code != 200 {
		t.Fatalf("own receipt recovery: %s", w.Body.String())
	}
	var found SubmissionResponse
	if json.Unmarshal(w.Body.Bytes(), &found) != nil || found != accepted {
		t.Fatal("lookup changed retained receipt")
	}
	for _, tc := range []struct{ token, gym, other string }{{serverTokenA, h.gymA, h.gymB}, {serverTokenB, h.gymB, h.gymA}} {
		w = serverCall(h.server, "balance", tc.token, struct{}{})
		if w.Code != 200 {
			t.Fatalf("own balance: %s", w.Body.String())
		}
		var result BalanceResponse
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Balance.GymID != tc.gym || bytes.Contains(w.Body.Bytes(), []byte(tc.other)) {
			t.Fatal("balance exposed another gym")
		}
	}
	if h.sessions.nested.Load() != 0 {
		t.Fatal("journal operation ran under nested service session lease")
	}
}

func TestServerRevocationAfterShareCommitIsUnknownAndRecoverable(t *testing.T) {
	h := newServerHarness(t)
	work := serverIssue(t, h, serverTokenA)
	prepared, e := prospectwork.PrepareManifestWork(work.Job, work.Template, h.source.PayoutAddress, 0, h.source.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	var winner prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, e := prepared.CheckNonce(context.Background(), nonce)
		if e != nil {
			t.Fatal(e)
		}
		if check.Block {
			winner = check
			break
		}
	}
	if winner.Candidate == nil {
		t.Fatal("bounded normal-PoW fixture search exhausted")
	}
	h.node.onSubmit = func() { h.sessions.revoke(serverTokenA) }
	w := serverCall(h.server, "submit", serverTokenA, submissionRequest{JobID: work.Job.ID, Nonce: winner.Nonce})
	serverFailure(t, w, 503, "outcome_unknown")
	w = serverCall(h.server, "lookup", serverTokenA, submissionRequest{JobID: work.Job.ID, Nonce: winner.Nonce})
	serverFailure(t, w, 401, "unauthorized")
	w = serverCall(h.server, "lookup", serverTokenRenewed, submissionRequest{JobID: work.Job.ID, Nonce: winner.Nonce})
	if w.Code != 200 {
		t.Fatalf("renewed original gym failed to recover committed share: %d %s", w.Code, w.Body.String())
	}
	var result SubmissionResponse
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Submission.AcceptedSequence != 1 || result.Submission.WorkHash != winner.WorkHash {
		t.Fatal("unknown outcome caused lost or duplicate share")
	}
	if h.sessions.nested.Load() != 0 {
		t.Fatal("nested account lease")
	}
}

func TestServerBusyBeforeAuthenticationAndRunDrains(t *testing.T) {
	h := newServerHarness(t)
	h.node.resolveEntered = make(chan struct{}, 1)
	h.node.resolveRelease = make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(h.node.resolveRelease) }) }
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan string, 1)
	go func() { done <- h.server.Run(ctx, func(origin string) { ready <- origin }) }()
	var origin string
	select {
	case origin = <-ready:
	case e := <-done:
		t.Fatal(e)
	case <-time.After(3 * time.Second):
		t.Fatal("service readiness deadline")
	}
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("service did not drain")
		}
	}()
	request := func(route string) (int, error) {
		r, _ := http.NewRequest(http.MethodPost, origin+pathPrefix+route, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+serverTokenA)
		res, e := (&http.Client{Timeout: 4 * time.Second}).Do(r)
		if e != nil {
			return 0, e
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, nil
	}
	requestDone := make(chan error, 1)
	go func() { _, e := request("issue"); requestDone <- e }()
	select {
	case <-h.node.resolveEntered:
	case e := <-requestDone:
		t.Fatal(e)
	case <-time.After(3 * time.Second):
		t.Fatal("fixture operation did not enter")
	}
	status, e := request("balance")
	if e != nil || status != 503 {
		t.Fatalf("second request queued behind active account lease: status=%d err=%v", status, e)
	}
	if h.sessions.nested.Load() != 0 {
		t.Fatal("busy request reached a nested lease")
	}
	cancel()
	select {
	case e := <-done:
		t.Fatalf("Run returned before actual callback drained: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-requestDone:
	case <-time.After(3 * time.Second):
		t.Fatal("request failed to drain")
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
		done <- nil
	case <-time.After(3 * time.Second):
		t.Fatal("Run failed to drain")
	}
	if e := h.server.Run(context.Background(), nil); e == nil {
		t.Fatal("stopped server restarted")
	}
}

func TestServerRejectsMismatchedSettlementSource(t *testing.T) {
	a, b := newServerHarness(t), newServerHarness(t)
	if a.source.AuthorityPin == b.source.AuthorityPin {
		t.Fatal("fixture authorities are not distinct")
	}
	if _, e := NewServer(ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, a.pool, b.settlement, a.sessions); e == nil {
		t.Fatal("mining and balance authorities were silently mixed")
	}
	for _, listen := range []string{"0.0.0.0:9311", "example.invalid:9311", "127.0.0.1:09311"} {
		if _, e := NewServer(ServerConfig{Version: 1, Listen: listen}, a.pool, a.settlement, a.sessions); e == nil {
			t.Fatalf("unsafe listener %s", listen)
		}
	}
}

func TestServerRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile-only guard")
	}
	_, e := NewServer(ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, nil, nil, nil)
	if e == nil {
		t.Fatal("cheap PoW service created")
	}
	if !errors.Is(e, poolshare.ErrProfile) {
		t.Fatalf("wrong profile guard: %v", e)
	}
}
