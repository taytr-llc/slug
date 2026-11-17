//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/registry"
)

func mbRawRequest(path string, body io.Reader) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, body)
	r.Host = "127.0.0.1:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+mbToken)
	return r
}

func TestManifestSecurityEarlyGatesAndBodyBounds(t *testing.T) {
	for _, mode := range []string{"unauthorized", "duplicate-token", "host", "origin", "query", "encoding", "method", "old-route", "content-type"} {
		t.Run(mode, func(t *testing.T) {
			s, _, node, _, _ := mbSetup(t)
			body := &securityUnreadableBody{}
			r := mbRawRequest("/v3/jobs", nil)
			r.Body = body
			switch mode {
			case "unauthorized":
				r.Header.Del("Authorization")
			case "duplicate-token":
				r.Header.Add("Authorization", "Bearer "+mbToken)
			case "host":
				r.Host = "remote.invalid:12345"
			case "origin":
				r.Header.Set("Origin", "null")
			case "query":
				r.URL.RawQuery = "unused=1"
			case "encoding":
				r.Header.Set("Content-Encoding", "gzip")
			case "method":
				r.Method = http.MethodGet
			case "old-route":
				r.URL.Path = "/v2/jobs"
			case "content-type":
				r.Header.Set("Content-Type", "text/plain")
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code < 400 || w.Code >= 500 || body.reads.Load() != 0 || node.resolves.Load() != 0 {
				t.Fatal("early gate read body or node", w.Code, body.reads.Load())
			}
		})
	}
	for _, declared := range []bool{false, true} {
		s, _, node, _, _ := mbSetup(t)
		r := mbRawRequest("/v3/admissions", strings.NewReader(strings.Repeat(" ", MaxRequestSize+1)))
		if !declared {
			r.ContentLength = -1
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		mbError(t, w, 413, "too_large", false)
		if node.resolves.Load() != 0 || node.checks.Load() != 0 {
			t.Fatal("oversized request reached node")
		}
	}
	for _, path := range []string{"/v3/jobs", "/v3/admissions"} {
		s, _, _, _, _ := mbSetup(t)
		limiter := s.clients[0].issue
		if path == "/v3/admissions" {
			limiter = s.clients[0].admit
		}
		limiter.SetLimit(0)
		for i := 0; i < limiter.Burst(); i++ {
			if !limiter.Allow() {
				t.Fatal("missing burst")
			}
		}
		body := &securityUnreadableBody{}
		r := mbRawRequest(path, nil)
		r.Body = body
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		mbError(t, w, 429, "rate_limited", false)
		if body.reads.Load() != 0 {
			t.Fatal("limited operation read body")
		}
	}
}

func TestManifestSecuritySessionPermitAndDeadline(t *testing.T) {
	s, store, node, f, _ := mbSetup(t)
	a := &mbSessions{key: f.Cases[0].Job.PublicKey}
	blocked := &blockedSessionAuthorizer{inner: a, entered: make(chan context.Context, 1), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(blocked.release) })
	s = mbSessionServer(t, s, store, node, blocked)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := mbRawRequest("/v3/jobs", strings.NewReader(`{"public_key":"`+a.key+`"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.ServeHTTP(w, r); close(done) }()
	select {
	case authCtx := <-blocked.entered:
		deadline, ok := authCtx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > operationTimeout {
			t.Fatal("authentication unbounded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no authentication")
	}
	mbError(t, mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": a.key}), 503, "busy", false)
	if blocked.calls.Load() != 1 {
		t.Fatal("busy request queued on lease")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled authentication did not drain")
	}
	mbError(t, w, 401, "unauthorized", false)
	release.Do(func() { close(blocked.release) })
	if got := mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": a.key}); got.Code != 200 {
		t.Fatal("permit retained after cancellation", got.Body.String())
	}
}

func TestManifestSecurityRunDrainsDisconnectedWork(t *testing.T) {
	s, store, node, f, _ := mbSetup(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	node.resolve = func(context.Context) error { close(entered); <-release; return nil }
	releaseWork := func() { once.Do(func() { close(release) }) }
	defer releaseWork()
	runCtx, stop := context.WithCancel(context.Background())
	defer stop()
	ready, runDone := make(chan string, 1), make(chan error, 1)
	go func() { runDone <- s.Run(runCtx, func(origin string) { ready <- origin }) }()
	var origin string
	select {
	case origin = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("Run not ready")
	}
	client, err := NewManifestClient(ManifestClientConfig{Version: 3, URL: origin, Token: mbToken, AuthorityPin: s.config.AuthorityPin}, s.policy)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callDone := make(chan error, 1)
	go func() { _, err := client.IssueManifestJob(ctx, f.Cases[0].Job.PublicKey); callDone <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("work not entered")
	}
	cancel()
	select {
	case err := <-callDone:
		var ce *ClientError
		if !errors.As(err, &ce) || ce.Code != "outcome_unknown" || !ce.OutcomeUnknown {
			t.Fatal("dispatched cancellation definite", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not cancel")
	}
	mbError(t, mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": f.Cases[0].Job.PublicKey}), 503, "busy", false)
	stop()
	select {
	case err := <-runDone:
		t.Fatal("Run returned before real work drained", err)
	case <-time.After(80 * time.Millisecond):
	}
	releaseWork()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run failed to drain")
	}
	info, err := store.Info()
	if err != nil || info.Jobs != 0 {
		t.Fatal("canceled work committed", info, err)
	}
	if err := s.Run(context.Background(), nil); err == nil {
		t.Fatal("server restarted")
	}
}

type mbSlowWriter struct {
	header  http.Header
	entered chan struct{}
	release chan struct{}
}

func (w *mbSlowWriter) Header() http.Header { return w.header }
func (w *mbSlowWriter) WriteHeader(int)     {}
func (w *mbSlowWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

func TestManifestSecurityReleasesPermitBeforeSocketWrite(t *testing.T) {
	s, _, _, f, _ := mbSetup(t)
	w := &mbSlowWriter{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(w.release) })
	go func() {
		s.ServeHTTP(w, mbRawRequest("/v3/jobs", strings.NewReader(`{"public_key":"`+f.Cases[0].Job.PublicKey+`"}`)))
		close(done)
	}()
	select {
	case <-w.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first write not reached")
	}
	second := mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": f.Cases[0].Job.PublicKey})
	if second.Code != 200 {
		t.Fatal("socket write retained permit", second.Body.String())
	}
	release.Do(func() { close(w.release) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("writer did not drain")
	}
}

func TestManifestSecurityLostIssuanceAcknowledgment(t *testing.T) {
	s, store, _, f, _ := mbSetup(t)
	var dispatched atomic.Int32
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched.Add(1)
		recorded := httptest.NewRecorder()
		s.ServeHTTP(recorded, r)
		if recorded.Code != 200 {
			t.Error("issuance did not commit", recorded.Body.String())
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer h.Close()
	c, err := NewManifestClient(ManifestClientConfig{Version: 3, URL: h.URL, Token: mbToken, AuthorityPin: s.config.AuthorityPin}, s.policy)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.IssueManifestJob(ctx, f.Cases[0].Job.PublicKey); err == nil {
		t.Fatal("cancelled preflight accepted")
	}
	if dispatched.Load() != 0 {
		t.Fatal("cancelled preflight dispatched")
	}
	_, err = c.IssueManifestJob(context.Background(), f.Cases[0].Job.PublicKey)
	var ce *ClientError
	if !errors.As(err, &ce) || ce.Code != "outcome_unknown" || !ce.OutcomeUnknown {
		t.Fatal("lost durable issue ACK not unknown", err)
	}
	info, err := store.Info()
	if err != nil || info.Jobs != 1 || dispatched.Load() != 1 {
		t.Fatal("missing durable single dispatch", info, err)
	}
}

func TestManifestSecurityPoisonIsLatched(t *testing.T) {
	s, _, node, f, _ := mbSetup(t)
	failure := s.storeFailure(registry.ErrPoisoned)
	if failure.code != "outcome_unknown" || !failure.unknown {
		t.Fatal("poison misclassified")
	}
	mbError(t, mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": f.Cases[0].Job.PublicKey}), 503, "outcome_unknown", true)
	if node.resolves.Load() != 0 {
		t.Fatal("poisoned authority continued")
	}
}

func TestManifestSecuritySixteenConnectionLimit(t *testing.T) {
	s, _, _, _, _ := mbSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- s.Run(ctx, func(origin string) { ready <- origin }) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("listener failed to drain")
		}
	}()
	var address string
	select {
	case origin := <-ready:
		address = strings.TrimPrefix(origin, "http://")
	case <-time.After(3 * time.Second):
		t.Fatal("listener not ready")
	}
	var held []net.Conn
	defer func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	}()
	for i := 0; i < 16; i++ {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	extra, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	if _, err := fmt.Fprintf(extra, "POST /v3/jobs HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}", address); err != nil {
		t.Fatal(err)
	}
	_ = extra.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var first [1]byte
	if n, err := extra.Read(first[:]); n != 0 || err == nil {
		t.Fatal("seventeenth connection was processed", n, err)
	} else if e, ok := err.(net.Error); !ok || !e.Timeout() {
		t.Fatal("unexpected connection error", err)
	}
	_ = held[0].Close()
	_ = extra.SetReadDeadline(time.Now().Add(3 * time.Second))
	r, err := http.ReadResponse(bufio.NewReader(extra), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatal("released connection failed expected auth gate", r.StatusCode)
	}
}

func TestManifestSecuritySessionRotationPreservesAllowance(t *testing.T) {
	s, store, node, f, _ := mbSetup(t)
	access := &testSessions{keys: map[string]string{mbToken: f.Cases[0].Job.PublicKey}}
	s = mbSessionServer(t, s, store, node, access)
	r := mbRawRequest("/v3/jobs", nil)
	first := s.authenticate(r)
	if first == nil {
		t.Fatal("missing credential")
	}
	first.issue.SetLimit(0)
	if !first.issue.Allow() || !first.issue.Allow() {
		t.Fatal("missing initial burst")
	}
	rotated := strings.Repeat("cd", 32)
	access.mu.Lock()
	delete(access.keys, mbToken)
	access.keys[rotated] = f.Cases[0].Job.PublicKey
	access.mu.Unlock()
	r.Header.Set("Authorization", "Bearer "+rotated)
	second := s.authenticate(r)
	if first != second || second.issue.Allow() {
		t.Fatal("token rotation reset gym allowance")
	}
	r.Header.Set("Authorization", "Bearer "+mbToken)
	if s.authenticate(r) != nil {
		t.Fatal("old session retained access")
	}
}
