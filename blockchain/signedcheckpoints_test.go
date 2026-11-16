// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
)

// signedCheckpointFixture signs synthetic block identities with the published
// RFC 8032 test seed. It never uses or creates an operator signing key.
func signedCheckpointFixture(t *testing.T, height int32,
	hash chainhash.Hash) (chaincfg.Params, chaincfg.SignedCheckpoint) {

	t.Helper()
	seed, err := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	if err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(seed)
	params := chaincfg.MainNetParams
	params.CheckpointPublicKey = append([]byte(nil), key.Public().(ed25519.PublicKey)...)
	checkpoint := chaincfg.SignedCheckpoint{
		Version:     chaincfg.SignedCheckpointVersion,
		Network:     params.Net,
		GenesisHash: *params.GenesisHash,
		Height:      height,
		Hash:        hash,
	}
	message, err := checkpoint.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.Signature = ed25519.Sign(key, message)
	return params, checkpoint
}

// syntheticCheckpointNodes creates only the in-memory parent/height/hash
// relationships observed by the report. These are not valid mined blocks and
// do not exercise or bypass production validation.
func syntheticCheckpointNodes(parent *blockNode, count int, branch byte) []*blockNode {
	nodes := make([]*blockNode, count)
	for i := range nodes {
		height := int32(0)
		if parent != nil {
			height = parent.height + 1
		}
		node := &blockNode{
			parent: parent,
			height: height,
			hash: chainhash.Hash{
				branch, byte(height), byte(height >> 8),
				byte(height >> 16), byte(height >> 24),
			},
		}
		nodes[i] = node
		parent = node
	}
	return nodes
}

func TestPrepareSignedCheckpoints(t *testing.T) {
	params, first := signedCheckpointFixture(t, 1, chainhash.Hash{1})
	_, second := signedCheckpointFixture(t, 2, chainhash.Hash{2})

	t.Run("immutable copies", func(t *testing.T) {
		params := params
		params.CheckpointPublicKey = append([]byte(nil), params.CheckpointPublicKey...)
		first := first
		first.Signature = append([]byte(nil), first.Signature...)
		input := []chaincfg.SignedCheckpoint{first, second}
		verified, publicKey, err := prepareSignedCheckpoints(&params, input)
		if err != nil {
			t.Fatal(err)
		}
		params.CheckpointPublicKey[0] ^= 1
		input[0].Signature[0] ^= 1
		input[0].Hash[0] ^= 1
		if err := verified[0].Verify(&params, ed25519.PublicKey(publicKey)); err != nil {
			t.Fatalf("caller mutation changed verified declaration: %v", err)
		}
		if verified[0].Hash != first.Hash {
			t.Fatal("input hash mutation reached stored declaration")
		}
	})

	tests := []struct {
		name        string
		publicKey   []byte
		checkpoints []chaincfg.SignedCheckpoint
		wantError   bool
	}{
		{"unset", nil, nil, false},
		{"configured without records", params.CheckpointPublicKey, nil, false},
		{"malformed empty key", []byte{1}, nil, true},
		{"missing authority", nil, []chaincfg.SignedCheckpoint{first}, true},
		{"ordered", params.CheckpointPublicKey, []chaincfg.SignedCheckpoint{first, second}, false},
		{"duplicate", params.CheckpointPublicKey, []chaincfg.SignedCheckpoint{first, first}, true},
		{"out of order", params.CheckpointPublicKey, []chaincfg.SignedCheckpoint{second, first}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := params
			params.CheckpointPublicKey = test.publicKey
			_, _, err := prepareSignedCheckpoints(&params, test.checkpoints)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
		})
	}

	t.Run("signature checked", func(t *testing.T) {
		modified := first
		modified.Hash[0] ^= 1
		if _, _, err := prepareSignedCheckpoints(&params,
			[]chaincfg.SignedCheckpoint{modified}); err == nil {
			t.Fatal("accepted altered signed block hash")
		}
	})
	t.Run("network checked", func(t *testing.T) {
		params := params
		params.Net++
		if _, _, err := prepareSignedCheckpoints(&params,
			[]chaincfg.SignedCheckpoint{first}); err == nil {
			t.Fatal("accepted checkpoint for another network")
		}
	})
}

