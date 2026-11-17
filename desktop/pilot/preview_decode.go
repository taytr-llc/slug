// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func previewDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymstable.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	root := map[string]bool{}
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return gymstable.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				if path != "snapshot.view" && path != "job.result" && path != "job.result.engine_result.rounds[].recovery" {
					return gymstable.ErrInvalid
				}
				return nil
			}
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
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
					return gymstable.ErrInvalid
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
					return gymstable.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymstable.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			count := 0
			bound := 0
			switch path {
			case "job.result.engine_result.rounds":
				bound = 3
			case "job.result.engine_result.rounds[].ticks":
				bound = 60
			}
			if t.Kind() == reflect.Array {
				bound = t.Len()
			}
			for d.More() {
				if count >= bound {
					return gymstable.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if (t.Kind() == reflect.Array && count != t.Len()) || (t.Kind() == reflect.Slice && count == 0) {
				return gymstable.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymstable.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 {
				return gymstable.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymstable.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			signed := strings.HasSuffix(path, ".manifest.portrait.shoulder_jitter")
			form := strings.HasSuffix(path, ".initial[].form_q")
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !(signed && value >= -6 || form && value >= -100000) || signed && value > 6 {
				return gymstable.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymstable.ErrInvalid
			}
		default:
			return gymstable.ErrInvalid
		}
		return nil
	}
	if err := walk(reflect.TypeOf(out).Elem(), "", 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymstable.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymstable.ErrInvalid
	}
	return root, nil
}
