// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

type ownedReferenceCase struct {
	Family       string          `json:"family"`
	Name         string          `json:"name"`
	Context      string          `json:"context"`
	Value        json.RawMessage `json:"value"`
	CanonicalHex string          `json:"canonical_hex"`
	SHA256       string          `json:"sha256"`
	RawHex       string          `json:"raw_hex"`
	Valid        bool            `json:"valid"`
}
type ownedReferenceFile struct {
	Contexts map[string]struct {
		Pin OwnedPin `json:"pin"`
	} `json:"contexts"`
	Positives, Negatives []ownedReferenceCase
	Raw                  []ownedReferenceCase `json:"raw_cases"`
}

func loadOwnedReference(t *testing.T) ownedReferenceFile {
	t.Helper()
	b, e := os.ReadFile("testdata/owned-reference/owned-consumers-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != "44f3d12b96a94f6d2d071270b6c90296cae1db50249ff4725ec5e28adc9bfa15" {
		t.Fatal("independent corpus changed")
	}
	var r ownedReferenceFile
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func ownedReferenceRoundTrip(f string, p OwnedPin, b []byte) ([]byte, error) {
	switch f {
	case "pin":
		v, e := ParseOwnedPin(b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedPin(v)
	case "status":
		v, e := ParseOwnedStatus(b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedStatus(v)
	case "item":
		var clock struct {
			Condition struct {
				AsOfMS int64 `json:"as_of_ms"`
			} `json:"condition"`
		}
		if e := json.Unmarshal(b, &clock); e != nil {
			return nil, e
		}
		v, e := ParseOwnedItem(p, b, clock.Condition.AsOfMS)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedItem(p, v, clock.Condition.AsOfMS)
	case "page":
		v, e := ParseOwnedPage(b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedPage(v)
	case "operation":
		v, e := ParseOwnedOperation(p, b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedOperation(p, v)
	case "history":
		v, e := ParseOwnedCareerHistoryCapture(b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedCareerHistoryCapture(v)
	case "history_page":
		v, e := ParseOwnedCareerHistoryPage(b)
		if e != nil {
			return nil, e
		}
		return MarshalOwnedCareerHistoryPage(v)
	}
	return nil, fmt.Errorf("other component family %s", f)
}
func ownedStableFamily(s string) bool { return s != "entry_pin" && s != "snapshot" }

// These are conditional independently modeled public facts. This test verifies
// codecs, immutable birth/history composition and arithmetic, not source work,
// replay completeness, a live owner capability or actual chain finality.
func TestOwnedStableIndependentReference(t *testing.T) {
	r := loadOwnedReference(t)
	positive, negative := 0, 0
	for _, v := range r.Positives {
		if !ownedStableFamily(v.Family) {
			continue
		}
		positive++
		t.Run("positive/"+v.Family+"/"+v.Name, func(t *testing.T) {
			b, e := hex.DecodeString(v.CanonicalHex)
			if e != nil {
				t.Fatal(e)
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != v.SHA256 {
				t.Fatal("reference hash")
			}
			out, e := ownedReferenceRoundTrip(v.Family, r.Contexts[v.Context].Pin, b)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(out, b) {
				t.Fatal("canonical bytes differ")
			}
		})
	}
	for _, v := range r.Negatives {
		if !ownedStableFamily(v.Family) {
			continue
		}
		negative++
		t.Run("negative/"+v.Family+"/"+v.Name, func(t *testing.T) {
			var compact bytes.Buffer
			if e := json.Compact(&compact, v.Value); e != nil {
				t.Fatal(e)
			}
			if _, e := ownedReferenceRoundTrip(v.Family, r.Contexts[v.Context].Pin, compact.Bytes()); e == nil {
				t.Fatal("invalid independent mutation accepted")
			}
		})
	}
	for _, v := range r.Raw {
		t.Run("raw/"+v.Name, func(t *testing.T) {
			b, e := hex.DecodeString(v.RawHex)
			if e != nil {
				t.Fatal(e)
			}
			_, e = ownedReferenceRoundTrip(v.Family, r.Contexts[v.Context].Pin, b)
			if (e == nil) != v.Valid {
				t.Fatalf("valid=%v error=%v", v.Valid, e)
			}
		})
	}
	if positive != 81 || negative != 126 || len(r.Raw) != 7 {
		t.Fatalf("unexpected scope %d/%d/%d", positive, negative, len(r.Raw))
	}
}
