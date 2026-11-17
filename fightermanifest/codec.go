// Copyright (c) 2026 The SLUG developers. ISC license.
package fightermanifest

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
)

// inspect bounds depth before invoking a JSON decoder. Canonical version1
// contains plain ASCII strings only; escapes, arrays, null and booleans fail.
func inspect(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxEncodedBytes {
		return ErrManifest
	}
	depth := 0
	quoted := false
	for _, c := range raw {
		if c > 127 || c == '\\' {
			return ErrManifest
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch c {
		case '{':
			depth++
			if depth > MaxDepth {
				return ErrManifest
			}
		case '}':
			depth--
			if depth < 0 {
				return ErrManifest
			}
		case '[', ']':
			return ErrManifest
		}
	}
	if quoted || depth != 0 {
		return ErrManifest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ErrManifest
	}
	if err := inspectObject(d, 1); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrManifest
	}
	return nil
}

func inspectObject(d *json.Decoder, depth int) error {
	if depth > MaxDepth {
		return ErrManifest
	}
	keys := make(map[string]bool)
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || keys[key] {
			return ErrManifest
		}
		keys[key] = true
		value, err := d.Token()
		if err != nil {
			return ErrManifest
		}
		switch v := value.(type) {
		case json.Delim:
			if v != '{' {
				return ErrManifest
			}
			if err := inspectObject(d, depth+1); err != nil {
				return err
			}
		case string:
		case json.Number:
			if _, err := strconv.ParseInt(string(v), 10, 64); err != nil {
				return ErrManifest
			}
		default:
			return ErrManifest
		}
	}
	token, err := d.Token()
	if err != nil || token != json.Delim('}') {
		return ErrManifest
	}
	return nil
}

// Parse accepts only the complete canonical record, including field order,
// minimal integer spelling and regeneration of every value from its seed.
func Parse(raw []byte) (Manifest, error) {
	if err := inspect(raw); err != nil {
		return Manifest{}, err
	}
	var m Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil {
		return Manifest{}, ErrManifest
	}
	canonical, err := Marshal(m)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Manifest{}, ErrManifest
	}
	return m, nil
}
