//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectservice

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

type securityFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
}

func securityFixtures(t *testing.T) []securityFixture {
	t.Helper()
	raw, err := os.ReadFile("../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []securityFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) != 2 {
		t.Fatalf("independent security fixtures: %v", err)
	}
	return fixtures
}

type securityNode struct {
	context    prospect.Context
	resolve    atomic.Int64
	checks     atomic.Int64
	proofs     atomic.Int64
	entered    chan struct{}
	release    chan struct{}
	once       sync.Once
	resolveErr error
	checkErr   error
}

func (node *securityNode) Resolve(ctx context.Context, _ prospectnode.Policy) (prospect.Context, error) {
	node.resolve.Add(1)
	if node.entered != nil {
		node.once.Do(func() { close(node.entered) })
		// Deliberately ignore cancellation to model an underlying operation
		// which cannot stop immediately, as with an active Argon calculation.
		<-node.release
	}
	if err := ctx.Err(); err != nil {
		return prospect.Context{}, err
	}
	if node.resolveErr != nil {
		return prospect.Context{}, node.resolveErr
	}
	return node.context, nil
}

func (node *securityNode) RecheckContext(ctx context.Context, _ prospect.Context) error {
	node.checks.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	return node.checkErr
}

func (node *securityNode) Recheck(ctx context.Context, _ prospect.Context, _ prospect.Proof) error {
	node.proofs.Add(1)
	return ctx.Err()
}

type securityHarness struct {
	server *Server
	node   *securityNode
	store  *registry.Store
	path   string
	origin string
	token  string
	key    string
	cancel context.CancelFunc
	done   chan struct{}
	runErr error
}

func startSecurityServer(t *testing.T, store *registry.Store, path string, node *securityNode, maximumJobs uint64) *securityHarness {
	t.Helper()
	f := securityFixtures(t)[0]
	// This is a public deterministic API test credential, never a real token,
	// wallet key, node RPC credential or league checkpoint key.
	rawToken := bytes.Repeat([]byte{0x19}, 32)
	digest := sha256.Sum256(rawToken)
	config := ServerConfig{Version: Version1, Listen: "127.0.0.1:0",
		Clients: []Credential{{PublicKey: f.Job.PublicKey, TokenSHA256: hex.EncodeToString(digest[:])}},
		MaxJobs: maximumJobs, MaxRecords: 10}
	policy := prospectnode.Policy{Version: 1, Network: f.Job.Context.Network,
		Genesis: f.Job.Context.Genesis, DNAVersion: f.Job.Context.DNAVersion,
		ProspectTarget: f.Job.Context.ProspectTarget}
	server, err := NewServer(config, policy, node, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &securityHarness{server: server, node: node, store: store, path: path,
		token: hex.EncodeToString(rawToken), key: f.Job.PublicKey, cancel: cancel, done: make(chan struct{})}
	ready := make(chan string, 1)
	go func() {
		h.runErr = server.Run(ctx, func(origin string) { ready <- origin })
		close(h.done)
	}()
	select {
	case h.origin = <-ready:
	case <-h.done:
		t.Fatalf("authority failed before readiness: %v", h.runErr)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("authority did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(7 * time.Second):
			t.Error("authority did not drain at cleanup")
		}
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}

func newSecurityServer(t *testing.T, node *securityNode, maximumJobs uint64) *securityHarness {
	t.Helper()
	f := securityFixtures(t)[0]
	path := filepath.Join(t.TempDir(), "birth-authority")
	store, err := registry.CreateBirth(path, f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	if node == nil {
		node = &securityNode{context: f.Job.Context}
	}
	return startSecurityServer(t, store, path, node, maximumJobs)
}

func securityRequest(h *securityHarness, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Host = strings.TrimPrefix(h.origin, "http://")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+h.token)
	return request
}

func securityResponse(t *testing.T, h *securityHarness, request *http.Request, status int, code string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	h.server.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("status=%d body=%s; want %d/%s", response.Code, response.Body, status, code)
	}
	if code != "" {
		var problem ErrorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem.Version != Version1 || problem.Error.Code != code {
			t.Fatalf("invalid sanitized error envelope: %s (%v)", response.Body, err)
		}
		if strings.Contains(response.Body.String(), h.token) {
			t.Fatal("error response exposed API credential")
		}
	}
	return response
}

type securityUnreadableBody struct{ reads atomic.Int64 }

func (body *securityUnreadableBody) Read([]byte) (int, error) {
	body.reads.Add(1)
	return 0, errors.New("private-body-must-not-be-read")
}

func (*securityUnreadableBody) Close() error { return nil }

func TestSecurityAuthenticationPrecedesBodyAndNode(t *testing.T) {
	h := newSecurityServer(t, nil, 100)
	for _, test := range []struct {
		name   string
		values []string
	}{
		{"missing", nil},
		{"wrong token", []string{"Bearer " + strings.Repeat("00", 32)}},
		{"wrong scheme", []string{"Basic " + h.token}},
		{"short token", []string{"Bearer 01"}},
		{"nonhex token", []string{"Bearer " + strings.Repeat("z", 64)}},
		{"duplicate authorization", []string{"Bearer " + h.token, "Bearer " + h.token}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &securityUnreadableBody{}
			request := securityRequest(h, "/v1/jobs", nil)
			request.Body = body
			request.Header.Del("Authorization")
			for _, value := range test.values {
				request.Header.Add("Authorization", value)
			}
			securityResponse(t, h, request, http.StatusUnauthorized, "unauthorized")
			if body.reads.Load() != 0 || h.node.resolve.Load() != 0 || h.node.checks.Load() != 0 || h.node.proofs.Load() != 0 {
				t.Fatal("unauthenticated request reached body or node work")
			}
		})
	}
}

