// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectjob

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/taytr-llc/slug/prospect"
)

func manifestDepth(raw []byte) bool {
	depth := 0
	quoted, escaped := false, false
	for _, c := range raw {
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{':
			depth++
			if depth > MaxManifestJobDepth {
				return false
			}
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		case '[', ']':
			return false
		}
	}
	return !quoted && !escaped && depth == 0
}

// ParseManifestJob accepts the job family's transport JSON grammar, then
// requires exact canonical values. Whitespace/order/decoded escaped spellings
// are not identity bytes. This never dispatches to or weakens the v1 parser.
func ParseManifestJob(raw []byte) (ManifestJob, error) {
	bad := errors.New("invalid manifest job JSON")
	if len(raw) == 0 || len(raw) > MaxManifestJobSize {
		return ManifestJob{}, bad
	}
	owned := bytes.Clone(raw)
	if !manifestDepth(owned) {
		return ManifestJob{}, bad
	}
	var job ManifestJob
	var context json.RawMessage
	fields := map[string]any{
		"version": &job.Version, "rule_version": &job.RuleVersion, "id": &job.ID,
		"authority_id": &job.AuthorityID, "descriptor_id": &job.DescriptorID,
		"context": &context, "public_key": &job.PublicKey, "gym_id": &job.GymID,
		"challenge": &job.Challenge, "issued_at_ms": &job.IssuedAtMS, "expires_at_ms": &job.ExpiresAtMS,
	}
	d := json.NewDecoder(bytes.NewReader(owned))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return ManifestJob{}, bad
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return ManifestJob{}, bad
		}
		name, ok := token.(string)
		target, known := fields[name]
		if !ok || !known || seen[name] {
			return ManifestJob{}, bad
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, target) != nil {
			return ManifestJob{}, bad
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(seen) != len(fields) {
		return ManifestJob{}, bad
	}
	if _, err = d.Token(); err != io.EOF {
		return ManifestJob{}, bad
	}
	job.Context, err = prospect.ParseContext(context)
	if err != nil {
		return ManifestJob{}, err
	}
	if err = ValidateManifest(job); err != nil {
		return ManifestJob{}, err
	}
	return job, nil
}

// MarshalManifestJob emits a canonical representation for fixtures/storage,
// not the binary JobID preimage and not evidence of trusted issuance.
func MarshalManifestJob(job ManifestJob) ([]byte, error) {
	if err := ValidateManifest(job); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(job)
	if err != nil || len(raw) > MaxManifestJobSize {
		return nil, errors.New("manifest job JSON exceeds limit")
	}
	return raw, nil
}
