// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// This message is written independently of SigningBytes: the documented domain,
// version 1, SLUG network magic, ascending raw hash bytes, and height 0x01020304.
const checkpointMessageHex = "534c5547207369676e656420636865636b706f696e7400" +
	"01000000" + "47554c53" +
	"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" +
	"04030201" +
	"202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f"

// signedCheckpointFixture uses the public RFC 8032 test seed. It is only test
// material, never an operator signing key or a production checkpoint.
func signedCheckpointFixture(t *testing.T) (SignedCheckpoint, Params, ed25519.PublicKey) {
	t.Helper()
	seed, err := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if hex.EncodeToString(publicKey) != "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a" {
		t.Fatal("unexpected RFC 8032 public key")
	}
	message, err := hex.DecodeString(checkpointMessageHex)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := SignedCheckpoint{
		Version: SignedCheckpointVersion,
		Network: wire.MainNet,
		Height:  0x01020304,
	}
	for i := range checkpoint.Hash {
		checkpoint.GenesisHash[i] = byte(i)
		checkpoint.Hash[i] = byte(i + 32)
	}
	// Sign the independently specified bytes, not SigningBytes's output.
	checkpoint.Signature = ed25519.Sign(privateKey, message)
	genesis := checkpoint.GenesisHash
	params := Params{Net: wire.MainNet, GenesisHash: &genesis}
	return checkpoint, params, publicKey
}

func TestSignedCheckpointSigningBytes(t *testing.T) {
	checkpoint, _, _ := signedCheckpointFixture(t)
	message, err := checkpoint.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(message) != checkpointMessageHex {
		t.Fatalf("wrong signing message: %x", message)
	}
	// A caller's mutation of the returned buffer cannot alter the checkpoint.
	message[0] ^= 1
	next, err := checkpoint.SigningBytes()
	if err != nil || hex.EncodeToString(next) != checkpointMessageHex {
		t.Fatalf("signing bytes alias mutable storage: %v", err)
	}
	checkpoint.Height = math.MaxInt32
	if _, err := checkpoint.SigningBytes(); err != nil {
		t.Fatalf("maximum supported height rejected: %v", err)
	}
}

