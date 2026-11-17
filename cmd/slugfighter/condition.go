// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/fightercondition"
)

func runConditionPreview(args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("condition", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	workHash := flags.String("work-hash", "", "public 32-byte work hash in display/RPC hex order")
	version := flags.Uint64("version", 1, "explicit condition projector version")
	born := flags.Int64("born-at-ms", 0, "supplied preview birth in Unix milliseconds; not authenticated admission")
	asOf := flags.Int64("as-of-ms", 0, "explicit preview observation time in Unix milliseconds")
	xp := flags.Int("pro-xp", 0, "supplied already-accounted professional XP, 0 through 15")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *version > math.MaxUint32 {
		return fmt.Errorf("invalid condition arguments")
	}
	seed, err := fighter.ParseSeed(*workHash)
	if err != nil {
		return err
	}
	dna, err := fighter.Generate(fighter.Version1, seed)
	if err != nil {
		return err
	}
	snapshot, err := fightercondition.Project(uint32(*version), dna, *born, *asOf, *xp)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(snapshot)
}
