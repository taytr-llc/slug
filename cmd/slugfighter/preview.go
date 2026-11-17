// Copyright (c) 2026 The SLUG developers. ISC license.

// slugfighter prints a deterministic preview from a public work hash. It does
// not validate qualifying work, create a fighter ID or change a registry.
package main

import (
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/taytr-llc/slug/fightermanifest"
)

func runManifestPreview(args []string, output, diagnostics io.Writer) error {
	if len(args) == 0 || args[0] != "preview" {
		return fmt.Errorf("usage: slugfighter preview --work-hash HEX [--version 1] [--digest]")
	}
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	workHash := flags.String("work-hash", "", "public 32-byte work hash in display/RPC hex order")
	version := flags.Uint64("version", 1, "explicit manifest version")
	digestOnly := flags.Bool("digest", false, "print only the preview manifest digest")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *version > math.MaxUint32 {
		return fmt.Errorf("invalid preview arguments")
	}
	manifest, err := fightermanifest.GenerateText(uint32(*version), *workHash)
	if err != nil {
		return err
	}
	if *digestOnly {
		digest, err := fightermanifest.Digest(manifest)
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, digest+"\n")
		return err
	}
	raw, err := fightermanifest.Marshal(manifest)
	if err != nil {
		return err
	}
	_, err = output.Write(raw)
	return err // Exact canonical bytes, without a newline.
}
