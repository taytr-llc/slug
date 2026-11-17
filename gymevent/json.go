// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Check closed shapes before Unmarshal can allocate any typed collection.
func decode(raw []byte, limit int, out any, canonical bool) error {
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
				if !strings.HasSuffix(path, ".latest_receipt") {
					return ErrInvalid
				}
				return nil
			}
			t = t.Elem()
		} else if tok == nil {
			return ErrInvalid
		}
		switch t.Kind() {
		case reflect.Struct:
			if depth > 12 || tok != json.Delim('{') {
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
			if depth > 12 || tok != json.Delim('[') {
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
			if !ok || len(s) > 256 {
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
	case strings.HasSuffix(path, ".participants"), strings.HasSuffix(path, ".captures"):
		return 2, true
	case strings.HasSuffix(path, ".captured_participants"):
		return 2, false
	case strings.HasSuffix(path, ".members"):
		return 5, false
	case strings.HasSuffix(path, ".slots"), strings.HasSuffix(path, ".assigned_at_ms"):
		return 10, true
	case strings.HasSuffix(path, ".events"):
		return 8, false
	}
	return 0, false
}

// Bound typed live inputs before json.Marshal allocates their serialized copies.
func valueBounds(v reflect.Value, path string, depth int) error {
	if depth > 12 || !v.IsValid() {
		return ErrInvalid
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if strings.HasSuffix(path, ".latest_receipt") {
				return nil
			}
			return ErrInvalid
		}
		return valueBounds(v.Elem(), path, depth)
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
		if len(v.String()) > 256 || !utf8.ValidString(v.String()) {
			return ErrInvalid
		}
	case reflect.Bool:
	case reflect.Int, reflect.Int32, reflect.Int64:
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
