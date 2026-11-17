// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fighterdescriptor encodes immutable static-appearance descriptors.
// Structural validity, compiled candidate support and production activation
// are separate boundaries. No descriptor authenticates issuance or ownership.
package fighterdescriptor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	Version1           uint32 = 1
	MaxDescriptorSize         = 2048
	MaxDescriptorDepth        = 1
	descriptorDomain          = "SLUG fighter issuance descriptor\x00v1\x00"
)

var (
	ErrDescriptor            = errors.New("invalid fighter descriptor")
	ErrUnsupportedDescriptor = errors.New("unsupported candidate fighter descriptor")
	ErrProductionUnavailable = errors.New("production fighter descriptors unavailable")
)

// Descriptor is value-only. Its canonical JSON has the declared field order;
// its ID is computed separately and is not a self-referential field.
type Descriptor struct {
	Version              uint32 `json:"version"`
	DNAVersion           uint32 `json:"dna_version"`
	ManifestVersion      uint32 `json:"manifest_version"`
	GeneratorID          string `json:"generator_id"`
	ManifestTableHash    string `json:"manifest_table_hash"`
	ManifestContractHash string `json:"manifest_contract_hash"`
	PortraitTraitVersion uint32 `json:"portrait_trait_version"`
	RendererVersion      uint32 `json:"renderer_version"`
	RendererID           string `json:"renderer_id"`
	RendererContractHash string `json:"renderer_contract_hash"`
}

func validIdentifier(s string) bool {
	if len(s) < 1 || len(s) > 96 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func hashShape(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func canonicalID(s string) bool { return hashShape(s) && s == strings.ToLower(s) }

// Normalize lowercases only the three exact-shape hash fields. It does not
// select an implementation, change identifier case or imply support.
func Normalize(d Descriptor) (Descriptor, error) {
	if d.Version != Version1 || d.DNAVersion == 0 || d.ManifestVersion == 0 || d.PortraitTraitVersion == 0 || d.RendererVersion == 0 || !validIdentifier(d.GeneratorID) || !validIdentifier(d.RendererID) || !hashShape(d.ManifestTableHash) || !hashShape(d.ManifestContractHash) || !hashShape(d.RendererContractHash) {
		return Descriptor{}, ErrDescriptor
	}
	d.ManifestTableHash = strings.ToLower(d.ManifestTableHash)
	d.ManifestContractHash = strings.ToLower(d.ManifestContractHash)
	d.RendererContractHash = strings.ToLower(d.RendererContractHash)
	return d, nil
}

// Validate checks already-canonical structural values, not compiled support.
func Validate(d Descriptor) error {
	n, err := Normalize(d)
	if err != nil || n != d {
		return ErrDescriptor
	}
	return nil
}

// Marshal returns owned compact ASCII canonical bytes. Unknown positive
// component selectors remain structurally encodable; unknown formats do not.
func Marshal(d Descriptor) ([]byte, error) {
	if err := Validate(d); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > MaxDescriptorSize {
		return nil, ErrDescriptor
	}
	return raw, nil
}

// Parse requires exact canonical bytes and rejects nested/oversized inputs
// before the JSON decoder. No caller buffer is retained or modified.
func Parse(raw []byte) (Descriptor, error) {
	if len(raw) == 0 || len(raw) > MaxDescriptorSize {
		return Descriptor{}, ErrDescriptor
	}
	owned := bytes.Clone(raw)
	depth := 0
	quoted := false
	for _, c := range owned {
		if c > 127 || c == '\\' {
			return Descriptor{}, ErrDescriptor
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch c {
		case '{':
			depth++
			if depth > MaxDescriptorDepth {
				return Descriptor{}, ErrDescriptor
			}
		case '}':
			depth--
			if depth < 0 {
				return Descriptor{}, ErrDescriptor
			}
		case '[', ']':
			return Descriptor{}, ErrDescriptor
		}
	}
	if quoted || depth != 0 {
		return Descriptor{}, ErrDescriptor
	}
	var d Descriptor
	fields := map[string]any{
		"version": &d.Version, "dna_version": &d.DNAVersion, "manifest_version": &d.ManifestVersion,
		"generator_id": &d.GeneratorID, "manifest_table_hash": &d.ManifestTableHash, "manifest_contract_hash": &d.ManifestContractHash,
		"portrait_trait_version": &d.PortraitTraitVersion, "renderer_version": &d.RendererVersion, "renderer_id": &d.RendererID, "renderer_contract_hash": &d.RendererContractHash,
	}
	decoder := json.NewDecoder(bytes.NewReader(owned))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Descriptor{}, ErrDescriptor
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Descriptor{}, ErrDescriptor
		}
		name, ok := token.(string)
		target, known := fields[name]
		if !ok || !known || seen[name] {
			return Descriptor{}, ErrDescriptor
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, target) != nil {
			return Descriptor{}, ErrDescriptor
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(fields) {
		return Descriptor{}, ErrDescriptor
	}
	if _, err = decoder.Token(); err != io.EOF {
		return Descriptor{}, ErrDescriptor
	}
	canonical, err := Marshal(d)
	if err != nil || !bytes.Equal(owned, canonical) {
		return Descriptor{}, ErrDescriptor
	}
	return d, nil
}

// ID fingerprints canonical descriptor bytes, not authority or activation.
func ID(d Descriptor) (string, error) {
	raw, err := Marshal(d)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(descriptorDomain))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CandidateDescriptor returns a fresh copy of the sole reviewed static bundle.
// Its artifact hashes identify archived bytes, not mutable documentation paths.
func CandidateDescriptor() Descriptor {
	return Descriptor{
		Version: 1, DNAVersion: 1, ManifestVersion: 1, GeneratorID: "slug-fighter-manifest-candidate-v1",
		ManifestTableHash:    "72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44",
		ManifestContractHash: "ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80",
		PortraitTraitVersion: 1, RendererVersion: 1, RendererID: "slug-fighter-portrait-candidate-v1",
		RendererContractHash: "8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5",
	}
}

// ResolveCandidate accepts only the canonical ID of the entire compiled tuple.
func ResolveCandidate(descriptorID string) (Descriptor, error) {
	if !canonicalID(descriptorID) {
		return Descriptor{}, ErrDescriptor
	}
	d := CandidateDescriptor()
	id, err := ID(d)
	if err != nil {
		return Descriptor{}, err
	}
	if descriptorID != id {
		return Descriptor{}, ErrUnsupportedDescriptor
	}
	return d, nil
}

// ResolveProduction has no activated entries, including the candidate tuple.
func ResolveProduction(descriptorID string) (Descriptor, error) {
	if !canonicalID(descriptorID) {
		return Descriptor{}, ErrDescriptor
	}
	return Descriptor{}, ErrProductionUnavailable
}
