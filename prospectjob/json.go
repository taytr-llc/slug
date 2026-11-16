// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectjob

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/taytr-llc/slug/prospect"
)

// ParseJob decodes the bounded, exact job schema and requires its canonical
// value. Every field is required exactly once. The nested context uses its own
// strict parser; neither parsing step authenticates the supplied assertions.
func ParseJob(data []byte) (Job, error) {
	if len(data) > MaxJobSize {
		return Job{}, fmt.Errorf("job JSON exceeds %d bytes", MaxJobSize)
	}
	var job Job
	var context json.RawMessage
	fields := map[string]any{
		"version": &job.Version, "rule_version": &job.RuleVersion, "id": &job.ID,
		"context": &context, "public_key": &job.PublicKey, "gym_id": &job.GymID,
		"challenge": &job.Challenge, "issued_at_ms": &job.IssuedAtMS,
		"expires_at_ms": &job.ExpiresAtMS,
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Job{}, errors.New("job must be a JSON object")
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Job{}, err
		}
		name, ok := token.(string)
		if !ok {
			return Job{}, errors.New("invalid job field name")
		}
		target, known := fields[name]
		if !known || seen[name] {
			return Job{}, fmt.Errorf("unknown or duplicate job field %q", name)
		}
		seen[name] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return Job{}, err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return Job{}, fmt.Errorf("job field %q must not be null", name)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return Job{}, fmt.Errorf("job field %q: %w", name, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return Job{}, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return Job{}, errors.New("trailing data after job JSON object")
	}
	if len(seen) != len(fields) {
		return Job{}, errors.New("missing required job fields")
	}
	job.Context, err = prospect.ParseContext(context)
	if err != nil {
		return Job{}, fmt.Errorf("job context: %w", err)
	}
	if err := Validate(job); err != nil {
		return Job{}, err
	}
	return job, nil
}
