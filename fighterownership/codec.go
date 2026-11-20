// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
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
	id, e := ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !hexValue(v.ProspectID, 32, true) || !hexValue(v.StateID, 32, true) || !hexValue(v.BirthAdmissionID, 32, true) || !gymKey(v.OwnerPublicKey) || v.Generation > 4096 {
		return ErrInvalid
	}
	var stateID string
	if v.Generation == 0 {
		var evidence InitialStateEvidence
		if decode(v.Evidence, 4096, &evidence) != nil || evidence.Version != 1 || evidence.PinID != id || evidence.ProspectID != v.ProspectID || evidence.AdmissionID != v.BirthAdmissionID || evidence.OwnerPublicKey != v.OwnerPublicKey {
			return ErrInvalid
		}
		stateID, e = objectDigest("SLUG fighter ownership state", evidence)
	} else {
		var evidence FinalStateEvidence
		if decode(v.Evidence, 4096, &evidence) != nil || evidence.Version != 1 || evidence.PinID != id || evidence.ProspectID != v.ProspectID || evidence.OwnerPublicKey != v.OwnerPublicKey || !hexValue(evidence.PriorStateID, 32, true) || !hexValue(evidence.TransferID, 32, true) || !hexValue(evidence.CheckpointPublicationID, 32, true) || !hexValue(evidence.SixtiethDescendantHash, 32, true) || evidence.SixtiethDescendantHeight < 60 || evidence.SixtiethDescendantHeight > math.MaxInt32 {
			return ErrInvalid
		}
		stateID, e = objectDigest("SLUG fighter ownership state", evidence)
	}
	if e != nil || stateID != v.StateID {
		return ErrInvalid
	}
	return nil
}
func ParseEffectiveState(p Pin, raw []byte) (EffectiveState, error) {
	var v EffectiveState
	if e := decode(raw, 4096, &v); e != nil {
		return v, e
	}
	return v, ValidateEffectiveState(p, v)
}
func MarshalEffectiveState(p Pin, v EffectiveState) ([]byte, error) {
	if e := ValidateEffectiveState(p, v); e != nil {
		return nil, e
	}
	return json.Marshal(v)
}

// InitialState computes a public projection; it grants no live Store capability.
func InitialState(p Pin, a AdmissionStatement) (EffectiveState, error) {
	aid, e := registry.PublicAdmissionID(p, a)
	if e != nil {
		return EffectiveState{}, ErrInvalid
	}
	pid, _ := ID(p)
	ev := InitialStateEvidence{1, pid, a.Record.Verified.ProspectID, aid, a.Record.Authorization.PublicKey}
	sid, e := objectDigest("SLUG fighter ownership state", ev)
	if e != nil {
		return EffectiveState{}, e
	}
	raw, _ := json.Marshal(ev)
	v := EffectiveState{1, pid, ev.ProspectID, sid, ev.OwnerPublicKey, aid, 0, raw}
	return v, ValidateEffectiveState(p, v)
}
func ValidateOwnershipMap(p Pin, m OwnershipMap) error {
	id, e := ID(p)
	if e != nil || m.Version != 1 || m.PinID != id || m.Items == nil || len(m.Items) > 1000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	var previous uint32
	for _, x := range m.Items {
		if x.AdmissionSequence != previous+1 || !hexValue(x.ProspectID, 32, true) || !hexValue(x.StateID, 32, true) || seen[x.ProspectID] {
			return ErrInvalid
		}
		previous = x.AdmissionSequence
		seen[x.ProspectID] = true
	}
	return nil
}
func OwnershipMapID(p Pin, m OwnershipMap) (string, error) {
	if e := ValidateOwnershipMap(p, m); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership map", m)
}
func FinalizedPrefixID(p Pin, m OwnershipMap) (string, error) {
	if e := ValidateOwnershipMap(p, m); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership finalized prefix", m)
}
func ParseOwnershipMap(p Pin, raw []byte) (OwnershipMap, error) {
	var m OwnershipMap
	if e := decode(raw, 262144, &m); e != nil {
		return m, e
	}
	return m, ValidateOwnershipMap(p, m)
}
func validPrefix(n uint32, id string) bool {
	return hexValue(id, 32, false) && (n == 0) == (id == zeroID)
}
func ValidateHead(p Pin, h HeadRef) error {
	id, e := ID(p)
	if e != nil || h.Version != 1 || h.PinID != id || h.AdmissionSequence > 1000 || h.PublicationSequence > 16384 || !validPrefix(h.AdmissionSequence, h.AdmissionHead) || !validPrefix(h.PublicationSequence, h.PublicationHead) || h.ChainHeight > math.MaxInt32 || !hexValue(h.ChainHash, 32, true) || !hexValue(h.ReplayDigest, 32, true) || !hexValue(h.FinalizedPrefixID, 32, true) || h.ConflictID != "" && !hexValue(h.ConflictID, 32, true) {
		return ErrInvalid
	}
	return nil
}
func HeadID(p Pin, h HeadRef) (string, error) {
	if e := ValidateHead(p, h); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership head", h)
}