func TestSignedCheckpointVerify(t *testing.T) {
	checkpoint, params, publicKey := signedCheckpointFixture(t)
	if err := checkpoint.Verify(&params, publicKey); err != nil {
		t.Fatalf("independently signed checkpoint rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*SignedCheckpoint)
	}{
		{"unknown version", func(c *SignedCheckpoint) { c.Version++ }},
		{"zero version", func(c *SignedCheckpoint) { c.Version = 0 }},
		{"network", func(c *SignedCheckpoint) { c.Network = wire.TestNet }},
		{"genesis", func(c *SignedCheckpoint) { c.GenesisHash[0] ^= 1 }},
		{"zero genesis", func(c *SignedCheckpoint) { c.GenesisHash = chainhash.Hash{} }},
		{"height", func(c *SignedCheckpoint) { c.Height++ }},
		{"zero height", func(c *SignedCheckpoint) { c.Height = 0 }},
		{"negative height", func(c *SignedCheckpoint) { c.Height = -1 }},
		{"block hash", func(c *SignedCheckpoint) { c.Hash[0] ^= 1 }},
		{"zero block hash", func(c *SignedCheckpoint) { c.Hash = chainhash.Hash{} }},
		{"signature bit", func(c *SignedCheckpoint) { c.Signature[0] ^= 1 }},
		{"no signature", func(c *SignedCheckpoint) { c.Signature = nil }},
		{"short signature", func(c *SignedCheckpoint) { c.Signature = c.Signature[:63] }},
		{"long signature", func(c *SignedCheckpoint) { c.Signature = append(c.Signature, 0) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := checkpoint
			changed.Signature = append([]byte(nil), checkpoint.Signature...)
			test.mutate(&changed)
			if err := changed.Verify(&params, publicKey); err == nil {
				t.Fatal("accepted modified checkpoint")
			}
		})
	}

	for _, size := range []int{0, 31, 33} {
		if err := checkpoint.Verify(&params, make(ed25519.PublicKey, size)); err == nil {
			t.Fatalf("accepted public key of size %d", size)
		}
	}
	wrongKey := append(ed25519.PublicKey(nil), publicKey...)
	wrongKey[0] ^= 1
	if err := checkpoint.Verify(&params, wrongKey); err == nil {
		t.Fatal("accepted a different authority key")
	}
	if err := checkpoint.Verify(nil, publicKey); err == nil {
		t.Fatal("accepted nil network parameters")
	}
	if err := checkpoint.Verify(&Params{Net: params.Net}, publicKey); err == nil {
		t.Fatal("accepted network without genesis")
	}
	wrongParams := params
	wrongParams.Net = wire.TestNet
	if err := checkpoint.Verify(&wrongParams, publicKey); err == nil {
		t.Fatal("accepted replay on another network")
	}
	otherGenesis := *params.GenesisHash
	otherGenesis[0] ^= 1
	wrongParams = params
	wrongParams.GenesisHash = &otherGenesis
	if err := checkpoint.Verify(&wrongParams, publicKey); err == nil {
		t.Fatal("accepted replay on another genesis with the same network magic")
	}
	var nilCheckpoint *SignedCheckpoint
	if err := nilCheckpoint.Verify(&params, publicKey); err == nil {
		t.Fatal("accepted nil checkpoint")
	}
}

func TestSignedCheckpointJSON(t *testing.T) {
	checkpoint, params, publicKey := signedCheckpointFixture(t)
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	// JSON hashes use display order, while the signed bytes use raw order.
	if !bytes.Contains(encoded, []byte(`"genesis":"1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"`)) ||
		!bytes.Contains(encoded, []byte(`"hash":"3f3e3d3c3b3a393837363534333231302f2e2d2c2b2a29282726252423222120"`)) {
		t.Fatalf("incorrect display hash encoding: %s", encoded)
	}
	var decoded SignedCheckpoint
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, checkpoint) {
		t.Fatal("checkpoint changed in JSON round trip")
	}
	if err := decoded.Verify(&params, publicKey); err != nil {
		t.Fatal(err)
	}

	// Whitespace, field order, and equivalent hex case do not affect the
	// signature: it authenticates the fixed binary message, not JSON text.
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	fields["hash"] = strings.ToUpper(fields["hash"].(string))
	alternate, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(alternate, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Verify(&params, publicKey); err != nil {
		t.Fatalf("JSON presentation affected signature: %v", err)
	}
}

func TestSignedCheckpointRejectsMalformedJSON(t *testing.T) {
	checkpoint, _, _ := signedCheckpointFixture(t)
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(encoded)
	cases := map[string]string{
		"array":             "[]",
		"null":              "null",
		"string":            `"checkpoint"`,
		"missing all":       "{}",
		"trailing object":   valid + `{}`,
		"trailing garbage":  valid + `garbage`,
		"unknown field":     strings.TrimSuffix(valid, "}") + `,"pubkey":"untrusted"}`,
		"duplicate field":   strings.TrimSuffix(valid, "}") + `,"height":16909060}`,
		"escaped duplicate": strings.TrimSuffix(valid, "}") + `,"heig\u0068t":16909060}`,
		"oversized":         valid + strings.Repeat(" ", MaxSignedCheckpointSize),
	}
	for _, field := range []string{"version", "network", "genesis", "height", "hash", "signature"} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &object); err != nil {
			t.Fatal(err)
		}
		delete(object, field)
		missing, _ := json.Marshal(object)
		cases["missing "+field] = string(missing)
		object[field] = json.RawMessage("null")
		null, _ := json.Marshal(object)
		cases["null "+field] = string(null)
	}
	for name, value := range map[string]string{
		"zero version":      `0`,
		"unknown version":   `2`,
		"version overflow":  `4294967296`,
		"negative version":  `-1`,
		"version float":     `1.0`,
		"version exponent":  `1e0`,
		"version as string": `"1"`,
	} {
		cases[name] = strings.Replace(valid, `"version":1`, `"version":`+value, 1)
	}
	for name, value := range map[string]string{
		"zero height":      `0`,
		"negative height":  `-1`,
		"height overflow":  `2147483648`,
		"height float":     `1.5`,
		"height as string": `"1"`,
	} {
		cases[name] = strings.Replace(valid, `"height":16909060`, `"height":`+value, 1)
	}
	for _, field := range []string{"genesis", "hash", "signature"} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &object); err != nil {
			t.Fatal(err)
		}
		length := 64
		if field == "signature" {
			length = 128
		}
		for name, value := range map[string]string{
			"short": strings.Repeat("a", length-2),
			"long":  strings.Repeat("a", length+2),
			"odd":   strings.Repeat("a", length-1),
			"bad":   strings.Repeat("z", length),
		} {
			object[field], _ = json.Marshal(value)
			data, _ := json.Marshal(object)
			cases[field+" "+name] = string(data)
		}
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			unchanged := checkpoint
			if err := unchanged.UnmarshalJSON([]byte(data)); err == nil {
				t.Fatalf("accepted malformed checkpoint %s", data)
			}
			if !reflect.DeepEqual(unchanged, checkpoint) {
				t.Fatal("failed decode mutated destination")
			}
		})
	}
	unsigned := checkpoint
	unsigned.Signature = nil
	if _, err := json.Marshal(unsigned); err == nil {
		t.Fatal("encoded a missing signature")
	}
	var nilCheckpoint *SignedCheckpoint
	if err := nilCheckpoint.UnmarshalJSON(encoded); err == nil {
		t.Fatal("decoded into nil destination")
	}
}
