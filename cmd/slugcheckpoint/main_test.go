// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
)

// This is the public test seed from RFC 8032, section 7.1, test 1. It is
// deliberately unsuitable as an operator key and is only used in temp files.
const testSeedHex = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
const testPublicKeyHex = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"

func seedFile(t *testing.T) string {
	t.Helper()
	seed, err := hex.DecodeString(testSeedHex)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test-only.seed")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func invoke(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	return stdout.String(), err
}

func TestKeygenKeepsSeedPrivateAndNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.seed")
	stdout, err := invoke("keygen", "--out", path)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed) != ed25519.SeedSize {
		t.Fatalf("seed size = %d", len(seed))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("seed permissions = %o", info.Mode().Perm())
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	want := hex.EncodeToString(publicKey) + "\n"
	if stdout != want {
		t.Fatal("keygen must print only the public key")
	}
	stdout, err = invoke("keygen", "--out", path)
	if err == nil || stdout != "" {
		t.Fatal("keygen must refuse an existing output without printing a key")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seed, after) {
		t.Fatal("keygen replaced the existing seed")
	}
}

func TestSignVerifyAndRejectTampering(t *testing.T) {
	keyPath := seedFile(t)
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	hash := chainhash.Hash{1}
	stdout, err := invoke("sign", "--key", keyPath, "--out", path,
		"--network", "regtest", "--height", "100", "--hash", hash.String(),
		"--genesis", chaincfg.RegressionNetParams.GenesisHash.String())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, testSeedHex) || !strings.Contains(stdout, testPublicKeyHex) {
		t.Fatal("sign output must identify only the public key")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err = invoke("verify", "--pubkey", testPublicKeyHex,
		"--network", "regtest", "--file", path)
	if err != nil || !strings.Contains(stdout, "Valid signature for height 100") {
		t.Fatalf("verification failed: %v (%s)", err, stdout)
	}
	_, err = invoke("verify", "--pubkey", testPublicKeyHex,
		"--network", "mainnet", "--file", path)
	if err == nil {
		t.Fatal("verified a checkpoint for a different network")
	}
	stdout, err = invoke("verify", "--pubkey", strings.Repeat("01", 32),
		"--network", "regtest", "--file", path)
	if err == nil || stdout != "" {
		t.Fatal("verified a checkpoint with an untrusted signing key")
	}

	// An existing signed file must survive attempted replacement byte for byte.
	_, err = invoke("sign", "--key", keyPath, "--out", path,
		"--network", "regtest", "--height", "101", "--hash", hash.String())
	if err == nil {
		t.Fatal("sign overwrote an existing checkpoint")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, encoded) {
		t.Fatal("existing signed checkpoint changed")
	}

	var checkpoint chaincfg.SignedCheckpoint
	if err := json.Unmarshal(encoded, &checkpoint); err != nil {
		t.Fatal(err)
	}
	checkpoint.Height++
	tampered, err := json.Marshal(&checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	stdout, err = invoke("verify", "--pubkey", testPublicKeyHex,
		"--network", "regtest", "--file", path)
	if err == nil || stdout != "" {
		t.Fatal("tampered checkpoint passed verification")
	}
}

func TestSigningRejectsUnsafeInputs(t *testing.T) {
	keyPath := seedFile(t)
	hash := chainhash.Hash{1}
	tests := []struct {
		name string
		args []string
	}{
		{"zero height", []string{"--height", "0"}},
		{"negative height", []string{"--height", "-1"}},
		{"height overflow", []string{"--height", "2147483648"}},
		{"short hash", []string{"--hash", "01"}},
		{"zero hash", []string{"--hash", strings.Repeat("0", 64)}},
		{"unknown network", []string{"--network", "testnet"}},
		{"wrong genesis", []string{"--genesis", hash.String()}},
		{"bad genesis", []string{"--genesis", "01"}},
		{"positional secret", []string{testSeedHex}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "checkpoint.json")
			args := []string{"sign", "--key", keyPath, "--out", path,
				"--height", "1", "--hash", hash.String()}
			stdout, err := invoke(append(args, test.args...)...)
			if err == nil || stdout != "" {
				t.Fatal("invalid input was accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("invalid input created an output file")
			}
		})
	}
}

func TestPrivateSeedSizeAndPermissions(t *testing.T) {
	for _, size := range []int{0, 31, 33, 64} {
		path := filepath.Join(t.TempDir(), "bad.seed")
		if err := os.WriteFile(path, bytes.Repeat([]byte{1}, size), 0600); err != nil {
			t.Fatal(err)
		}
		stdout, err := invoke("sign", "--key", path,
			"--out", filepath.Join(t.TempDir(), "checkpoint.json"),
			"--height", "1", "--hash", (chainhash.Hash{1}).String())
		if err == nil || stdout != "" {
			t.Fatalf("accepted private seed of length %d", size)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	path := seedFile(t)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	stdout, err := invoke("sign", "--key", path,
		"--out", filepath.Join(t.TempDir(), "checkpoint.json"),
		"--height", "1", "--hash", (chainhash.Hash{1}).String())
	if err == nil || stdout != "" {
		t.Fatal("accepted a group/world-readable private seed")
	}
}

func TestVerifyBoundsInputAndPublicKeyLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte{' '}, chaincfg.MaxSignedCheckpointSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, err := invoke("verify", "--pubkey", testPublicKeyHex, "--file", path)
	if err == nil || stdout != "" {
		t.Fatal("accepted an oversized checkpoint file")
	}
	stdout, err = invoke("verify", "--pubkey", "01", "--file", path)
	if err == nil || stdout != "" {
		t.Fatal("accepted a short public key")
	}
	stdout, err = invoke("verify", "--pubkey", testPublicKeyHex, "--file", t.TempDir())
	if err == nil || stdout != "" {
		t.Fatal("accepted a nonregular input")
	}
}

func TestExclusiveOutputRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks may require elevated permissions on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "existing")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(link, []byte("replace"), 0600); err == nil {
		t.Fatal("followed an existing output symlink")
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "preserve" {
		t.Fatal("changed the symlink's target")
	}
}
