// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
)

func TestPayoutHTTPIndependentReference(t *testing.T) {
	raw, err := os.ReadFile("../poolsettle/testdata/payout-history/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "7aaf45191dc4ac5d34d5ab930c2e6461d50f389ac22611278a897da938cc1825" {
		t.Fatal("independent payout corpus changed")
	}
	type expected struct {
		Binding string `json:"binding_id"`
		Gym     string `json:"gym_id"`
		Script  string `json:"recipient_script"`
	}
	type entry struct {
		Name     string          `json:"name"`
		Expected expected        `json:"expected"`
		History  json.RawMessage `json:"history"`
		JSON     string          `json:"response_json"`
		Body     string          `json:"http_body"`
		SHA      string          `json:"http_sha256"`
	}
	var corpus struct {
		Core struct {
			Positives []entry `json:"positives"`
			Negatives []entry `json:"negatives"`
			Raw       []struct {
				Name     string   `json:"name"`
				Valid    bool     `json:"valid"`
				Expected expected `json:"expected"`
				Hex      string   `json:"raw_hex"`
			} `json:"raw_cases"`
		} `json:"core"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Core.Positives) != 34 || len(corpus.Core.Negatives) != 43 || len(corpus.Core.Raw) != 12 {
		t.Fatal("reference inventory")
	}
	accept := func(raw []byte, want expected) (PayoutHistoryResponse, bool) {
		var got PayoutHistoryResponse
		if decodePayoutResponse(raw, &got) != nil || got.Version != Version1 || got.BindingID != want.Binding {
			return got, false
		}
		value, err := poolsettle.ValidatePayoutHistory(got.History, want.Gym)
		if err != nil {
			return got, false
		}
		got.History = value
		return got, true
	}
	for _, tc := range corpus.Core.Positives {
		t.Run(tc.Name, func(t *testing.T) {
			got, ok := accept([]byte(tc.Body), tc.Expected)
			if !ok {
				t.Fatal("independent valid HTTP body refused")
			}
			encoded, err := json.Marshal(got)
			if err != nil || string(encoded) != tc.JSON || !bytes.Equal(append(encoded, '\n'), []byte(tc.Body)) {
				t.Fatal("canonical response bytes differ from independent producer")
			}
			h := sha256.Sum256([]byte(tc.Body))
			if hex.EncodeToString(h[:]) != tc.SHA {
				t.Fatal("independent complete body checksum differs")
			}
		})
	}
	for _, tc := range corpus.Core.Negatives {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := json.Marshal(struct {
				Version uint32          `json:"version"`
				Binding string          `json:"binding_id"`
				History json.RawMessage `json:"history"`
			}{1, tc.Expected.Binding, tc.History})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := accept(body, tc.Expected); ok {
				t.Fatal("independent negative HTTP history accepted")
			}
		})
	}
	for _, tc := range corpus.Core.Raw {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := hex.DecodeString(tc.Hex)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := accept(body, tc.Expected); ok != tc.Valid {
				t.Fatal("independent lexical/source verdict differs")
			}
		})
	}
}
