// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"io"
	"unicode/utf8"
)

// Daily HTTP framing is deliberately separate from legacy TrimSpace behavior.
// Only outer SP/TAB/CR/LF is permitted; the cap includes that outer framing.
func decodeDailyStrict(raw []byte, limit int, value any) error {
	return decodeSelectedWork(raw, limit, value, "work")
}
func decodeSelectedWork(raw []byte, limit int, value any, workPath string) error {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return errConfig
	}
	raw = bytes.Trim(raw, " \t\r\n")
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return errConfig
	}
	// Bound containers before typed decoding. Template arrays retain the original
	// limits; other messages have no arrays. Field names are bounded too.
	arrays := map[string]int{workPath + ".template.block.transactions": 16666, workPath + ".template.block.transactions[].depends": 16666, workPath + ".template.block.mutable": 1024, workPath + ".template.block.capabilities": 1024}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return errConfig
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if depth > 16 {
			return errConfig
		}
		switch delim {
		case '{':
			count := 0
			seen := make(map[string]bool)
			for d.More() {
				t, err := d.Token()
				key, ok := t.(string)
				if err != nil || !ok || len(key) > 64 || seen[key] || count >= 64 {
					return errConfig
				}
				seen[key] = true
				count++
				next := key
				if path != "" {
					next = path + "." + key
				}
				if err := walk(next, depth+1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return errConfig
			}
		case '[':
			maximum, ok := arrays[path]
			if !ok {
				return errConfig
			}
			count := 0
			for d.More() {
				if count >= maximum {
					return errConfig
				}
				count++
				if err := walk(path+"[]", depth+1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return errConfig
			}
		default:
			return errConfig
		}
		return nil
	}
	if walk("", 1) != nil {
		return errConfig
	}
	if _, err := d.Token(); err != io.EOF {
		return errConfig
	}
	return decodeStrict(raw, limit, value)
}

var dailyBalanceFields = []payoutField{
	{name: "gym_id", kind: 's', limit: 64},
	{name: "confirmed", kind: 'o', fields: payoutFractionFields},
	{name: "available", kind: 'o', fields: payoutFractionFields},
	{name: "reserved_atoms", kind: 's', limit: 19},
	{name: "paid_atoms", kind: 's', limit: 19},
	{name: "reason", kind: 's', limit: 32},
}
var dailyMoneyFields = []payoutField{
	{name: "version", kind: 'n', limit: 1},
	{name: "binding_id", kind: 's', limit: 64},
	{name: "settlement_config_id", kind: 's', limit: 64},
	{name: "recipient_script", kind: 's', limit: 44},
}
var dailyBalanceResponseFields = append(append([]payoutField{}, dailyMoneyFields...),
	payoutField{name: "balance", kind: 'o', limit: poolsettle.MaxPayoutBalanceSize, fields: dailyBalanceFields},
	payoutField{name: "observed_at_ms", kind: 'n', limit: 15})
var dailyPageRequestFields = []payoutField{
	{name: "revision", kind: 's', limit: 20}, {name: "high_batch", kind: 's', limit: 5}, {name: "before_batch", kind: 's', limit: 5},
}
var dailyPageFields = []payoutField{
	{name: "version", kind: 'n', limit: 1}, {name: "binding_id", kind: 's', limit: 64}, {name: "settlement_config_id", kind: 's', limit: 64},
	{name: "revision", kind: 's', limit: 20}, {name: "high_batch", kind: 's', limit: 5}, {name: "before_batch", kind: 's', limit: 5}, {name: "next_before", kind: 's', limit: 5},
	{name: "done", kind: 'b'}, {name: "recipient_script", kind: 's', limit: 44},
	{name: "balance", kind: 'o', limit: poolsettle.MaxPayoutBalanceSize, fields: dailyBalanceFields}, {name: "observed_at_ms", kind: 'n', limit: 15},
	{name: "items", kind: 'a', limit: poolsettle.MaxDailyPayoutSlots, fields: []payoutField{
		{name: "batch_sequence", kind: 's', limit: 5}, {name: "payment", kind: 'o', limit: poolsettle.MaxPayoutItemSize, fields: payoutItemFields},
	}},
}
var dailyErrorFields = []payoutField{{name: "version", kind: 'n', limit: 1}, {name: "error", kind: 'o', fields: []payoutField{{name: "code", kind: 's', limit: 32}, {name: "outcome_unknown", kind: 'b'}}}}

func decodeDailyError(raw []byte, v *ErrorResponse) error {
	return decodeDailyValue(raw, MaxResponseSize, dailyErrorFields, v)
}
func decodeDailyBalance(raw []byte, v *DailyBalanceResponse) error {
	return decodeDailyValue(raw, MaxResponseSize, dailyBalanceResponseFields, v)
}
func decodeDailyPage(raw []byte, v *poolsettle.DailyPayoutPage) error {
	return decodeDailyValue(raw, MaxPayoutResponseSize, dailyPageFields, v)
}
func decodeDailyPageRequest(raw []byte, v *poolsettle.DailyPayoutRequest) error {
	return decodeDailyValue(raw, MaxRequestSize, dailyPageRequestFields, v)
}

type dailyScanner struct {
	raw []byte
	i   int
}

func (s *dailyScanner) take(value string) bool {
	if !bytes.HasPrefix(s.raw[s.i:], []byte(value)) {
		return false
	}
	s.i += len(value)
	return true
}

func (s *dailyScanner) object(fields []payoutField) bool {
	if !s.take("{") {
		return false
	}
	for i, field := range fields {
		if i > 0 && !s.take(",") || !s.take("\""+field.name+"\":") || !s.value(field) {
			return false
		}
	}
	return s.take("}")
}

func (s *dailyScanner) value(field payoutField) bool {
	start := s.i
	switch field.kind {
	case 's':
		if !s.take("\"") {
			return false
		}
		for s.i < len(s.raw) && s.raw[s.i] != '"' {
			b := s.raw[s.i]
			if b < 0x20 || b > 0x7e || b == '\\' || s.i-start > field.limit {
				return false
			}
			s.i++
		}
		return s.i-start-1 <= field.limit && s.take("\"")
	case 'n':
		for s.i < len(s.raw) && s.raw[s.i] >= '0' && s.raw[s.i] <= '9' {
			if s.i-start >= field.limit {
				return false
			}
			s.i++
		}
		return s.i > start && (s.i-start == 1 || s.raw[start] != '0')
	case 'f':
		return s.take("false")
	case 'b':
		return s.take("false") || s.take("true")
	case 'o':
		return s.object(field.fields) && (field.limit == 0 || s.i-start <= field.limit)
	case 'a':
		if !s.take("[") {
			return false
		}
		if s.take("]") {
			return true
		}
		for count := 0; count < field.limit; count++ {
			itemStart := s.i
			if !s.object(field.fields) || s.i-itemStart > poolsettle.MaxPayoutItemSize {
				return false
			}
			if s.take("]") {
				return true
			}
			if !s.take(",") {
				return false
			}
		}
	}
	return false
}

func decodeDailyValue(raw []byte, limit int, fields []payoutField, value any) error {
	if len(raw) == 0 || len(raw) > limit {
		return errConfig
	}
	interior := bytes.Trim(raw, " \t\r\n")
	scanner := dailyScanner{raw: interior}
	if !scanner.object(fields) || scanner.i != len(interior) || json.Unmarshal(interior, value) != nil {
		return errConfig
	}
	encoded, err := json.Marshal(value)
	if err != nil || !bytes.Equal(encoded, interior) {
		return errConfig
	}
	return nil
}
