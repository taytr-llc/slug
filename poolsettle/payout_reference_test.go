// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPayoutHistoryIndependentReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/payout-history/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "7aaf45191dc4ac5d34d5ab930c2e6461d50f389ac22611278a897da938cc1825" {
		t.Fatal("independently frozen corpus changed")
	}
	type entry struct {
		Name     string `json:"name"`
		Expected struct {
			Gym string `json:"gym_id"`
		} `json:"expected"`
		History json.RawMessage `json:"history"`
	}
	var corpus struct {
		Core struct {
			Positives []entry `json:"positives"`
			Negatives []entry `json:"negatives"`
		} `json:"core"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Core.Positives) != 34 || len(corpus.Core.Negatives) != 43 {
		t.Fatal("reference inventory differs")
	}
	for _, tc := range corpus.Core.Positives {
		t.Run(tc.Name, func(t *testing.T) {
			var value PayoutHistory
			if json.Unmarshal(tc.History, &value) != nil {
				t.Fatal("reference input type")
			}
			got, err := ValidatePayoutHistory(value, tc.Expected.Gym)
			if err != nil || !reflect.DeepEqual(got, value) {
				t.Fatal("independent valid history", err)
			}
			if len(got.Items) > 0 {
				before := value.Items[0]
				got.Items[0].Atoms++
				if value.Items[0] != before {
					t.Fatal("reference input aliased")
				}
			}
		})
	}
	for _, tc := range corpus.Core.Negatives {
		t.Run(tc.Name, func(t *testing.T) {
			// A typed shape checker cannot receive unknown JSON fields. The
			// service reference test independently exercises its real preflight.
			var value PayoutHistory
			d := json.NewDecoder(bytes.NewReader(tc.History))
			d.DisallowUnknownFields()
			if d.Decode(&value) != nil {
				return
			}
			if got, err := ValidatePayoutHistory(value, tc.Expected.Gym); err == nil || !reflect.DeepEqual(got, PayoutHistory{}) {
				t.Fatal("independent invalid history accepted")
			}
		})
	}
}
