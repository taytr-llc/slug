// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// slugfighter previews versioned fighter DNA offline from a supplied work hash.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/taytr-llc/slug/fighter"
)

const usage = "usage: slugfighter [-dna-version=1] <64-hex-work-hash>"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "slugfighter:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "preview" {
		return runManifestPreview(args, stdout, stderr)
	}
	flags := flag.NewFlagSet("slugfighter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.Uint64("dna-version", 1, "DNA derivation version (currently 1)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, usage)
		fmt.Fprintln(stderr, "Preview DNA offline; does not validate qualifying work or grant ownership.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("exactly one 64-hex work hash is required")
	}
	if *version > math.MaxUint32 {
		return errors.New("dna-version exceeds uint32 range")
	}
	seed, err := fighter.ParseSeed(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("invalid work hash: %w", err)
	}
	dna, err := fighter.Generate(uint32(*version), seed)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(dna)
}
