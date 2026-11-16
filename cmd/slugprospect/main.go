// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// slugprospect prepares public commitments and verifies prospect proofs offline.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/taytr-llc/slug/prospect"
)

const usage = "usage: slugprospect {commitment|verify} [options]"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "slugprospect:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, err := fmt.Fprintln(stdout, usage+"\nOffline tools; context is an explicit trust input, not independently authenticated.")
		return err
	case "commitment", "verify":
	default:
		return errors.New(usage)
	}

	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextPath := flags.String("context", "", "Trusted context JSON file (required)")
	var gymID, proofPath *string
	if command == "commitment" {
		gymID = flags.String("gym-id", "", "Public gym identifier, 64 hex characters (required)")
	} else {
		proofPath = flags.String("proof", "", "Untrusted proof JSON file (required)")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: slugprospect %s [options]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *contextPath == "" {
		return errors.New("--context is required")
	}
	if command == "commitment" && *gymID == "" {
		return errors.New("--gym-id is required")
	}
	if command == "verify" && *proofPath == "" {
		return errors.New("--proof is required")
	}
	data, err := readBoundedFile(*contextPath, prospect.MaxContextSize)
	if err != nil {
		return fmt.Errorf("read context: %w", err)
	}
	context, err := prospect.ParseContext(data)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if command == "commitment" {
		script, err := prospect.CommitmentScript(context, *gymID)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Version   uint32 `json:"version"`
			ScriptHex string `json:"script_hex"`
		}{Version: prospect.Version1, ScriptHex: hex.EncodeToString(script)})
	}

	data, err = readBoundedFile(*proofPath, prospect.MaxProofSize)
	if err != nil {
		return fmt.Errorf("read proof: %w", err)
	}
	proof, err := prospect.ParseProof(data)
	if err != nil {
		return err
	}
	verified, err := prospect.Verify(context, proof)
	if err != nil {
		return err
	}
	return encoder.Encode(verified)
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input file exceeds %d bytes", limit)
	}
	return data, nil
}
