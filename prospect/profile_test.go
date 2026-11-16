//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCheapProfileFailsClosed(t *testing.T) {
	// Even invalid context must fail at the profile gate, before parsing or
	// hashing can produce a production-looking verification result.
	got, err := Verify(Context{}, Proof{})
	if err == nil || !strings.Contains(err.Error(), "slugtestpow") || got != (Verified{}) {
		t.Fatalf("cheap profile did not fail closed: %+v, %v", got, err)
	}
	raw, err := os.ReadFile("testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context Context `json:"context"`
		Proof   Proof   `json:"proof"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("load production fixtures: %v", err)
	}
	for _, fixture := range fixtures {
		got, err := Verify(fixture.Context, fixture.Proof)
		if err == nil || !strings.Contains(err.Error(), "slugtestpow") || got != (Verified{}) {
			t.Fatalf("cheap profile accepted production fixture: %+v, %v", got, err)
		}
	}
}
