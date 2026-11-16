// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const configTestKey = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"

func TestServerConfigStrictAndCanonical(t *testing.T) {
	config := ServerConfig{Version: 1, Listen: "[::1]:0", Clients: []Credential{{PublicKey: strings.ToUpper(configTestKey), TokenSHA256: strings.Repeat("AB", 32)}}, MaxJobs: 2, MaxRecords: 3}
	raw, _ := json.Marshal(config)
	got, err := ParseServerConfig(raw)
	if err != nil || got.Clients[0].PublicKey != configTestKey || got.Clients[0].TokenSHA256 != strings.Repeat("ab", 32) {
		t.Fatalf("canonical config: %+v %v", got, err)
	}
	got.Clients[0].PublicKey = "changed"
	if config.Clients[0].PublicKey == "changed" {
		t.Fatal("returned config aliases caller slice")
	}
	for _, listen := range []string{"localhost:9", "0.0.0.0:9", "[::]:9", "192.0.2.1:9", "127.0.0.1", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:01", "[::1%lo0]:9"} {
		c := config
		c.Listen = listen
		if _, err := ValidateServerConfig(c); err == nil {
			t.Errorf("accepted listener %s", listen)
		}
	}
	for _, mutate := range []func(*ServerConfig){
		func(c *ServerConfig) { c.Version = 2 }, func(c *ServerConfig) { c.MaxJobs = 0 }, func(c *ServerConfig) { c.MaxRecords = 0 },
		func(c *ServerConfig) { c.MaxJobs = 1000000001 }, func(c *ServerConfig) { c.MaxRecords = 1000000001 },
		func(c *ServerConfig) { c.Clients = nil }, func(c *ServerConfig) { c.Clients = append(c.Clients, c.Clients[0]) },
		func(c *ServerConfig) { c.Clients = make([]Credential, 65) },
		func(c *ServerConfig) {
			c.Clients = []Credential{{PublicKey: "02" + strings.Repeat("ff", 32), TokenSHA256: strings.Repeat("ab", 32)}}
		},
	} {
		c := config
		mutate(&c)
		if _, err := ValidateServerConfig(c); err == nil {
			t.Fatal("accepted invalid service config")
		}
	}
	bad := [][]byte{
		append([]byte(`{"version":1,`), raw[1:]...),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"\u0076ersion":1`), 1),
		bytes.Replace(raw, []byte(`"max_jobs":2`), []byte(`"max_jobs":null`), 1),
		bytes.Replace(raw, []byte(`"max_jobs":2`), []byte(`"max_jobs":2.0`), 1),
		bytes.Replace(raw, []byte(`"max_jobs":2,`), nil, 1),
		bytes.Replace(raw, []byte(`"public_key":`), []byte(`"PUBLIC_KEY":`), 1),
		bytes.Replace(raw, []byte(`"public_key":`), []byte(`"private_key":"SECRET","public_key":`), 1),
		append(append([]byte{}, raw...), []byte(` {}`)...),
		append(bytes.Repeat([]byte(" "), MaxServerConfigSize), raw...),
		bytes.Replace(raw, []byte("AB"), []byte{0xff}, 1),
	}
	for i, input := range bad {
		if _, err := ParseServerConfig(input); err == nil {
			t.Errorf("accepted invalid JSON %d", i)
		} else if strings.Contains(err.Error(), "SECRET") {
			t.Fatal("leaked input")
		}
	}
}

func TestClientConfigEndpointAndRedaction(t *testing.T) {
	base := ClientConfig{Version: 1, URL: "http://127.0.0.1:8765/", Token: strings.Repeat("AB", 32)}
	for _, endpoint := range []string{"http://127.0.0.1", "http://[::1]/"} {
		config := base
		config.URL = endpoint
		got, parsed, err := validateClientConfig(config)
		if err != nil || parsed.Port() != "80" || !strings.HasSuffix(got.URL, ":80") {
			t.Fatalf("implicit HTTP port does not match the server Host contract: %+v %v", got, err)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:8765/", "http://[::1]:8765", "https://authority.example", "https://127.0.0.1:8765"} {
		c := base
		c.URL = endpoint
		raw, _ := json.Marshal(c)
		got, err := ParseClientConfig(raw)
		if err != nil || got.Token != strings.ToLower(c.Token) || strings.HasSuffix(got.URL, "/") {
			t.Errorf("valid endpoint: %+v %v", got, err)
		}
	}
	for _, endpoint := range []string{"http://localhost:9", "http://192.0.2.1:9", "https://user:SECRET@authority.example", "https://authority.example/path", "https://authority.example/?token=SECRET", "https://authority.example/#SECRET", "https://authority.example/?", "https://authority.example/%2f", "https://[::1%25lo0]", "https://authority.example:0", "https://authority.example:65536", "https://authority.example:", "ftp://authority.example", "//authority.example"} {
		c := base
		c.URL = endpoint
		raw, _ := json.Marshal(c)
		if _, err := ParseClientConfig(raw); err == nil {
			t.Errorf("accepted endpoint %s", endpoint)
		} else if strings.Contains(err.Error(), "SECRET") {
			t.Fatal("credential leaked")
		}
	}
	for _, mutate := range []func(*ClientConfig){func(c *ClientConfig) { c.Token = "SECRET" }, func(c *ClientConfig) { c.Version = 2 }, func(c *ClientConfig) { c.CACertificate = "PEM on HTTP" }} {
		c := base
		mutate(&c)
		raw, _ := json.Marshal(c)
		if _, err := ParseClientConfig(raw); err == nil {
			t.Fatal("accepted invalid client config")
		}
	}
	raw, _ := json.Marshal(base)
	for _, input := range [][]byte{[]byte(`null`), bytes.Replace(raw, []byte(`"token":`), []byte(`"Token":`), 1), append([]byte(`{"token":"SECRET",`), raw[1:]...), append(bytes.Repeat([]byte(" "), MaxClientConfigSize), raw...)} {
		if _, err := ParseClientConfig(input); err == nil {
			t.Fatal("accepted malformed secret config")
		}
	}
}

func TestRequestShapesExcludeCallerAuthority(t *testing.T) {
	raw := []byte(`{"public_key":"` + configTestKey + `"}`)
	if got, err := ParseIssueRequest(raw); err != nil || got != configTestKey {
		t.Fatal(got, err)
	}
	for _, key := range []string{"issued_at_ms", "challenge", "context", "expires_at_ms", "policy", "private_key"} {
		bad := append([]byte(`{"`+key+`":1,`), raw[1:]...)
		if _, err := ParseIssueRequest(bad); err == nil {
			t.Errorf("accepted authority field %s", key)
		}
	}
	for _, input := range [][]byte{[]byte(`{"public_key":null}`), []byte(`{"public_key":"` + configTestKey + `","\u0070ublic_key":"` + configTestKey + `"}`), append(raw, []byte(`[]`)...)} {
		if _, err := ParseIssueRequest(input); err == nil {
			t.Fatal("accepted invalid issue JSON")
		}
	}
	for _, input := range []string{`{}`, `{"proof":null,"authorization":{}}`, `{"proof":{},"authorization":{},"context":{}}`, `{"proof":{},"authorization":{},"\u0070roof":{}}`} {
		if _, _, err := ParseSubmission([]byte(input)); err == nil {
			t.Fatal("accepted invalid submission")
		}
	}
}
