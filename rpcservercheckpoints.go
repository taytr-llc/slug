// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/ed25519"
	"encoding/hex"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
)

// handleGetCheckpointInfo reports signed statements and any local conflicts.
// It has no mutation path into chain selection, validation, or the block index.
func handleGetCheckpointInfo(s *rpcServer, _ interface{}, _ <-chan struct{}) (interface{}, error) {
	return checkpointInfoResult(s.cfg.Chain.SignedCheckpointSnapshot()), nil
}

func checkpointInfoResult(snapshot blockchain.SignedCheckpointSnapshot) *btcjson.GetCheckpointInfoResult {
	result := &btcjson.GetCheckpointInfoResult{
		Mode:                "report-only",
		AuthorityConfigured: len(snapshot.PublicKey) == ed25519.PublicKeySize,
		PublicKey:           hex.EncodeToString(snapshot.PublicKey),
		RequiredDescendants: blockchain.SignedCheckpointBurialDepth,
		BestHeight:          snapshot.BestHeight,
		BestHash:            snapshot.BestHash.String(),
		Checkpoints:         make([]btcjson.SignedCheckpointResult, 0, len(snapshot.Checkpoints)),
	}
	for _, status := range snapshot.Checkpoints {
		checkpoint := status.SignedCheckpoint
		entry := btcjson.SignedCheckpointResult{
			Version:     checkpoint.Version,
			Network:     uint32(checkpoint.Network),
			Genesis:     checkpoint.GenesisHash.String(),
			Height:      checkpoint.Height,
			Hash:        checkpoint.Hash.String(),
			Signature:   hex.EncodeToString(checkpoint.Signature),
			Status:      status.Status,
			Descendants: status.Descendants,
		}
		if status.LocalHash != nil {
			entry.LocalHash = status.LocalHash.String()
		}
		if status.Status == "conflict" {
			result.Conflict = true
		}
		result.Checkpoints = append(result.Checkpoints, entry)
	}
	return result
}
