//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospect

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestTryHashHeaderIndependentAndSharedGate(t *testing.T) {
	for _, f := range proofFixtures(t) {
		raw, e := hex.DecodeString(f.Proof.Header)
		if e != nil {
			t.Fatal(e)
		}
		var h wire.BlockHeader
		if h.Deserialize(bytes.NewReader(raw)) != nil {
			t.Fatal("header")
		}
		hashMu.Lock()
		got, e := TryHashHeader(context.Background(), h)
		hashMu.Unlock()
		if !errors.Is(e, ErrHeaderHashBusy) || got != (chainhash.Hash{}) {
			t.Fatal("separate or waiting hash gate", e)
		}
		got, e = TryHashHeader(context.Background(), h)
		if e != nil || got.String() != f.Expected.WorkHash {
			t.Fatal("independent normal hash changed", got, e)
		}
	}
}

// The second Err call is the last cancellation check under the shared gate
// before synchronous hashing. Hold that exact boundary instead of polling a
// gate that may be released before the observer is scheduled (or hit a cache).
// The captured nil result linearizes before the test cancels the parent.
type headerCancellationContext struct {
	context.Context
	calls            atomic.Int32
	entered, proceed chan struct{}
}

func (c *headerCancellationContext) Err() error {
	e := c.Context.Err()
	if c.calls.Add(1) == 2 {
		close(c.entered)
		<-c.proceed
	}
	return e
}

var cancellationNonce atomic.Uint32

func TestTryHashHeaderCancellationJoinsActualWork(t *testing.T) {
	f := proofFixtures(t)[0]
	raw, _ := hex.DecodeString(f.Proof.Header)
	var h wire.BlockHeader
	h.Deserialize(bytes.NewReader(raw))
	// This test has no expected digest; avoid the exact-header cache warmed by
	// the independent fixture test, including repeated tests in one process.
	h.Nonce = 0x73310000 + cancellationNonce.Add(1)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &headerCancellationContext{Context: parent, entered: make(chan struct{}), proceed: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		got, e := TryHashHeader(ctx, h)
		if got != (chainhash.Hash{}) && e != nil {
			done <- errors.New("cancel returned a hash")
			return
		}
		done <- e
	}()
	select {
	case <-ctx.entered:
	case e := <-done:
		t.Fatal("hash failed before owned gate", e)
	case <-time.After(2 * time.Second):
		t.Fatal("hash never acquired existing gate")
	}
	held := !hashMu.TryLock()
	if !held {
		hashMu.Unlock()
	}
	cancel()
	close(ctx.proceed)
	select {
	case e := <-done:
		if !held || !errors.Is(e, context.Canceled) {
			t.Fatal("owned synchronous cancellation", held, e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled hash did not join")
	}
	if !hashMu.TryLock() {
		t.Fatal("returned with hash gate active")
	}
	hashMu.Unlock()
}
