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

func walletActivityDecode(raw []byte, limit, maxDepth int, out any) error {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return gymstable.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return gymstable.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				if path != "snapshot" && path != "job" && path != "page" && path != "detail" && path != "page.items[].block" && path != "page.items[].fee_atoms" && path != "detail.item.block" && path != "detail.item.fee_atoms" && path != "snapshot.binding" && path != "job.capture" {
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
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			count := 0
			bound := 0
			if strings.HasSuffix(path, ".items") {
				bound = 8
			}
			if path == "detail.outputs" {
				bound = 256
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
			if (t.Kind() == reflect.Array && count != t.Len()) || (t.Kind() == reflect.Slice && count == 0 && !strings.HasSuffix(path, ".items") && path != "detail.outputs") {
				return gymstable.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymstable.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			limit := 64
			switch {
			case strings.HasSuffix(path, ".authority_origin"):
				limit = 256
			case strings.HasSuffix(path, ".root_public_key"):
				limit = 128
			case strings.HasSuffix(path, ".gym_public_key"):
				limit = 66
			case strings.HasSuffix(path, ".address"):
				limit = 90
			}
			if !ok || len(s) > limit {
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
			signed := strings.HasSuffix(path, ".net_atoms")
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && (!signed || value < -2100000000000000) {
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
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return gymstable.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return gymstable.ErrInvalid
	}
	return nil
}
