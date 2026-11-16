// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func recordNode(args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags, dbPath := commandFlags("record-node", stderr)
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Trusted node RPC configuration file (required; contains credentials)")
	proofPath := flags.String("proof", "", "Untrusted proof JSON file (required)")
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if *policyPath == "" || *rpcPath == "" || *proofPath == "" {
		return errors.New("--policy, --rpc-config and --proof are required")
	}
	data, err := readBoundedFile(*policyPath, prospectnode.MaxPolicySize)
	if err != nil {
		return fmt.Errorf("read policy: %w", err)
	}
	policy, err := prospectnode.ParsePolicy(data)
	if err != nil {
		return err
	}
	data, err = readBoundedFile(*proofPath, prospect.MaxProofSize)
	if err != nil {
		return fmt.Errorf("read proof: %w", err)
	}
	proof, err := prospect.ParseProof(data)
	if err != nil {
		return err
	}
	data, err = readBoundedFile(*rpcPath, prospectnode.MaxRPCConfigSize)
	if err != nil {
		return fmt.Errorf("read RPC configuration: %w", err)
	}
	rpcConfig, err := prospectnode.ParseRPCConfig(data)
	if err != nil {
		return errors.New("invalid RPC configuration")
	}
	client, err := prospectnode.NewClient(rpcConfig)
	if err != nil {
		return errors.New("could not initialize trusted node RPC client")
	}
	defer client.Close()
	resolved, err := client.Resolve(ctx, policy)
	if err != nil {
		return err
	}
	store, err := registry.Open(*dbPath, false)
	if err != nil {
		return err
	}
	entry, inserted, recordErr := store.RecordChecked(resolved, proof, func() error {
		return client.Recheck(ctx, resolved, proof)
	})
	if err := errors.Join(recordErr, store.Close()); err != nil {
		return err
	}
	// The source label describes this call's node observation. The immutable
	// stored verification result remains conditional on its supplied context.
	return writeJSON(stdout, struct {
		ContextSource string          `json:"context_source"`
		Inserted      bool            `json:"inserted"`
		Record        registry.Record `json:"record"`
	}{"trusted-node-snapshot", inserted, entry})
}
