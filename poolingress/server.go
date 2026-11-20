// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/netutil"
)

// Server owns one immutable TLS ingress lifetime; private upstream services,
// stores and signer owners remain caller-owned and must outlive Run's join.
type Server struct {
	config           Config
	host             string
	targets          [3]*url.URL
	tls              *tls.Config
	transport        *http.Transport
	mu               sync.Mutex
	started, closing bool
	cancel           context.CancelFunc
	life             context.Context
	done             chan struct{}
	result           error
	calls            sync.WaitGroup
	slots            chan struct{}
}

func New(c Config, certificatePEM, privateKeyPEM []byte) (*Server, error) {
	external, targets, e := validateConfig(c)
	if e != nil {
		return nil, e
	}
	tc, e := certificateConfig(c, external, certificatePEM, privateKeyPEM)
	if e != nil {
		return nil, e
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true, MaxConnsPerHost: 8, ForceAttemptHTTP2: false}
	return &Server{config: c, host: external.Host, targets: targets, tls: tc, transport: transport, done: make(chan struct{}), slots: make(chan struct{}, 8)}, nil
}
func (s *Server) Config() Config {
	if s == nil {
		return Config{}
	}
	return s.config
}
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.closing {
		return false
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return false
	}
	s.calls.Add(1)
	return true
}
func (s *Server) finish()          { <-s.slots; s.calls.Done() }
func (s *Server) stopping()        { s.mu.Lock(); s.closing = true; s.mu.Unlock() }
func (s *Server) complete(e error) { s.mu.Lock(); s.result = e; close(s.done); s.mu.Unlock() }

// Run may be called once. onReady receives the actual literal bound address;
// it never changes the independently selected HTTPS origin/certificate.
func (s *Server) Run(ctx context.Context, onReady func(string)) error {
	if s == nil || s.done == nil || ctx == nil {
		return ErrConfig
	}
	s.mu.Lock()
	if s.started || s.closing {
		s.mu.Unlock()
		return ErrClosed
	}
	s.started = true
	base, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.life = base
	s.mu.Unlock()
	result := error(nil)
	defer func() { s.stopping(); cancel(); s.transport.CloseIdleConnections(); s.calls.Wait(); s.complete(result) }()
	var lc net.ListenConfig
	listener, e := lc.Listen(base, "tcp", s.config.Listen)
	if e != nil {
		if base.Err() == nil {
			result = ErrUnavailable
		}
		return result
	}
	httpServer := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return base }, ErrorLog: log.New(io.Discard, "", 0), TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){}}
	served := make(chan error, 1)
	go func() {
		served <- httpServer.Serve(tls.NewListener(netutil.LimitListener(listener, 16), s.tls.Clone()))
	}()
	if onReady != nil {
		onReady(listener.Addr().String())
	}
	var serveErr error
	joined := false
	select {
	case serveErr = <-served:
		joined = true
	case <-base.Done():
	}
	s.stopping()
	cancel()
	closeErr := httpServer.Close()
	if !joined {
		serveErr = <-served
	}
	if closeErr != nil || serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		result = ErrUnavailable
	}
	return result
}

// Close refuses new work, cancels the original Run, and joins its listener and
// all admitted handlers. It does not close or replace any private upstream.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.done == nil {
		return ErrConfig
	}
	s.mu.Lock()
	if !s.started && !s.closing {
		close(s.done)
	}
	s.closing = true
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	<-done
	s.transport.CloseIdleConnections()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}
func fail(w http.ResponseWriter, code int) { http.Error(w, "pool ingress unavailable", code) }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if s == nil {
		fail(w, http.StatusServiceUnavailable)
		return
	}
	selected, ok := requestRoute(r, s.host)
	if !ok || r.TLS == nil || r.Body == nil {
		fail(w, http.StatusBadRequest)
		return
	}
	if !s.begin() {
		fail(w, http.StatusServiceUnavailable)
		return
	}
	defer s.finish()
	ctx, cancel := context.WithTimeout(r.Context(), selected.timeout)
	defer cancel()
	stopOwner := context.AfterFunc(s.life, cancel)
	defer stopOwner()
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(selected.timeout)
	readDeadline := time.Now().Add(10 * time.Second)
	if deadline.Before(readDeadline) {
		readDeadline = deadline
	}
	if controller.SetReadDeadline(readDeadline) != nil || controller.SetWriteDeadline(deadline) != nil {
		fail(w, http.StatusServiceUnavailable)
		return
	}
	bodyDone := make(chan struct{})
	stopBody := context.AfterFunc(ctx, func() { defer close(bodyDone); _ = r.Body.Close() })
	defer func() {
		if !stopBody() {
			<-bodyDone
		}
	}()
	body, e := io.ReadAll(io.LimitReader(r.Body, int64(selected.requestLimit)+1))
	if e != nil || len(body) > selected.requestLimit || int64(len(body)) != r.ContentLength || ctx.Err() != nil {
		fail(w, http.StatusBadRequest)
		return
	}
	// A fresh request with only these two headers prevents forwarded identity,
	// hop headers, cookies and idempotency-based retry. Body has no GetBody.
	target := *s.targets[selected.upstream]
	target.Path = r.URL.Path
	outbound, e := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), io.NopCloser(bytes.NewReader(body)))
	if e != nil {
		fail(w, http.StatusBadGateway)
		return
	}
	outbound.ContentLength = int64(len(body))
	outbound.Host = target.Host
	outbound.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if selected.upstream != 2 {
		outbound.Header.Set("Authorization", r.Header.Get("Authorization"))
	}
	response, e := s.transport.RoundTrip(outbound)
	if e != nil {
		fail(w, http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	limit := selected.responseLimit
	// Every existing non-success envelope is smaller than this finite cap.
	if response.StatusCode != http.StatusOK {
		limit = 65536
		if selected.upstream != 0 {
			limit = 73728
		}
	}
	if response.StatusCode != http.StatusOK && (response.StatusCode < 400 || response.StatusCode > 599) || !jsonMedia(response.Header.Values("Content-Type")) || len(response.Header.Values("Content-Encoding")) != 0 || len(response.Trailer) != 0 || response.ContentLength > int64(limit) {
		fail(w, http.StatusBadGateway)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if e != nil || len(raw) > limit || len(response.Trailer) != 0 || ctx.Err() != nil {
		fail(w, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(raw)
}
