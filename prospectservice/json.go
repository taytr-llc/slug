// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

// parseObject checks exact decoded keys, including escaped duplicate aliases.
// Errors intentionally omit input text: configs and transport bodies may hold
// credentials or sensitive caller data.
func parseObject(data []byte, limit int, required ...string) (map[string]json.RawMessage, error) {
	invalid := errors.New("invalid JSON object")
	if len(data) > limit || !utf8.Valid(data) {
		return nil, invalid
	}
	allowed := make(map[string]bool, len(required))
	for _, key := range required {
		allowed[key] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	fields := make(map[string]json.RawMessage, len(required))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := token.(string)
		if !ok || !allowed[key] {
			return nil, invalid
		}
		if _, exists := fields[key]; exists {
			return nil, invalid
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, invalid
		}
		fields[key] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, invalid
	}
	if _, err := decoder.Token(); err != io.EOF || len(fields) != len(allowed) {
		return nil, invalid
	}
	return fields, nil
}

func decodeField(fields map[string]json.RawMessage, key string, dst any) error {
	raw, exists := fields[key]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, dst) != nil {
		return errors.New("invalid JSON field")
	}
	return nil
}

func encodeJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw)+1 > MaxResponseSize {
		return nil, errors.New("invalid or oversized response")
	}
	return append(raw, '\n'), nil
}

// ParseIssueRequest accepts only the public gym key. Policy, time, challenge,
// lifetime and chain context are supplied exclusively by the authority.
func ParseIssueRequest(data []byte) (string, error) {
	fields, err := parseObject(data, MaxRequestSize, "public_key")
	var key string
	if err != nil || decodeField(fields, "public_key", &key) != nil {
		return "", errors.New("invalid job request")
	}
	return normalizePublicKey(key)
}

// ParseSubmission strictly decodes public evidence; authorization and work
// validation still happen against the authority's stored job.
func ParseSubmission(data []byte) (prospect.Proof, gymauth.Authorization, error) {
	invalid := errors.New("invalid admission request")
	fields, err := parseObject(data, MaxRequestSize, "proof", "authorization")
	if err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, invalid
	}
	proof, err := prospect.ParseProof(fields["proof"])
	if err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, invalid
	}
	auth, err := gymauth.ParseAuthorization(fields["authorization"])
	if err != nil {
		return prospect.Proof{}, gymauth.Authorization{}, invalid
	}
	return proof, auth, nil
}
