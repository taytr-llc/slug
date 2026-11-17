// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

// Bound depth, array cardinality and token shapes before decoding DTO slices.
// The final byte round trip also enforces required fields, exact names/order,
// number spellings and no whitespace/escapes. No data is retained from raw.
func decodeManifestJSON(raw []byte, limit, depth int, value any) error {
	bad := fmt.Errorf("%w: invalid canonical manifest authority JSON", ErrCorrupt)
	if len(raw) == 0 || len(raw) > limit {
		return bad
	}
	for _, c := range raw {
		if c > 127 {
			return bad
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(string, int) error
	visit = func(path string, level int) error {
		t, err := d.Token()
		if err != nil || t == nil {
			return bad
		}
		if delim, ok := t.(json.Delim); ok {
			if level > depth {
				return bad
			}
			switch delim {
			case '{':
				seen := make(map[string]bool)
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return bad
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return bad
					}
					seen[name] = true
					child := name
					if path != "" {
						child = path + "." + name
					}
					if err := visit(child, level+1); err != nil {
						return err
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim('}') {
					return bad
				}
			case '[':
				if path != "proof.merkle_branch" {
					return bad
				}
				n := 0
				for d.More() {
					n++
					if n > 20 {
						return bad
					}
					child, err := d.Token()
					if err != nil {
						return bad
					}
					text, ok := child.(string)
					if !ok || !manifestCanonicalHash(text) {
						return bad
					}
				}
				end, err := d.Token()
				if err != nil || end != json.Delim(']') {
					return bad
				}
			default:
				return bad
			}
		}
		return nil
	}
	if err := visit("", 1); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return bad
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return bad
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return bad
	}
	return nil
}

func manifestCanonicalHash(value string) bool {
	n, err := normalizeHash(value)
	return err == nil && n == value
}

func manifestHex(value string, min, max int) (string, error) {
	if len(value) < min*2 || len(value) > max*2 || len(value)%2 != 0 {
		return "", fmt.Errorf("invalid manifest evidence hexadecimal length")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("invalid manifest evidence hexadecimal value")
	}
	return strings.ToLower(value), nil
}

func manifestEvidence(proof prospect.Proof, auth gymauth.Authorization) (prospect.Proof, gymauth.Authorization, error) {
	bad := fmt.Errorf("manifest admission requires bounded proof and authorization v2")
	if proof.Version != prospect.Version2 || auth.Version != gymauth.Version2 || len(proof.MerkleBranch) > 20 {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	var err error
	if proof.GymID, err = manifestHex(proof.GymID, 32, 32); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	if proof.JobID, err = manifestHex(proof.JobID, 32, 32); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	if proof.Header, err = manifestHex(proof.Header, 80, 80); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	if proof.Coinbase, err = manifestHex(proof.Coinbase, 1, 16384); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	branch := make([]string, len(proof.MerkleBranch))
	for i, sibling := range proof.MerkleBranch {
		branch[i], err = manifestHex(sibling, 32, 32)
		if err != nil {
			return prospect.Proof{}, gymauth.Authorization{}, bad
		}
	}
	proof.MerkleBranch = branch
	if auth.PublicKey, err = manifestHex(auth.PublicKey, 33, 33); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	if auth.Signature, err = manifestHex(auth.Signature, 8, 72); err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	return proof, auth, nil
}

func manifestSameProof(a, b prospect.Proof) bool {
	if a.Version != b.Version || a.GymID != b.GymID || a.Header != b.Header || a.Coinbase != b.Coinbase || a.JobID != b.JobID || len(a.MerkleBranch) != len(b.MerkleBranch) {
		return false
	}
	for i := range a.MerkleBranch {
		if a.MerkleBranch[i] != b.MerkleBranch[i] {
			return false
		}
	}
	return true
}
