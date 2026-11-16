// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"crypto/ed25519"
	"fmt"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
)

// SignedCheckpointBurialDepth is the number of descendants required for a
// signed checkpoint to be locally mature under SLUG decision D62. The
// checkpoint block itself is not counted as one of these descendants.
const SignedCheckpointBurialDepth int32 = 60

// SignedCheckpointStatus describes a signed declaration relative to the
// current best chain. A declaration remains available even when the local
// chain is pending, has insufficient burial, or conflicts with it.
type SignedCheckpointStatus struct {
	SignedCheckpoint chaincfg.SignedCheckpoint

	// Status is pending when the best chain has not reached the declared
	// height, conflict when its hash differs, provisional when it matches
	// but has fewer than 60 descendants, and finalized when it matches
	// with at least 60 descendants. These are local observations; a reorg
	// can change them without revoking the signed declaration.
	Status string

	// LocalHash is the best-chain hash at the declared height, or nil if
	// that height has not yet been reached.
	LocalHash *chainhash.Hash

	// Descendants counts blocks after a matching checkpoint in the best
	// chain. It is zero when the checkpoint is pending or conflicting.
	Descendants int32
}

// SignedCheckpointSnapshot is a coherent view of signed declarations and the
// current best chain. All slices and hash pointers belong to the caller.
// Signed checkpoints are reports, not inputs to consensus or chain selection.
type SignedCheckpointSnapshot struct {
	BestHeight  int32
	BestHash    chainhash.Hash
	PublicKey   []byte
	Checkpoints []SignedCheckpointStatus
}

// prepareSignedCheckpoints verifies and copies caller-supplied declarations
// before chain construction. Unlike the existing unsigned Checkpoints field,
// these records must never be used to skip validation or prevent a reorg.
func prepareSignedCheckpoints(params *chaincfg.Params,
	checkpoints []chaincfg.SignedCheckpoint) ([]chaincfg.SignedCheckpoint, []byte, error) {

	if params == nil {
		return nil, nil, fmt.Errorf("signed checkpoint parameters are nil")
	}
	publicKey := append([]byte(nil), params.CheckpointPublicKey...)
	if len(publicKey) != 0 && len(publicKey) != ed25519.PublicKeySize {
		return nil, nil, fmt.Errorf("signed checkpoint public key must be %d bytes",
			ed25519.PublicKeySize)
	}
	if len(checkpoints) == 0 {
		return nil, publicKey, nil
	}
	if len(publicKey) == 0 {
		return nil, nil, fmt.Errorf("signed checkpoints require a configured public key")
	}

	verified := make([]chaincfg.SignedCheckpoint, len(checkpoints))
	var previousHeight int32
	for i, checkpoint := range checkpoints {
		if checkpoint.Height <= previousHeight {
			return nil, nil, fmt.Errorf("signed checkpoints must be sorted by increasing positive height")
		}
		checkpoint.Signature = append([]byte(nil), checkpoint.Signature...)
		if err := checkpoint.Verify(params, ed25519.PublicKey(publicKey)); err != nil {
			return nil, nil, fmt.Errorf("signed checkpoint at height %d: %w",
				checkpoint.Height, err)
		}
		verified[i] = checkpoint
		previousHeight = checkpoint.Height
	}

	return verified, publicKey, nil
}

// SignedCheckpointSnapshot returns the signed league declarations and their
// current local-chain status. It does not change chain selection, validation,
// or the immutable signed records, including after a conflicting reorg.
//
// This function is safe for concurrent access.
func (b *BlockChain) SignedCheckpointSnapshot() SignedCheckpointSnapshot {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	snapshot := SignedCheckpointSnapshot{
		BestHeight:  -1,
		PublicKey:   append([]byte(nil), b.checkpointPublicKey...),
		Checkpoints: make([]SignedCheckpointStatus, 0, len(b.signedCheckpoints)),
	}
	tip := b.bestChain.Tip()
	if tip != nil {
		snapshot.BestHeight = tip.height
		snapshot.BestHash = tip.hash
	}

	for _, checkpoint := range b.signedCheckpoints {
		checkpoint.Signature = append([]byte(nil), checkpoint.Signature...)
		status := SignedCheckpointStatus{
			SignedCheckpoint: checkpoint,
			Status:           "pending",
		}
		node := b.bestChain.NodeByHeight(checkpoint.Height)
		if node != nil {
			localHash := node.hash
			status.LocalHash = &localHash
			status.Status = "conflict"
			if localHash == checkpoint.Hash {
				status.Descendants = snapshot.BestHeight - checkpoint.Height
				status.Status = "provisional"
				if status.Descendants >= SignedCheckpointBurialDepth {
					status.Status = "finalized"
				}
			}
		}
		snapshot.Checkpoints = append(snapshot.Checkpoints, status)
	}

	return snapshot
}
