// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func entryDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymentry.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	root := map[string]bool{}
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return gymentry.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				return nil
			}
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymentry.ErrInvalid
			}
			names := map[string]reflect.StructField{}
			required := map[string]bool{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")
				if tag[0] == "-" {
					continue
				}
				names[tag[0]] = f
				if len(tag) == 1 || tag[1] != "omitempty" {
					required[tag[0]] = true
				}
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				f, known := names[name]
				if e != nil || !ok || len(name) > 64 || !known || seen[name] {
					return gymentry.ErrInvalid
				}
				seen[name] = true
				next := name
				if path != "" {
					next = path + "." + name
				}
				if e = walk(f.Type, next, depth+1); e != nil {
					return e
				}
			}
			for name := range required {
				if !seen[name] {
					return gymentry.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymentry.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymentry.ErrInvalid
			}
			count := 0
			bound := 0
			switch {
			case strings.HasSuffix(path, "prospect_ids"), strings.HasSuffix(path, "saved_ids"), strings.HasSuffix(path, "members"):
				bound = 5
			case path == "history.events":
				bound = 8
			}

			if t.Kind() == reflect.Array {
				bound = t.Len()
			}
			for d.More() {
				if count >= bound {
					return gymentry.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if t.Kind() == reflect.Array && count != t.Len() {
				return gymentry.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymentry.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 {
				return gymentry.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymentry.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymentry.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			signed := strings.HasSuffix(path, ".manifest.portrait.shoulder_jitter")
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !(signed && value >= -6) || signed && value > 6 {
				return gymentry.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymentry.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymentry.ErrInvalid
			}
		default:
			return gymentry.ErrInvalid
		}
		return nil
	}
	if err := walk(reflect.TypeOf(out).Elem(), "", 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymentry.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymentry.ErrInvalid
	}
	return root, nil
}

func MarshalEntryRequest(r EntryRequest) ([]byte, error) {
	if !entryRequestValid(r) {
		return nil, gymentry.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxEntryRequestSize {
		return nil, gymentry.ErrInvalid
	}
	return raw, nil
}
func ParseEntryRequest(raw []byte) (EntryRequest, error) {
	var r EntryRequest
	if _, e := entryDecode(raw, MaxEntryRequestSize, 10, &r); e != nil || !entryRequestValid(r) {
		return EntryRequest{}, gymentry.ErrInvalid
	}
	return r, nil
}
func ParseEntryResponse(raw []byte) (EntryResponse, error) {
	var r EntryResponse
	if _, e := entryDecode(raw, MaxEntryResponseSize, 10, &r); e != nil || !entryResponseValid(&r) {
		return EntryResponse{}, gymentry.ErrInvalid
	}
	return r, nil
}
func MarshalEntryResponse(r EntryResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and complete semantic validation on an owned copy.
	budget := MaxEntryResponseSize
	if !entryResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymentry.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymentry.ErrInvalid
	}
	owned, e := ParseEntryResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func entryResponseBudget(v reflect.Value, budget *int, depth int) bool {
	spend := func(n int) bool {
		if n < 0 || n > *budget {
			return false
		}
		*budget -= n
		return true
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return spend(4)
		}
		return entryResponseBudget(v.Elem(), budget, depth)
	case reflect.Struct:
		if depth >= 10 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Tag.Get("json") == "-" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !entryResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 10 || v.Len() > 8 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !entryResponseBudget(v.Index(i), budget, depth+1) {
				return false
			}
		}
	case reflect.String:
		s := v.String()
		return len(s) <= 4096 && utf8.ValidString(s) && len(s) <= (*budget-2)/6 && spend(2+6*len(s))
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return spend(32)
	default:
		return false
	}
	return true
}
