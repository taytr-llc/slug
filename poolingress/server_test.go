// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testMaterial struct {
	cert, key []byte
	spki      string
	roots     *x509.CertPool
}

// These public repeated-byte seeds are synthetic transport test material only.
// Standard-library X.509 is a peer fixture, not an independent byte oracle.
func material(t *testing.T, name string, expiry time.Time) testMaterial {
	t.Helper()
	caKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x74}, 32))
	serverKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x75}, 32))
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	ca := &x509.Certificate{SerialNumber: big.NewInt(74), Subject: pkix.Name{CommonName: "PUBLIC SYNTHETIC INGRESS TEST CA"}, NotBefore: start, NotAfter: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, e := x509.CreateCertificate(rand.Reader, ca, ca, caKey.Public(), caKey)
	if e != nil {
		t.Fatal(e)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(75), Subject: pkix.Name{CommonName: "PUBLIC SYNTHETIC INGRESS TEST LEAF"}, DNSNames: []string{name}, NotBefore: start, NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, ca, serverKey.Public(), caKey)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(parsed.RawSubjectPublicKeyInfo)
	key, e := x509.MarshalPKCS8PrivateKey(serverKey)
	if e != nil {
		t.Fatal(e)
	}
	cert := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})...)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		t.Fatal("CA")
	}
	return testMaterial{cert, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), hex.EncodeToString(sum[:]), roots}
}
func configFor(m testMaterial, upstream string) Config {
	return Config{Version: 1, Listen: "127.0.0.1:0", Origin: "https://pool.example", CertificateSPKI: m.spki, PoolURL: upstream, AdmissionURL: upstream, OwnershipURL: upstream}
}

type runningIngress struct {
	s        *Server
	addr     string
	client   *http.Client
	done     chan error
	cancel   context.CancelFunc
	material testMaterial
}

