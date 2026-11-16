//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/crypto/argon2"
)

func TestTestPoWGenesisHashes(t *testing.T) {
	if !wire.TestPoW {
		t.Fatal("slugtestpow build selected production proof of work")
	}
	tests := []struct {
		params *Params
		raw    []byte
	}{
		{&MainNetParams, genesisBlockBytes},
		{&RegressionNetParams, regTestGenesisBlockBytes},
		{&TestNet3Params, testNet3GenesisBlockBytes},
		{&TestNet4Params, testNet4GenesisBlockBytes},
		{&SimNetParams, simNetGenesisBlockBytes},
		{&SigNetParams, sigNetGenesisBlockBytes},
	}
	for _, test := range tests {
		t.Run(test.params.Name, func(t *testing.T) {
			// Derive the expected digest directly from the existing fixed
			// serialized genesis fixture and literal test parameters.
			header := test.raw[:80]
			digest := argon2.IDKey(header, header, 1, 64, 1, 32)
			var want chainhash.Hash
			copy(want[:], digest)
			if *test.params.GenesisHash != want {
				t.Errorf("configured genesis %s, want %s", test.params.GenesisHash, want)
			}
			if got := test.params.GenesisBlock.BlockHash(); got != want {
				t.Errorf("computed genesis %s, want %s", got, want)
			}
		})
	}
	if MainNetParams.GenesisHash.String() == "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd" {
		t.Fatal("test proof of work unexpectedly retained the production genesis identifier")
	}
}
