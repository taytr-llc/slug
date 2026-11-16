// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// slugregistry stores verified prospect evidence in an explicit local database.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
)

const usage = "usage: slugregistry {init|record|get|list|export} [options]"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "slugregistry:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(stdout, usage+"\nLocal evidence storage; records establish neither ownership nor fighter birth.")
		return err
	case "init":
		return initialize(args[1:], stdout, stderr)
	case "record":
		return record(args[1:], stdout, stderr)
	case "get", "list", "export":
		return read(args[0], args[1:], stdout, stderr)
	default:
		return errors.New(usage)
	}
}

func commandFlags(command string, stderr io.Writer) (*flag.FlagSet, *string) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: slugregistry %s [options]\n", command)
		flags.PrintDefaults()
	}
	return flags, flags.String("db", "", "Local registry directory (required)")
}

func parseFlags(flags *flag.FlagSet, args []string, dbPath *string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *dbPath == "" {
		return errors.New("--db is required")
	}
	return nil
}

func writeJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func initialize(args []string, stdout, stderr io.Writer) error {
	flags, dbPath := commandFlags("init", stderr)
	network := flags.Uint64("network", 0, "Nonzero uint32 network magic (required)")
	genesis := flags.String("genesis", "", "Displayed genesis hash, 64 hex characters (required)")
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if *network == 0 || *network > math.MaxUint32 {
		return errors.New("--network must be between 1 and 4294967295")
	}
	if *genesis == "" {
		return errors.New("--genesis is required")
	}
	store, err := registry.Create(*dbPath, uint32(*network), *genesis)
	if err != nil {
		return err
	}
	if err := store.Close(); err != nil {
		return err
	}
	return writeJSON(stdout, struct {
		Initialized bool   `json:"initialized"`
		Network     uint32 `json:"network"`
		Genesis     string `json:"genesis"`
	}{true, uint32(*network), strings.ToLower(*genesis)})
}

func record(args []string, stdout, stderr io.Writer) error {
	flags, dbPath := commandFlags("record", stderr)
	contextPath := flags.String("context", "", "Trusted context JSON file (required)")
	proofPath := flags.String("proof", "", "Untrusted proof JSON file (required)")
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if *contextPath == "" || *proofPath == "" {
		return errors.New("--context and --proof are required")
	}
	data, err := readBoundedFile(*contextPath, prospect.MaxContextSize)
	if err != nil {
		return fmt.Errorf("read context: %w", err)
	}
	context, err := prospect.ParseContext(data)
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
	store, err := registry.Open(*dbPath, false)
	if err != nil {
		return err
	}
	entry, inserted, recordErr := store.Record(context, proof)
	if err := errors.Join(recordErr, store.Close()); err != nil {
		return err
	}
	return writeJSON(stdout, struct {
		Inserted bool            `json:"inserted"`
		Record   registry.Record `json:"record"`
	}{inserted, entry})
}

func read(command string, args []string, stdout, stderr io.Writer) error {
	flags, dbPath := commandFlags(command, stderr)
	var id *string
	var after *uint64
	var limit *int
	switch command {
	case "get":
		id = flags.String("id", "", "Prospect ID, 64 hex characters (required)")
	case "list":
		after = flags.Uint64("after", 0, "Return sequences strictly greater than this value")
		limit = flags.Int("limit", registry.MaxPageSize, "Maximum records to return (1-100)")
	}
	if err := parseFlags(flags, args, dbPath); err != nil {
		return err
	}
	if command == "get" && *id == "" {
		return errors.New("--id is required")
	}
	if command == "list" && (*limit < 1 || *limit > registry.MaxPageSize) {
		return fmt.Errorf("--limit must be between 1 and %d", registry.MaxPageSize)
	}
	store, err := registry.Open(*dbPath, true)
	if err != nil {
		return err
	}
	if command == "export" {
		exportErr := store.Export(stdout)
		return errors.Join(exportErr, store.Close())
	}
	var result any
	var readErr error
	if command == "get" {
		result, readErr = store.Get(*id)
	} else {
		var entries []registry.Record
		entries, readErr = store.List(*after, *limit)
		if entries == nil {
			entries = []registry.Record{}
		}
		result = entries
	}
	if err := errors.Join(readErr, store.Close()); err != nil {
		return err
	}
	return writeJSON(stdout, result)
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
