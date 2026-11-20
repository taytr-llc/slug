// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"os"
	"testing"
)

type downstreamCase struct {
	Family       string `json:"family"`
	Name         string `json:"name"`
	Scope        string `json:"scope"`
	CanonicalHex string `json:"canonical_hex"`
	SHA256       string `json:"sha256"`
	MessageHex   string `json:"message_hex"`
	PublicKey    string `json:"public_key"`
	Digest       string `json:"digest"`
	PreimageHex  string `json:"preimage_hex"`
	FileHex      string `json:"file_hex"`
	FileSHA256   string `json:"file_sha256"`
	PayloadHex   string `json:"payload_hex"`
	Value        json.RawMessage
	RawHex       string `json:"raw_hex"`
	Valid        bool
}
type downstreamReference struct {
	Contexts struct {
		Pins     map[string]json.RawMessage
		EventPin gymevent.OwnedPin `json:"event_pin"`
	}
	Positives               []downstreamCase `json:"codec_positives"`
	Compositions, Negatives []downstreamCase
	Raw                     []downstreamCase `json:"raw_cases"`
}

func downstreamRef(t *testing.T) downstreamReference {
	t.Helper()
	raw, e := os.ReadFile("../gymseed/testdata/owned-reference/owned-downstream-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "018049df53a0e381bbd6233d58e6438efc9f159bc56fffe9b4dd75fb4ed75a2c" {
		t.Fatal("independent downstream reference changed")
	}
	var r downstreamReference
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func downstreamBytes(t *testing.T, c downstreamCase) []byte {
	t.Helper()
	b, e := hex.DecodeString(c.CanonicalHex)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != c.SHA256 {
		t.Fatal("reference case digest")
	}
	return b
}
func downstreamComposition(t *testing.T, r downstreamReference, family, name string) downstreamCase {
	t.Helper()
	for _, c := range r.Compositions {
		if c.Family == family && c.Name == name {
			return c
		}
	}
	t.Fatalf("missing %s/%s", family, name)
	return downstreamCase{}
}

func TestOwnedCardIndependentReference(t *testing.T) {
	r := downstreamRef(t)
	var p OwnedPin
	if e := json.Unmarshal(r.Contexts.Pins["card"], &p); e != nil {
		t.Fatal(e)
	}
	positive, negative := 0, 0
	for _, c := range append(append([]downstreamCase{}, r.Positives...), r.Negatives...) {
		if c.Family != "card_pin" {
			continue
		}
		b := downstreamBytes(t, c)
		v, e := ParseOwnedPin(b)
		if c.Name == "candidate" {
			positive++
			if e != nil {
				t.Fatal(e)
			}
			out, e := MarshalOwnedPin(v)
			if e != nil || !bytes.Equal(out, b) {
				t.Fatal("pin bytes")
			}
		} else {
			negative++
			if e == nil {
				t.Fatalf("accepted %s", c.Name)
			}
		}
	}
	if positive != 1 || negative != 7 {
		t.Fatalf("coverage %d/%d", positive, negative)
	}
	c := downstreamComposition(t, r, "domain", "card input")
	var v struct {
		Version        uint32
		CardID         string `json:"card_id"`
		SnapshotDigest string `json:"snapshot_digest"`
		SeedEventKey   string `json:"seed_event_key"`
		TargetHeight   uint32 `json:"target_height"`
		TargetHash     string `json:"target_hash"`
		RulesID        string `json:"rules_id"`
	}
	if json.Unmarshal(c.Value, &v) != nil {
		t.Fatal("input decode")
	}
	if got := ownedInputID(v.CardID, v.SnapshotDigest, v.SeedEventKey, v.TargetHeight, v.TargetHash, v.RulesID); got != c.Digest {
		t.Fatal("owned card input domain/order")
	}
	for _, c := range r.Positives {
		if c.Family != "event_receipt" {
			continue
		}
		var receipt gymevent.OwnedSnapshotReceipt
		_ = json.Unmarshal(c.Value, &receipt)
		id, e := OwnedCardID(p, receipt.Input.Event)
		if e != nil || id != v.CardID {
			t.Fatalf("card identity %v", e)
		}
	}
}
