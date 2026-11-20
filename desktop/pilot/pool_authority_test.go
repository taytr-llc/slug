// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type poolDesktopSessions struct {
	key    string
	mu     sync.Mutex
	tokens map[string]bool
	active atomic.Bool
}

func (s *poolDesktopSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if !s.active.CompareAndSwap(false, true) {
		return errors.New("nested fixture account lease")
	}
	defer s.active.Store(false)
	s.mu.Lock()
	valid := s.tokens[token]
	s.mu.Unlock()
	if !valid {
		return gymaccount.ErrUnauthorized
	}
	return fn(s.key, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.tokens[token] {
			return gymaccount.ErrUnauthorized
		}
		return ctx.Err()
	})
}

type poolDesktopHarness struct {
	authority *PoolSessionAuthority
	config    PoolAuthorityConfig
	account   ManifestAccountConfig
	sessions  *poolDesktopSessions
	mu        sync.Mutex
	token     string
	view      bool
	requests  []string
	hook      func(http.ResponseWriter, *http.Request)
	response  poolservice.BalanceResponse
}

func newPoolDesktopHarness(t *testing.T) *poolDesktopHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal profile integration")
	}
	identity, _ := gymaccount.FixtureIdentity("zero256")
	p := policy()
	descriptor := fighterdescriptor.CandidateDescriptor()
	descriptorID, e := fighterdescriptor.ID(descriptor)
	if e != nil {
		t.Fatal(e)
	}
	source := poolshare.Config{Version: 1, AuthorityPin: registry.ManifestAuthorityPin{Version: registry.Version5, Network: p.Network, Genesis: p.Genesis, AuthorityID: strings.Repeat("a", 64), DescriptorID: descriptorID}, Policy: p, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 32, MaxBytes: 1 << 20}
	h := &poolDesktopHarness{token: strings.Repeat("1", 64), view: true, sessions: &poolDesktopSessions{key: identity.GymPublicKey, tokens: map[string]bool{strings.Repeat("1", 64): true, strings.Repeat("2", 64): true}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the tiny request just as the real service does. Leaving its
		// body unread prevents net/http's disconnect watcher from starting,
		// which would strand this mock's cancellation wait during cleanup.
		if _, err := io.Copy(io.Discard, io.LimitReader(r.Body, poolservice.MaxRequestSize+1)); err != nil {
			return
		}
		h.mu.Lock()
		h.requests = append(h.requests, r.Header.Get("Authorization"))
		hook := h.hook
		h.mu.Unlock()
		if h.sessions.active.Load() {
			t.Error("desktop held account lease across HTTP")
		}
		if hook != nil {
			hook(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.response)
	}))
	t.Cleanup(server.Close)
	id, e := poolservice.BindingID(source)
	if e != nil {
		t.Fatal(e)
	}
	h.response = poolservice.BalanceResponse{Version: 1, BindingID: id, Balance: poolsettle.Balance{GymID: identity.GymID, Confirmed: poolsettle.Fraction{Numerator: "990000000", Denominator: "1"}, Available: poolsettle.Fraction{Numerator: "990000000", Denominator: "1"}, Reason: "fee-budget"}, ObservedAtMS: 1795000000000}
	cc := poolservice.ClientConfig{Version: 1, URL: server.URL, Token: h.token, PublicKey: identity.GymPublicKey, Source: source}
	trust, e := poolservice.TransportTrustID(cc)
	if e != nil {
		t.Fatal(e)
	}
	h.config = PoolAuthorityConfig{Version: 1, URL: server.URL, PublicKey: identity.GymPublicKey, Source: source, TransportTrustID: trust}
	h.account = ManifestAccountConfig{Account: AccountConfig{Fixture: "zero256", AccountID: strings.Repeat("b", 64), AuthorityID: strings.Repeat("c", 64), Authorizer: h.sessions, MiningToken: func(context.Context) (string, error) { h.mu.Lock(); defer h.mu.Unlock(); return h.token, nil }}, ViewID: strings.Repeat("d", 64), Generation: 1}
	h.account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if !h.sessions.active.CompareAndSwap(false, true) {
			return errors.New("nested fixture view lease")
		}
		defer h.sessions.active.Store(false)
		h.mu.Lock()
		valid := h.view
		h.mu.Unlock()
		if !valid {
			return gymaccount.ErrUnauthorized
		}
		return fn(gymaccount.Account{ID: h.account.Account.AccountID, Generation: 1}, func() error {
			h.mu.Lock()
			defer h.mu.Unlock()
			if !h.view {
				return gymaccount.ErrUnauthorized
			}
			return ctx.Err()
		})
	}
	h.authority, e = NewPoolSessionAuthority(h.config, h.account)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.authority.Close() })
	return h
}

