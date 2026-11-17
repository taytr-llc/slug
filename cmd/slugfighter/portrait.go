// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
)

func runPortraitPreview(args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("portrait", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	workHash := flags.String("work-hash", "", "public 32-byte work hash in display/RPC hex order")
	version := flags.Uint64("version", 1, "explicit portrait renderer version")
	digestOnly := flags.Bool("digest", false, "print only the preview SVG digest")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *version > math.MaxUint32 {
		return fmt.Errorf("invalid portrait arguments")
	}
	// Renderer v1 has an explicit compatibility mapping to manifest v1.
	manifest, err := fightermanifest.GenerateText(fightermanifest.Version1, *workHash)
	if err != nil {
		return err
	}
	if *digestOnly {
		digest, err := fighterportrait.Digest(uint32(*version), manifest)
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, digest+"\n")
		return err
	}
	raw, err := fighterportrait.Render(uint32(*version), manifest)
	if err != nil {
		return err
	}
	_, err = output.Write(raw)
	return err // Exact SVG bytes, without a newline.
}
