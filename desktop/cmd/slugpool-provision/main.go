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
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := pooloperator.RunProvisionCommand(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	// The command already emitted its bounded, closed-code diagnostic. Never
	// print private underlying errors or retry a retained failed cleanup owner.
	if err != nil {
		os.Exit(1)
	}
}