func runTestIngress(t *testing.T, c Config, m testMaterial) runningIngress {
	t.Helper()
	s, e := New(c, m.cert, m.key)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	var transport *http.Transport
	joined := false
	t.Cleanup(func() {
		cancel()
		if !joined {
			select {
			case e := <-done:
				if e != nil {
					t.Error("Run cleanup", e)
				}
			case <-time.After(5 * time.Second):
				t.Error("Run cleanup did not join")
				return
			}
		}
		if e := s.Close(); e != nil {
			t.Error("Close", e)
		}
		if transport != nil {
			transport.CloseIdleConnections()
		}
	})
	go func() { done <- s.Run(ctx, func(addr string) { ready <- addr }) }()
	var addr string
	select {
	case addr = <-ready:
	case e := <-done:
		joined = true
		t.Fatalf("Run before ready: %v", e)
	case <-time.After(5 * time.Second):
		t.Fatal("ingress did not start")
	}
	transport = &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, addr)
	}, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: m.roots}, DisableKeepAlives: true, DisableCompression: true}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return runningIngress{s, addr, client, done, cancel, m}
}
func request(t *testing.T, path string, body []byte) *http.Request {
	t.Helper()
	r, e := http.NewRequest(http.MethodPost, "https://pool.example"+path, bytes.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Content-Type", "application/json")
	if path != "/slug/ownership/v2" {
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("1", 64))
	}
	return r
}
func readResponse(t *testing.T, client *http.Client, r *http.Request) (int, []byte, http.Header) {
	t.Helper()
	response, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(response.Body)
	if e != nil {
		t.Fatal(e)
	}
	return response.StatusCode, raw, response.Header
}
func TestIngressTLSExactForwarding(t *testing.T) {
	type observed struct {
		path, host, auth, media string
		body                    []byte
		headers                 http.Header
	}
	seen := make(chan observed, 16)
	response := []byte("{\"opaque\":\"exact backend bytes\"}\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen <- observed{r.URL.Path, r.Host, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), raw, r.Header.Clone()}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "must-not-forward=1")
		w.Header().Set("X-Private-Diagnostic", "must-not-forward")
		_, _ = w.Write(response)
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	// Mutating caller PEM buffers after New must not mutate retained TLS state.
	clear(m.cert)
	clear(m.key)
	for _, path := range []string{"/pool/v1/issue", "/pool/v1/work", "/pool/v1/submit", "/pool/v1/lookup", "/pool/v1/balance", "/pool/v1/payouts", "/v3/admissions", "/slug/ownership/v2"} {
		body := []byte("{\"literal\":1}")
		r := request(t, path, body)
		r.Header.Set("Cookie", "private=not-forwarded")
		r.Header.Set("X-Other", "not-forwarded")
		r.Header.Set("Idempotency-Key", "not-forwarded")
		status, raw, headers := readResponse(t, run.client, r)
		if status != 200 || !bytes.Equal(raw, response) || headers.Get("Set-Cookie") != "" || headers.Get("X-Private-Diagnostic") != "" {
			t.Fatal("response changed or leaked", status)
		}
		v := <-seen
		wantAuth := r.Header.Get("Authorization")
		if v.path != path || v.host != strings.TrimPrefix(upstream.URL, "http://") || v.auth != wantAuth || v.media != "application/json" || !bytes.Equal(v.body, body) {
			t.Fatal("original request changed")
		}
		if v.headers.Get("Cookie") != "" || v.headers.Get("X-Other") != "" || v.headers.Get("Idempotency-Key") != "" || v.headers.Get("X-Forwarded-For") != "" {
			t.Fatal("unselected header forwarded")
		}
	}
	got := run.s.Config()
	got.Origin = "https://replacement.invalid"
	if run.s.Config().Origin != "https://pool.example" {
		t.Fatal("config alias")
	}
}
func TestIngressNetworkRefusesWithoutDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	for _, test := range []struct {
		name   string
		change func(*http.Request)
	}{
		{"jobs bypass", func(r *http.Request) { r.URL.Path = "/v3/jobs" }}, {"admin", func(r *http.Request) { r.URL.Path = "/admin" }}, {"wrong host", func(r *http.Request) { r.Host = "wrong.example" }}, {"origin", func(r *http.Request) { r.Header.Set("Origin", "https://pool.example") }}, {"forwarded", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "127.0.0.1") }}, {"duplicate auth", func(r *http.Request) { r.Header.Add("Authorization", r.Header.Get("Authorization")) }}, {"missing auth", func(r *http.Request) { r.Header.Del("Authorization") }}, {"method", func(r *http.Request) { r.Method = "GET" }}, {"query", func(r *http.Request) { r.URL.ForceQuery = true }}, {"ownership bearer", func(r *http.Request) { r.URL.Path = "/slug/ownership/v2" }},
		{"unknown length chunked", func(r *http.Request) {
			r.ContentLength = -1
			r.GetBody = nil
			r.Body = io.NopCloser(strings.NewReader("{}"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := request(t, "/pool/v1/issue", []byte("{}"))
			test.change(r)
			status, _, _ := readResponse(t, run.client, r)
			if status != 400 {
				t.Fatal(status)
			}
		})
	}
	r := request(t, "/pool/v1/submit", bytes.Repeat([]byte{' '}, 1025))
	status, _, _ := readResponse(t, run.client, r)
	if status != 400 {
		t.Fatal("request cap", status)
	}
	if calls.Load() != 0 {
		t.Fatal("refused request reached backend")
	}
}
func TestIngressUpstreamFailureNoRetry(t *testing.T) {
	var calls, followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1); w.WriteHeader(200) }))
	t.Cleanup(target.Close)
	mode := atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch mode.Load() {
		case 0:
			w.Header().Set("Location", target.URL)
			w.WriteHeader(307)
			_, _ = w.Write([]byte("{}"))
		case 1:
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write([]byte("not gzip"))
		case 2:
			w.Header().Add("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		case 3:
			_, _ = w.Write(bytes.Repeat([]byte{' '}, 65537))
		case 4:
			w.WriteHeader(503)
			_, _ = w.Write([]byte("{\"error\":{\"code\":\"outcome_unknown\"}}\n"))
		case 5:
			_, _ = w.Write(bytes.Repeat([]byte{' '}, 73729))
		}
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	for i := int32(0); i < 6; i++ {
		mode.Store(i)
		path := "/pool/v1/balance"
		if i == 5 {
			path = "/v3/admissions"
		}
		status, raw, _ := readResponse(t, run.client, request(t, path, []byte("{}")))
		if i == 4 {
			if status != 503 || string(raw) != "{\"error\":{\"code\":\"outcome_unknown\"}}\n" {
				t.Fatal("backend uncertainty altered")
			}
		} else if status != 502 {
			t.Fatal("bad upstream response accepted", i, status)
		}
	}
	if calls.Load() != 6 || followed.Load() != 0 {
		t.Fatal("retry/redirect occurred", calls.Load(), followed.Load())
	}
}
func TestIngressCertificateTrustAndProtocol(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	good := configFor(m, upstream.URL)
	bad := good
	bad.CertificateSPKI = strings.Repeat("2", 64)
	if _, e := New(bad, m.cert, m.key); e == nil {
		t.Fatal("wrong SPKI")
	}
	wrong := material(t, "other.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	bad = good
	bad.CertificateSPKI = wrong.spki
	if _, e := New(bad, wrong.cert, wrong.key); e == nil {
		t.Fatal("wrong SAN")
	}
	expired := material(t, "pool.example", time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, e := New(good, expired.cert, expired.key); e == nil {
		t.Fatal("expired certificate")
	}
	if _, e := New(good, bytes.Repeat([]byte{'x'}, 65537), m.key); e == nil {
		t.Fatal("oversized chain")
	}
	if _, e := New(good, m.cert, bytes.Repeat([]byte{'x'}, 16385)); e == nil {
		t.Fatal("oversized key")
	}
	run := runTestIngress(t, good, m)
	for _, test := range []struct {
		name   string
		config *tls.Config
	}{
		{"untrusted CA", &tls.Config{ServerName: "pool.example", RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS13}},
		{"wrong SNI", &tls.Config{ServerName: "other.example", RootCAs: m.roots, MinVersion: tls.VersionTLS13}},
		{"TLS12", &tls.Config{ServerName: "pool.example", RootCAs: m.roots, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", run.addr, test.config)
			if conn != nil {
				_ = conn.Close()
			}
			if e == nil {
				t.Fatal("untrusted handshake")
			}
		})
	}
	conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", run.addr, &tls.Config{ServerName: "pool.example", RootCAs: m.roots, MinVersion: tls.VersionTLS13, NextProtos: []string{"http/1.1"}})
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if conn.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatal("TLS version")
	}
}
func TestIngressCloseCancelsAndJoins(t *testing.T) {
	entered := make(chan bool, 1)
	exited := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(exited)
		// net/http starts monitoring client disconnects after consuming the body.
		// Match the real upstream's read-before-work behavior before waiting.
		body, e := io.ReadAll(io.LimitReader(r.Body, 3))
		ok := e == nil && bytes.Equal(body, []byte("{}"))
		entered <- ok
		if !ok {
			return
		}
		select {
		case <-r.Context().Done():
		case <-release: // Cleanup only; never satisfies the cancellation assertion.
		}
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	requestDone := make(chan struct{})
	originalRequest := request(t, "/pool/v1/submit", []byte("{}"))
	requestContext, cancelRequest := context.WithCancel(originalRequest.Context())
	originalRequest = originalRequest.WithContext(requestContext)
	go func() {
		defer close(requestDone)
		response, e := run.client.Do(originalRequest)
		if e == nil {
			_ = response.Body.Close()
		}
	}()
	t.Cleanup(func() {
		cancelRequest()
		close(release)
		upstream.CloseClientConnections()
		select {
		case <-requestDone:
		case <-time.After(3 * time.Second):
			t.Error("client cleanup did not join")
		}
	})
	select {
	case ok := <-entered:
		if !ok {
			t.Fatal("backend received wrong request body")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backend not entered")
	}
	closed := make(chan error, 1)
	go func() { closed <- run.s.Close() }()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not join")
	}
	select {
	case <-requestDone:
	case <-time.After(3 * time.Second):
		t.Fatal("client unjoined")
	}
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream context not cancelled")
	}
	if e := run.s.Run(context.Background(), nil); e != ErrClosed {
		t.Fatal("restarted", e)
	}
	if len(run.s.slots) != 0 {
		t.Fatal("handler retained")
	}
}
func TestIngressCloseInterruptsBodyAndUnusedOwner(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	c := configFor(m, upstream.URL)
	unused, e := New(c, m.cert, m.key)
	if e != nil {
		t.Fatal(e)
	}
	if unused.Close() != nil || unused.Close() != nil || unused.Run(context.Background(), nil) != ErrClosed {
		t.Fatal("unused lifecycle")
	}
	run := runTestIngress(t, c, m)
	conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", run.addr, &tls.Config{ServerName: "pool.example", RootCAs: m.roots, MinVersion: tls.VersionTLS13})
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	_, e = fmt.Fprintf(conn, "POST /pool/v1/submit HTTP/1.1\r\nHost: pool.example\r\nContent-Type: application/json\r\nAuthorization: Bearer %s\r\nContent-Length: 2\r\n\r\n", strings.Repeat("1", 64))
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for len(run.s.slots) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(run.s.slots) != 1 {
		t.Fatal("slow body not admitted")
	}
	closed := make(chan error, 1)
	go func() { closed <- run.s.Close() }()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("slow body Close did not join")
	}
	if calls.Load() != 0 || len(run.s.slots) != 0 {
		t.Fatal("partial body dispatched or handler leaked")
	}
}

