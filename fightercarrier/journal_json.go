// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// scanJSON bounds containers, keys and strings before encoding/json can
// allocate slices or strings from untrusted state bytes.
type journalJSONScan struct {
	b       []byte
	i       int
	objects int
	tokens  int
}

func (s *journalJSONScan) space() {
	for s.i < len(s.b) && (s.b[s.i] == ' ' || s.b[s.i] == '\n' || s.b[s.i] == '\r' || s.b[s.i] == '\t') {
		s.i++
	}
}
func (s *journalJSONScan) str(limit int) ([]byte, bool) {
	start := s.i
	if s.i >= len(s.b) || s.b[s.i] != '"' {
		return nil, false
	}
	s.i++
	for s.i < len(s.b) {
		c := s.b[s.i]
		s.i++
		if s.i-start > limit+2 {
			return nil, false
		}
		if c == '"' {
			return s.b[start:s.i], true
		}
		if c < 32 || c > 127 {
			return nil, false
		}
		if c == '\\' {
			if s.i >= len(s.b) {
				return nil, false
			}
			s.i++
		}
	}
	return nil, false
}
func (s *journalJSONScan) value(depth int, key string) bool {
	s.tokens++
	if s.tokens > 262144 {
		return false
	}
	if depth > 16 {
		return false
	}
	s.space()
	start := s.i
	if start >= len(s.b) {
		return false
	}
	switch s.b[s.i] {
	case '{':
		s.objects++
		if s.objects > 8192 {
			return false
		}
		s.i++
		s.space()
		count := 0
		seen := make(map[string]bool)
		if s.i < len(s.b) && s.b[s.i] == '}' {
			s.i++
			break
		}
		for {
			s.space()
			raw, ok := s.str(128)
			if !ok {
				return false
			}
			var k string
			if json.Unmarshal(raw, &k) != nil || seen[k] {
				return false
			}
			seen[k] = true
			s.tokens++
			if s.tokens > 262144 {
				return false
			}
			count++
			if count > 64 {
				return false
			}
			s.space()
			if s.i >= len(s.b) || s.b[s.i] != ':' {
				return false
			}
			s.i++
			if !s.value(depth+1, k) {
				return false
			}
			s.space()
			if s.i >= len(s.b) {
				return false
			}
			c := s.b[s.i]
			s.i++
			if c == '}' {
				break
			}
			if c != ',' {
				return false
			}
		}
	case '[':
		limit := 0
		switch key {
		case "records":
			limit = journalMaxRecords
		case "inputs":
			limit = 16
		case "invalid_indices":
			limit = 127
		case "activation_cursors", "cursors_before", "cursors_after":
			limit = 8
		case "accounts":
			limit = 4
		default:
			return false
		}
		s.i++
		s.space()
		count := 0
		if s.i < len(s.b) && s.b[s.i] == ']' {
			s.i++
			break
		}
		for {
			count++
			if count > limit {
				return false
			}
			pos := s.i
			if !s.value(depth+1, "") {
				return false
			}
			if key == "records" && s.i-pos > 49152 {
				return false
			}
			s.space()
			if s.i >= len(s.b) {
				return false
			}
			c := s.b[s.i]
			s.i++
			if c == ']' {
				break
			}
			if c != ',' {
				return false
			}
		}
	case '"':
		if _, ok := s.str(8192); !ok {
			return false
		}
	case 'n':
		switch key {
		case "preparation", "signed_package", "sign_authorization", "commit_dispatch", "spend_dispatch", "commit_observation", "spend_observation", "new_funding", "restored_allocation", "recovery_funding":
		default:
			return false
		}
		if len(s.b)-s.i < 4 || string(s.b[s.i:s.i+4]) != "null" {
			return false
		}
		s.i += 4
	case 't':
		if len(s.b)-s.i < 4 || string(s.b[s.i:s.i+4]) != "true" {
			return false
		}
		s.i += 4
	case 'f':
		if len(s.b)-s.i < 5 || string(s.b[s.i:s.i+5]) != "false" {
			return false
		}
		s.i += 5
	default:
		if s.b[s.i] == '-' {
			s.i++
		}
		for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
			s.i++
		}
		if s.i-start > 20 {
			return false
		}
		n, e := strconv.ParseInt(string(s.b[start:s.i]), 10, 64)
		if e != nil || strconv.FormatInt(n, 10) != string(s.b[start:s.i]) {
			return false
		}
	}
	if key == "preparation" && s.i-start > 16384 {
		return false
	}
	return true
}
func decodeJournal(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrCorrupt
	}
	s := journalJSONScan{b: raw}
	if !s.value(0, "") {
		return ErrCorrupt
	}
	s.space()
	if s.i != len(raw) {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrCorrupt
	}
	b, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, b) {
		return ErrCorrupt
	}
	return nil
}
