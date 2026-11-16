//go:build slugtestpow

package integration

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/crypto/argon2"
)

// chainTipsFixturesForProfile preserves the canonical transaction payloads and
// fork topology, but derives parent hashes and nonces with an independent Argon2
// call. It does not use BlockHash to produce the expected fixture identities.
func chainTipsFixturesForProfile(t *testing.T, fixtures []string) []string {
	t.Helper()
	independentHash := func(header *wire.BlockHeader) chainhash.Hash {
		var encoded bytes.Buffer
		if err := header.Serialize(&encoded); err != nil {
			t.Fatal(err)
		}
		data := encoded.Bytes()
		// The test-only profile fixes memory at 64 KiB; all other Argon2
		// parameters match production. Keep this oracle independent of wire.
		return chainhash.Hash(argon2.IDKey(data, data, 1, 64, 1, 32))
	}
	params := &chaincfg.RegressionNetParams
	genesisHash := independentHash(&params.GenesisBlock.Header)
	if genesisHash != *params.GenesisHash {
		t.Fatal("test profile genesis differs from independent Argon2 calculation")
	}
	parents := []int{-1, 0, 1, 2, 0, 4, 5, 6, 5}
	if len(fixtures) != len(parents) {
		t.Fatal("unexpected chain tip fixture count")
	}
	hashes := make([]chainhash.Hash, len(fixtures))
	for i, fixture := range fixtures {
		block := getBlockFromString(t, fixture).MsgBlock()
		block.Header.PrevBlock = genesisHash
		if parents[i] >= 0 {
			block.Header.PrevBlock = hashes[parents[i]]
		}
		target := blockchain.CompactToBig(block.Header.Bits)
		for nonce := uint32(0); ; nonce++ {
			block.Header.Nonce = nonce
			hashes[i] = independentHash(&block.Header)
			if blockchain.HashToBig(&hashes[i]).Cmp(target) <= 0 {
				break
			}
			if nonce == math.MaxUint32 {
				t.Fatal("no proof of work for chain tip fixture")
			}
		}
		if block.BlockHash() != hashes[i] {
			t.Fatal("test profile hash differs from independent Argon2 calculation")
		}
		var encoded bytes.Buffer
		if err := block.Serialize(&encoded); err != nil {
			t.Fatal(err)
		}
		fixtures[i] = hex.EncodeToString(encoded.Bytes())
	}
	return fixtures
}
