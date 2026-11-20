// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Literal expectations were frozen before the implementation; no production
// parser, router or Go output generated these expected values.
func TestIngressIndependentProtocol(t *testing.T) {
	raw, e := os.ReadFile("testdata/protocol-cases.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "5da5b679082c24058f82a7f013c2ea685a500b5c62e644c0ad845d162dff39ed" {
		t.Fatal("independent input changed")
	}
	var corpus struct {
		Version int
		Origin  string
		Cases   []struct {
			Name, Path, Method, Host, Query string
			Accept, Absolute, Trailer       bool
			ForceQuery                      bool   `json:"force_query"`
			RawPath                         string `json:"raw_path"`
			Upstream                        string
			RequestLimit                    int `json:"request_limit"`
			ResponseLimit                   int `json:"response_limit"`
			Headers                         map[string][]string
		}
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 50 || corpus.Version != 1 {
		t.Fatal("catalog inventory")
	}
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			r := &http.Request{Method: c.Method, URL: &url.URL{Path: c.Path, RawPath: c.RawPath, RawQuery: c.Query, ForceQuery: c.ForceQuery}, ProtoMajor: 1, ProtoMinor: 1, Host: "pool.example", Header: make(http.Header), ContentLength: 2}
			r.Header.Set("Content-Type", "application/json")
			if c.Path != "/slug/ownership/v2" {
				r.Header.Set("Authorization", "Bearer "+strings.Repeat("1", 64))
			}
			if c.Host != "" {
				r.Host = c.Host
			}
			if c.Absolute {
				r.URL.Scheme = "https"
				r.URL.Host = "pool.example"
			}
			if c.Trailer {
				r.Trailer = http.Header{"X-Trailer": []string{"value"}}
			}
			for k, v := range c.Headers {
				r.Header.Del(k)
				for _, value := range v {
					r.Header.Add(k, value)
				}
			}
			selected, ok := requestRoute(r, "pool.example")
			if ok != c.Accept {
				t.Fatalf("accept = %v, want %v", ok, c.Accept)
			}
			if ok {
				names := []string{"pool", "admission", "ownership"}
				if names[selected.upstream] != c.Upstream || selected.requestLimit != c.RequestLimit || selected.responseLimit != c.ResponseLimit {
					t.Fatal("frozen route limits")
				}
			}
		})
	}
}
func TestIngressPublicConfigRefusals(t *testing.T) {
	good := Config{Version: 1, Listen: "0.0.0.0:443", Origin: "https://pool.example", CertificateSPKI: strings.Repeat("1", 64), PoolURL: "http://127.0.0.1:8001", AdmissionURL: "http://127.0.0.1:8002", OwnershipURL: "http://[::1]:8003"}
	if ValidateConfig(good) != nil {
		t.Fatal("valid public config")
	}
	for _, test := range []struct {
		name   string
		change func(*Config)
	}{
		{"version", func(c *Config) { c.Version = 2 }},
		{"oversized listen", func(c *Config) { c.Listen = strings.Repeat("1", 129) }},
		{"oversized SPKI", func(c *Config) { c.CertificateSPKI = strings.Repeat("1", 100000) }}, {"listen dns", func(c *Config) { c.Listen = "localhost:443" }}, {"listen leading zero", func(c *Config) { c.Listen = "0.0.0.0:0443" }},
		{"origin HTTP", func(c *Config) { c.Origin = "http://pool.example" }}, {"origin path", func(c *Config) { c.Origin += "/" }}, {"origin upper", func(c *Config) { c.Origin = "https://POOL.example" }}, {"origin empty port", func(c *Config) { c.Origin += ":" }},
		{"origin user", func(c *Config) { c.Origin = "https://user@pool.example" }}, {"origin query", func(c *Config) { c.Origin += "?" }}, {"origin fragment", func(c *Config) { c.Origin += "#x" }},
		{"backend DNS", func(c *Config) { c.PoolURL = "http://localhost:8001" }}, {"backend remote", func(c *Config) { c.PoolURL = "http://192.0.2.1:8001" }}, {"backend no port", func(c *Config) { c.PoolURL = "http://127.0.0.1" }}, {"backend zero port", func(c *Config) { c.PoolURL = "http://127.0.0.1:0" }}, {"backend path", func(c *Config) { c.PoolURL += "/pool" }}, {"backend query", func(c *Config) { c.AdmissionURL += "?" }}, {"backend TLS", func(c *Config) { c.OwnershipURL = "https://127.0.0.1:8003" }}, {"SPKI zero", func(c *Config) { c.CertificateSPKI = strings.Repeat("0", 64) }}, {"SPKI uppercase", func(c *Config) { c.CertificateSPKI = strings.Repeat("A", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := good
			test.change(&bad)
			if ValidateConfig(bad) == nil {
				t.Fatal("accepted")
			}
		})
	}
}
