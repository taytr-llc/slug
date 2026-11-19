// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type journalCase struct {
	Name     string `json:"name"`
	Identity struct {
		JournalID string `json:"journal_id"`
		Pin       Pin    `json:"pin"`
	} `json:"identity"`
	BindingID       string `json:"binding_id"`
	CanonicalMarker string `json:"canonical_marker"`
	Phases          []struct {
		Name               string    `json:"name"`
		Operation          Operation `json:"operation"`
		CanonicalOperation string    `json:"canonical_operation"`
		State              state     `json:"state"`
		CanonicalState     string    `json:"canonical_state"`
		StateSHA256        string    `json:"state_sha256"`
	} `json:"phases"`
}

func journalReference(t *testing.T) []journalCase {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "ee4ba4ecc89289b76e2ce805e4d8f88ced79c6b69542d71ee74a7a37b1c1bd29" {
		t.Fatal("reference changed")
	}
	var v struct {
		Cases []journalCase `json:"cases"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Cases) != 2 {
		t.Fatal("reference shape")
	}
	return v.Cases
}
func TestWithdrawalJournalIndependentReference(t *testing.T) {
	for _, c := range journalReference(t) {
		t.Run(c.Name, func(t *testing.T) {
			id := Identity{c.Identity.JournalID, c.Identity.Pin}
			b, e := BindingID(id)
			if e != nil || b != c.BindingID {
				t.Fatalf("binding %s %v", b, e)
			}
			m, _ := json.Marshal(markerFor(id, b))
			if string(m) != c.CanonicalMarker {
				t.Fatal("marker")
			}
			for _, p := range c.Phases {
				t.Run(p.Name, func(t *testing.T) {
					var s state
					if e := decode([]byte(p.CanonicalState), MaxStateBytes, &s); e != nil {
						t.Fatal(e)
					}
					if e := validateState(context.Background(), s, id, b); e != nil {
						t.Fatal(e)
					}
					raw, _ := json.Marshal(s)
					if string(raw) != p.CanonicalState {
						t.Fatal("state bytes")
					}
					h := sha256.Sum256(raw)
					if hex.EncodeToString(h[:]) != p.StateSHA256 {
						t.Fatal("state hash")
					}
					o, _ := json.Marshal(p.Operation)
					if string(o) != p.CanonicalOperation {
						t.Fatal("operation bytes")
					}
				})
			}
		})
	}
}
func TestWithdrawalTransactionIndependentReference(t *testing.T) {
	raw, e := os.ReadFile("testdata/reference/journal-reference/wallet-reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d7c7b1e4c94e02f734a162b5974749853c91828ea1a0a4c4446c379bddc841c3" {
		t.Fatal("wallet corpus changed")
	}
	type good struct {
		Name      string `json:"name"`
		Draft     Draft  `json:"draft"`
		Canonical string `json:"canonical_draft"`
		Digest    string `json:"draft_digest"`
		Expected  struct {
			TxID string `json:"txid"`
			Raw  string `json:"raw_hex"`
		} `json:"expected_signed"`
		SHA string `json:"signed_sha256"`
	}
	var v struct {
		Cases          []good `json:"cases"`
		NegativeDrafts []struct {
			Name  string `json:"name"`
			Draft Draft  `json:"draft"`
		} `json:"negative_drafts"`
		NegativeSigned []struct {
			Name string `json:"name"`
			Case string `json:"case"`
			Raw  string `json:"raw_hex"`
		} `json:"negative_signed"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Cases) != 24 || len(v.NegativeDrafts) != 13 || len(v.NegativeSigned) != 5 {
		t.Fatal("wallet shape")
	}
	drafts := map[string]Draft{}
	for _, c := range v.Cases {
		drafts[c.Name] = c.Draft
		t.Run(c.Name, func(t *testing.T) {
			d, e := ParseDraft([]byte(c.Canonical))
			if e != nil {
				t.Fatal(e)
			}
			dg, e := DraftDigest(d)
			if e != nil || dg != c.Digest {
				t.Fatalf("digest %v", e)
			}
			b, _ := json.Marshal(d)
			if !bytes.Equal(b, []byte(c.Canonical)) {
				t.Fatal("canonical")
			}
			signed, _ := hex.DecodeString(c.Expected.Raw)
			s, e := ValidateSigned(d, signed)
			if e != nil || s.TxID != c.Expected.TxID || s.RawSHA256 != c.SHA || s.Raw != c.Expected.Raw {
				t.Fatalf("signed %v", e)
			}
		})
	}
	for _, c := range v.NegativeDrafts {
		t.Run(c.Name, func(t *testing.T) {
			if _, e := DraftDigest(c.Draft); e == nil {
				t.Fatal("accepted invalid draft")
			}
		})
	}
	for _, c := range v.NegativeSigned {
		t.Run(c.Name, func(t *testing.T) {
			b, _ := hex.DecodeString(c.Raw)
			d, ok := drafts[c.Case]
			if !ok {
				t.Fatal("missing case")
			}
			if _, e := ValidateSigned(d, b); e == nil {
				t.Fatal("accepted invalid signature")
			}
		})
	}
}
