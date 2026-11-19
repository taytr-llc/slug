// Copyright (c) 2026 The SLUG developers. ISC license.
// slugevidence checks explicitly supplied public evidence without any network,
// wallet, registry or source discovery.
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

	"github.com/taytr-llc/slug/fighterevidence"
	"github.com/taytr-llc/slug/gymstable"
)

const usage = "usage: slugevidence verify --source-pin PIN_FILE --file EVIDENCE_FILE"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "slugevidence:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(stdout, usage)
		return err
	}
	if len(args) == 0 || args[0] != "verify" {
		return errors.New(usage)
	}
	if err := fighterevidence.CheckProfile(); err != nil {
		return err
	}
	if ctx == nil {
		return fighterevidence.ErrInvalid
	}
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pinPath := flags.String("source-pin", "", "Independently selected exact stable pin (required)")
	filePath := flags.String("file", "", "Public evidence file (required)")
	flags.Usage = func() { fmt.Fprintln(stderr, usage); flags.PrintDefaults() }
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *pinPath == "" || *filePath == "" {
		return errors.New(usage)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(fighterevidence.ErrCancelled, err)
	}
	input, err := readBounded(*pinPath, fighterevidence.MaxPinSize)
	if err != nil {
		return errors.New("cannot read bounded source pin")
	}
	pin, err := gymstable.ParsePin(input)
	if err != nil {
		return fighterevidence.ErrSourceMismatch
	}
	input, err = readBounded(*filePath, fighterevidence.MaxSize)
	if err != nil {
		return errors.New("cannot read bounded evidence file")
	}
	report, err := fighterevidence.VerifySupplied(ctx, pin, input)
	if err != nil {
		return err
	}
	// stdout is a single machine-readable report; human claims and limitations
	// use stderr. No evidence contents or file paths are printed.
	if err = json.NewEncoder(stdout).Encode(report); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stderr, fighterevidence.SuccessHeadline+"\n"+fighterevidence.Limitations)
	return err
}

func readBounded(path string, limit int) (raw []byte, resultErr error) {
	// Open first without waiting for a special-file peer, then inspect that
	// exact descriptor. A pathname stat followed by Open can race a FIFO swap.
	f, err := openInput(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, f.Close())
		if resultErr != nil {
			raw = nil
		}
	}()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, fighterevidence.ErrInvalid
	}
	raw, err = io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fighterevidence.ErrInvalid
	}
	return raw, nil
}
