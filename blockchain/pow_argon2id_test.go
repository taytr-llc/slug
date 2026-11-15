// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestCheckProofOfWorkArgon2id is the proof test for SLUG change set 1.
// It grinds a regtest-difficulty header through the new Argon2id
// BlockHash until the target is met, then asserts the chain's own
// proof-of-work check accepts the block and rejects the same header
// with a tampered nonce. Regtest bits (0x207fffff, target ~2^255) keep
// the grind to a handful of ~340 ms hashes.
func TestCheckProofOfWorkArgon2id(t *testing.T) {
	params := chaincfg.RegressionNetParams

	header := wire.BlockHeader{
		Version:    1,
		PrevBlock:  *params.GenesisHash,
		MerkleRoot: params.GenesisBlock.Header.MerkleRoot,
		Timestamp:  time.Unix(1760000000, 0),
		Bits:       params.PowLimitBits,
	}

	target := CompactToBig(header.Bits)

	// The new hash must not be the old double SHA256 of the same
	// header; otherwise the swap is not actually in effect.
	var raw bytes.Buffer
	if err := header.Serialize(&raw); err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if got, want := header.BlockHash(), chainhash.DoubleHashH(raw.Bytes()); got == want {
		t.Fatalf("BlockHash equals double SHA256 (%v); Argon2id swap "+
			"not in effect", got)
	}

	// Grind the nonce until the Argon2id hash meets the target.
	var hash chainhash.Hash
	found := false
	for nonce := uint32(0); nonce < 10000; nonce++ {
		header.Nonce = nonce
		hash = header.BlockHash()
		if HashToBig(&hash).Cmp(target) <= 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no nonce under the regtest target after 10000 attempts")
	}
	t.Logf("mined nonce %d, hash %v", header.Nonce, hash)

	// NewMsgBlock copies the header, so rebuild the block from the
	// header each time the nonce changes.
	blockFor := func(h wire.BlockHeader) *btcutil.Block {
		return btcutil.NewBlock(wire.NewMsgBlock(&h))
	}

	if err := CheckProofOfWork(blockFor(header), params.PowLimit); err != nil {
		t.Fatalf("CheckProofOfWork rejected a mined header: %v", err)
	}

	// Tamper: find a nonce whose hash misses the target and assert
	// the check rejects it.
	rejected := false
	for nonce := header.Nonce + 1; nonce < header.Nonce+10000; nonce++ {
		header.Nonce = nonce
		hash = header.BlockHash()
		if HashToBig(&hash).Cmp(target) > 0 {
			if err := CheckProofOfWork(blockFor(header), params.PowLimit); err == nil {
				t.Fatalf("CheckProofOfWork accepted tampered nonce %d "+
					"with hash %v above target", nonce, hash)
			}
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatalf("no above-target nonce found to test rejection")
	}
}
