// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func unitConfig(t *testing.T) Config {
	t.Helper()
	data, err := os.ReadFile("testdata/identity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Config struct {
			Version       uint32          `json:"version"`
			Issuer        string          `json:"issuer"`
			Audience      string          `json:"audience"`
			ResetAudience string          `json:"reset_audience"`
			EpochClaim    string          `json:"epoch_claim"`
			PublicJWKS    json.RawMessage `json:"public_jwks"`
		} `json:"config"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	c := corpus.Config
	return Config{Version: c.Version, Issuer: c.Issuer, Audience: c.Audience, ResetAudience: c.ResetAudience, EpochClaim: c.EpochClaim, PublicJWKS: c.PublicJWKS}
}

func mutateKeys(t *testing.T, c Config, mutate func(map[string]any)) Config {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(c.PublicJWKS, &object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	var err error
	c.PublicJWKS, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestVerifierTrustConfiguration(t *testing.T) {
	valid := unitConfig(t)
	tests := map[string]func(*Config){
		"version":            func(c *Config) { c.Version = 2 },
		"http":               func(c *Config) { c.Issuer = "http://identity.slug.invalid" },
		"userinfo":           func(c *Config) { c.Issuer = "https://secret@identity.slug.invalid" },
		"query":              func(c *Config) { c.Issuer += "?" },
		"fragment":           func(c *Config) { c.Issuer += "#" },
		"empty audience":     func(c *Config) { c.Audience = "" },
		"shared audience":    func(c *Config) { c.ResetAudience = c.Audience },
		"oversized audience": func(c *Config) { c.Audience = strings.Repeat("a", 257) },
		"reserved epoch":     func(c *Config) { c.EpochClaim = "iat" },
		"epoch case":         func(c *Config) { c.EpochClaim = "Slug_Auth_Epoch" },
		"epoch null":         func(c *Config) { c.EpochClaim = "" },
		"oversized config":   func(c *Config) { c.PublicJWKS = bytes.Repeat([]byte(" "), MaxJWKSSize+1) },
		"trailing config":    func(c *Config) { c.PublicJWKS = append(c.PublicJWKS, []byte("{}")...) },
		"duplicate keys":     func(c *Config) { c.PublicJWKS = []byte(`{"keys":[],"\u006beys":[]}`) },
		"empty keys":         func(c *Config) { c.PublicJWKS = []byte(`{"keys":[]}`) },
		"null keys":          func(c *Config) { c.PublicJWKS = []byte(`{"keys":null}`) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c := valid
			c.PublicJWKS = append([]byte(nil), valid.PublicJWKS...)
			change(&c)
			if v, err := NewVerifier(c); !errors.Is(err, ErrConfig) || v != nil {
				t.Fatalf("accepted configuration: %v", err)
			}
		})
	}
	keyChanges := map[string]func(map[string]any){
		"private material": func(k map[string]any) { k["d"] = "public-test-secret-must-not-appear" },
		"key operations":   func(k map[string]any) { k["key_ops"] = []string{"verify"} },
		"wrong algorithm":  func(k map[string]any) { k["alg"] = "HS256" },
		"encryption key":   func(k map[string]any) { k["use"] = "enc" },
		"null algorithm":   func(k map[string]any) { k["alg"] = nil },
		"EC key":           func(k map[string]any) { k["kty"] = "EC" },
		"empty kid":        func(k map[string]any) { k["kid"] = "" },
		"kid control":      func(k map[string]any) { k["kid"] = "bad\nkid" },
		"wrong exponent":   func(k map[string]any) { k["e"] = "Aw" },
		"padded modulus":   func(k map[string]any) { k["n"] = k["n"].(string) + "=" },
		"leading zero modulus": func(k map[string]any) {
			b, _ := base64.RawURLEncoding.DecodeString(k["n"].(string))
			k["n"] = base64.RawURLEncoding.EncodeToString(append([]byte{0}, b...))
		},
		"undersized modulus": func(k map[string]any) { k["n"] = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{255}, 255)) },
		"oversized modulus":  func(k map[string]any) { k["n"] = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{255}, 513)) },
		"even modulus": func(k map[string]any) {
			b, _ := base64.RawURLEncoding.DecodeString(k["n"].(string))
			b[len(b)-1] &= 254
			k["n"] = base64.RawURLEncoding.EncodeToString(b)
		},
	}
	for name, change := range keyChanges {
		t.Run(name, func(t *testing.T) {
			c := mutateKeys(t, valid, func(o map[string]any) { change(o["keys"].([]any)[0].(map[string]any)) })
			if v, err := NewVerifier(c); !errors.Is(err, ErrConfig) || v != nil {
				t.Fatalf("accepted public key: %v", err)
			}
		})
	}
	for _, count := range []int{2, MaxKeys + 1} {
		t.Run("duplicate or excess key", func(t *testing.T) {
			c := mutateKeys(t, valid, func(o map[string]any) {
				k := o["keys"].([]any)[0]
				a := make([]any, count)
				for i := range a {
					a[i] = k
				}
				o["keys"] = a
			})
			if _, err := NewVerifier(c); !errors.Is(err, ErrConfig) {
				t.Fatalf("accepted keyset: %v", err)
			}
		})
	}
}

func TestConfigDigestPinsSemanticsAndOwnsInput(t *testing.T) {
	c := unitConfig(t)
	base, err := NewVerifier(c)
	if err != nil {
		t.Fatal(err)
	}
	if !canonicalID(base.ConfigDigest()) {
		t.Fatal("missing canonical trust digest")
	}
	optional := mutateKeys(t, c, func(o map[string]any) { k := o["keys"].([]any)[0].(map[string]any); delete(k, "use"); delete(k, "alg") })
	optional.PublicJWKS = append([]byte(" \n"), optional.PublicJWKS...)
	equivalent, err := NewVerifier(optional)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent.ConfigDigest() != base.ConfigDigest() {
		t.Fatal("semantically equivalent public configuration changed digest")
	}
	two := mutateKeys(t, c, func(o map[string]any) {
		k := o["keys"].([]any)[0].(map[string]any)
		k2 := map[string]any{}
		for n, v := range k {
			k2[n] = v
		}
		k2["kid"] = "second"
		o["keys"] = []any{k, k2}
	})
	v2, err := NewVerifier(two)
	if err != nil {
		t.Fatal(err)
	}
	reversed := mutateKeys(t, two, func(o map[string]any) { a := o["keys"].([]any); a[0], a[1] = a[1], a[0] })
	v3, err := NewVerifier(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if v2.ConfigDigest() != v3.ConfigDigest() || v2.ConfigDigest() == base.ConfigDigest() {
		t.Fatal("key order or keyset digest incorrect")
	}
	for _, change := range []func(*Config){func(c *Config) { c.Issuer += "/" }, func(c *Config) { c.Audience += "2" }, func(c *Config) { c.ResetAudience += "2" }, func(c *Config) { c.EpochClaim += "_v2" }} {
		changed := c
		change(&changed)
		v, err := NewVerifier(changed)
		if err != nil {
			t.Fatal(err)
		}
		if v.ConfigDigest() == base.ConfigDigest() {
			t.Fatal("trust configuration change was not pinned")
		}
	}
	for i := range c.PublicJWKS {
		c.PublicJWKS[i] = 0
	}
	if base.keys["rfc7515-a2"].N.BitLen() != 2048 || base.ConfigDigest() != equivalent.ConfigDigest() {
		t.Fatal("verifier retained mutable caller keyset")
	}
}