func TestPoolDesktopCredentialRefreshPreservesBinding(t *testing.T) {
	h := newPoolDesktopHarness(t)
	for _, token := range []string{strings.Repeat("1", 64), strings.Repeat("2", 64)} {
		h.mu.Lock()
		h.token = token
		h.mu.Unlock()
		r, e := h.authority.Balance(context.Background())
		if e != nil || r.Balance.Reason != "fee-budget" || r.Balance.Confirmed.Numerator != "990000000" {
			t.Fatalf("balance lost: %+v %v", r, e)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 2 || h.requests[0] != "Bearer "+strings.Repeat("1", 64) || h.requests[1] != "Bearer "+strings.Repeat("2", 64) {
		t.Fatal("credential not refreshed per operation")
	}
	if h.authority.SourceConfig() != h.config.Source || h.authority.TransportTrustID() != h.config.TransportTrustID {
		t.Fatal("refresh changed pool binding")
	}
}
func TestPoolDesktopRejectsCrossAccountAndTrustChange(t *testing.T) {
	h := newPoolDesktopHarness(t)
	bad := h.config
	bad.TransportTrustID = strings.Repeat("0", 64)
	if a, e := NewPoolSessionAuthority(bad, h.account); e == nil {
		a.Close()
		t.Fatal("changed trust accepted")
	}
	other, _ := gymaccount.FixtureIdentity("incrementing256")
	h.sessions.key = other.GymPublicKey
	if _, e := h.authority.Balance(context.Background()); e == nil {
		t.Fatal("different gym token accepted")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 0 {
		t.Fatal("different gym credential left desktop")
	}
}
func TestPoolDesktopDiscardsResponseAfterAccountChange(t *testing.T) {
	h := newPoolDesktopHarness(t)
	h.hook = func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.view = false
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.response)
	}
	result, e := h.authority.Balance(context.Background())
	if e == nil || result.Balance.GymID != "" {
		t.Fatal("response published into expired view")
	}
}
func TestPoolDesktopMutationKeepsOriginalCredentialOnUncertainty(t *testing.T) {
	h := newPoolDesktopHarness(t)
	h.hook = func(w http.ResponseWriter, r *http.Request) {
		h.sessions.mu.Lock()
		h.sessions.tokens[strings.Repeat("1", 64)] = false
		h.sessions.mu.Unlock()
		h.mu.Lock()
		h.token = strings.Repeat("2", 64)
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_ = json.NewEncoder(w).Encode(poolservice.ErrorResponse{Version: 1, Error: poolservice.APIError{Code: "outcome_unknown", OutcomeUnknown: true}})
	}
	_, e := h.authority.Issue(context.Background())
	var ce *poolservice.ClientError
	if !errors.As(e, &ce) || !ce.OutcomeUnknown {
		t.Fatal("uncertain issuance relabeled", e)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 1 || h.requests[0] != "Bearer "+strings.Repeat("1", 64) {
		t.Fatal("retried with replacement credential")
	}
}
func TestPoolDesktopCloseCancelsAndDrains(t *testing.T) {
	h := newPoolDesktopHarness(t)
	entered := make(chan struct{})
	h.hook = func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }
	done := make(chan error, 1)
	go func() { _, e := h.authority.Balance(context.Background()); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	if e := h.authority.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("closed call succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close failed to drain")
	}
	if _, e := h.authority.Balance(context.Background()); e == nil {
		t.Fatal("closed adapter reused")
	}
}
func TestPoolDesktopCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if _, e := NewPoolSessionAuthority(PoolAuthorityConfig{}, ManifestAccountConfig{}); !errors.Is(e, poolshare.ErrProfile) {
		t.Fatal(e)
	}
}
