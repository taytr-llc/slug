// Copyright (c) 2026 The SLUG developers. ISC license.

// Package ownedcodec is the closed, canonical encoding leaf for the explicit
// owned-fighter families. It accepts no source or ownership authority.
package ownedcodec

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid owned encoding")
var rawType = reflect.TypeOf(json.RawMessage{})

// SignedCheckpoint has an inherited custom JSON encoder. The prewalk must
// validate that exact public shape, never its internal hash/signature fields.
var checkpointType = reflect.TypeOf(chaincfg.SignedCheckpoint{})

type checkpointShape struct {
	Version   uint32 `json:"version"`
	Network   uint32 `json:"network"`
	Genesis   string `json:"genesis"`
	Height    int32  `json:"height"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

func Hash(label string, value any) (string, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("SLUG owned " + label + "\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	h.Write(n[:])
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Hex(s string, n int) bool {
	if len(s) != n*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func Fits(v any, limit int) bool { b, e := json.Marshal(v); return e == nil && len(b) <= limit }
func Equal(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}

// Decode validates the complete ordered shape before allocating DTO slices.
// RawMessage is only used by the independently validated core state-evidence
// union; its own concrete validator must run after this bounded prewalk.
func Decode(raw []byte, limit int, out any) error {
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
	var generic func(any, int) error
	scalar := func(v any) error {
		switch x := v.(type) {
		case string:
			if len(x) > 131072 {
				return ErrInvalid
			}
		case json.Number:
			s := string(x)
			if strings.ContainsAny(s, ".eE+") {
				return ErrInvalid
			}
			n, e := strconv.ParseInt(s, 10, 64)
			if e != nil || n > 9007199254740991 || n < -9007199254740991 {
				return ErrInvalid
			}
		case bool, nil:
		default:
			return ErrInvalid
		}
		return nil
	}
	generic = func(v any, depth int) error {
		if depth > 24 {
			return ErrInvalid
		}
		q, ok := v.(json.Delim)
		if !ok {
			return scalar(v)
		}
		switch q {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				s, ok := k.(string)
				if e != nil || !ok || seen[s] || len(seen) >= 64 {
					return ErrInvalid
				}
				seen[s] = true
				x, e := d.Token()
				if e != nil || generic(x, depth+1) != nil {
					return ErrInvalid
				}
			}
			v, e := d.Token()
			if e != nil || v != json.Delim('}') {
				return ErrInvalid
			}
		case '[':
			n := 0
			for d.More() {
				n++
				if n > 128 {
					return ErrInvalid
				}
				x, e := d.Token()
				if e != nil || generic(x, depth+1) != nil {
					return ErrInvalid
				}
			}
			v, e := d.Token()
			if e != nil || v != json.Delim(']') {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	}
	visit = func(t reflect.Type, depth int) error {
		if depth > 24 {
			return ErrInvalid
		}
		v, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		if t == rawType {
			return generic(v, depth)
		}
		if t.Kind() == reflect.Pointer {
			if v == nil {
				return nil
			}
			t = t.Elem()
		}
		if t == checkpointType {
			t = reflect.TypeOf(checkpointShape{})
		}
		switch t.Kind() {
		case reflect.Struct:
			if v != json.Delim('{') {
				return ErrInvalid
			}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name == "-" {
					continue
				}
				if name == "" || !d.More() {
					return ErrInvalid
				}
				k, e := d.Token()
				if e != nil || k != name {
					return ErrInvalid
				}
				if e = visit(f.Type, depth+1); e != nil {
					return e
				}
			}
			if d.More() {
				return ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		case reflect.Slice, reflect.Array:
			if v != json.Delim('[') {
				return ErrInvalid
			}
			cap := 128
			if t.Kind() == reflect.Array {
				cap = t.Len()
			}
			n := 0
			for d.More() {
				if n >= cap {
					return ErrInvalid
				}
				if e = visit(t.Elem(), depth+1); e != nil {
					return e
				}
				n++
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') || t.Kind() == reflect.Array && n != cap {
				return ErrInvalid
			}
		case reflect.String:
			s, ok := v.(string)
			if !ok || len(s) > 131072 {
				return ErrInvalid
			}
		case reflect.Bool:
			if _, ok := v.(bool); !ok {
				return ErrInvalid
			}
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
			n, ok := v.(json.Number)
			if !ok {
				return ErrInvalid
			}
			s := string(n)
			if strings.ContainsAny(s, "-.eE+") {
				return ErrInvalid
			}
			x, e := strconv.ParseUint(s, 10, t.Bits())
			if e != nil || x > 9007199254740991 {
				return ErrInvalid
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, ok := v.(json.Number)
			if !ok {
				return ErrInvalid
			}
			s := string(n)
			if strings.ContainsAny(s, ".eE+") {
				return ErrInvalid
			}
			x, e := strconv.ParseInt(s, 10, t.Bits())
			if e != nil || x > 9007199254740991 || x < -9007199254740991 {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	}
	if visit(t.Elem(), 1) != nil {
		return ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrInvalid
	}
	b, e := json.Marshal(out)
	if e != nil || !bytes.Equal(b, raw) {
		return ErrInvalid
	}
	return nil
}
