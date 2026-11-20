//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospect

import (
	"errors"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"testing"
)

func TestTryHashHeaderCheapProfileFirst(t *testing.T) {
	hashMu.Lock()
	defer hashMu.Unlock()
	got, e := TryHashHeader(nil, wire.BlockHeader{})
	if !errors.Is(e, ErrHeaderHashProfile) || got != (chainhash.Hash{}) {
		t.Fatal("profile did not refuse before context/gate", e)
	}
}
