// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/taytr-llc/slug/desktop/pooloperator"
)

func main() {
	// Broken stdout/stderr must return EPIPE so command-owned cleanup runs.
	// Go otherwise exits on SIGPIPE for descriptors 1 and 2.
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := pooloperator.RunCommand(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	// RunCommand already emitted a bounded closed-code diagnostic and retains
	// failed owners in err. Do not print its private underlying error tree.
	if err != nil {
		os.Exit(1)
	}
}
