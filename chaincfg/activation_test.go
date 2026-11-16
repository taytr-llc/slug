// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package chaincfg

import (
	"testing"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestPhaseAActivationParameters(t *testing.T) {
	p := &MainNetParams
	if p.BIP0034Height != 62 || p.BIP0065Height != 62 || p.BIP0066Height != 62 || p.BIP0034Hash != nil {
		t.Fatal("Phase-A buried activation must preserve history through61 and retain BIP30 checks")
	}
	for _, id := range []uint32{DeploymentCSV, DeploymentSegwit, DeploymentTaproot} {
		if p.Deployments[id].EffectiveAlwaysActiveHeight() != 62 {
			t.Fatalf("deployment %d activation changed", id)
		}
	}
	if !wire.TestPoW && p.GenesisHash.String() != "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd" {
		t.Fatal("activation changed production genesis identity")
	}
	if uint32(p.Net) != 0x534c5547 || p.PowLimitBits != 0x207fffff || p.CoinbaseMaturity != 100 || p.SubsidyReductionInterval != 1050000 || p.TargetTimePerBlock != 2*time.Minute || len(p.Checkpoints) != 0 {
		t.Fatal("activation changed unrelated chain parameters or added enforced checkpoints")
	}
}
