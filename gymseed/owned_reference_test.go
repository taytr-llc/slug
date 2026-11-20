// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"os"
	"strings"
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
	raw, e := os.ReadFile("testdata/owned-reference/owned-downstream-v1.json")
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

func TestOwnedSeedIndependentReference(t *testing.T) {
	r := downstreamRef(t)
	var p OwnedPin
	if e := json.Unmarshal(r.Contexts.Pins["seed"], &p); e != nil {
		t.Fatal(e)
	}
	positives, negatives := 0, 0
	for _, c := range r.Positives {
		if c.Family != "seed_pin" && c.Family != "event_receipt" && c.Family != "seed_declaration" && c.Family != "seed_signed_declaration" && c.Family != "seed_record" {
			continue
		}
		positives++
		t.Run(c.Family, func(t *testing.T) {
			b := downstreamBytes(t, c)
			var out []byte
			var e error
			switch c.Family {
			case "seed_pin":
				var v OwnedPin
				v, e = ParseOwnedPin(b)
				if e == nil {
					out, e = MarshalOwnedPin(v)
				}
			case "event_receipt":
				var v gymevent.OwnedSnapshotReceipt
				v, e = gymevent.ParseOwnedSnapshotReceipt(r.Contexts.EventPin, b)
				if e == nil {
					out, e = gymevent.MarshalOwnedSnapshotReceipt(r.Contexts.EventPin, v)
				}
			case "seed_declaration":
				var v OwnedDeclaration
				v, e = ParseOwnedDeclaration(p, b)
				if e == nil {
					out, e = MarshalOwnedDeclaration(p, v)
				}
			case "seed_signed_declaration":
				var v OwnedSignedDeclaration
				v, e = ParseOwnedSignedDeclaration(p, b)
				if e == nil {
					out, e = MarshalOwnedSignedDeclaration(p, v)
				}
			case "seed_record":
				var v OwnedRecord
				v, e = ParseOwnedRecord(p, b)
				if e == nil {
					out, e = MarshalOwnedRecord(p, v)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(b, out) {
				t.Fatal("canonical bytes differ")
			}
		})
	}
	for _, c := range r.Negatives {
		if c.Family != "seed_pin" && !(c.Family == "signature" && strings.HasPrefix(c.Name, "seed declaration/")) {
			continue
		}
		negatives++
		t.Run("negative/"+c.Name, func(t *testing.T) {
			b := downstreamBytes(t, c)
			var e error
			if c.Family == "seed_pin" {
				_, e = ParseOwnedPin(b)
			} else {
				_, e = ParseOwnedSignedDeclaration(p, b)
			}
			if e == nil {
				t.Fatal("independent mutation accepted")
			}
		})
	}
	for _, c := range r.Raw {
		if c.Family != "seed_pin" {
			continue
		}
		t.Run("raw/"+c.Name, func(t *testing.T) {
			b, _ := hex.DecodeString(c.RawHex)
			_, e := ParseOwnedPin(b)
			if (e == nil) != c.Valid {
				t.Fatalf("valid=%v error=%v", c.Valid, e)
			}
		})
	}
	if positives != 5 || negatives != 10 {
		t.Fatalf("coverage %d/%d", positives, negatives)
	}
	for _, c := range r.Compositions {
		if c.Family == "signature" && c.Name == "seed declaration" {
			var v OwnedSignedDeclaration
			if json.Unmarshal(c.Value, &v) != nil {
				t.Fatal("signature decode")
			}
			message, _ := hex.DecodeString(c.MessageHex)
			if !bytes.Equal(ownedDeclarationMessage(v.Declaration), message) {
				t.Fatal("seed signed display bytes or wrong domain")
			}
		}
		if c.Family == "domain" && c.Name == "seed declaration" {
			var v OwnedDeclaration
			_ = json.Unmarshal(c.Value, &v)
			got, e := OwnedDeclarationID(v)
			if e != nil || got != c.Digest {
				t.Fatalf("declaration id %v", e)
			}
		}
		if c.Family == "domain" && c.Name == "seed record" {
			var v OwnedRecord
			if e := json.Unmarshal(downstreamBytes(t, c), &v); e != nil {
				t.Fatal(e)
			}
			got, e := OwnedRecordDigest(p, v)
			if e != nil || got != c.Digest {
				t.Fatalf("record id %v", e)
			}
		}
		if c.Family == "seed_marker" {
			var v ownedMarker
			_ = json.Unmarshal(c.Value, &v)
			b, e := json.Marshal(v)
			want, _ := hex.DecodeString(c.FileHex)
			if e != nil || !bytes.Equal(append(b, '\n'), want) {
				t.Fatal("marker bytes")
			}
		}
	}
}
