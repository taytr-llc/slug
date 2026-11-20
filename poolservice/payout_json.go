// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"encoding/json"

	"github.com/taytr-llc/slug/poolsettle"
)

// This closed preflight walks bytes before decoding any slices or strings.
// The declared tree permits only the items array and at most four containers.
type payoutField struct {
	name   string
	kind   byte
	limit  int
	fields []payoutField
}

var payoutFractionFields = []payoutField{
	{name: "numerator", kind: 's', limit: 4096},
	{name: "denominator", kind: 's', limit: 4096},
}
var payoutBalanceFields = []payoutField{
	{name: "gym_id", kind: 's', limit: 64},
	{name: "confirmed", kind: 'o', fields: payoutFractionFields},
	{name: "available", kind: 'o', fields: payoutFractionFields},
	{name: "reserved_atoms", kind: 'n', limit: 16},
	{name: "paid_atoms", kind: 'n', limit: 16},
	{name: "reason", kind: 's', limit: 32},
}
var payoutItemFields = []payoutField{
	{name: "day", kind: 's', limit: 10},
	{name: "created_at_ms", kind: 'n', limit: 15},
	{name: "txid", kind: 's', limit: 64},
	{name: "output_index", kind: 'n', limit: 3},
	{name: "atoms", kind: 'n', limit: 16},
	{name: "recorded_state", kind: 's', limit: 9},
}
var payoutHistoryFields = []payoutField{
	{name: "version", kind: 'n', limit: 1},
	{name: "recipient_script", kind: 's', limit: 44},
	{name: "balance", kind: 'o', limit: poolsettle.MaxPayoutBalanceSize, fields: payoutBalanceFields},
	{name: "settlement_observed_at_ms", kind: 'n', limit: 15},
	{name: "items", kind: 'a', limit: poolsettle.MaxPayoutItems, fields: payoutItemFields},
}
var payoutResponseFields = []payoutField{
	{name: "version", kind: 'n', limit: 1},
	{name: "binding_id", kind: 's', limit: 64},
	{name: "history", kind: 'o', limit: poolsettle.MaxPayoutHistorySize, fields: payoutHistoryFields},
}
var payoutErrorFields = []payoutField{
	{name: "version", kind: 'n', limit: 1},
	{name: "error", kind: 'o', fields: []payoutField{
		{name: "code", kind: 's', limit: 32},
		{name: "outcome_unknown", kind: 'f'},
	}},
}

type payoutScanner struct {
	raw []byte
	i   int
}

func (s *payoutScanner) take(value string) bool {
	if !bytes.HasPrefix(s.raw[s.i:], []byte(value)) {
		return false
	}
	s.i += len(value)
	return true
}

func (s *payoutScanner) object(fields []payoutField) bool {
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

func (s *payoutScanner) value(field payoutField) bool {
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

func decodePayoutValue(raw []byte, limit int, fields []payoutField, value any) error {
	if len(raw) == 0 || len(raw) > limit {
		return errConfig
	}
	interior := bytes.Trim(raw, " \t\r\n")
	scanner := payoutScanner{raw: interior}
	if !scanner.object(fields) || scanner.i != len(interior) || json.Unmarshal(interior, value) != nil {
		return errConfig
	}
	encoded, err := json.Marshal(value)
	if err != nil || !bytes.Equal(encoded, interior) {
		return errConfig
	}
	return nil
}

func decodePayoutResponse(raw []byte, result *PayoutHistoryResponse) error {
	return decodePayoutValue(raw, MaxPayoutResponseSize, payoutResponseFields, result)
}

func decodePayoutError(raw []byte, result *ErrorResponse) error {
	return decodePayoutValue(raw, MaxResponseSize, payoutErrorFields, result)
}

func payoutRequest(raw []byte) bool {
	return len(raw) <= MaxRequestSize && bytes.Equal(bytes.Trim(raw, " \t\r\n"), []byte("{}"))
}
