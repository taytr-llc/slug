//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import "testing"

func TestBlockHashMemoizationPreservesArgon2(t *testing.T) {
	if !TestPoW || powArgon2Memory != 64 {
		t.Fatal("slugtestpow build must use its explicit test parameters")
	}
	testBlockHashMemoizationPreservesArgon2(t, 64)
}
