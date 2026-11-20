// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/fighterevidence"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestEvidenceCLIRequiresExplicitBoundedFiles(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal entry point")
	}
	for _, args := range [][]string{nil, {"verify"}, {"verify", "--file", "missing"}, {"verify", "--source-pin", "missing"}, {"verify", "--source-pin", "missing", "--file", "missing", "extra"}} {
		var out, errout bytes.Buffer
		if run(t.Context(), args, &out, &errout) == nil || out.Len() != 0 {
			t.Fatal("missing explicit input accepted")
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bounded.json")
	if e := os.WriteFile(path, []byte("abc"), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := readBounded(path, 3); e != nil || string(b) != "abc" {
		t.Fatal("exact limit", e)
	}
	if _, e := readBounded(path, 2); e == nil {
		t.Fatal("limit+1 accepted")
	}
	if _, e := readBounded(dir, 100); e == nil {
		t.Fatal("directory input")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out, errout bytes.Buffer
	if e := run(ctx, []string{"verify", "--source-pin", path, "--file", path}, &out, &errout); !errors.Is(e, fighterevidence.ErrCancelled) || out.Len() != 0 {
		t.Fatal("cancelled input read", e)
	}
}

func TestEvidenceCLIProfileBeforeFiles(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	var out, errout bytes.Buffer
	e := run(t.Context(), []string{"verify", "--source-pin", "/nonexistent/secret-pin", "--file", "/nonexistent/secret-file"}, &out, &errout)
	if !errors.Is(e, fighterevidence.ErrProfile) || out.Len() != 0 || errout.Len() != 0 {
		t.Fatal("profile refusal did not precede file access", e)
	}
}
