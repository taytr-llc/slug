// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterevidence

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

func validString(s string, n *shape) bool {
	if len(s) < n.min || len(s) > n.max || n.hex && len(s)%2 != 0 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		// All inherited string vocabularies are ASCII with no escaped characters.
		if c < 32 || c > 126 || c == '\\' || c == '"' || c == '<' || c == '>' || c == '&' {
			return false
		}
		if n.hex && !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type scanner struct {
	raw []byte
	at  int
	end int
}

func (s *scanner) take(v string) bool {
	if len(v) > s.end-s.at || !bytes.HasPrefix(s.raw[s.at:s.end], []byte(v)) {
		return false
	}
	s.at += len(v)
	return true
}
func (s *scanner) value(n *shape, depth int) bool {
	start := s.at
	if depth > 10 || s.at >= s.end {
		return false
	}
	previousEnd := s.end
	if n.maxBytes > 0 {
		s.end = min(s.end, start+n.maxBytes)
	}
	defer func() { s.end = previousEnd }()
	switch n.kind {
	case 'o':
		if !s.take("{") {
			return false
		}
		for i, f := range n.fields {
			if i != 0 && !s.take(",") {
				return false
			}
			if !s.take("\""+f.name+"\":") || !s.value(f.node, depth+1) {
				return false
			}
		}
		if !s.take("}") {
			return false
		}
	case 'a':
		if !s.take("[") {
			return false
		}
		count := 0
		for s.at < s.end && s.raw[s.at] != ']' {
			if count >= n.max || count != 0 && !s.take(",") || !s.value(n.elem, depth+1) {
				return false
			}
			count++
		}
		if count < n.min || !s.take("]") {
			return false
		}
	case 's':
		if !s.take("\"") {
			return false
		}
		begin := s.at
		for s.at < s.end && s.raw[s.at] != '"' {
			if s.at-begin >= n.max {
				return false
			}
			s.at++
		}
		if !validString(string(s.raw[begin:s.at]), n) || !s.take("\"") {
			return false
		}
	case 'n':
		for s.at < s.end && s.raw[s.at] >= '0' && s.raw[s.at] <= '9' {
			if s.at-start >= 20 {
				return false
			}
			s.at++
		}
		if s.at == start || s.at-start > 1 && s.raw[start] == '0' {
			return false
		}
		v, err := strconv.ParseUint(string(s.raw[start:s.at]), 10, 64)
		if err != nil || v > n.maxNumber {
			return false
		}
	case 'b':
		if !s.take("true") && !s.take("false") {
			return false
		}
	default:
		return false
	}
	return n.maxBytes == 0 || s.at-start <= n.maxBytes
}

// typedSize rejects excessive caller-owned slices/strings before Marshal can
// allocate proportional output. It also binds the exact DTO layout to our
// explicit schema; the parser never discovers new fields through reflection.
func typedSize(v reflect.Value, n *shape, depth int) (int, bool) {
	if depth > 10 {
		return 0, false
	}
	size := 0
	switch n.kind {
	case 'o':
		if v.Kind() != reflect.Struct || v.NumField() != len(n.fields) {
			return 0, false
		}
		size = 2
		for i, f := range n.fields {
			if strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0] != f.name {
				return 0, false
			}
			x, ok := typedSize(v.Field(i), f.node, depth+1)
			if !ok {
				return 0, false
			}
			size += len(f.name) + 3 + x
			if i != 0 {
				size++
			}
		}
	case 'a':
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array || v.Kind() == reflect.Slice && v.IsNil() || v.Len() < n.min || v.Len() > n.max {
			return 0, false
		}
		size = 2
		for i := range v.Len() {
			x, ok := typedSize(v.Index(i), n.elem, depth+1)
			if !ok {
				return 0, false
			}
			size += x
			if i != 0 {
				size++
			}
		}
	case 's':
		if v.Kind() != reflect.String || !validString(v.String(), n) {
			return 0, false
		}
		size = len(v.String()) + 2
	case 'n':
		var value uint64
		switch v.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			value = v.Uint()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if v.Int() < 0 {
				return 0, false
			}
			value = uint64(v.Int())
		default:
			return 0, false
		}
		if value > n.maxNumber {
			return 0, false
		}
		size = len(strconv.FormatUint(value, 10))
	case 'b':
		if v.Kind() != reflect.Bool {
			return 0, false
		}
		size = 5
		if v.Bool() {
			size = 4
		}
	default:
		return 0, false
	}
	return size, n.maxBytes == 0 || size <= n.maxBytes
}

// Parse checks closed, bounded, canonical bytes only. It makes no work claim.
func Parse(raw []byte) (Archive, error) {
	if len(raw) == 0 || len(raw) > MaxSize {
		return Archive{}, ErrInvalid
	}
	s := scanner{raw: raw, end: len(raw)}
	if !s.value(archiveShape, 1) || s.at != len(raw) {
		return Archive{}, ErrInvalid
	}
	var a Archive
	if json.Unmarshal(raw, &a) != nil || a.Format != Format || a.Version != Version {
		return Archive{}, ErrInvalid
	}
	encoded, err := json.Marshal(a)
	if err != nil || !bytes.Equal(raw, encoded) {
		return Archive{}, ErrInvalid
	}
	return a, nil
}

func checkedBytes(a Archive) ([]byte, error) {
	if _, ok := typedSize(reflect.ValueOf(a), archiveShape, 1); !ok {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(a)
	if err != nil || len(raw) > MaxSize {
		return nil, ErrInvalid
	}
	return raw, nil
}
