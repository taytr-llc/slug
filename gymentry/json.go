// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

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
				return nil
			}
			t = t.Elem()
		} else if tok == nil {
			return ErrInvalid
		}
		switch t.Kind() {
		case reflect.Struct:
			if depth > 10 || tok != json.Delim('{') {
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
			if depth > 10 || tok != json.Delim('[') {
				return ErrInvalid
			}
			limit := 0
			switch {
			case strings.HasSuffix(path, ".prospect_ids"), strings.HasSuffix(path, ".saved_ids"), strings.HasSuffix(path, ".members"):
				limit = MaxMembers
			case strings.HasSuffix(path, ".events"):
				limit = MaxPageSize
			default:
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
			if e != nil || end != json.Delim(']') {
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
			if strings.HasSuffix(path, ".item.manifest.portrait.shoulder_jitter") {
				v, e := strconv.ParseInt(s, 10, 8)
				if e != nil || v < -6 || v > 6 || strconv.FormatInt(v, 10) != s {
					return ErrInvalid
				}
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