func TestSecurityRejectsBrowserAndRoutingInputsBeforeBody(t *testing.T) {
	h := newSecurityServer(t, nil, 100)
	for _, test := range []struct {
		name   string
		change func(*http.Request)
	}{
		{"foreign Host", func(r *http.Request) { r.Host = "attacker.example" }},
		{"localhost hostname", func(r *http.Request) { r.Host = "localhost:1234" }},
		{"Origin", func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }},
		{"null Origin", func(r *http.Request) { r.Header.Set("Origin", "null") }},
		{"compression", func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }},
		{"query", func(r *http.Request) { r.URL.RawQuery = "target=ff" }},
		{"method", func(r *http.Request) { r.Method = http.MethodGet }},
		{"unknown path", func(r *http.Request) { r.URL.Path = "/debug/pprof" }},
		{"path suffix", func(r *http.Request) { r.URL.Path = "/v1/jobs/" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &securityUnreadableBody{}
			request := securityRequest(h, "/v1/jobs", nil)
			request.Body = body
			test.change(request)
			response := httptest.NewRecorder()
			h.server.ServeHTTP(response, request)
			if response.Code < 400 || response.Code >= 500 || body.reads.Load() != 0 {
				t.Fatalf("routing/browser input reached body or succeeded: %d %s reads=%d", response.Code, response.Body, body.reads.Load())
			}
			if h.node.resolve.Load() != 0 || h.node.checks.Load() != 0 || h.node.proofs.Load() != 0 {
				t.Fatal("routing/browser rejection reached node")
			}
		})
	}
}

func TestSecurityCredentialBindsBothRoutesToItsGymKey(t *testing.T) {
	h := newSecurityServer(t, nil, 100)
	other := securityFixtures(t)[1]
	issue, _ := json.Marshal(map[string]string{"public_key": other.Job.PublicKey})
	securityResponse(t, h, securityRequest(h, "/v1/jobs", bytes.NewReader(issue)), http.StatusForbidden, "forbidden")
	admission, _ := json.Marshal(struct {
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}{other.Proof, other.Authorization})
	securityResponse(t, h, securityRequest(h, "/v1/admissions", bytes.NewReader(admission)), http.StatusForbidden, "forbidden")
	if h.node.resolve.Load() != 0 || h.node.checks.Load() != 0 || h.node.proofs.Load() != 0 {
		t.Fatal("foreign credential key reached node work")
	}
}

