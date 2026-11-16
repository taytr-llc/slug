// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
)

func prepareWorkNode(args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags := flag.NewFlagSet("prepare-work-node", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jobPath := flags.String("job", "", "Public issued-job JSON file (required)")
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Trusted node RPC configuration file (required; contains credentials)")
	payout := flags.String("payout-address", "", "Public SLUG mainnet reward address (required)")
	extraNonce := flags.Uint64("extra-nonce", 0, "Coinbase extra nonce; deterministic default 0")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: slugprospect prepare-work-node [options]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *jobPath == "" || *policyPath == "" || *rpcPath == "" || *payout == "" {
		return errors.New("--job, --policy, --rpc-config and --payout-address are required")
	}
	data, err := readBoundedFile(*jobPath, prospectjob.MaxJobSize)
	if err != nil {
		return fmt.Errorf("read job: %w", err)
	}
	job, err := prospectjob.ParseJob(data)
	if err != nil {
		return err
	}
	if err := checkJobTime(job, time.Now()); err != nil {
		return err
	}
	data, err = readBoundedFile(*policyPath, prospectnode.MaxPolicySize)
	if err != nil {
		return fmt.Errorf("read policy: %w", err)
	}
	policy, err := prospectnode.ParsePolicy(data)
	if err != nil {
		return err
	}
	if policy.Network != job.Context.Network || policy.Genesis != job.Context.Genesis ||
		policy.DNAVersion != job.Context.DNAVersion || policy.ProspectTarget != job.Context.ProspectTarget {
		return errors.New("job does not match configured prospect policy")
	}
	data, err = readBoundedFile(*rpcPath, prospectnode.MaxRPCConfigSize)
	if err != nil {
		return errors.New("could not read RPC configuration")
	}
	config, err := prospectnode.ParseRPCConfig(data)
	if err != nil {
		return errors.New("invalid RPC configuration")
	}
	client, err := prospectnode.NewClient(config)
	if err != nil {
		return errors.New("could not initialize trusted node RPC client")
	}
	defer client.Close()
	template, err := client.ResolveTemplate(ctx, policy)
	if err != nil {
		return err
	}
	candidate, err := prospectwork.Build(job, template, *payout, *extraNonce)
	if err != nil {
		return err
	}
	if err := client.Proposal(ctx, template.Context, candidate.BlockHex); err != nil {
		return err
	}
	if err := checkJobTime(job, time.Now()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		ProposalAccepted bool                   `json:"proposal_accepted"`
		WorkVerified     bool                   `json:"work_verified"`
		Admitted         bool                   `json:"admitted"`
		Candidate        prospectwork.Candidate `json:"candidate"`
	}{true, false, false, candidate})
}

// This is only a client-side hint. The issuing authority alone decides whether
// a job is still eligible at admission; local wall time cannot grant freshness.
func checkJobTime(job prospectjob.Job, now time.Time) error {
	if now.UnixMilli() < job.IssuedAtMS || now.UnixMilli() >= job.ExpiresAtMS {
		return errors.New("job is outside its issuance window according to the local clock")
	}
	return nil
}
