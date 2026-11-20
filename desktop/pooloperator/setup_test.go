// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

const opSetupTestReferenceSHA = "ee81ba3c98de3fd9fe1a456f5dbe68edb66fb28a2ec9cb78fc124ee8ec31be9d"

type opSetupTestPositive struct {
	Name      string `json:"name"`
	Setup     Setup  `json:"setup"`
	Canonical string `json:"canonical_hex"`
	ID        string `json:"setup_id"`
	Intent    string `json:"intent_hex"`
	Ready     string `json:"ready_hex"`
}
type opSetupTestReferences struct {
	Version   uint32                `json:"version"`
	Positives []opSetupTestPositive `json:"positives"`
	Refusals  []struct {
		Name  string `json:"name"`
		Setup Setup  `json:"setup"`
	} `json:"refusals"`
	RawRefusals []struct {
		Name string `json:"name"`
		Raw  string `json:"raw_hex"`
	} `json:"raw_refusals"`
}

func opSetupTestReference(t *testing.T) opSetupTestReferences {
	t.Helper()
	f, e := os.Open("testdata/setup-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 1<<20))
	ce := f.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 288946 || hex.EncodeToString(sum[:]) != opSetupTestReferenceSHA {
		t.Fatal("independent setup reference changed")
	}
	var out opSetupTestReferences
	if json.Unmarshal(raw, &out) != nil || out.Version != 1 || len(out.Positives) != 3 || len(out.Refusals) != 21 || len(out.RawRefusals) != 5 {
		t.Fatal("setup reference scope")
	}
	return out
}
func opSetupTestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal("fixture hex", e)
	}
	return b
}
func TestOperatorSetupIndependentCodec(t *testing.T) {
	refs := opSetupTestReference(t)
	for _, v := range refs.Positives {
		t.Run(v.Name, func(t *testing.T) {
			if e := ValidateSetup(v.Setup); e != nil {
				t.Fatal("independent positive", e)
			}
			raw, e := MarshalSetup(v.Setup)
			if e != nil || !bytes.Equal(raw, opSetupTestHex(t, v.Canonical)) {
				t.Fatal("canonical bytes", e)
			}
			id, e := SetupID(v.Setup)
			if e != nil || id != v.ID {
				t.Fatal("independent setup ID", e)
			}
			parsed, e := ParseSetup(raw)
			if e != nil || !reflect.DeepEqual(parsed, v.Setup) {
				t.Fatal("exact parse", e)
			}
			if !bytes.Equal(append(append([]byte(nil), raw...), '\n'), opSetupTestHex(t, v.Intent)) {
				t.Fatal("intent bytes")
			}
			ready, e := readyBytes(v.Setup)
			if e != nil || !bytes.Equal(ready, opSetupTestHex(t, v.Ready)) {
				t.Fatal("independent READY bytes", e)
			}
			// Decode owns each nested slice. Mutating a disclosure cannot retarget the
			// original selection or modify the immutable input bytes.
			parsed.Settlement.Destinations[0].Script = "mutated disclosed copy"
			again, e := ParseSetup(raw)
			if e != nil || !reflect.DeepEqual(again, v.Setup) {
				t.Fatal("owned parse", e)
			}
		})
	}
}
func TestOperatorSetupIndependentRefusals(t *testing.T) {
	refs := opSetupTestReference(t)
	for _, v := range refs.Refusals {
		t.Run(v.Name, func(t *testing.T) {
			if e := ValidateSetup(v.Setup); e == nil {
				t.Fatal("independent cross-field refusal accepted")
			}
			if _, e := MarshalSetup(v.Setup); e == nil {
				t.Fatal("invalid setup encoded")
			}
			if _, e := SetupID(v.Setup); e == nil {
				t.Fatal("invalid setup received ID")
			}
		})
	}
	for _, v := range refs.RawRefusals {
		t.Run("raw-"+v.Name, func(t *testing.T) {
			if _, e := ParseSetup(opSetupTestHex(t, v.Raw)); e == nil {
				t.Fatal("noncanonical raw accepted")
			}
		})
	}
	for _, raw := range [][]byte{nil, {}, bytes.Repeat([]byte{' '}, MaxSetupBytes+1)} {
		if _, e := ParseSetup(raw); e == nil {
			t.Fatal("length boundary accepted")
		}
	}
}
