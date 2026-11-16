// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/prospect"
)

// checkJSON visits every object, including optional metadata, before typed
// decoding. Decoded keys catch escaped duplicates; a depth cap bounds recursion.
func checkJSON(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("template JSON is too deeply nested")
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
		return errors.New("invalid or null template JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok || seen[key] {
				return errors.New("duplicate or invalid template JSON key")
			}
			seen[key] = true
			if err := checkJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid template JSON delimiter")
	}
	end, err := decoder.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return errors.New("invalid template JSON terminator")
	}
	return nil
}

func required(data []byte, keys ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return nil, errors.New("template object required")
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return nil, errors.New("required template field missing")
		}
	}
	return fields, nil
}

func exactFields(fields map[string]json.RawMessage, allowed ...string) error {
	known := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		known[key] = true
	}
	for key := range fields {
		if !known[key] {
			return errors.New("unknown template field")
		}
	}
	return nil
}

// ParseTemplate parses one bounded GBT coinbasevalue result. Critical fields
// must be present; duplicates, nulls, unknown fields and trailing input fail.
// Context must have come from a separately trusted, bracketed node snapshot.
func ParseTemplate(context prospect.Context, data []byte) (Template, error) {
	if err := productionOnly(); err != nil {
		return Template{}, err
	}
	if len(data) == 0 || len(data) > MaxTemplateSize || !utf8.Valid(data) {
		return Template{}, errors.New("invalid template size or text encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := checkJSON(decoder, 0); err != nil {
		return Template{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Template{}, errors.New("trailing template JSON")
	}
	fields, err := required(data, "bits", "curtime", "height", "previousblockhash", "version", "transactions",
		"coinbasevalue", "coinbaseaux", "target", "mintime", "maxtime", "weightlimit", "sizelimit", "sigoplimit", "noncerange")
	if err != nil {
		return Template{}, err
	}
	if err := exactFields(fields, "bits", "curtime", "height", "previousblockhash", "version", "transactions",
		"coinbasevalue", "coinbaseaux", "target", "mintime", "maxtime", "weightlimit", "sizelimit", "sigoplimit", "noncerange",
		"coinbasetxn", "workid", "default_witness_commitment", "longpollid", "longpolluri", "submitold", "expires", "mutable", "capabilities", "reject-reason"); err != nil {
		return Template{}, err
	}
	aux, err := required(fields["coinbaseaux"], "flags")
	if err != nil || exactFields(aux, "flags") != nil {
		return Template{}, errors.New("invalid coinbase auxiliary fields")
	}
	var transactions []json.RawMessage
	if json.Unmarshal(fields["transactions"], &transactions) != nil || len(transactions) > maxTransactions {
		return Template{}, errors.New("invalid template transaction list")
	}
	for _, transaction := range transactions {
		entry, err := required(transaction, "data", "txid", "hash", "depends", "fee", "sigops", "weight")
		if err != nil || exactFields(entry, "data", "txid", "hash", "depends", "fee", "sigops", "weight") != nil {
			return Template{}, errors.New("invalid transaction fields")
		}
	}
	var block btcjson.GetBlockTemplateResult
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&block); err != nil {
		return Template{}, errors.New("invalid template field or type")
	}
	template := Template{Context: context, Block: block}
	if err := ValidateTemplate(template); err != nil {
		return Template{}, err
	}
	return template, nil
}
