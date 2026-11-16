// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

// TestSlugBlockSubsidy pins the SLUG emission schedule (D32, change
// set 2): a 10 SLUG starting subsidy that halves every 1,050,000
// blocks on the SLUG main network.
func TestSlugBlockSubsidy(t *testing.T) {
	params := &chaincfg.MainNetParams

	if params.SubsidyReductionInterval != 1050000 {
		t.Fatalf("SubsidyReductionInterval = %d, want 1050000",
			params.SubsidyReductionInterval)
	}

	tests := []struct {
		height int32
		want   int64
	}{
		{0, 10 * btcutil.SatoshiPerBitcoin},       // genesis era
		{1049999, 10 * btcutil.SatoshiPerBitcoin}, // last block before halving
		{1050000, 5 * btcutil.SatoshiPerBitcoin},  // first halving
		{2100000, 250000000},                      // second halving: 2.5 SLUG
		{30450000, 1},                             // last nonzero subsidy era
		{31500000, 0},                             // subsidy exhausted
	}
	for _, test := range tests {
		got := CalcBlockSubsidy(test.height, params)
		if got != test.want {
			t.Errorf("CalcBlockSubsidy(%d) = %d, want %d",
				test.height, got, test.want)
		}
	}
}
