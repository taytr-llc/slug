// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// slugregistryd serves one explicitly configured private local birth authority.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "slugregistryd:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("slugregistryd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dbPath := flags.String("db", "", "Existing schema-3 (v1) or schema-4 (v2) registry directory (required)")
	policyPath := flags.String("policy", "", "Explicit prospect policy JSON file (required)")
	rpcPath := flags.String("rpc-config", "", "Private trusted-node RPC configuration file (required)")
	servicePath := flags.String("service-config", "", "Local authority server configuration file (required)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: slugregistryd --db PATH --policy FILE --rpc-config FILE --service-config FILE")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *dbPath == "" || *policyPath == "" || *rpcPath == "" || *servicePath == "" {
		return errors.New("explicit database, policy, RPC and service configuration paths are required")
	}
	data, err := readBoundedFile(*policyPath, prospectnode.MaxPolicySize)
	if err != nil {
		return errors.New("could not read prospect policy")
	}
	policy, err := prospectnode.ParsePolicy(data)
	if err != nil {
		return errors.New("invalid prospect policy")
	}
	data, err = readBoundedFile(*rpcPath, prospectnode.MaxRPCConfigSize)
	if err != nil {
		return errors.New("could not read RPC configuration")
	}
	rpcConfig, err := prospectnode.ParseRPCConfig(data)
	if err != nil {
		return errors.New("invalid RPC configuration")
	}
	data, err = readBoundedFile(*servicePath, prospectservice.MaxServerConfigSize)
	if err != nil {
		return errors.New("could not read authority configuration")
	}
	serviceConfig, err := prospectservice.ParseServerConfig(data)
	if err != nil {
		return errors.New("invalid authority configuration")
	}
	node, err := prospectnode.NewClient(rpcConfig)
	if err != nil {
		return errors.New("could not initialize trusted node client")
	}
	defer node.Close()
	store, err := registry.Open(*dbPath, false)
	if err != nil {
		return errors.New("could not open existing birth registry")
	}
	server, err := prospectservice.NewServer(serviceConfig, policy, node, store)
	if err != nil {
		return errors.Join(err, store.Close())
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var outputErr error
	runErr := server.Run(ctx, func(origin string) {
		outputErr = json.NewEncoder(stdout).Encode(struct {
			Version   uint32 `json:"version"`
			Listening string `json:"listen"`
		}{serviceConfig.Version, origin})
		if outputErr != nil {
			cancel()
		}
	})
	// Run drains actual registry work, including an uncancellable Argon call,
	// before returning. Only then may the database lock be released.
	return errors.Join(runErr, outputErr, store.Close())
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	// Reject accidental pipes/devices before Open, which can block on a FIFO.
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("input is unreadable or exceeds its size bound")
	}
	return data, nil
}
