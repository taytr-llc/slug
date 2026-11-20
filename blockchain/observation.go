// Copyright (c) 2026 The SLUG developers. ISC license.
package blockchain

import (
	"errors"
	"math"

	"github.com/taytr-llc/slug/chainhash/v2"
)

var (
	ErrObservationBusy        = errors.New("chain observation busy")
	ErrObservationUnavailable = errors.New("chain observation unavailable")
)

// BestObservation is an owned atomic observation, not a finality declaration.
// Revision is process-local and must be paired with the RPC lifetime identity.
type BestObservation struct {
	Hash     chainhash.Hash
	Height   int32
	Revision uint64
}

// advanceObservationLocked tracks every best-state replacement. The caller
// holds stateLock. Exhaustion disables only observations, never chain updates.
func (b *BlockChain) advanceObservationLocked() {
	if b.observationExhausted {
		return
	}
	if b.observationRevision == math.MaxUint64 {
		b.observationExhausted = true
		return
	}
	b.observationRevision++
}

// BestObservation returns one current tip/revision without waiting for a chain
// mutation. Its lock order matches consensus writers. A caller must retry busy
// outside its own authority fences; an older BestSnapshot is not a substitute.
func (b *BlockChain) BestObservation() (BestObservation, error) {
	if b == nil {
		return BestObservation{}, ErrObservationUnavailable
	}
	if !b.chainLock.TryRLock() {
		return BestObservation{}, ErrObservationBusy
	}
	defer b.chainLock.RUnlock()
	if !b.stateLock.TryRLock() {
		return BestObservation{}, ErrObservationBusy
	}
	defer b.stateLock.RUnlock()
	if b.observationExhausted || b.observationRevision == 0 || b.stateSnapshot == nil || b.stateSnapshot.Height < 0 {
		return BestObservation{}, ErrObservationUnavailable
	}
	return BestObservation{b.stateSnapshot.Hash, b.stateSnapshot.Height, b.observationRevision}, nil
}
