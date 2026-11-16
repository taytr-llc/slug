// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
)

func nodeCommand(command string, args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Trusted node RPC configuration file (required; contains credentials)")
	var proofPath *string
	if command != "context-node" {
		proofPath = flags.String("proof", "", "Untrusted proof JSON file (required)")
	}
	var authorizationPath *string
	if command == "verify-auth-node" {
		authorizationPath = flags.String("authorization", "", "Public gym-key authorization JSON file (required)")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: slugprospect %s [options]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *policyPath == "" || *rpcPath == "" {
		return errors.New("--policy and --rpc-config are required")
	}
	if proofPath != nil && *proofPath == "" {
		return errors.New("--proof is required")
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
	var proof prospect.Proof
	if proofPath != nil {
		data, err = readBoundedFile(*proofPath, prospect.MaxProofSize)
		if err != nil {
			return fmt.Errorf("read proof: %w", err)
		}
		proof, err = prospect.ParseProof(data)
		if err != nil {
			return err
		}
	}
	data, err = readBoundedFile(*rpcPath, prospectnode.MaxRPCConfigSize)
	if err != nil {
		return fmt.Errorf("read RPC configuration: %w", err)
	}
	rpcConfig, err := prospectnode.ParseRPCConfig(data)
	if err != nil {
		// A malformed field name or value could itself contain a secret.
		return errors.New("invalid RPC configuration")
	}
	client, err := prospectnode.NewClient(rpcConfig)
	if err != nil {
		return errors.New("could not initialize trusted node RPC client")
	}
	defer client.Close()
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if command == "context-node" {
		resolved, err := client.Resolve(ctx, policy)
		if err != nil {
			return err
		}
		return encoder.Encode(resolved)
	}
	var resolved prospect.Context
	var verified prospect.Verified
	authentication := ""
	if authorizationPath == nil {
		resolved, verified, err = client.Verify(ctx, policy, proof)
	} else {
		resolved, err = client.Resolve(ctx, policy)
		if err == nil {
			err = gymauth.VerifyAuthorization(resolved, proof, authorization)
		}
		if err == nil {
			verified, err = prospect.Verify(resolved, proof)
		}
		if err == nil {
			err = client.Recheck(ctx, resolved, proof)
		}
		authentication = "gym-key-control"
	}
	if err != nil {
		return err
	}
	return encoder.Encode(struct {
		ContextSource  string            `json:"context_source"`
		Authentication string            `json:"authentication,omitempty"`
		BirthVerified  *bool             `json:"birth_verified,omitempty"`
		Context        prospect.Context  `json:"context"`
		Verified       prospect.Verified `json:"verified"`
	}{"trusted-node-snapshot", authentication, unverifiedJobBirth(proof), resolved, verified})
}
