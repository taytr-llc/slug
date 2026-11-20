//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospect

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"runtime"
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
func TestTryHashHeaderCancellationJoinsActualWork(t *testing.T) {
	f := proofFixtures(t)[0]
	raw, _ := hex.DecodeString(f.Proof.Header)
	var h wire.BlockHeader
	h.Deserialize(bytes.NewReader(raw))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		var got chainhash.Hash
		var e error
		for {
			got, e = TryHashHeader(ctx, h)
			if !errors.Is(e, ErrHeaderHashBusy) {
				break
			}
			runtime.Gosched()
		}
		if got != (chainhash.Hash{}) && e != nil {
			done <- errors.New("cancel returned a hash")
			return
		}
		done <- e
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if !hashMu.TryLock() {
			break
		}
		hashMu.Unlock()
		select {
		case e := <-done:
			t.Fatal("hash finished before cancellation probe", e)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("hash never acquired existing gate")
		}
		runtime.Gosched()
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled hash did not join")
	}
	if !hashMu.TryLock() {
		t.Fatal("returned with hash gate active")
	}
	hashMu.Unlock()
}
