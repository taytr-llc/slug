//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectnode"
)

func TestWalletActivityRuntimeProfileRefusal(t *testing.T) {
	// Deliberately uninitialized owners prove refusal precedes any resource use.
	child := new(PoolChild)
	if _, e := child.BeginWalletActivity(context.Background(), "", ""); !errors.Is(e, gymstable.ErrProfile) {
		t.Fatal(e)
	}
	if _, e := child.WalletActivityDetail(context.Background(), "", "", ""); !errors.Is(e, gymstable.ErrProfile) {
		t.Fatal(e)
	}
	if _, e := new(walletRuntime).captureWalletActivity(context.Background(), prospectnode.RPCConfig{}); !errors.Is(e, ErrActivityUnavailable) {
		t.Fatal(e)
	}
}
