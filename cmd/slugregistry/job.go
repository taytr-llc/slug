// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func jobNode(command string, args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags, dbPath := commandFlags(command, stderr)
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Trusted node RPC configuration file (required; contains credentials)")
	var publicKey, proofPath, authorizationPath *string
	if command == "issue-job-node" {
		publicKey = flags.String("public-key", "", "Compressed secp256k1 public key, 66 hex characters (required)")
	} else {
		proofPath = flags.String("proof", "", "Untrusted version-2 proof JSON file (required)")
		authorizationPath = flags.String("authorization", "", "Public version-2 gym-key authorization JSON file (required)")
	}
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if *policyPath == "" || *rpcPath == "" {
		return errors.New("--policy and --rpc-config are required")
	}
	if publicKey != nil && *publicKey == "" {
		return errors.New("--public-key is required")
	}
	if proofPath != nil && (*proofPath == "" || *authorizationPath == "") {
		return errors.New("--proof and --authorization are required")
	}
	data, err := readBoundedFile(*policyPath, prospectnode.MaxPolicySize)
	if err != nil {
		return fmt.Errorf("read policy: %w", err)
	}
	policy, err := prospectnode.ParsePolicy(data)
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
	var proof prospect.Proof
	var authorization gymauth.Authorization
	if proofPath != nil {
		data, err = readBoundedFile(*proofPath, prospect.MaxProofSize)
		if err != nil {
			return fmt.Errorf("read proof: %w", err)
		}
		proof, err = prospect.ParseProof(data)
		if err != nil {
			return err
		}
		authorization, err = readAuthorization(*authorizationPath)
		if err != nil {
			return err
		}
	}
	// Only new issuance/admission creates a client. A historical duplicate can
	// be recovered during a node outage or after an operator policy change.
	var client *prospectnode.Client
	defer func() {
		if client != nil {
			client.Close()
		}
	}()
	connect := func() error {
		if client != nil {
			return nil
		}
		var err error
		client, err = prospectnode.NewClient(rpcConfig)
		if err != nil {
			return errors.New("could not initialize trusted node RPC client")
		}
		return nil
	}
	if command == "issue-job-node" {
		if err := connect(); err != nil {
			return err
		}
		resolved, err := client.Resolve(ctx, policy)
		if err != nil {
			return err
		}
		store, err := registry.Open(*dbPath, false)
		if err != nil {
			return err
		}
		job, issueErr := store.IssueJob(resolved, *publicKey, func() error {
			return client.RecheckContext(ctx, resolved)
		})
		if err := errors.Join(issueErr, store.Close()); err != nil {
			return err
		}
		return writeJSON(stdout, job)
	}
	store, err := registry.Open(*dbPath, false)
	if err != nil {
		return err
	}
	entry, inserted, recordErr := store.RecordJob(proof, authorization, func(resolved prospect.Context, proof prospect.Proof) error {
		if resolved.Version != policy.Version || resolved.Network != policy.Network ||
			!strings.EqualFold(resolved.Genesis, policy.Genesis) || resolved.DNAVersion != policy.DNAVersion ||
			!strings.EqualFold(resolved.ProspectTarget, policy.ProspectTarget) {
			return errors.New("issued job does not match the current prospect policy")
		}
		if err := connect(); err != nil {
			return err
		}
		return client.Recheck(ctx, resolved, proof)
	})
	if err := errors.Join(recordErr, store.Close()); err != nil {
		return err
	}
	source := "historical-receipt"
	if inserted {
		source = "trusted-node-snapshot"
	}
	return writeJSON(stdout, struct {
		ContextSource      string          `json:"context_source"`
		CurrentNodeChecked bool            `json:"current_node_checked"`
		Authentication     string          `json:"authentication"`
		BirthRule          string          `json:"birth_rule"`
		Inserted           bool            `json:"inserted"`
		Record             registry.Record `json:"record"`
	}{source, inserted, "gym-key-control", "job-issuance", inserted, entry})
}
