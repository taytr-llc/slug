//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionCheapCommandRefusalBeforeEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "never-created")
	out, e := os.CreateTemp(t.TempDir(), "out-")
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	errout, e := os.CreateTemp(t.TempDir(), "err-")
	if e != nil {
		t.Fatal(e)
	}
	defer errout.Close()
	args := []string{"create", "--policy-file", "/missing/policy", "--root", root, "--tls-certificate-file", "/missing/cert", "--wallet-public-passphrase-file", "/missing/public", "--wallet-unlock-passphrase-file", "/missing/private"}
	e = RunProvisionCommand(context.Background(), args, out, errout)
	if !errors.Is(e, ErrProvisionInvalid) {
		t.Fatal("cheap profile reached file selection", e)
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("cheap profile created output")
	}
}
