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

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func recordNode(command string, args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags, dbPath := commandFlags(command, stderr)
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Trusted node RPC configuration file (required; contains credentials)")
	proofPath := flags.String("proof", "", "Untrusted proof JSON file (required)")
	var authorizationPath *string
	if command == "record-auth-node" {
		authorizationPath = flags.String("authorization", "", "Public gym-key authorization JSON file (required)")
	}
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if *policyPath == "" || *rpcPath == "" || *proofPath == "" {
		return errors.New("--policy, --rpc-config and --proof are required")
	}
	if authorizationPath != nil && *authorizationPath == "" {
		return errors.New("--authorization is required")
	}
	var authorization gymauth.Authorization
	if authorizationPath != nil {
		var err error
		authorization, err = readAuthorization(*authorizationPath)
		if err != nil {
			return err
		}
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
	check := func() error {
		return client.Recheck(ctx, resolved, proof)
	}
	var entry registry.Record
	var inserted bool
	var recordErr error
	authentication := ""
	if authorizationPath == nil {
		entry, inserted, recordErr = store.RecordChecked(resolved, proof, check)
	} else {
		entry, inserted, recordErr = store.RecordAuthorizedChecked(resolved, proof, authorization, check)
		authentication = "gym-key-control"
	}
	if err := errors.Join(recordErr, store.Close()); err != nil {
		return err
	}
	// The source label describes this call's node observation. The immutable
	// stored verification result remains conditional on its supplied context.
	return writeJSON(stdout, struct {
		ContextSource  string          `json:"context_source"`
		Authentication string          `json:"authentication,omitempty"`
		Inserted       bool            `json:"inserted"`
		Record         registry.Record `json:"record"`
	}{"trusted-node-snapshot", authentication, inserted, entry})
}