func TestSignedCheckpointSnapshot(t *testing.T) {
	nodes := syntheticCheckpointNodes(nil, 64, 1)
	params, checkpoint := signedCheckpointFixture(t, 3, nodes[3].hash)
	verified, publicKey, err := prepareSignedCheckpoints(&params,
		[]chaincfg.SignedCheckpoint{checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	chain := &BlockChain{
		bestChain:           newChainView(nodes[0]),
		signedCheckpoints:   verified,
		checkpointPublicKey: publicKey,
	}

	for _, test := range []struct {
		height      int32
		status      string
		descendants int32
	}{
		{0, "pending", 0},
		{3, "provisional", 0},
		{62, "provisional", 59},
		{63, "finalized", 60},
	} {
		chain.bestChain.SetTip(nodes[test.height])
		snapshot := chain.SignedCheckpointSnapshot()
		if snapshot.BestHeight != test.height || snapshot.BestHash != nodes[test.height].hash {
			t.Fatalf("incorrect best tip in snapshot: %+v", snapshot)
		}
		status := snapshot.Checkpoints[0]
		if status.Status != test.status || status.Descendants != test.descendants {
			t.Fatalf("height %d: status = %s/%d, want %s/%d", test.height,
				status.Status, status.Descendants, test.status, test.descendants)
		}
		if test.status == "pending" {
			if status.LocalHash != nil {
				t.Fatal("pending checkpoint has a local hash")
			}
		} else if status.LocalHash == nil || *status.LocalHash != checkpoint.Hash {
			t.Fatal("matching checkpoint has the wrong local hash")
		}
	}

	// A reorganization is allowed to cross a signed declaration. The
	// declaration is retained, but the report exposes the conflict.
	fork := syntheticCheckpointNodes(nodes[2], 62, 2)
	chain.bestChain.SetTip(fork[len(fork)-1])
	snapshot := chain.SignedCheckpointSnapshot()
	status := snapshot.Checkpoints[0]
	if status.Status != "conflict" || status.LocalHash == nil ||
		*status.LocalHash != fork[0].hash || status.Descendants != 0 {
		t.Fatalf("conflicting fork not reported: %+v", status)
	}
	if status.SignedCheckpoint.Hash != checkpoint.Hash ||
		!bytes.Equal(status.SignedCheckpoint.Signature, checkpoint.Signature) {
		t.Fatal("conflicting reorg replaced the signed declaration")
	}
	if chain.HasCheckpoints() || chain.LatestCheckpoint() != nil ||
		!chain.VerifyCheckpoint(checkpoint.Height, &fork[0].hash) {
		t.Fatal("signed reporting altered existing checkpoint enforcement")
	}
	if chain.bestChain.Tip() != fork[len(fork)-1] {
		t.Fatal("reporting a conflict changed the selected best chain")
	}

	// Returning to the signed chain restores the local observation without
	// replacing the official signed record or tracking a mutable finality bit.
	chain.bestChain.SetTip(nodes[63])
	snapshot = chain.SignedCheckpointSnapshot()
	if snapshot.Checkpoints[0].Status != "finalized" {
		t.Fatal("return to the signed chain was not reported")
	}
	snapshot.PublicKey[0] ^= 1
	snapshot.Checkpoints[0].SignedCheckpoint.Signature[0] ^= 1
	snapshot.Checkpoints[0].SignedCheckpoint.Hash[0] ^= 1
	(*snapshot.Checkpoints[0].LocalHash)[0] ^= 1
	again := chain.SignedCheckpointSnapshot()
	if again.Checkpoints[0].Status != "finalized" ||
		again.Checkpoints[0].SignedCheckpoint.Hash != checkpoint.Hash ||
		*again.Checkpoints[0].LocalHash != checkpoint.Hash ||
		!bytes.Equal(again.PublicKey, publicKey) ||
		!bytes.Equal(again.Checkpoints[0].SignedCheckpoint.Signature, checkpoint.Signature) {
		t.Fatal("mutating a returned snapshot changed chain state")
	}
}

func TestSignedCheckpointSnapshotConcurrentReorg(t *testing.T) {
	nodes := syntheticCheckpointNodes(nil, 64, 1)
	fork := syntheticCheckpointNodes(nodes[2], 61, 2)
	params, checkpoint := signedCheckpointFixture(t, 3, nodes[3].hash)
	verified, publicKey, err := prepareSignedCheckpoints(&params,
		[]chaincfg.SignedCheckpoint{checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	chain := &BlockChain{
		bestChain:           newChainView(nodes[63]),
		signedCheckpoints:   verified,
		checkpointPublicKey: publicKey,
	}

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 1000; i++ {
			tip := nodes[63]
			if i%2 == 0 {
				tip = fork[len(fork)-1]
			}
			chain.chainLock.Lock()
			chain.bestChain.SetTip(tip)
			chain.chainLock.Unlock()
		}
	}()
	for i := 0; i < 1000; i++ {
		snapshot := chain.SignedCheckpointSnapshot()
		status := snapshot.Checkpoints[0]
		wantStatus := "finalized"
		wantLocalHash := nodes[3].hash
		if snapshot.BestHash == fork[len(fork)-1].hash {
			wantStatus = "conflict"
			wantLocalHash = fork[0].hash
		}
		if snapshot.BestHeight != 63 || status.Status != wantStatus ||
			status.LocalHash == nil || *status.LocalHash != wantLocalHash {
			t.Errorf("snapshot mixed observations across reorg: %+v", snapshot)
			break
		}
	}
	workers.Wait()
}
