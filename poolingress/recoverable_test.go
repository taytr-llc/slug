// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecoverableIngressOriginalTLSAndJoin(t *testing.T) {
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
	owner, err := NewRecoverableDaily(c, material.cert, material.key)
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
	request, _ := http.NewRequest(http.MethodPost, c.Origin+"/pool/v3/issue-lookup", strings.NewReader("{}"))
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
		if path != "/pool/v3/issue-lookup" {
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

func TestRecoverableIngressCatalogAndCrossMode(t *testing.T) {
	for _, name := range []string{"issue", "issue-lookup", "work", "submit", "lookup", "balance", "payouts"} {
		r, ok := selectedRouteFor("/pool/v3/"+name, 3)
		if !ok {
			t.Fatal("missing selected route", name)
		}
		oldName := name
		if oldName == "issue-lookup" {
			oldName = "work"
		}
		base, ok := selectedRouteFor("/pool/v2/"+oldName, 2)
		if !ok || r != base {
			t.Fatal("changed existing bounds")
		}
		for _, version := range []uint32{1, 2} {
			if _, ok = selectedRouteFor("/pool/v3/"+name, version); ok {
				t.Fatal("old mode adopted selected route")
			}
		}
		for _, prefix := range []string{"/pool/v1/", "/pool/v2/"} {
			if _, ok = selectedRouteFor(prefix+name, 3); ok {
				t.Fatal("selected mode accepted old route")
			}
		}
	}
	for _, path := range []string{"/v3/admissions", "/slug/ownership/v2"} {
		a, ok := selectedRouteFor(path, 3)
		b, old := routeFor(path)
		if !ok || !old || a != b {
			t.Fatal("adjacent route changed")
		}
	}
	if _, ok := selectedRouteFor("/pool/v2/issue-lookup", 2); ok {
		t.Fatal("plain daily grew new route")
	}
	c := Config{Version: 2, Listen: "127.0.0.1:0", Origin: "https://pool.example", CertificateSPKI: strings.Repeat("1", 64), PoolURL: "http://127.0.0.1:8001", AdmissionURL: "http://127.0.0.1:8002", OwnershipURL: "http://127.0.0.1:8003"}
	if ValidateRecoverableDailyConfig(c) != nil {
		t.Fatal("base2 refused")
	}
	c.Version = 3
	if ValidateRecoverableDailyConfig(c) == nil {
		t.Fatal("invented ingress config3 accepted")
	}
}
