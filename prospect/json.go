// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ParseContext decodes the strict context JSON schema. The complete input,
// including surrounding whitespace, is bounded. Parsing does not authenticate
// the context or replace the semantic checks performed by the verifier.
func ParseContext(data []byte) (Context, error) {
	var context Context
	fields := map[string]any{
		"version":         &context.Version,
		"network":         &context.Network,
		"genesis":         &context.Genesis,
		"dna_version":     &context.DNAVersion,
		"prospect_target": &context.ProspectTarget,
		"parent_hash":     &context.ParentHash,
		"parent_height":   &context.ParentHeight,
		"expected_bits":   &context.ExpectedBits,
		"min_timestamp":   &context.MinTimestamp,
		"max_timestamp":   &context.MaxTimestamp,
	}
	if err := parseObject(data, MaxContextSize, fields); err != nil {
		return Context{}, fmt.Errorf("context: %w", err)
	}
	return context, nil
}

// ParseProof decodes the strict proof JSON schema without verifying its work.
// Every field is required once, including an empty merkle_branch array for a
// coinbase-only template. Null array elements are never accepted as strings.
func ParseProof(data []byte) (Proof, error) {
	var proof Proof
	var branch []json.RawMessage
	var jobID *string
	fields := map[string]any{
		"version":       &proof.Version,
		"gym_id":        &proof.GymID,
		"header":        &proof.Header,
		"coinbase":      &proof.Coinbase,
		"merkle_branch": &branch,
		"job_id":        &jobID,
	}
	if err := parseObject(data, MaxProofSize, fields, "job_id"); err != nil {
		return Proof{}, fmt.Errorf("proof: %w", err)
	}
	if proof.Version == Version1 && jobID != nil {
		return Proof{}, errors.New("version-1 proof must not include job_id")
	}
	if jobID != nil {
		proof.JobID = *jobID
	}
	if _, err := proofJob(proof); err != nil {
		return Proof{}, err
	}
	if len(branch) > maxMerkleBranch {
		return Proof{}, fmt.Errorf("proof: merkle_branch exceeds %d siblings", maxMerkleBranch)
	}
	proof.MerkleBranch = make([]string, len(branch))
	for i, value := range branch {
		if isNull(value) {
			return Proof{}, fmt.Errorf("proof: merkle_branch[%d] must not be null", i)
		}
		if err := json.Unmarshal(value, &proof.MerkleBranch[i]); err != nil {
			return Proof{}, fmt.Errorf("proof: merkle_branch[%d]: %w", i, err)
		}
	}
	return proof, nil
}

func isNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

// parseObject uses decoded field names so escaped spellings cannot bypass the
// duplicate check. Decoding directly into a struct would also accept duplicate
// fields, null scalar values, and case-insensitive field-name aliases.
func parseObject(data []byte, maxSize int, fields map[string]any, optional ...string) error {
	if len(data) > maxSize {
		return fmt.Errorf("JSON exceeds %d bytes", maxSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("must be a JSON object")
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return errors.New("invalid field name")
		}
		target, ok := fields[name]
		if !ok {
			return fmt.Errorf("unknown field %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate field %q", name)
		}
		seen[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if isNull(value) {
			return fmt.Errorf("field %q must not be null", name)
		}
		if err := json.Unmarshal(value, target); err != nil {
			return fmt.Errorf("field %q: %w", name, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing data after JSON object")
	}
	for name := range fields {
		if seen[name] {
			continue
		}
		isOptional := false
		for _, allowed := range optional {
			if name == allowed {
				isOptional = true
				break
			}
		}
		if !isOptional {
			return errors.New("missing required fields")
		}
	}
	return nil
}
