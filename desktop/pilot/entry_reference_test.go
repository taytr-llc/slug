// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strconv"
	"testing"
)

func TestEntryIndependentTransport(t *testing.T) {
	if wire.TestPoW {
		t.Skip("independent public transport pins normal PoW")
	}
	raw, e := os.ReadFile("../../gymentry/testdata/reference/entry_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d8f226964429e8068bbe8300c73b6061f9a6dfd4e737a06ddde40c5eaa992bee" {
		t.Fatal("reference hash")
	}
	var doc struct {
		Cases []struct {
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Raw       string `json:"raw_hex"`
			Accepted  bool   `json:"accepted"`
			Canonical string `json:"canonical"`
		} `json:"codec_cases"`
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	count := 0
	for i, c := range doc.Cases {
		if c.Kind != "EntryRequest" && c.Kind != "EntryResponse" {
			continue
		}
		count++
		t.Run(c.Name+"-"+strconv.Itoa(i), func(t *testing.T) {
			b, e := hex.DecodeString(c.Raw)
			if e != nil {
				t.Fatal(e)
			}
			var actual []byte
			if c.Kind == "EntryRequest" {
				r, err := ParseEntryRequest(b)
				e = err
				if e == nil {
					actual, e = MarshalEntryRequest(r)
				}
			} else {
				r, err := ParseEntryResponse(b)
				e = err
				if e == nil {
					actual, e = MarshalEntryResponse(r)
				}
			}
			if (e == nil) != c.Accepted {
				t.Fatalf("accepted=%v want%v: %v", e == nil, c.Accepted, e)
			}
			if c.Accepted && string(actual) != c.Canonical {
				t.Fatalf("canonical mismatch: actual SHA256 %x, expected SHA256 %x", sha256.Sum256(actual), sha256.Sum256([]byte(c.Canonical)))
			}
		})
	}
	if count < 300 {
		t.Fatal("missing transport cases", count)
	}
}