func TestSecurityRequestBodyBounds(t *testing.T) {
	for _, knownLength := range []bool{true, false} {
		t.Run(map[bool]string{true: "declared", false: "streaming"}[knownLength], func(t *testing.T) {
			h := newSecurityServer(t, nil, 100)
			body := bytes.Repeat([]byte(" "), MaxRequestSize+1)
			request := securityRequest(h, "/v1/admissions", bytes.NewReader(body))
			if !knownLength {
				request.ContentLength = -1
			}
			securityResponse(t, h, request, http.StatusRequestEntityTooLarge, "too_large")
			if h.node.resolve.Load() != 0 || h.node.proofs.Load() != 0 {
				t.Fatal("oversized body reached node")
			}
		})
	}
}

func TestSecurityCancellationKeepsWorkSlotAndShutdownDrains(t *testing.T) {
	f := securityFixtures(t)[0]
	node := &securityNode{context: f.Job.Context, entered: make(chan struct{}), release: make(chan struct{})}
	h := newSecurityServer(t, node, 100)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(node.release) }) }
	defer release()
	body, _ := json.Marshal(map[string]string{"public_key": h.key})
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, h.origin+"/v1/jobs", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+h.token)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	responseDone := make(chan struct{})
	go func() {
		response, _ := client.Do(request)
		if response != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		close(responseDone)
	}()
	select {
	case <-node.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first operation did not enter node work")
	}
	cancelRequest()
	select {
	case <-responseDone:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP client did not observe cancellation")
	}
	// The disconnected caller does not release capacity until its actual
	// underlying work ends. A second request must never wait behind it.
	started := time.Now()
	securityResponse(t, h, securityRequest(h, "/v1/jobs", bytes.NewReader(body)), http.StatusServiceUnavailable, "busy")
	if time.Since(started) > time.Second || node.resolve.Load() != 1 {
		t.Fatal("busy request queued behind active work")
	}
	h.cancel()
	select {
	case <-h.done:
		t.Fatal("Run returned while actual handler work remained active")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not finish after actual handler work drained")
	}
	if h.runErr != nil && !errors.Is(h.runErr, context.Canceled) {
		t.Fatalf("shutdown error: %v", h.runErr)
	}
	if node.checks.Load() != 0 {
		t.Fatal("cancelled resolution reached issuance guard")
	}
}

func TestSecurityJobCapacitySurvivesReopen(t *testing.T) {
	h := newSecurityServer(t, nil, 1)
	body, _ := json.Marshal(map[string]string{"public_key": h.key})
	response := securityResponse(t, h, securityRequest(h, "/v1/jobs", bytes.NewReader(body)), http.StatusOK, "")
	job, err := prospectjob.ParseJob(response.Body.Bytes())
	if err != nil {
		t.Fatalf("issued job failed canonical parsing: %v", err)
	}
	securityResponse(t, h, securityRequest(h, "/v1/jobs", bytes.NewReader(body)), http.StatusServiceUnavailable, "capacity")
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("authority failed to stop")
	}
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := registry.Open(h.path, false)
	if err != nil {
		t.Fatal(err)
	}
	restarted := startSecurityServer(t, store, h.path, &securityNode{context: job.Context}, 1)
	securityResponse(t, restarted, securityRequest(restarted, "/v1/jobs", bytes.NewReader(body)), http.StatusServiceUnavailable, "capacity")
	if restarted.node.resolve.Load() != 0 || restarted.node.checks.Load() != 0 {
		t.Fatal("durable capacity rejection contacted node")
	}
	stored, err := store.GetJob(job.ID)
	if err != nil || stored != job {
		t.Fatalf("capacity rejection replaced existing issuance: %+v, %v", stored, err)
	}
}

