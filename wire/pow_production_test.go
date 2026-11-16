//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import "testing"

func TestBlockHashMemoizationPreservesArgon2(t *testing.T) {
	if TestPoW || powArgon2Memory != 256*1024 {
		t.Fatal("default build must use production proof-of-work parameters")
	}
	testBlockHashMemoizationPreservesArgon2(t, 256*1024)
}
