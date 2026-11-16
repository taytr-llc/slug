//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/crypto/argon2"
)

// TestProductionProofVector pins the change set 1 proof recorded in HANDOVER.
// Its fixed bytes and digest were independently checked using direct Argon2id,
// without BlockHash or chaincfg-derived input. It must never run in cheap mode.
func TestProductionProofVector(t *testing.T) {
	if wire.TestPoW {
		t.Fatal("default build selected test proof of work")
	}
	raw, err := hex.DecodeString("01000000" +
		"a6d3a6e36fb423771f58cacd3cfa39c701893cc29c8f2408ba7d814c3fb1bde1" +
		"3ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a" +
		"0078e768ffff7f2001000000")
	if err != nil {
		t.Fatal(err)
	}
	const want = "5f42cf87f3acc67ff351adc85f0df64b4cc19e3e278fd46ce63c76511dc8d719"
	digest := argon2.IDKey(raw, raw, 1, 256*1024, 1, 32)
	var independentlyDerived chainhash.Hash
	copy(independentlyDerived[:], digest)
	if independentlyDerived.String() != want {
		t.Fatalf("independent production digest %s, want %s", independentlyDerived, want)
	}
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if header.Nonce != 1 || header.PrevBlock != *chaincfg.RegressionNetParams.GenesisHash {
		t.Fatal("proof vector no longer extends the production regtest genesis at nonce 1")
	}
	if got := header.BlockHash(); got != independentlyDerived {
		t.Fatalf("production BlockHash %s, want %s", got, independentlyDerived)
	}
	block := btcutil.NewBlock(wire.NewMsgBlock(&header))
	if err := CheckProofOfWork(block, chaincfg.RegressionNetParams.PowLimit); err != nil {
		t.Fatalf("production proof vector rejected: %v", err)
	}
}
