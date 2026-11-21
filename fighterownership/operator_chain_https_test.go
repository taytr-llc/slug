//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// These ephemeral transport keys authenticate only the real local TLS server.
// Network/genesis expectations come from the existing independent profile.
func operatorChainHTTPSCertificate(t *testing.T, hostname string) (tls.Certificate, string) {
	t.Helper()
	caPub, caKey, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "owned chain test CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, e := x509.CreateCertificate(rand.Reader, ca, ca, caPub, caKey)
	if e != nil {
		t.Fatal(e)
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{hostname}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, leaf, ca, pub, caKey)
	if e != nil {
		t.Fatal(e)
	}
	return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
}

type operatorChainHTTPSFixture struct {
	mu           sync.Mutex
	methods      []string
	authFailures int
	server       *httptest.Server
	config       prospectnode.RPCConfig
}

func newOperatorChainHTTPSFixture(t *testing.T, profile registry.OperatorProfile, hostname, response string) *operatorChainHTTPSFixture {
	t.Helper()
	certificate, ca := operatorChainHTTPSCertificate(t, hostname)
	f := &operatorChainHTTPSFixture{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     uint64            `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.methods = append(f.methods, request.Method)
		user, password, ok := r.BasicAuth()
		if !ok || user != "fixture" || password != "fixture-password" {
			f.authFailures++
			f.mu.Unlock()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.mu.Unlock()
		if response == "redirect" {
			http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
			return
		}
		var result any
		switch request.Method {
		case "getcurrentnet":
			if len(request.Params) != 0 {
				http.Error(w, "invalid network parameters", 400)
				return
			}
			result = profile.Pin().Source.Network
			if response == "wrong-network" {
				result = uint32(1)
			}
		case "getblockhash":
			if len(request.Params) != 1 || string(request.Params[0]) != "0" {
				http.Error(w, "invalid genesis parameters", 400)
				return
			}
			result = profile.Pin().Source.Genesis
			if response == "wrong-genesis" {
				result = strings.Repeat("1", 64)
			}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, request.ID})
	})
	f.server = httptest.NewUnstartedServer(h)
	f.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	f.server.StartTLS()
	t.Cleanup(f.server.Close)
	u, e := url.Parse(f.server.URL)
	if e != nil {
		t.Fatal(e)
	}
	_, port, e := net.SplitHostPort(u.Host)
	if e != nil {
		t.Fatal(e)
	}
	// Use a real resolvable DNS name with the unchanged production dialer.
	// The old constructor rejects this name before making either identity RPC.
	u.Host = net.JoinHostPort("localhost", port)
	f.config = prospectnode.RPCConfig{Version: 1, URL: u.String(), Username: "fixture", Password: "fixture-password", CACertificate: ca}
	return f
}

func (f *operatorChainHTTPSFixture) observed() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.methods...), f.authFailures
}

func TestOperatorOwnedChainSourceExplicitHTTPS(t *testing.T) {
	profile := operatorCoreProfile(t)
	f := newOperatorChainHTTPSFixture(t, profile, "localhost", "valid")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := NewOperatorOwnedChainSource(ctx, profile, f.config, ctx)
	if e != nil {
		t.Fatal("operator HTTPS with original explicit CA and verified DNS identity", e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	methods, failures := f.observed()
	if strings.Join(methods, ",") != "getcurrentnet,getblockhash" || failures != 0 {
		t.Fatalf("original authenticated network/genesis calls: %v failures=%d", methods, failures)
	}
	tr, ok := s.http.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil || tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil || tr.TLSClientConfig.InsecureSkipVerify || s.http.Timeout != 5*time.Second || tr.DialContext == nil || !tr.DisableKeepAlives {
		t.Fatal("original verified, bounded, nonreplaying transport changed")
	}
	if s.endpoint != f.config.URL || s.pin != profile.Pin() || !s.codec.operator {
		t.Fatal("selected source changed")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(s.Alive(), ErrClosed) {
		t.Fatal("closed original source still alive")
	}
}

func TestOperatorOwnedChainSourceHTTPSRefusals(t *testing.T) {
	profile := operatorCoreProfile(t)
	for _, tc := range []struct {
		name, hostname, response string
		calls, authFailures      int
	}{
		{"wrong-ca", "localhost", "valid", 0, 0},
		{"wrong-name", "unselected.invalid", "valid", 0, 0},
		{"wrong-network", "localhost", "wrong-network", 1, 0},
		{"wrong-genesis", "localhost", "wrong-genesis", 2, 0},
		{"wrong-auth", "localhost", "valid", 1, 1},
		{"empty-ca", "localhost", "valid", 0, 0},
		{"nonloopback-http", "localhost", "valid", 0, 0},
		{"redirect", "localhost", "redirect", 1, 0},
		{"desktop-still-loopback", "localhost", "valid", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOperatorChainHTTPSFixture(t, profile, tc.hostname, tc.response)
			cfg := f.config
			switch tc.name {
			case "wrong-ca":
				_, cfg.CACertificate = operatorChainHTTPSCertificate(t, "localhost")
			case "wrong-auth":
				cfg.Password = "unselected-password"
			case "empty-ca":
				cfg.CACertificate = ""
			case "nonloopback-http":
				cfg.URL = strings.Replace(cfg.URL, "https://", "http://", 1)
				cfg.CACertificate = ""
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var source *OwnedChainSource
			var e error
			if tc.name == "desktop-still-loopback" {
				// Valid legacy Pin1, so refusal cannot come merely from passing an
				// operator Pin2 through the legacy codec before checking the URL.
				source, e = NewOwnedChainSource(ctx, chainReferencePin(t), cfg, ctx)
			} else {
				source, e = NewOperatorOwnedChainSource(ctx, profile, cfg, ctx)
			}
			if source != nil {
				_ = source.Close()
				t.Fatal("refused constructor returned owner")
			}
			if e == nil {
				t.Fatal("unselected transport/identity accepted")
			}
			methods, failures := f.observed()
			if len(methods) != tc.calls || failures != tc.authFailures {
				t.Fatalf("refusal boundary: methods=%v authFailures=%d want calls=%d failures=%d", methods, failures, tc.calls, tc.authFailures)
			}
		})
	}
}
