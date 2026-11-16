//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

func init() {
	// Reduced-cost Argon2 produces different identifiers even for unchanged
	// genesis headers. Params point to these variables, so update all of them
	// before any caller can create a test chain. This file never enters a
	// production build; the production genesis constants remain unchanged.
	genesisHash = genesisBlock.BlockHash()
	regTestGenesisHash = regTestGenesisBlock.BlockHash()
	testNet3GenesisHash = testNet3GenesisBlock.BlockHash()
	testNet4GenesisHash = testNet4GenesisBlock.BlockHash()
	simNetGenesisHash = simNetGenesisBlock.BlockHash()
	sigNetGenesisHash = sigNetGenesisBlock.BlockHash()
}
