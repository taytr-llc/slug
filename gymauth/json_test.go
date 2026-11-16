// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gymauth

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/prospect"
)

func codecFixture(t *testing.T) (prospect.Context, prospect.Proof, Authorization) {
	t.Helper()
	data, err := os.ReadFile("testdata/authorizations_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context       prospect.Context `json:"context"`
		Proof         prospect.Proof   `json:"proof"`
		Authorization Authorization    `json:"authorization"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("read independent fixture: %v", err)
	}
	return fixtures[0].Context, fixtures[0].Proof, fixtures[0].Authorization
}

func TestAuthorizationJSONContract(t *testing.T) {
	_, _, authorization := codecFixture(t)
	encoded, err := json.Marshal(authorization)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseAuthorization(encoded)
	if err != nil || parsed != authorization {
		t.Fatalf("valid authorization changed: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for key := range fields {
		for _, mode := range []string{"missing", "null", "type"} {
			t.Run(key+"/"+mode, func(t *testing.T) {
				mutant := make(map[string]json.RawMessage)
				for k, v := range fields {
					mutant[k] = v
				}
				switch mode {
				case "missing":
					delete(mutant, key)
				case "null":
					mutant[key] = json.RawMessage("null")
				case "type":
					mutant[key] = json.RawMessage("[]")
				}
				data, _ := json.Marshal(mutant)
				if _, err := ParseAuthorization(data); err == nil {
					t.Fatal("accepted invalid required field")
				}
			})
		}
	}
	for name, data := range map[string][]byte{
		"unknown":              append(append([]byte{}, encoded[:len(encoded)-1]...), []byte(",\"action\":\"transfer\"}")...),
		"escaped duplicate":    append(append([]byte{}, encoded[:len(encoded)-1]...), []byte(",\"\\u0076ersion\":1}")...),
		"trailing":             append(append([]byte{}, encoded...), []byte(" {}")...),
		"oversized whitespace": append(append([]byte{}, encoded...), []byte(strings.Repeat(" ", MaxAuthorizationSize))...),
		"array":                []byte("[]"), "null": []byte("null"), "empty": []byte("{}"), "truncated": encoded[:len(encoded)-1],
		"invalid UTF8": []byte("{\"signature\":\"\xff\"}"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAuthorization(data); err == nil {
				t.Fatal("accepted invalid authorization JSON")
			}
		})
	}
	for name, mutate := range map[string]func(*Authorization){
		"unknown version":     func(a *Authorization) { a.Version = 3 },
		"short key":           func(a *Authorization) { a.PublicKey = a.PublicKey[2:] },
		"uncompressed key":    func(a *Authorization) { a.PublicKey = "04" + a.PublicKey[2:] + strings.Repeat("1", 64) },
		"invalid point":       func(a *Authorization) { a.PublicKey = "02" + strings.Repeat("f", 64) },
		"empty signature":     func(a *Authorization) { a.Signature = "" },
		"invalid scalar":      func(a *Authorization) { a.Signature = "3006020100020101" },
		"noncanonical suffix": func(a *Authorization) { a.Signature += "00" },
		"nonhex signature":    func(a *Authorization) { a.Signature = "zz" + a.Signature[2:] },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := authorization
			mutate(&mutant)
			data, _ := json.Marshal(mutant)
			if _, err := ParseAuthorization(data); err == nil {
				t.Fatal("accepted invalid authorization encoding")
			}
		})
	}
	upper := authorization
	upper.PublicKey = strings.ToUpper(upper.PublicKey)
	upper.Signature = strings.ToUpper(upper.Signature)
	data, _ := json.Marshal(upper)
	parsed, err = ParseAuthorization(data)
	if err != nil || parsed != authorization {
		t.Fatalf("hex case normalization: %v", err)
	}
}

func TestIdentityNeedsOnlyChainAndPublicKey(t *testing.T) {
	context, _, authorization := codecFixture(t)
	full, err := GymID(context, authorization.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	minimal := prospect.Context{Version: context.Version, Network: context.Network, Genesis: context.Genesis}
	got, err := GymID(minimal, authorization.PublicKey)
	if err != nil || got != full {
		t.Fatalf("identity depends on template or policy: %v", err)
	}
	for _, mutate := range []func(*prospect.Context){
		func(c *prospect.Context) { c.Version = 0 }, func(c *prospect.Context) { c.Network = 0 }, func(c *prospect.Context) { c.Genesis = strings.Repeat("0", 64) },
	} {
		bad := minimal
		mutate(&bad)
		if _, err := GymID(bad, authorization.PublicKey); err == nil {
			t.Fatal("accepted invalid identity context")
		}
	}
}

func TestBoundedAdmissionInputsAndMissingSigningKey(t *testing.T) {
	context, proof, authorization := codecFixture(t)
	for name, mutate := range map[string]func(*prospect.Proof){
		"version":              func(p *prospect.Proof) { p.Version = 2 },
		"header size":          func(p *prospect.Proof) { p.Header += "00" },
		"header hex":           func(p *prospect.Proof) { p.Header = "zz" + p.Header[2:] },
		"coinbase zero":        func(p *prospect.Proof) { p.Coinbase = "" },
		"coinbase oversized":   func(p *prospect.Proof) { p.Coinbase = strings.Repeat("00", 16385) },
		"coinbase odd":         func(p *prospect.Proof) { p.Coinbase += "0" },
		"coinbase invalid hex": func(p *prospect.Proof) { p.Coinbase = "zz" },
		"branch oversized":     func(p *prospect.Proof) { p.MerkleBranch = make([]string, 21) },
		"short sibling":        func(p *prospect.Proof) { p.MerkleBranch = []string{"00"} },
		"invalid sibling":      func(p *prospect.Proof) { p.MerkleBranch = []string{strings.Repeat("z", 64)} },
		"wrong gym":            func(p *prospect.Proof) { p.GymID = strings.Repeat("1", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := proof
			mutate(&bad)
			if _, err := AdmissionDigest(context, bad, authorization.PublicKey); err == nil {
				t.Fatal("accepted malformed admission input")
			}
		})
	}
	for _, key := range []*btcec.PrivateKey{nil, {}} {
		if auth, err := SignAdmission(context, proof, key); err == nil || auth != (Authorization{}) {
			t.Fatal("accepted empty private key")
		}
	}
}
