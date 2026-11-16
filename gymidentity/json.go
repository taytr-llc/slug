// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// parseObject rejects duplicate decoded names at every depth, including in
// otherwise ignored claims. Numbers stay textual until their exact typed use.
func parseObject(data []byte, limit int) (map[string]any, error) {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) {
		return nil, ErrAssertion
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := jsonValue(d, 0)
	if err != nil {
		return nil, ErrAssertion
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrAssertion
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, ErrAssertion
	}
	return m, nil
}

func jsonValue(d *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, ErrAssertion
	}
	t, err := d.Token()
	if err != nil {
		return nil, ErrAssertion
	}
	switch v := t.(type) {
	case json.Delim:
		switch v {
		case '{':
			m := make(map[string]any)
			for d.More() {
				t, err := d.Token()
				k, ok := t.(string)
				if err != nil || !ok || strings.ContainsRune(k, utf8.RuneError) {
					return nil, ErrAssertion
				}
				if _, exists := m[k]; exists {
					return nil, ErrAssertion
				}
				x, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				m[k] = x
			}
			if t, err = d.Token(); err != nil || t != json.Delim('}') {
				return nil, ErrAssertion
			}
			return m, nil
		case '[':
			a := make([]any, 0)
			for d.More() {
				x, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, x)
			}
			if t, err = d.Token(); err != nil || t != json.Delim(']') {
				return nil, ErrAssertion
			}
			return a, nil
		default:
			return nil, ErrAssertion
		}
	case string:
		// encoding/json replaces unpaired surrogate escapes with RuneError.
		// This restricted assertion profile rejects that ambiguous spelling.
		if strings.ContainsRune(v, utf8.RuneError) {
			return nil, ErrAssertion
		}
	}
	return t, nil
}

func stringField(m map[string]any, name string, max int) (string, error) {
	s, ok := m[name].(string)
	if !ok || len(s) == 0 || len(s) > max || !utf8.ValidString(s) {
		return "", ErrAssertion
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f || c == utf8.RuneError {
			return "", ErrAssertion
		}
	}
	return s, nil
}

func uintField(m map[string]any, name string) (uint64, error) {
	n, ok := m[name].(json.Number)
	if !ok {
		return 0, ErrAssertion
	}
	s := string(n)
	if len(s) == 0 || len(s) > 20 || (len(s) > 1 && s[0] == '0') {
		return 0, ErrAssertion
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrAssertion
		}
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, ErrAssertion
	}
	return v, nil
}

func onlyFields(m map[string]any, names ...string) bool {
	for k := range m {
		found := false
		for _, n := range names {
			if k == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func rawURL(s string, limit int) ([]byte, error) {
	if len(s) == 0 || len(s) > base64.RawURLEncoding.EncodedLen(limit) {
		return nil, ErrAssertion
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) > limit || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrAssertion
	}
	return b, nil
}

func canonicalID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
