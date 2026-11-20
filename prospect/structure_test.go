// Copyright (c) 2026 The SLUG developers. ISC license.
package prospect

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestProofStructureNeverEntersWorkGate(t *testing.T) {
	raw, err := os.ReadFile("testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context Context `json:"context"`
		Proof   Proof   `json:"proof"`
	}
	if json.Unmarshal(raw, &fixtures) != nil || len(fixtures) < 2 {
		t.Fatal("independent structural fixtures")
	}
	// A regression which starts hashing blocks on the real gate. On failure we
	// release it and join the call before reporting, never abandon Argon work.
	hashMu.Lock()
	done := make(chan error, 1)
	go func() {
		for _, f := range fixtures {
			if e := ValidateProofStructure(f.Context, f.Proof); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	select {
	case err = <-done:
		hashMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		hashMu.Unlock()
		<-done
		t.Fatal("pure structure helper entered work gate")
	}
	for _, f := range fixtures {
		bad := f.Proof
		bad.Coinbase = "00"
		if ValidateProofStructure(f.Context, bad) == nil {
			t.Fatal("invalid coinbase accepted")
		}
		bad = f.Proof
		bad.MerkleBranch = []string{bad.GymID}
		if ValidateProofStructure(f.Context, bad) == nil {
			t.Fatal("invalid Merkle path accepted")
		}
		c := f.Context
		c.ExpectedBits++
		if ValidateProofStructure(c, f.Proof) == nil {
			t.Fatal("header context mismatch accepted")
		}
	}
}
