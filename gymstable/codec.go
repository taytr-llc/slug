// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

func hashID(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func ValidatePin(p Pin) (Pin, error) {
	if (p.Version != Version1 && p.Version != SharedPinVersion2) || !hashID(p.AccountAuthorityID) || !hashID(p.AccountID) || p.Source.Version != registry.Version5 || !hashID(p.Source.AuthorityID) || p.Source.AuthorityID == p.AccountAuthorityID || p.ConditionVersion != fightercondition.Version1 || p.ProjectorID != fightercondition.ProjectorID {
		return Pin{}, ErrInvalid
	}
	a, _ := gymaccount.FixtureIdentity("zero256")
	b, _ := gymaccount.FixtureIdentity("incrementing256")
	if p.Identity != a && p.Identity != b {
		return Pin{}, ErrInvalid
	}
	if p.Source.Network != p.Identity.Network || p.Source.Genesis != p.Identity.Genesis {
		return Pin{}, ErrInvalid
	}
	if _, e := fighterdescriptor.ResolveCandidate(p.Source.DescriptorID); e != nil {
		return Pin{}, ErrInvalid
	}
	raw, e := json.Marshal(p)
	if e != nil || len(raw) > MaxMarkerSize {
		return Pin{}, ErrInvalid
	}
	return p, nil
}
func MarshalPin(p Pin) ([]byte, error) {
	p, e := ValidatePin(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func ParsePin(raw []byte) (Pin, error) {
	var p Pin
	if decode(raw, MaxMarkerSize, 3, &p, false) != nil {
		return Pin{}, ErrInvalid
	}
	return ValidatePin(p)
}
func domainHash(domain string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func ID(p Pin) (string, error) {
	raw, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	if p.Version == SharedPinVersion2 {
		return domainHash("SLUG candidate shared stable\x00v1\x00", raw), nil
	}
	return domainHash("SLUG candidate stable\x00v1\x00", raw), nil
}
func ValidateSlotRequest(r SlotRequest) (SlotRequest, error) {
	if r.ExpectedRevision > MaxOperations || r.Slot < 1 || r.Slot > RosterSlots || (r.ProspectID != "" && !hashID(r.ProspectID)) {
		return SlotRequest{}, ErrInvalid
	}
	return r, nil
}
func MarshalSlotRequest(r SlotRequest) ([]byte, error) {
	r, e := ValidateSlotRequest(r)
	if e != nil {
		return nil, e
	}
	return json.Marshal(r)
}
func ParseSlotRequest(raw []byte) (SlotRequest, error) {
	var r SlotRequest
	if decode(raw, MaxRequestSize, 1, &r, false) != nil {
		return SlotRequest{}, ErrInvalid
	}
	return ValidateSlotRequest(r)
}

// Preflight closed shapes before populating DTO slices. This private codec is
// for pins, requests and journal rows (none contain signed portrait numbers).
func decode(raw []byte, limit, depth int, out any, canonical bool) error {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) (any, error)
	visit = func(level int) (any, error) {
		t, e := d.Token()
		if e != nil {
			return nil, ErrCorrupt
		}
		switch x := t.(type) {
		case json.Delim:
			if level > depth {
				return nil, ErrCorrupt
			}
			switch x {
			case '{':
				m := map[string]any{}
				for d.More() {
					if len(m) >= 32 {
						return nil, ErrCorrupt
					}
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok {
						return nil, ErrCorrupt
					}
					if _, ok = m[s]; ok {
						return nil, ErrCorrupt
					}
					v, e := visit(level + 1)
					if e != nil {
						return nil, e
					}
					m[s] = v
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return nil, ErrCorrupt
				}
				return m, nil
			case '[':
				a := []any{}
				for d.More() {
					if len(a) >= RosterSlots {
						return nil, ErrCorrupt
					}
					v, e := visit(level + 1)
					if e != nil {
						return nil, e
					}
					a = append(a, v)
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return nil, ErrCorrupt
				}
				return a, nil
			default:
				return nil, ErrCorrupt
			}
		case json.Number:
			s := string(x)
			if s == "" || (len(s) > 1 && s[0] == '0') {
				return nil, ErrCorrupt
			}
			for _, c := range s {
				if c < '0' || c > '9' {
					return nil, ErrCorrupt
				}
			}
			n, e := strconv.ParseUint(s, 10, 64)
			if e != nil || n > MaxSafeInteger {
				return nil, ErrCorrupt
			}
			return x, nil
		case string:
			if len(x) > 4096 {
				return nil, ErrCorrupt
			}
			return x, nil
		case bool, nil:
			return x, nil
		default:
			return nil, ErrCorrupt
		}
	}
	v, e := visit(1)
	if e != nil {
		return e
	}
	if _, e = d.Token(); e != io.EOF {
		return ErrCorrupt
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct || !shape(v, t.Elem()) {
		return ErrCorrupt
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrCorrupt
	}
	if canonical {
		encoded, e := json.Marshal(out)
		if e != nil || !bytes.Equal(raw, encoded) {
			return ErrCorrupt
		}
	}
	return nil
}
func shape(v any, t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		if v == nil {
			return true
		}
		return shape(v, t.Elem())
	}
	if v == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		count := 0
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = f.Name
			}
			count++
			x, ok := m[tag]
			if !ok || !shape(x, f.Type) {
				return false
			}
		}
		return count == len(m)
	case reflect.Array:
		a, ok := v.([]any)
		if !ok || len(a) != t.Len() {
			return false
		}
		for _, x := range a {
			if !shape(x, t.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := v.(string)
		return ok
	case reflect.Bool:
		_, ok := v.(bool)
		return ok
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		_, ok := v.(json.Number)
		return ok
	default:
		return false
	}
}
