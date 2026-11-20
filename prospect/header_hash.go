// Copyright (c) 2026 The SLUG developers. ISC license.
package prospect

import (
	"context"
	"errors"
	"fmt"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var (
	ErrHeaderHashBusy    = errors.New("prospect header hash busy")
	ErrHeaderHashProfile = errors.New("header hashing requires production PoW; slugtestpow is unsupported")
)

// TryHashHeader shares Verify's existing Argon arena gate. It proves neither
// source provenance, canonicality nor target satisfaction. The value is owned;
// a running hash is joined synchronously even when cancellation arrives.
func TryHashHeader(ctx context.Context, header wire.BlockHeader) (chainhash.Hash, error) {
	if wire.TestPoW {
		return chainhash.Hash{}, ErrHeaderHashProfile
	}
	if ctx == nil {
		return chainhash.Hash{}, fmt.Errorf("nil header hash context")
	}
	if e := ctx.Err(); e != nil {
		return chainhash.Hash{}, e
	}
	if !hashMu.TryLock() {
		return chainhash.Hash{}, ErrHeaderHashBusy
	}
	defer hashMu.Unlock()
	if e := ctx.Err(); e != nil {
		return chainhash.Hash{}, e
	}
	hash := header.BlockHash()
	if e := ctx.Err(); e != nil {
		return chainhash.Hash{}, e
	}
	return hash, nil
}
