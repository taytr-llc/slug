// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectnode

import (
	"encoding/json"
	"strings"
	"testing"
)

func testPolicy() Policy {
	return Policy{Version: 1, Network: mainnetNetwork, Genesis: mainnetGenesis,
		DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
}

func TestStrictConfiguration(t *testing.T) {
	policyJSON, err := json.Marshal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	config := RPCConfig{Version: 1, URL: "http://127.0.0.1:29332", Username: "test", Password: "secret"}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		valid []byte
		bound int
		parse func([]byte) error
	}{
		{"policy", policyJSON, MaxPolicySize, func(data []byte) error { _, err := ParsePolicy(data); return err }},
		{"RPC", configJSON, MaxRPCConfigSize, func(data []byte) error { _, err := ParseRPCConfig(data); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.parse(tc.valid); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(tc.valid, &fields); err != nil {
				t.Fatal(err)
			}
			for key := range fields {
				for _, mode := range []string{"missing", "null", "wrong_type"} {
					t.Run(key+"/"+mode, func(t *testing.T) {
						copyFields := make(map[string]json.RawMessage)
						for k, v := range fields {
							copyFields[k] = v
						}
						switch mode {
						case "missing":
							delete(copyFields, key)
						case "null":
							copyFields[key] = json.RawMessage("null")
						case "wrong_type":
							copyFields[key] = json.RawMessage("[]")
						}
						data, _ := json.Marshal(copyFields)
						if tc.parse(data) == nil {
							t.Fatal("accepted invalid field")
						}
					})
				}
			}
			for _, invalid := range [][]byte{
				append(append([]byte{}, tc.valid...), []byte(" {}")...),
				append(append([]byte{}, tc.valid[:len(tc.valid)-1]...), []byte(",\"extra\":1}")...),
				append(append([]byte{}, tc.valid[:len(tc.valid)-1]...), []byte(",\"\\u0076ersion\":1}")...),
				append(append([]byte{}, tc.valid...), []byte(strings.Repeat(" ", tc.bound))...),
				[]byte("[]"), []byte("null"), []byte("{}"), []byte("{\"version\":\"\xff\"}"),
			} {
				if tc.parse(invalid) == nil {
					t.Fatal("accepted malformed or oversized configuration")
				}
			}
		})
	}
}

func TestPolicyIsExplicitPinnedAndNormalized(t *testing.T) {
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.Version = 2 }, func(p *Policy) { p.Network = 1 },
		func(p *Policy) { p.DNAVersion = 2 }, func(p *Policy) { p.Genesis = strings.Repeat("1", 64) },
		func(p *Policy) { p.ProspectTarget = "1" }, func(p *Policy) { p.ProspectTarget = strings.Repeat("0", 64) },
	} {
		policy := testPolicy()
		mutate(&policy)
		if _, err := validatePolicy(policy); err == nil {
			t.Fatal("accepted invalid policy")
		}
	}
	policy := testPolicy()
	policy.Genesis = strings.ToUpper(policy.Genesis)
	policy.ProspectTarget = strings.ToUpper(policy.ProspectTarget)
	got, err := validatePolicy(policy)
	if err != nil || got != testPolicy() {
		t.Fatalf("normalization failed: %v", err)
	}
}

func TestEndpointRules(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:29332", "http://127.20.30.40:29332", "http://[::1]:29332", "https://node.example:29332"} {
		config := RPCConfig{Version: 1, URL: endpoint, Username: "test", Password: "secret"}
		if _, err := validateRPCConfig(config); err != nil {
			t.Errorf("rejected valid endpoint: %v", err)
		}
	}
	for _, endpoint := range []string{
		"http://localhost:29332", "http://node.example", "http://192.168.1.2:29332",
		"http://0.0.0.0:29332", "http://[::]:29332", "http://127.0.0.1.example",
		"http://2130706433", "http://0177.0.0.1", "http://127.1", "http://[::1%25lo0]",
		"https://user:secret@node.example", "https://node.example?secret", "https://node.example?", "https://node.example#secret", "https://node.example#",
		"file:///tmp/rpc", "//node.example", "https:node.example", "https:///node.example",
	} {
		config := RPCConfig{Version: 1, URL: endpoint, Username: "test", Password: "secret"}
		if _, err := validateRPCConfig(config); err == nil {
			t.Errorf("accepted unsafe endpoint %q", endpoint)
		}
	}
	for _, mutate := range []func(*RPCConfig){
		func(c *RPCConfig) { c.Version = 2 }, func(c *RPCConfig) { c.Username = "" }, func(c *RPCConfig) { c.Password = "" },
		func(c *RPCConfig) { c.Username = "user:alias" }, func(c *RPCConfig) { c.CACertificate = "unused" },
		func(c *RPCConfig) { c.Password = strings.Repeat("x", MaxRPCConfigSize) },
	} {
		config := RPCConfig{Version: 1, URL: "http://127.0.0.1", Username: "test", Password: "secret"}
		mutate(&config)
		if _, err := validateRPCConfig(config); err == nil {
			t.Fatal("accepted invalid RPC configuration")
		}
	}
}
