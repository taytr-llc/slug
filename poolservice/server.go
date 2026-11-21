// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/net/netutil"
	"golang.org/x/time/rate"
)

var errServer = errors.New("pool service unavailable")

func nonnil(v any) bool {
	if v == nil {
		return false
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice, reflect.Chan:
		return !r.IsNil()
	}
	return true
}

func validateListen(c ServerConfig) (ServerConfig, error) {
	host, port, e := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	n, x := strconv.ParseUint(port, 10, 16)
	if c.Version != Version1 || e != nil || x != nil || ip == nil || !ip.IsLoopback() || strings.Contains(host, "%") || strconv.FormatUint(n, 10) != port {
		return ServerConfig{}, errServer
	}
	c.Listen = net.JoinHostPort(ip.String(), port)
	return c, nil
}

// NewServer borrows a matched concrete pool/settlement pair. The operator must
// keep their source, session authority and node alive until Run fully drains.
// No static token or signing-key fallback is provided.
func NewServer(c ServerConfig, pool *poolshare.Pool, settlement *poolsettle.Store, sessions prospectservice.SessionAuthorizer) (*Server, error) {
	if wire.TestPoW {
		return nil, poolshare.ErrProfile
	}
	c, err := validateListen(c)
	if err != nil || pool == nil || settlement == nil || !nonnil(sessions) {
		return nil, errServer
	}
	page, err := pool.AccountingPage(context.Background(), 0, 0, 1)
	if err != nil {
		return nil, errServer
	}
	source, err := settlement.SourceConfig(context.Background())
	if err != nil || source != page.Config {
		return nil, errServer
	}
	binding, err := BindingID(source)
	if err != nil {
		return nil, errServer
	}
	return &Server{config: c, source: source, binding: binding, pool: pool, settlement: settlement, sessions: sessions, active: make(chan struct{}, 1), rates: make(map[string]*clientRates)}, nil
}
func (s *Server) SourceConfig() poolshare.Config { return s.source }
func (s *Server) BindingID() string              { return s.binding }

func (s *Server) stop() { s.mu.Lock(); s.closing = true; s.mu.Unlock() }
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.handlers.Add(1)
	return true
}

// Run owns one literal-loopback listener. Cancellation refuses new handlers,
// cancels active work and joins it before returning. Stores remain caller-owned.
func (s *Server) Run(ctx context.Context, onReady func(string)) error {
	if s == nil || ctx == nil {
		return errServer
	}
	s.mu.Lock()
	if s.started || s.closing {
		s.mu.Unlock()
		return errServer
	}
	s.started = true
	s.mu.Unlock()
	if ctx.Err() != nil {
		s.stop()
		s.handlers.Wait()
		return nil
	}
	listener, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		s.stop()
		s.handlers.Wait()
		return errServer
	}
	base, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return base }, ErrorLog: log.New(io.Discard, "", 0)}
	done := make(chan error, 1)
	go func() { done <- server.Serve(netutil.LimitListener(listener, 16)) }()
	if onReady != nil {
		onReady("http://" + listener.Addr().String())
	}
	var result error
	finished := false
	select {
	case result = <-done:
		finished = true
	case <-ctx.Done():
	}
	s.stop()
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	if server.Shutdown(shutdown) != nil {
		_ = server.Close()
	}
	stop()
	if !finished {
		result = <-done
	}
	s.handlers.Wait()
	if ctx.Err() != nil || errors.Is(result, http.ErrServerClosed) {
		return nil
	}
	return errServer
}

func validHost(r *http.Request) bool {
	host, port, e := net.SplitHostPort(r.Host)
	ip := net.ParseIP(host)
	n, x := strconv.ParseUint(port, 10, 16)
	if e != nil || x != nil || n == 0 || strconv.FormatUint(n, 10) != port || ip == nil || !ip.IsLoopback() || strings.Contains(host, "%") {
		return false
	}
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		h, p, e := net.SplitHostPort(local.String())
		if e != nil || p != port || !ip.Equal(net.ParseIP(h)) {
			return false
		}
	}
	return true
}
func requestAction(r *http.Request) string { return requestActionSelected(r, pathPrefix) }
func requestActionSelected(r *http.Request, prefix string) string {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.IsAbs() || r.URL.Host != "" || r.URL.RawPath != "" || len(r.Header.Values("Content-Encoding")) != 0 {
		return ""
	}
	values := r.Header.Values("Content-Type")
	if len(values) != 1 {
		return ""
	}
	kind, params, e := mime.ParseMediaType(values[0])
	if e != nil || kind != "application/json" {
		return ""
	}
	for k, v := range params {
		if k != "charset" || !strings.EqualFold(v, "utf-8") {
			return ""
		}
	}
	switch r.URL.Path {
	case prefix + "issue":
		return "issue"
	case prefix + "work":
		return "work"
	case prefix + "submit":
		return "submit"
	case prefix + "lookup":
		return "lookup"
	case prefix + "balance":
		return "balance"
	case prefix + "payouts":
		return "payouts"
	}
	return ""
}
func bearer(r *http.Request) string {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) != 71 || !strings.HasPrefix(values[0], "Bearer ") {
		return ""
	}
	value := values[0][7:]
	raw, e := hex.DecodeString(value)
	if e != nil || len(raw) != 32 {
		return ""
	}
	return hex.EncodeToString(raw)
}
func identity(source poolshare.Config) prospect.Context {
	p := source.Policy
	return prospect.Context{Version: p.Version, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion}
}

