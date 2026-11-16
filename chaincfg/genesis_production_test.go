//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestProductionGenesisHashes(t *testing.T) {
	if wire.TestPoW {
		t.Fatal("default build selected test proof of work")
	}
	tests := []struct {
		params *Params
		want   string
	}{
		{&MainNetParams, "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"},
		{&TestNet4Params, "c5b61fa53d45e0940dd1de9c9505982b7308dc7ccfecb216d5cd0f697fdf2d69"},
	}
	for _, test := range tests {
		if got := test.params.GenesisBlock.BlockHash(); got.String() != test.want {
			t.Errorf("%s genesis hash %s, want %s", test.params.Name, got, test.want)
		}
	}
}
