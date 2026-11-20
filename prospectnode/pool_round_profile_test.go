//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectnode

import (
	"context"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospect"
)

func TestPoolRoundCheapProfileRefusesBeforeNetwork(t *testing.T) {
	// The nil client makes any attempted transport access fail the test.
	var client *Client
	got, err := client.ObservePoolBlock(context.Background(), prospect.Context{}, prospect.Proof{}, "")
	if err == nil || !strings.Contains(err.Error(), "slugtestpow") || got != (PoolBlockObservation{}) {
		t.Fatal("cheap-profile observation was not refused before network", got, err)
	}
}
