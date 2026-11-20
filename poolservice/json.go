// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

func decodeStrict(raw []byte, limit int, value any) error {
	bad := errors.New("invalid pool service JSON")
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return bad
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return bad
	}
	depth, quoted, escape := 0, false, false
	for _, b := range raw {
		if quoted {
			if escape {
				escape = false
			} else if b == '\\' {
				escape = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		if depth < 0 || depth > 16 {
			return bad
		}
	}
	if quoted || depth != 0 {
		return bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return bad
	}
	canonical, e := json.Marshal(value)
	if e != nil || !bytes.Equal(raw, canonical) {
		return bad
	}
	return nil
}
func errorStatus(code string) int {
	switch code {
	case "invalid_request":
		return 400
	case "unauthorized":
		return 401
	case "forbidden":
		return 403
	case "not_found":
		return 404
	case "expired":
		return 409
	case "rejected":
		return 422
	case "too_large":
		return 413
	case "rate_limited":
		return 429
	case "busy", "capacity", "unavailable", "outcome_unknown":
		return 503
	}
	return 0
}
