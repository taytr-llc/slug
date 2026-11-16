// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
)

func serviceCommand(command string, args []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("service-config", "", "Private authority client configuration file (required)")
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	var publicKey, jobPath, proofPath, authPath *string
	if command == "issue-job-service" {
		publicKey = flags.String("public-key", "", "Compressed public gym key, 66 hex characters (required)")
	} else {
		jobPath = flags.String("job", "", "Expected public issued-job JSON file (required)")
		proofPath = flags.String("proof", "", "Public version-2 work proof file (required)")
		authPath = flags.String("authorization", "", "Public version-2 authorization file (required)")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: slugregistry %s [options]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" || *policyPath == "" {
		return errors.New("--service-config and --policy are required; positional arguments are not accepted")
	}
	if publicKey != nil && *publicKey == "" {
		return errors.New("--public-key is required")
	}
	if jobPath != nil && (*jobPath == "" || *proofPath == "" || *authPath == "") {
		return errors.New("--job, --proof and --authorization are required")
	}
	data, err := readBoundedFile(*configPath, prospectservice.MaxClientConfigSize)
	if err != nil {
		return errors.New("could not read service client configuration")
	}
	config, err := prospectservice.ParseClientConfig(data)
	if err != nil {
		return errors.New("invalid service client configuration")
	}
	data, err = readBoundedFile(*policyPath, prospectnode.MaxPolicySize)
	if err != nil {
		return errors.New("could not read prospect policy")
	}
	policy, err := prospectnode.ParsePolicy(data)
	if err != nil {
		return err
	}
	client, err := prospectservice.NewClient(config, policy)
	if err != nil {
		return err
	}
	defer client.Close()
	if publicKey != nil {
		job, err := client.IssueJob(ctx, *publicKey)
		if err != nil {
			return err
		}
		return writeJSON(stdout, job)
	}
	data, err = readBoundedFile(*jobPath, prospectjob.MaxJobSize)
	if err != nil {
		return fmt.Errorf("read job: %w", err)
	}
	job, err := prospectjob.ParseJob(data)
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
	authorization, err := readAuthorization(*authPath)
	if err != nil {
		return err
	}
	result, err := client.Submit(ctx, job, proof, authorization)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}
