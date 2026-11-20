// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestMinerIndependentCodec(t *testing.T) {
	raw, e := os.ReadFile("testdata/reference/v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d6b517f13cb0f738ab5579e366416c40482bd22935b09fe53d9fa8da4151f4c5" {
		t.Fatal("independent corpus changed")
	}
	var v struct {
		Positives []struct {
			Name, Kind string
			Value      json.RawMessage
			Canonical  string `json:"canonical_hex"`
			Digest     string
		}
		Refusals []struct {
			Name, Kind string
			Raw        string `json:"raw_hex"`
		}
		Tokens []struct {
			Token, Audience string
			ConfigID        string `json:"config_id"`
			TokenHash       string `json:"token_hash"`
			ReuseHash       string `json:"reuse_hash"`
		}
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Positives) != 8 || len(v.Refusals) != 32 || len(v.Tokens) != 3 {
		t.Fatal("inventory")
	}
	for _, x := range v.Positives {
		t.Run(x.Name, func(t *testing.T) {
			b, e := hex.DecodeString(x.Canonical)
			if e != nil {
				t.Fatal(e)
			}
			if x.Kind == "config" {
				var c Config
				if boundedJSON(b, maxMarkerBytes, &c) != nil || !validConfig(c) {
					t.Fatal("positive")
				}
				id, e := configDigest(c)
				if e != nil || id != x.Digest {
					t.Fatal("digest")
				}
				marker, e := encodeMarker(c)
				if e != nil || string(marker) != string(b)+"\n" {
					t.Fatal("marker")
				}
				if _, e = decodeMarker(marker); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := decodeState(b); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
	for _, x := range v.Refusals {
		t.Run(x.Name, func(t *testing.T) {
			b, e := hex.DecodeString(x.Raw)
			if e != nil {
				t.Fatal(e)
			}
			if x.Kind == "config" {
				var c Config
				if boundedJSON(b, maxMarkerBytes, &c) == nil && validConfig(c) {
					t.Fatal("accepted invalid config")
				}
			} else if _, e := decodeState(b); e == nil {
				t.Fatal("accepted invalid state")
			}
		})
	}
	for _, x := range v.Tokens {
		got, e := tokenHashes(x.ConfigID, x.Audience, x.Token)
		if e != nil || got.TokenHash != x.TokenHash || got.ReuseHash != x.ReuseHash {
			t.Fatal("token digest")
		}
	}
}
