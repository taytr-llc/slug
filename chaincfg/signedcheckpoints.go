// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	// SignedCheckpointVersion is the supported checkpoint signing format.
	SignedCheckpointVersion uint32 = 1

	// MaxSignedCheckpointSize bounds a checkpoint JSON file. File readers
	// should enforce this limit before allocating or decoding the input.
	MaxSignedCheckpointSize = 4096

	signedCheckpointDomain = "SLUG signed checkpoint\x00"
)

// SignedCheckpoint authenticates a league checkpoint for one chain. Its
// signature does not establish that the block exists, has sufficient depth, or
// belongs to the best chain; callers must check those conditions separately.
// The public key is supplied by the verifier, never by the checkpoint itself.
// This type alone does not change consensus validation or chain selection.
type SignedCheckpoint struct {
	Version     uint32
	Network     wire.BitcoinNet
	GenesisHash chainhash.Hash
	Height      int32
	Hash        chainhash.Hash
	Signature   []byte
}

// SigningBytes returns the canonical message signed with ordinary Ed25519.
// The format is the ASCII domain "SLUG signed checkpoint" followed by a NUL,
// uint32 version, uint32 network magic, 32-byte genesis hash, uint32 height, and
// 32-byte block hash. Integers are little-endian; hashes are in their internal
// byte order, as in the wire protocol, not their reversed display order.
// Heights must be positive signed 32-bit values. The signature and JSON
// representation are not part of this message.
func (c *SignedCheckpoint) SigningBytes() ([]byte, error) {
	if c == nil {
		return nil, errors.New("nil signed checkpoint")
	}
	if c.Version != SignedCheckpointVersion {
		return nil, fmt.Errorf("unsupported signed checkpoint version %d", c.Version)
	}
	if c.Height <= 0 {
		return nil, errors.New("signed checkpoint height must be positive")
	}
	if c.GenesisHash == (chainhash.Hash{}) || c.Hash == (chainhash.Hash{}) {
		return nil, errors.New("signed checkpoint hashes must not be zero")
	}

	message := make([]byte, len(signedCheckpointDomain)+4+4+32+4+32)
	offset := copy(message, signedCheckpointDomain)
	binary.LittleEndian.PutUint32(message[offset:], c.Version)
	offset += 4
	binary.LittleEndian.PutUint32(message[offset:], uint32(c.Network))
	offset += 4
	offset += copy(message[offset:], c.GenesisHash[:])
	binary.LittleEndian.PutUint32(message[offset:], uint32(c.Height))
	offset += 4
	copy(message[offset:], c.Hash[:])
	return message, nil
}

// Verify checks the signature against an explicitly trusted public key and
// binds it to the selected network and genesis. It does not trust a public key
// carried in the input, nor does it determine chain membership or finality.
func (c *SignedCheckpoint) Verify(params *Params, publicKey ed25519.PublicKey) error {
	message, err := c.SigningBytes()
	if err != nil {
		return err
	}
	if params == nil || params.GenesisHash == nil {
		return errors.New("signed checkpoint requires network genesis parameters")
	}
	if c.Network != params.Net || c.GenesisHash != *params.GenesisHash {
		return errors.New("signed checkpoint belongs to a different network or genesis")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("signed checkpoint public key must be %d bytes", ed25519.PublicKeySize)
	}
	if len(c.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("signed checkpoint signature must be %d bytes", ed25519.SignatureSize)
	}
	if !ed25519.Verify(publicKey, message, c.Signature) {
		return errors.New("invalid signed checkpoint signature")
	}
	return nil
}

// MarshalJSON encodes hashes in their usual display order and the signature
// as hexadecimal. The signature authenticates SigningBytes, not this JSON.
func (c SignedCheckpoint) MarshalJSON() ([]byte, error) {
	if _, err := c.SigningBytes(); err != nil {
		return nil, err
	}
	if len(c.Signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signed checkpoint signature must be %d bytes", ed25519.SignatureSize)
	}

	return json.Marshal(struct {
		Version   uint32 `json:"version"`
		Network   uint32 `json:"network"`
		Genesis   string `json:"genesis"`
		Height    int32  `json:"height"`
		Hash      string `json:"hash"`
		Signature string `json:"signature"`
	}{
		Version:   c.Version,
		Network:   uint32(c.Network),
		Genesis:   c.GenesisHash.String(),
		Height:    c.Height,
		Hash:      c.Hash.String(),
		Signature: hex.EncodeToString(c.Signature),
	})
}

// UnmarshalJSON decodes a bounded, strict checkpoint object. Every field is
// required exactly once; unknown fields, nulls, abbreviated hashes, wrong
// lengths, and trailing data are rejected. Decoding does not verify a signature.
func (c *SignedCheckpoint) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("nil signed checkpoint destination")
	}
	if len(data) > MaxSignedCheckpointSize {
		return errors.New("signed checkpoint exceeds maximum size")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("signed checkpoint must be a JSON object")
	}

	fields := make(map[string]json.RawMessage, 6)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return errors.New("invalid signed checkpoint field name")
		}
		switch name {
		case "version", "network", "genesis", "height", "hash", "signature":
		default:
			return fmt.Errorf("unknown signed checkpoint field %q", name)
		}
		if _, exists := fields[name]; exists {
			return fmt.Errorf("duplicate signed checkpoint field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("signed checkpoint field %q must not be null", name)
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing data after signed checkpoint")
	}
	if len(fields) != 6 {
		return errors.New("signed checkpoint is missing required fields")
	}

	var decoded SignedCheckpoint
	for name, target := range map[string]interface{}{
		"version": &decoded.Version,
		"network": &decoded.Network,
		"height":  &decoded.Height,
	} {
		if err := json.Unmarshal(fields[name], target); err != nil {
			return fmt.Errorf("signed checkpoint %s: %w", name, err)
		}
	}
	for name, target := range map[string]*chainhash.Hash{
		"genesis": &decoded.GenesisHash,
		"hash":    &decoded.Hash,
	} {
		var value string
		if err := json.Unmarshal(fields[name], &value); err != nil {
			return fmt.Errorf("signed checkpoint %s: %w", name, err)
		}
		if len(value) != chainhash.MaxHashStringSize {
			return fmt.Errorf("signed checkpoint %s must be 64 hex characters", name)
		}
		if err := chainhash.Decode(target, value); err != nil {
			return fmt.Errorf("signed checkpoint %s: %w", name, err)
		}
	}
	var signature string
	if err := json.Unmarshal(fields["signature"], &signature); err != nil {
		return fmt.Errorf("signed checkpoint signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize*2 {
		return errors.New("signed checkpoint signature must be 128 hex characters")
	}
	decoded.Signature, err = hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("signed checkpoint signature: %w", err)
	}
	if _, err := decoded.SigningBytes(); err != nil {
		return err
	}
	*c = decoded
	return nil
}
