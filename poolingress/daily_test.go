// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDailyIngressCatalogAndCrossMode(t *testing.T) {
	for _, path := range []string{"issue", "work", "submit", "lookup", "balance", "payouts"} {
		old := "/pool/v1/" + path
		daily := "/pool/v2/" + path
		a, ok := routeFor(old)
		if !ok {
			t.Fatal("legacy route")
		}
		b, ok := selectedRouteFor(daily, 2)
		if !ok || a != b {
			t.Fatal("daily changed inherited bounds")
		}
		if _, ok = selectedRouteFor(old, 2); ok {
			t.Fatal("daily accepts legacy route")
		}
		if _, ok = routeFor(daily); ok {
			t.Fatal("legacy accepts daily route")
		}
		r := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: daily}, RequestURI: daily, ProtoMajor: 1, ProtoMinor: 1, Host: "pool.example", Header: http.Header{"Content-Type": []string{"application/json"}, "Authorization": []string{"Bearer " + strings.Repeat("1", 64)}}, ContentLength: 2}
		if _, ok = requestSelectedRoute(r, "pool.example", 2); !ok {
			t.Fatal("valid daily request")
		}
		r.TransferEncoding = []string{"chunked"}
		if _, ok = requestSelectedRoute(r, "pool.example", 2); ok {
			t.Fatal("chunked request")
		}
	}
	for _, path := range []string{"/v3/admissions", "/slug/ownership/v2"} {
		a, ok := routeFor(path)
		b, daily := selectedRouteFor(path, 2)
		if !ok || !daily || a != b {
			t.Fatal("adjacent route changed")
		}
	}
	c := Config{Version: 2, Listen: "127.0.0.1:0", Origin: "https://pool.example", CertificateSPKI: strings.Repeat("1", 64), PoolURL: "http://127.0.0.1:8001", AdmissionURL: "http://127.0.0.1:8002", OwnershipURL: "http://127.0.0.1:8003"}
	if ValidateDailyConfig(c) != nil || ValidateConfig(c) == nil {
		t.Fatal("selected config2")
	}
	c.Version = 1
	if ValidateDailyConfig(c) == nil {
		t.Fatal("daily config1")
	}
}
func TestDailyIngressOriginalTLSAndJoin(t *testing.T) {
	seen := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil || string(raw) != "{}" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("1", 64) || r.Header.Get("X-Forwarded-For") != "" {
			t.Error("forwarding changed")
		}
		seen <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}\n"))
	}))
	defer upstream.Close()
	material := material(t, "pool.example", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	c := configFor(material, upstream.URL)
	c.Version = 2
	if _, err := New(c, material.cert, material.key); err == nil {
		t.Fatal("legacy adopted daily")
	}
	owner, err := NewDaily(c, material.cert, material.key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan string, 1)
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Run did not join")
				return
			}
		}
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	}()
	go func() { done <- owner.Run(ctx, func(addr string) { ready <- addr }) }()
	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		joined = true
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("ready deadline")
	}
	tr := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, addr)
	}, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: material.roots}, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	request, _ := http.NewRequest(http.MethodPost, c.Origin+"/pool/v2/balance", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("1", 64))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	select {
	case path := <-seen:
		if path != "/pool/v2/balance" {
			t.Fatal("rewritten route")
		}
	default:
		t.Fatal("upstream missing")
	}
	cancel()
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel join")
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
}
