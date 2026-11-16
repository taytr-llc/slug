// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/taytr-llc/slug/chaincfg/v2"
)

// loadSignedCheckpoints authenticates bounded local files before startup.
// They remain separate from --addcheckpoint and --nocheckpoints because a
// league statement must never change block acceptance or enable fast-add.
func loadSignedCheckpoints(paths []string, params *chaincfg.Params) ([]chaincfg.SignedCheckpoint, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if params == nil || len(params.CheckpointPublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("no valid checkpoint authority is compiled for this network")
	}
	checkpoints := make([]chaincfg.SignedCheckpoint, 0, len(paths))
	for _, path := range paths {
		checkpoint, err := readSignedCheckpoint(cleanAndExpandPath(path))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := checkpoint.Verify(params, params.CheckpointPublicKey); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Height < checkpoints[j].Height
	})
	for i := 1; i < len(checkpoints); i++ {
		if checkpoints[i-1].Height == checkpoints[i].Height {
			return nil, fmt.Errorf("duplicate signed checkpoint height %d", checkpoints[i].Height)
		}
	}
	return checkpoints, nil
}

func readSignedCheckpoint(path string) (chaincfg.SignedCheckpoint, error) {
	var checkpoint chaincfg.SignedCheckpoint
	file, err := os.Open(path)
	if err != nil {
		return checkpoint, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, chaincfg.MaxSignedCheckpointSize+1))
	if err != nil {
		return checkpoint, err
	}
	// json.Unmarshal trims outer whitespace before calling UnmarshalJSON;
	// enforce the full file limit here rather than relying on the codec.
	if len(data) > chaincfg.MaxSignedCheckpointSize {
		return checkpoint, fmt.Errorf("signed checkpoint file exceeds %d bytes", chaincfg.MaxSignedCheckpointSize)
	}
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}
