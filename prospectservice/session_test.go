//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// Public fixture tokens only. This double exercises the transport's lease
// contract; gymaccount separately verifies actual durable session semantics.
type testSessions struct {
	mu      sync.Mutex
	keys    map[string]string
	expired bool
}

func (s *testSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keys[token]
	check := func() error {
		if !ok || s.expired || ctx.Err() != nil {
			return errors.New("private session failure")
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	return fn(key, check)
}

func sessionSetup(t *testing.T) (*Server, *registry.Store, *testSessions, clientFixture, *v2Node) {
	t.Helper()
	f := v2Reference(t)
	store, err := registry.CreateMining(filepath.Join(t.TempDir(), "birth"), f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	access := &testSessions{keys: map[string]string{clientTestToken: f.Job.PublicKey}}
	config := v2Config(f)
	config.Clients = nil
	node := &v2Node{srvNode: &srvNode{context: f.Job.Context}, source: "current-parent"}
	server, err := NewSessionServer(config, clientPolicy(f), node, store, access)
	if err != nil {
		t.Fatal(err)
	}
	return server, store, access, f, node
}

func TestSessionServerRequiresExplicitModeAndRejectsCrossGym(t *testing.T) {
	server, store, access, f, node := sessionSetup(t)
	config := v2Config(f)
	if _, err := NewSessionServer(config, clientPolicy(f), node, store, access); err == nil {
		t.Fatal("accepted static credential fallback")
	}
	config.Clients = nil
	if _, err := NewServer(config, clientPolicy(f), node, store); err == nil {
		t.Fatal("legacy server accepted no credential")
	}
	if _, err := NewSessionServer(config, clientPolicy(f), node, store, nil); err == nil {
		t.Fatal("accepted missing session authority")
	}
	config.Version = Version1
	if _, err := NewSessionServer(config, clientPolicy(f), node, store, access); err == nil {
		t.Fatal("silently upgraded v1 transport")
	}
	sessionError(t, srvRequest(t, server, strings.Repeat("ab", 32), "/v2/jobs", map[string]any{"public_key": f.Job.PublicKey}), 401, "unauthorized", false)
	other := "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	if other == f.Job.PublicKey {
		other = "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"
	}
	sessionError(t, srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": other}), 403, "forbidden", false)
	if node.resolves.Load() != 0 {
		t.Fatal("unauthorized request reached trusted node")
	}
	access.mu.Lock()
	delete(access.keys, clientTestToken)
	access.mu.Unlock()
	sessionError(t, srvRequest(t, server, clientTestToken, "/v2/admissions", map[string]any{"proof": f.Proof, "authorization": f.Authorization}), 401, "unauthorized", false)
}

type expiryNode struct {
	*v2Node
	expire func()
}

func (n *expiryNode) Resolve(ctx context.Context, policy prospectnode.Policy) (prospect.Context, error) {
	n.expire() // Called while the session lease is held, simulating clock expiry.
	return n.v2Node.Resolve(ctx, policy)
}

func TestSessionExpiryDuringNodeResolutionRejectsBeforeJobWrite(t *testing.T) {
	server, store, access, f, node := sessionSetup(t)
	server.node = &expiryNode{v2Node: node, expire: func() { access.expired = true }}
	sessionError(t, srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": f.Job.PublicKey}), 401, "unauthorized", false)
	info, err := store.Info()
	if err != nil || info.Jobs != 0 {
		t.Fatalf("expired lease wrote a job: %+v %v", info, err)
	}
}

type revokeOnRead struct {
	io.Reader
	once sync.Once
	f    func()
}

func (r *revokeOnRead) Read(p []byte) (int, error) {
	r.once.Do(r.f)
	return r.Reader.Read(p)
}

func TestSessionRevocationBetweenHeaderAndBodyAuthentication(t *testing.T) {
	server, store, access, f, node := sessionSetup(t)
	body := &revokeOnRead{Reader: strings.NewReader(`{"public_key":"` + f.Job.PublicKey + `"}`), f: func() {
		access.mu.Lock()
		delete(access.keys, clientTestToken)
		access.mu.Unlock()
	}}
	request := httptest.NewRequest(http.MethodPost, "/v2/jobs", body)
	request.Host = "127.0.0.1:29334"
	request.Header.Set("Authorization", "Bearer "+clientTestToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	sessionError(t, response, 401, "unauthorized", false)
	info, err := store.Info()
	if err != nil || info.Jobs != 0 || node.resolves.Load() != 0 {
		t.Fatal("revoked credential retained new-job access")
	}
}

func TestSessionRotationPreservesGymRateLimit(t *testing.T) {
	server, _, access, f, _ := sessionSetup(t)
	request := func(token string) *clientCredential {
		r := httptest.NewRequest(http.MethodPost, "/v2/jobs", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return server.authenticate(r)
	}
	first := request(clientTestToken)
	if first == nil || !first.issue.Allow() || !first.issue.Allow() {
		t.Fatal("missing initial burst")
	}
	rotated := strings.Repeat("cd", 32)
	access.mu.Lock()
	delete(access.keys, clientTestToken)
	access.keys[rotated] = f.Job.PublicKey
	access.mu.Unlock()
	second := request(rotated)
	if first != second || second.issue.Allow() || request(clientTestToken) != nil {
		t.Fatal("rotation replenished allowance or preserved revoked access")
	}
}

type blockedSessionAuthorizer struct {
	inner   SessionAuthorizer
	calls   atomic.Int32
	entered chan context.Context
	release chan struct{}
}

func (s *blockedSessionAuthorizer) WithSession(ctx context.Context, token string,
	fn func(string, func() error) error) error {

	if s.calls.Add(1) == 1 {
		s.entered <- ctx
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.release:
		}
	}
	return s.inner.WithSession(ctx, token, fn)
}

func TestSessionAuthenticationSharesFailFastPermitAndDeadline(t *testing.T) {
	server, _, access, fixture, _ := sessionSetup(t)
	blocked := &blockedSessionAuthorizer{inner: access,
		entered: make(chan context.Context, 1), release: make(chan struct{})}
	server.sessions = blocked
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(blocked.release) }) })
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": fixture.Job.PublicKey})
	}()
	var authentication context.Context
	select {
	case authentication = <-blocked.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first request never reached session authentication")
	}
	deadline, ok := authentication.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > operationTimeout {
		t.Fatal("session authentication was outside the operation deadline")
	}
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		secondDone <- srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": fixture.Job.PublicKey})
	}()
	select {
	case response := <-secondDone:
		sessionError(t, response, 503, "busy", false)
	case <-time.After(time.Second):
		t.Fatal("second request queued behind the account authorization lease")
	}
	if blocked.calls.Load() != 1 {
		t.Fatal("busy request entered the account authorizer")
	}
	release.Do(func() { close(blocked.release) })
	select {
	case response := <-firstDone:
		if response.Code != http.StatusOK {
			t.Fatalf("first request did not complete: HTTP %d %s", response.Code, response.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not drain after authentication resumed")
	}
}

func TestSessionAuthenticationCancellationReleasesPermit(t *testing.T) {
	server, _, access, fixture, _ := sessionSetup(t)
	blocked := &blockedSessionAuthorizer{inner: access,
		entered: make(chan context.Context, 1), release: make(chan struct{})}
	server.sessions = blocked
	t.Cleanup(func() { close(blocked.release) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/v2/jobs", strings.NewReader(`{"public_key":"`+fixture.Job.PublicKey+`"}`)).WithContext(ctx)
	request.Host = "127.0.0.1:29334"
	request.Header.Set("Authorization", "Bearer "+clientTestToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-blocked.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not enter session authentication")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled authentication did not return")
	}
	sessionError(t, response, 401, "unauthorized", false)
	next := srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": fixture.Job.PublicKey})
	if next.Code != http.StatusOK {
		t.Fatalf("canceled authentication retained operation permit: HTTP %d %s", next.Code, next.Body.String())
	}
}

func sessionError(t *testing.T, response *httptest.ResponseRecorder, status int, code string, unknown bool) {
	t.Helper()
	var failure ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || response.Code != status || failure.Version != Version2 || failure.Error.Code != code || failure.Error.OutcomeUnknown != unknown {
		t.Fatalf("HTTP %d %s, want %d/%s/unknown=%v (%v)", response.Code, response.Body.String(), status, code, unknown, err)
	}
}