// Authentication leases never surround Pool or MinerBalance calls, which take
// their own original-token leases. This avoids nested account-lock deadlocks.
func (s *Server) authenticate(ctx context.Context, token, expected string) (string, string, error) {
	if token == "" {
		return "", "", errServer
	}
	var key, gym string
	var operationErr error
	called := false
	err := s.sessions.WithSession(ctx, token, func(public string, recheck func() error) (e error) {
		defer func() { operationErr = e }()
		if called {
			return errServer
		}
		called = true
		if len(public) != 66 || public != strings.ToLower(public) || recheck == nil || expected != "" && public != expected {
			return errServer
		}
		g, e := gymauth.GymID(identity(s.source), public)
		if e != nil {
			return errServer
		}
		if e = recheck(); e != nil {
			return errServer
		}
		if ctx.Err() != nil {
			return errServer
		}
		key, gym = public, g
		return nil
	})
	if err != nil || operationErr != nil || !called || ctx.Err() != nil {
		return "", "", errServer
	}
	return key, gym, nil
}
func (s *Server) allowance(key, action string) *rate.Limiter {
	// Every access occurs under the single operation permit. Rotation of a
	// token never resets the original gym's allowance.
	r := s.rates[key]
	if r == nil {
		if len(s.rates) >= 64 {
			return nil
		}
		r = &clientRates{issue: rate.NewLimiter(1, 2), submit: rate.NewLimiter(4, 4), read: rate.NewLimiter(4, 8)}
		s.rates[key] = r
	}
	switch action {
	case "issue":
		return r.issue
	case "submit":
		return r.submit
	default:
		return r.read
	}
}
func publicHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func writeFailure(w http.ResponseWriter, code string) {
	raw, _ := json.Marshal(ErrorResponse{Version: Version1, Error: APIError{Code: code, OutcomeUnknown: code == "outcome_unknown"}})
	publicHeaders(w)
	w.WriteHeader(errorStatus(code))
	_, _ = w.Write(append(raw, '\n'))
}
func operationFailure(err error, mutating bool) string {
	if mutating {
		switch {
		case errors.Is(err, poolshare.ErrOutcomeUnknown):
			return "outcome_unknown"
		case errors.Is(err, poolshare.ErrLowWork):
			return "rejected"
		case errors.Is(err, poolshare.ErrExpired):
			return "expired"
		case errors.Is(err, poolshare.ErrNotFound):
			return "not_found"
		case errors.Is(err, poolshare.ErrInvalid):
			return "invalid_request"
		case errors.Is(err, poolshare.ErrBusy):
			return "busy"
		case errors.Is(err, poolshare.ErrCapacity):
			return "capacity"
		}
		return "outcome_unknown"
	}
	switch {
	case errors.Is(err, poolshare.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, poolshare.ErrNotFound):
		return "not_found"
	case errors.Is(err, poolshare.ErrInvalid), errors.Is(err, poolsettle.ErrInvalid):
		return "invalid_request"
	case errors.Is(err, poolshare.ErrBusy), errors.Is(err, poolsettle.ErrBusy):
		return "busy"
	case errors.Is(err, poolshare.ErrCapacity), errors.Is(err, poolsettle.ErrCapacity):
		return "capacity"
	}
	return "unavailable"
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s != nil && s.daily != nil {
		s.serveDailyHTTP(w, r)
		return
	}
	if s == nil || wire.TestPoW || !s.begin() {
		writeFailure(w, "unavailable")
		return
	}
	defer s.handlers.Done()
	if !validHost(r) || len(r.Header.Values("Origin")) != 0 {
		writeFailure(w, "forbidden")
		return
	}
	action := requestAction(r)
	if action == "" {
		writeFailure(w, "invalid_request")
		return
	}
	select {
	case s.active <- struct{}{}:
	default:
		writeFailure(w, "busy")
		return
	}
	data, failure := func() ([]byte, string) {
		defer func() { <-s.active }()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		token := bearer(r)
		key, gym, e := s.authenticate(ctx, token, "")
		if e != nil {
			return nil, "unauthorized"
		}
		limit := s.allowance(key, action)
		if limit == nil {
			return nil, "capacity"
		}
		if !limit.Allow() {
			return nil, "rate_limited"
		}
		if r.ContentLength > MaxRequestSize {
			return nil, "too_large"
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, MaxRequestSize+1))
		if len(body) > MaxRequestSize {
			return nil, "too_large"
		}
		if e != nil {
			return nil, "invalid_request"
		}
		if ctx.Err() != nil {
			return nil, "unavailable"
		}
		var jobID string
		var nonce uint32
		switch action {
		case "payouts":
			if !payoutRequest(body) {
				return nil, "invalid_request"
			}
		case "issue", "balance":
			var empty struct{}
			if decodeStrict(body, MaxRequestSize, &empty) != nil {
				return nil, "invalid_request"
			}
		case "work":
			var input workRequest
			if decodeStrict(body, MaxRequestSize, &input) != nil || !hex64(input.JobID, false) {
				return nil, "invalid_request"
			}
			jobID = input.JobID
		case "submit", "lookup":
			var input submissionRequest
			if decodeStrict(body, MaxRequestSize, &input) != nil || !hex64(input.JobID, false) {
				return nil, "invalid_request"
			}
			jobID, nonce = input.JobID, input.Nonce
		}
		var result any
		mutating := action == "issue" || action == "submit"
		maxSize := MaxResponseSize
		switch action {
		case "payouts":
			var history poolsettle.PayoutHistory
			history, e = s.settlement.MinerPayoutHistory(ctx, s.sessions, token)
			if e == nil {
				if history.Balance.GymID != gym {
					return nil, "unavailable"
				}
				result = PayoutHistoryResponse{Version1, s.binding, history}
				maxSize = MaxPayoutResponseSize
			}
		case "issue", "work":
			var work poolshare.MiningWork
			if action == "issue" {
				work, e = s.pool.IssueMiningWork(ctx, token)
			} else {
				work, e = s.pool.RecoverMiningWork(ctx, token, jobID)
			}
			if e == nil {
				if work.Job.PublicKey != key || work.Job.GymID != gym {
					return nil, operationFailure(errServer, mutating)
				}
				result = WorkResponse{Version1, s.binding, work}
				maxSize = MaxWorkResponseSize
			}
		case "submit", "lookup":
			var receipt poolshare.Submission
			if action == "submit" {
				receipt, e = s.pool.Submit(ctx, token, jobID, nonce)
			} else {
				receipt, e = s.pool.Lookup(ctx, token, jobID, nonce)
			}
			if e == nil {
				if receipt.GymID != gym {
					return nil, operationFailure(errServer, mutating)
				}
				result = SubmissionResponse{Version1, s.binding, receipt}
			}
		case "balance":
			var balance poolsettle.Balance
			var observed time.Time
			balance, observed, e = s.settlement.MinerBalance(ctx, s.sessions, token)
			if e == nil {
				if balance.GymID != gym {
					return nil, "unavailable"
				}
				var ms int64
				if !observed.IsZero() {
					ms = observed.UnixMilli()
				}
				result = BalanceResponse{Version1, s.binding, balance, ms}
			}
		}
		if e != nil {
			if action == "payouts" {
				if ctx.Err() != nil {
					return nil, "unavailable"
				}
				return nil, payoutFailure(e)
			}
			// Issuance can fail after the underlying job was synchronized but
			// before the complete pool work was acknowledged. No new-issue retry
			// is silently described as recovery of that original job.
			if action == "issue" && !errors.Is(e, poolshare.ErrBusy) {
				return nil, "outcome_unknown"
			}
			return nil, operationFailure(e, mutating)
		}
		encoded, e := json.Marshal(result)
		if e != nil || len(encoded)+1 > maxSize || ctx.Err() != nil {
			return nil, operationFailure(errServer, mutating)
		}
		if _, _, e = s.authenticate(ctx, token, key); e != nil {
			if mutating {
				return nil, "outcome_unknown"
			}
			return nil, "unauthorized"
		}
		return append(encoded, '\n'), ""
	}()
	if failure != "" {
		writeFailure(w, failure)
		return
	}
	// Slow readers cannot occupy the hash/account operation slot after the
	// synchronized operation and bounded encoding are complete.
	publicHeaders(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
