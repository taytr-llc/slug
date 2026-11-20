// Copyright (c) 2026 The SLUG developers. ISC license.
package ownedcodec

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"os"
	"testing"
)

func TestOwnedCodecInheritedCheckpointShape(t *testing.T) {
	b, e := os.ReadFile("../../gymseed/testdata/owned-reference/owned-downstream-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != "018049df53a0e381bbd6233d58e6438efc9f159bc56fffe9b4dd75fb4ed75a2c" {
		t.Fatal("reference changed")
	}
	var ref struct {
		Raw []struct {
			Family, Name string
			RawHex       string `json:"raw_hex"`
			Valid        bool
		} `json:"raw_cases"`
		Compositions []struct {
			Family       string
			CanonicalHex string `json:"canonical_hex"`
			MessageHex   string `json:"message_hex"`
			PublicKey    string `json:"public_key"`
		}
	}
	if e = json.Unmarshal(b, &ref); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, c := range ref.Raw {
		if c.Family != "checkpoint" {
			continue
		}
		count++
		t.Run(c.Name, func(t *testing.T) {
			raw, _ := hex.DecodeString(c.RawHex)
			var cp chaincfg.SignedCheckpoint
			e := Decode(raw, 1024, &cp)
			if (e == nil) != c.Valid {
				t.Fatalf("valid=%v error=%v", c.Valid, e)
			}
			if e == nil {
				out, e := json.Marshal(cp)
				if e != nil || !bytes.Equal(out, raw) {
					t.Fatal("custom checkpoint JSON mismatch")
				}
			}
		})
	}
	if count != 6 {
		t.Fatal("raw checkpoint coverage")
	}
	for _, c := range ref.Compositions {
		if c.Family != "checkpoint" {
			continue
		}
		raw, _ := hex.DecodeString(c.CanonicalHex)
		var cp struct{ Signature string }
		if json.Unmarshal(raw, &cp) != nil {
			t.Fatal("signature decode")
		}
		signature, _ := hex.DecodeString(cp.Signature)
		message, _ := hex.DecodeString(c.MessageHex)
		public, _ := hex.DecodeString(c.PublicKey)
		if !ed25519.Verify(public, message, signature) {
			t.Fatal("inherited fixture signing bytes")
		}
	}
}
