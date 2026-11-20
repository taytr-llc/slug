// Copyright (c) 2026 The SLUG developers. ISC license.
package blockchain

import (
	"errors"
	"math"
	"sync"
	"testing"
)

func TestBestObservationChainTransitions(t *testing.T) {
	chain, params := fullBlockHeaderChain(t)
	read := func() BestObservation {
		t.Helper()
		o, err := chain.BestObservation()
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	genesis := read()
	if genesis.Revision != 1 || genesis.Height != 0 || genesis.Hash != *params.GenesisHash {
		t.Fatal("new chain observation is not genesis revision1")
	}
	if read() != genesis {
		t.Fatal("read changed observation")
	}
	a1 := newNativeTestBlock(t, params, nativeTestGenesis(params), 0)
	a2 := newNativeTestBlock(t, params, a1, 0)
	acceptFullBlock(t, chain, a1, true)
	first := read()
	acceptFullBlock(t, chain, a2, true)
	second := read()
	if first.Revision != 2 || second.Revision != 3 || first.Hash != *a1.Hash() || second.Hash != *a2.Hash() {
		t.Fatal("connect did not advance exact tip/revision")
	}
	if _, _, err := chain.ProcessBlock(a2, BFNone); err == nil {
		t.Fatal("duplicate accepted")
	}
	if read() != second {
		t.Fatal("rejected duplicate changed observation")
	}
	if err := chain.InvalidateBlock(a2.Hash()); err != nil {
		t.Fatal(err)
	}
	returned := read()
	if returned.Hash != first.Hash || returned.Height != first.Height || returned.Revision != 4 {
		t.Fatal("same-hash return failed to expose ABA")
	}
	if err := chain.ReconsiderBlock(a2.Hash()); err != nil {
		t.Fatal(err)
	}
	if o := read(); o.Hash != second.Hash || o.Revision != 5 {
		t.Fatal("reconsider revision")
	}

	// Real detach/attach writers race atomic readers. Busy is explicit, never
	// a mixed hash/height/revision or a wait while a writer owns the chain.
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			o, err := chain.BestObservation()
			if errors.Is(err, ErrObservationBusy) {
				continue
			}
			if err != nil {
				t.Error(err)
				return
			}
			if o.Revision < 5 || (o.Revision%2 == 0 && (o.Hash != first.Hash || o.Height != 1)) ||
				(o.Revision%2 == 1 && (o.Hash != second.Hash || o.Height != 2)) {
				t.Error("mixed concurrent observation")
				return
			}
		}
	}()
	for i := 0; i < 4; i++ {
		if err := chain.InvalidateBlock(a2.Hash()); err != nil {
			t.Error(err)
			break
		}
		if err := chain.ReconsiderBlock(a2.Hash()); err != nil {
			t.Error(err)
			break
		}
	}
	close(done)
	wg.Wait()

	// Counter exhaustion is deliberately independent of consensus success.
	chain.chainLock.Lock()
	chain.stateLock.Lock()
	chain.observationRevision = math.MaxUint64
	chain.stateLock.Unlock()
	chain.chainLock.Unlock()
	if read().Revision != math.MaxUint64 {
		t.Fatal("last revision unavailable early")
	}
	a3 := newNativeTestBlock(t, params, a2, 0)
	acceptFullBlock(t, chain, a3, true)
	if chain.BestSnapshot().Hash != *a3.Hash() {
		t.Fatal("observation exhaustion stopped chain")
	}
	if _, err := chain.BestObservation(); !errors.Is(err, ErrObservationUnavailable) {
		t.Fatal("revision wrapped")
	}
	if err := chain.InvalidateBlock(a3.Hash()); err != nil {
		t.Fatal(err)
	}
	if chain.BestSnapshot().Hash != *a2.Hash() {
		t.Fatal("exhaustion stopped disconnect")
	}
	if _, err := chain.BestObservation(); !errors.Is(err, ErrObservationUnavailable) {
		t.Fatal("exhaustion was not sticky")
	}
}

func TestBestObservationNeverWaitsOnWriter(t *testing.T) {
	chain := &BlockChain{stateSnapshot: &BestState{}, observationRevision: 1}
	chain.chainLock.Lock()
	_, err := chain.BestObservation()
	chain.chainLock.Unlock()
	if !errors.Is(err, ErrObservationBusy) {
		t.Fatal("chain writer did not refuse observation")
	}
	chain.stateLock.Lock()
	_, err = chain.BestObservation()
	chain.stateLock.Unlock()
	if !errors.Is(err, ErrObservationBusy) {
		t.Fatal("state writer did not refuse observation")
	}
	var absent *BlockChain
	if _, err = absent.BestObservation(); !errors.Is(err, ErrObservationUnavailable) {
		t.Fatal("nil owner")
	}
}
