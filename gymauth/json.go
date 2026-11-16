// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gymauth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// ParseAuthorization requires all three fields once, rejects unknown fields,
// nulls, aliases and trailing data, and validates key/signature encodings.
// Signature validity for a particular admission still requires verification.
func ParseAuthorization(data []byte) (Authorization, error) {
	var authorization Authorization
	if len(data) > MaxAuthorizationSize || !utf8.Valid(data) {
		return authorization, errors.New("authorization JSON exceeds its size limit or has invalid encoding")
	}
	fields := map[string]any{"version": &authorization.Version, "public_key": &authorization.PublicKey, "signature": &authorization.Signature}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Authorization{}, errors.New("authorization must be a JSON object")
	}
	seen := make(map[string]bool, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Authorization{}, errors.New("invalid authorization JSON key")
		}
		name, ok := token.(string)
		if !ok {
			return Authorization{}, errors.New("invalid authorization JSON key")
		}
		target, ok := fields[name]
		if !ok || seen[name] {
			return Authorization{}, errors.New("unknown or duplicate authorization field")
		}
		seen[name] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return Authorization{}, errors.New("invalid or null authorization field")
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return Authorization{}, errors.New("invalid authorization field type")
		}
	}
	if _, err := decoder.Token(); err != nil || len(seen) != len(fields) {
		return Authorization{}, errors.New("missing authorization field or incomplete object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Authorization{}, errors.New("trailing authorization data")
	}
	if authorization.Version != Version1 && authorization.Version != Version2 {
		return Authorization{}, errors.New("unsupported authorization version")
	}
	if _, _, err := publicKey(authorization.PublicKey); err != nil {
		return Authorization{}, err
	}
	if _, err := canonicalSignature(authorization.Signature); err != nil {
		return Authorization{}, err
	}
	authorization.PublicKey = strings.ToLower(authorization.PublicKey)
	authorization.Signature = strings.ToLower(authorization.Signature)
	return authorization, nil
}
