// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Check closed shapes before Unmarshal can allocate any typed collection.
func decode(raw []byte, limit int, out any, canonical bool) error {
	if m, ok := out.(*metadata); ok {
		return decodeCareerMetadata(raw, limit, m, canonical)
	}
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return ErrInvalid
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		tok, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if tok == nil {

				return nil
			}
			t = t.Elem()
		} else if tok == nil {
			return ErrInvalid
		}
		if t == reflect.TypeOf(chaincfg.SignedCheckpoint{}) {
			t = reflect.TypeOf(checkpointShape{})
		}
		switch t.Kind() {
		case reflect.Struct:
			if depth > 20 || tok != json.Delim('{') {
				return ErrInvalid
			}
			fields := map[string]reflect.Type{}
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
				fields[tag] = f.Type
			}
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				key, ok := k.(string)
				if e != nil || !ok || len(key) > 64 || seen[key] {
					return ErrInvalid
				}
				ft, ok := fields[key]
				if !ok {
					return ErrInvalid
				}
				seen[key] = true
				if e = walk(ft, path+"."+key, depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') || len(seen) != len(fields) {
				return ErrInvalid
			}
			return nil
		case reflect.Slice, reflect.Array:
			if depth > 20 || tok != json.Delim('[') {
				return ErrInvalid
			}
			limit, exact := arrayLimit(path)
			if limit == 0 {
				return ErrInvalid
			}
			n := 0
			for d.More() {
				if n >= limit {
					return ErrInvalid
				}
				if e = walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				n++
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') || (exact && n != limit) {
				return ErrInvalid
			}
			return nil
		case reflect.String:
			s, ok := tok.(string)
			if !ok || !ascii(s) || len(s) > stringLimit(path) {
				return ErrInvalid
			}
			return nil
		case reflect.Bool:
			if _, ok := tok.(bool); !ok {
				return ErrInvalid
			}
			return nil
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint32, reflect.Uint64:
			n, ok := tok.(json.Number)
			if !ok {
				return ErrInvalid
			}
			s := string(n)

			if s == "-1" && signedScan(path) && t.Kind() == reflect.Int32 {
				return nil
			}
			v, e := strconv.ParseUint(s, 10, 64)
			if e != nil || v > MaxSafeInteger || strconv.FormatUint(v, 10) != s {
				return ErrInvalid
			}
			return nil
		}
		return ErrInvalid
	}
	if e := walk(t.Elem(), "", 1); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(out) != nil {
		return ErrInvalid
	}
	if canonical {
		b, e := json.Marshal(out)
		if e != nil || !bytes.Equal(b, raw) {
			return ErrInvalid
		}
	}
	return nil
}
func owned[T any](v T, limit int, validate func(T) error) (T, error) {
	var out T
	if valueBounds(reflect.ValueOf(v), "", 1) != nil {
		return out, ErrInvalid
	}
	b, e := json.Marshal(v)
	if e != nil || decode(b, limit, &out, false) != nil {
		return out, ErrInvalid
	}
	if e = validate(out); e != nil {
		var z T
		return z, e
	}
	return out, nil
}
func equal(a, b any) bool { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return bytes.Equal(x, y) }

func arrayLimit(path string) (int, bool) {
	switch {
	case strings.HasSuffix(path, ".participants"), strings.HasSuffix(path, ".corners"):
		return 2, true
	case strings.HasSuffix(path, ".conflicts"):
		return 8, false
	case strings.HasSuffix(path, ".scan_restarts"), strings.HasSuffix(path, ".funding_restarts"), strings.HasSuffix(path, ".prior_inclusions"):
		return 16, false
	case strings.HasSuffix(path, ".merkle_branch"):
		return 20, false
	}
	return 0, false
}
func ascii(s string) bool {
	for i := range s {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// Bound typed live inputs before json.Marshal allocates their serialized copies.
func valueBounds(v reflect.Value, path string, depth int) error {
	if depth > 20 || !v.IsValid() {
		return ErrInvalid
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return valueBounds(v.Elem(), path, depth)
	}
	if v.Type() == reflect.TypeOf(chaincfg.SignedCheckpoint{}) {
		c := v.Interface().(chaincfg.SignedCheckpoint)
		if len(c.Signature) != 64 {
			return ErrInvalid
		}
		return nil
	}
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = f.Name
			}
			if e := valueBounds(v.Field(i), path+"."+key, depth+1); e != nil {
				return e
			}
		}
	case reflect.Slice, reflect.Array:
		max, exact := arrayLimit(path)
		if max == 0 || v.Len() > max || (exact && v.Len() != max) || (v.Kind() == reflect.Slice && v.IsNil()) {
			return ErrInvalid
		}
		for i := 0; i < v.Len(); i++ {
			if e := valueBounds(v.Index(i), path+"[]", depth+1); e != nil {
				return e
			}
		}
	case reflect.String:
		if len(v.String()) > stringLimit(path) || !ascii(v.String()) {
			return ErrInvalid
		}
	case reflect.Bool:
	case reflect.Int, reflect.Int32, reflect.Int64:
		if v.Int() == -1 && v.Kind() == reflect.Int32 && signedScan(path) {
			return nil
		}
		if v.Int() < 0 || uint64(v.Int()) > MaxSafeInteger {
			return ErrInvalid
		}
	case reflect.Uint32, reflect.Uint64:
		if v.Uint() > MaxSafeInteger {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type checkpointShape struct {
	Version   uint32 `json:"version"`
	Network   uint32 `json:"network"`
	Genesis   string `json:"genesis"`
	Height    int32  `json:"height"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

func stringLimit(path string) int {
	if strings.HasSuffix(path, ".raw_tx") {
		return 2 * MaxRawTxSize
	}
	return 256
}
func signedScan(path string) bool {
	return path == ".height" || path == ".complete_to_height" || strings.HasSuffix(path, ".cursor.height") || strings.HasSuffix(path, ".cursor.complete_to_height") || strings.HasSuffix(path, ".funding_cursor.height") || strings.HasSuffix(path, ".funding_cursor.complete_to_height") || strings.HasSuffix(path, ".previous.height") || strings.HasSuffix(path, ".previous.complete_to_height") || strings.HasSuffix(path, ".funding_height") || strings.HasSuffix(path, ".scan_height") || strings.HasSuffix(path, ".scan_tip_height")
}
