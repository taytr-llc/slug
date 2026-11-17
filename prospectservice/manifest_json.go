// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Bound all container allocations before typed decoding, including unexpected
// fields. Exact schemas are checked separately with decoded names. The paths
// permit only the specified bounded arrays, never arbitrary nested containers.
func manifestJSONShape(raw []byte, limit, maxDepth int, arrays map[string]int) error {
	bad := errors.New("invalid manifest transport JSON")
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(string, int) error
	value = func(path string, depth int) error {
		token, err := d.Token()
		if err != nil || token == nil {
			return bad
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return nil
		}
		if depth > maxDepth {
			return bad
		}
		switch delimiter {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				token, err := d.Token()
				name, ok := token.(string)
				if err != nil || !ok || seen[name] {
					return bad
				}
				seen[name] = true
				next := name
				if path != "" {
					next = path + "." + name
				}
				if err := value(next, depth+1); err != nil {
					return err
				}
			}
			token, err = d.Token()
			if err != nil || token != json.Delim('}') {
				return bad
			}
		case '[':
			max, ok := arrays[path]
			if !ok {
				return bad
			}
			count := 0
			for d.More() {
				count++
				if count > max {
					return bad
				}
				if err := value(path+"[]", depth+1); err != nil {
					return err
				}
			}
			token, err = d.Token()
			if err != nil || token != json.Delim(']') {
				return bad
			}
		default:
			return bad
		}
		return nil
	}
	if err := value("", 1); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return bad
	}
	return nil
}