func TestSecurityRateLimitRejectionPrecedesBody(t *testing.T) {
	for _, test := range []struct {
		path  string
		burst int
		limit float64
	}{
		{"/v1/jobs", 2, 1},
		{"/v1/admissions", 4, 4},
	} {
		t.Run(test.path, func(t *testing.T) {
			h := newSecurityServer(t, nil, 100)
			limiter := h.server.clients[0].issue
			if test.path == "/v1/admissions" {
				limiter = h.server.clients[0].admit
			}
			if limiter.Burst() != test.burst || float64(limiter.Limit()) != test.limit {
				t.Fatal("configured API rate limit differs from the bounded contract")
			}
			// Freeze refill only in this package test so slow/race builds cannot
			// make the boundary assertion dependent on wall-clock scheduling.
			limiter.SetLimit(0)
			for i := 0; i < test.burst; i++ {
				securityResponse(t, h, securityRequest(h, test.path, strings.NewReader("{}")), http.StatusBadRequest, "invalid_request")
			}
			body := &securityUnreadableBody{}
			request := securityRequest(h, test.path, nil)
			request.Body = body
			securityResponse(t, h, request, http.StatusTooManyRequests, "rate_limited")
			if body.reads.Load() != 0 || h.node.resolve.Load() != 0 || h.node.proofs.Load() != 0 {
				t.Fatal("rate-limited request reached body or node work")
			}
		})
	}
}

func TestSecurityBackendErrorsAreSanitized(t *testing.T) {
	f := securityFixtures(t)[0]
	secret := "backend-secret-user-password-http://rpc.invalid/private"
	for _, phase := range []string{"resolve", "final-node-check"} {
		t.Run(phase, func(t *testing.T) {
			node := &securityNode{context: f.Job.Context}
			if phase == "resolve" {
				node.resolveErr = errors.New(secret)
			} else {
				node.checkErr = errors.New(secret)
			}
			h := newSecurityServer(t, node, 100)
			body, _ := json.Marshal(map[string]string{"public_key": h.key})
			response := securityResponse(t, h, securityRequest(h, "/v1/jobs", bytes.NewReader(body)), http.StatusServiceUnavailable, "unavailable")
			if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "rpc.invalid") {
				t.Fatal("backend details escaped into the public error envelope")
			}
			info, err := h.store.Info()
			if err != nil || info.Jobs != 0 || info.Records != 0 {
				t.Fatalf("failed node check changed durable state: %+v, %v", info, err)
			}
		})
	}
}

func TestSecurityConnectionLimitBoundsIdleClients(t *testing.T) {
	h := newSecurityServer(t, nil, 100)
	address := strings.TrimPrefix(h.origin, "http://")
	var held []net.Conn
	defer func() {
		for _, connection := range held {
			_ = connection.Close()
		}
	}()
	for i := 0; i < 16; i++ {
		connection, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, connection)
	}
	extra, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	if _, err := fmt.Fprintf(extra, "POST /v1/jobs HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}", address); err != nil {
		t.Fatal(err)
	}
	if err := extra.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var first [1]byte
	if count, err := extra.Read(first[:]); count != 0 || err == nil {
		t.Fatalf("seventeenth connection was processed while sixteen remained open: %d, %v", count, err)
	} else if networkError, ok := err.(net.Error); !ok || !networkError.Timeout() {
		t.Fatalf("expected bounded pending connection, got %v", err)
	}
	if err := held[0].Close(); err != nil {
		t.Fatal(err)
	}
	if err := extra.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(extra), nil)
	if err != nil {
		t.Fatalf("queued TCP connection did not proceed after capacity release: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected first response after releasing connection capacity: %d", response.StatusCode)
	}
}