func TestIngressExactBodyBoundariesAndErrorCap(t *testing.T) {
	responseSize := atomic.Int32{}
	responseCode := atomic.Int32{}
	responseCode.Store(200)
	received := make(chan int, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- len(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(responseCode.Load()))
		_, _ = w.Write(bytes.Repeat([]byte{' '}, int(responseSize.Load())))
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	for _, tc := range []struct {
		path              string
		request, response int
	}{{"/pool/v1/work", 1024, 16842752}, {"/pool/v1/payouts", 1024, 131072}, {"/v3/admissions", 65536, 73728}, {"/slug/ownership/v2", 1024, 73728}} {
		responseSize.Store(int32(tc.response))
		status, raw, _ := readResponse(t, run.client, request(t, tc.path, bytes.Repeat([]byte{' '}, tc.request)))
		if status != 200 || len(raw) != tc.response || <-received != tc.request {
			t.Fatal("exact limit changed", tc.path, status, len(raw))
		}
	}
	responseCode.Store(503)
	responseSize.Store(65537)
	status, _, _ := readResponse(t, run.client, request(t, "/pool/v1/work", []byte("{}")))
	if status != 502 {
		t.Fatal("pool error exceeded original 65536 cap", status)
	}
	<-received
}

func TestIngressLostAcknowledgmentIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	status, raw, _ := readResponse(t, run.client, request(t, "/pool/v1/submit", []byte("{}")))
	if status != 502 || calls.Load() != 1 || !bytes.Equal(raw, []byte("pool ingress unavailable\n")) {
		t.Fatal("uncertain operation replayed or backend text disclosed", status, calls.Load())
	}
}

