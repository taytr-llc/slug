// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// historyDecode extends no legacy grammar. Its only variable arrays are the
// new at-most-eight row slices; all fixed arrays must have their exact length.
// Validate closed structure before allocating/populating a public DTO.
func historyDecode(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return ErrInvalid
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(reflect.Type, int) error
	visit = func(t reflect.Type, depth int) error {
		if depth > 8 {
			return ErrInvalid
		}
		v, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		switch t.Kind() {
		case reflect.Struct:
			if v != json.Delim('{') {
				return ErrInvalid
			}
			fields := map[string]reflect.Type{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name == "" || name == "-" {
					return ErrInvalid
				}
				fields[name] = f.Type
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				ft, known := fields[name]
				if e != nil || !ok || !known || seen[name] {
					return ErrInvalid
				}
				seen[name] = true
				if e = visit(ft, depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') || len(seen) != len(fields) {
				return ErrInvalid
			}
		case reflect.Array, reflect.Slice:
			if v != json.Delim('[') {
				return ErrInvalid
			}
			count := 0
			cap := 8
			if t.Kind() == reflect.Array {
				cap = t.Len()
			}
			for d.More() {
				if count >= cap {
					return ErrInvalid
				}
				if e = visit(t.Elem(), depth+1); e != nil {
					return e
				}
				count++
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') || t.Kind() == reflect.Array && count != t.Len() {
				return ErrInvalid
			}
		case reflect.String:
			s, ok := v.(string)
			if !ok || len(s) > 4096 {
				return ErrInvalid
			}
		case reflect.Bool:
			if _, ok := v.(bool); !ok {
				return ErrInvalid
			}
		case reflect.Uint32, reflect.Uint64, reflect.Int64:
			n, ok := v.(json.Number)
			if !ok {
				return ErrInvalid
			}
			s := string(n)
			if s == "" {
				return ErrInvalid
			}
			for _, ch := range s {
				if ch < '0' || ch > '9' {
					return ErrInvalid
				}
			}
			x, e := strconv.ParseUint(s, 10, 64)
			if e != nil || x > MaxSafeInteger {
				return ErrInvalid
			}
			if t.Kind() == reflect.Uint32 && x > 1<<32-1 {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	}
	if e := visit(t.Elem(), 1); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	if e := json.Unmarshal(raw, out); e != nil {
		return ErrInvalid
	}
	return nil
}
