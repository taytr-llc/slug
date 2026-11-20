// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/registry"
)

const zeroID = "0000000000000000000000000000000000000000000000000000000000000000"

func hexValue(v string, n int, nonzero bool) bool {
	if len(v) != 2*n || nonzero && v == strings.Repeat("0", 2*n) {
		return false
	}
	for _, c := range []byte(v) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func gymKey(v string) bool {
	if !hexValue(v, 33, true) {
		return false
	}
	raw, _ := hex.DecodeString(v)
	p, e := btcec.ParsePubKey(raw)
	return e == nil && bytes.Equal(p.SerializeCompressed(), raw)
}
func objectDigest(domain string, v any) (string, error) {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > MaxPageBytes {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0, 'v', '1', 0})
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(len(raw)))
	h.Write(b[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func decode(raw []byte, cap int, v any) error {
	if len(raw) == 0 || len(raw) > cap {
		return ErrInvalid
	}
	for _, c := range raw {
		if c > 127 {
			return ErrInvalid
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var visit func(int) error
	visit = func(depth int) error {
		tokens++
		if depth > 12 || tokens > 32768 {
			return ErrInvalid
		}
		t, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		switch x := t.(type) {
		case json.Delim:
			if x == '{' {
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok || seen[s] || len(seen) >= 64 {
						return ErrInvalid
					}
					seen[s] = true
					if e = visit(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim('}') {
					return ErrInvalid
				}
			} else if x == '[' {
				n := 0
				for d.More() {
					n++
					if n > 1000 {
						return ErrCapacity
					}
					if e = visit(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim(']') {
					return ErrInvalid
				}
			} else {
				return ErrInvalid
			}
		case json.Number:
			if strings.ContainsAny(string(x), ".eE+") {
				return ErrInvalid
			}
		}
		return nil
	}
	if e := visit(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrInvalid
	}
	want, e := json.Marshal(v)
	if e != nil || !bytes.Equal(raw, want) {
		return ErrInvalid
	}
	return nil
}
func ValidatePin(p Pin) (Pin, error) {
	if e := registry.ValidatePublicManifestPin(p); e != nil {
		return Pin{}, ErrSource
	}
	return p, nil
}
func ID(p Pin) (string, error) { return registry.PublicManifestPinID(p) }
func cloneState(v EffectiveState) EffectiveState {
	v.Evidence = append(json.RawMessage(nil), v.Evidence...)
	return v
}
func cloneAdmission(v AdmissionStatement) AdmissionStatement {
	v.Record.Proof.MerkleBranch = append([]string{}, v.Record.Proof.MerkleBranch...)
	return v
}
func cloneItem(v OwnedItem) OwnedItem {
	return OwnedItem{cloneAdmission(v.Admission), cloneState(v.State)}
}
func ValidateEffectiveState(p Pin, v EffectiveState) error {
	return (ownershipCodec{}).ValidateEffectiveState(p, v)
}
func ParseEffectiveState(p Pin, raw []byte) (EffectiveState, error) {
	return (ownershipCodec{}).ParseEffectiveState(p, raw)
}
func MarshalEffectiveState(p Pin, v EffectiveState) ([]byte, error) {
	return (ownershipCodec{}).MarshalEffectiveState(p, v)
}

// InitialState computes a public projection; it grants no live Store capability.
func InitialState(p Pin, a AdmissionStatement) (EffectiveState, error) {
	return (ownershipCodec{}).InitialState(p, a)
}
func ValidateOwnershipMap(p Pin, m OwnershipMap) error {
	return (ownershipCodec{}).ValidateOwnershipMap(p, m)
}
func OwnershipMapID(p Pin, m OwnershipMap) (string, error) {
	return (ownershipCodec{}).OwnershipMapID(p, m)
}
func FinalizedPrefixID(p Pin, m OwnershipMap) (string, error) {
	return (ownershipCodec{}).FinalizedPrefixID(p, m)
}
func ParseOwnershipMap(p Pin, raw []byte) (OwnershipMap, error) {
	return (ownershipCodec{}).ParseOwnershipMap(p, raw)
}
func validPrefix(n uint32, id string) bool {
	return hexValue(id, 32, false) && (n == 0) == (id == zeroID)
}
func ValidateHead(p Pin, h HeadRef) error {
	return (ownershipCodec{}).ValidateHead(p, h)
}
func HeadID(p Pin, h HeadRef) (string, error) {
	return (ownershipCodec{}).HeadID(p, h)
}