func TestIngressAdmissionCapacityAndJoin(t *testing.T) {
	entered := make(chan bool, 16)
	exited := make(chan struct{}, 16)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { exited <- struct{}{} }()
		body, e := io.ReadAll(io.LimitReader(r.Body, 3))
		ok := e == nil && bytes.Equal(body, []byte("{}"))
		entered <- ok
		if !ok {
			return
		}
		select {
		case <-r.Context().Done():
		case <-release: // Cleanup only, after the cancellation assertions.
		}
	}))
	t.Cleanup(upstream.Close)
	m := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	run := runTestIngress(t, configFor(m, upstream.URL), m)
	clients := make(chan struct{}, 8)
	t.Cleanup(func() {
		run.cancel()
		close(release)
		upstream.CloseClientConnections()
		for i := 0; i < 8; i++ {
			select {
			case <-clients:
			case <-time.After(5 * time.Second):
				t.Error("capacity client unjoined")
				return
			}
		}
	})
	for i := 0; i < 8; i++ {
		r := request(t, "/v3/admissions", []byte("{}"))
		go func() {
			defer func() { clients <- struct{}{} }()
			response, e := run.client.Do(r)
			if e == nil {
				_ = response.Body.Close()
			}
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case ok := <-entered:
			if !ok {
				t.Fatal("backend received wrong request body")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("capacity requests not entered")
		}
	}
	status, _, _ := readResponse(t, run.client, request(t, "/v3/admissions", []byte("{}")))
	if status != 503 {
		t.Fatal("ninth request admitted", status)
	}
	if e := run.s.Close(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 8; i++ {
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			t.Fatal("upstream handler unjoined")
		}
	}
}
