// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedFlagsRejectOtherModesBeforeEffects(t *testing.T) {
	for _, mode := range []string{"candidate-hosted-recovery", "candidate-hosted-pool", "candidate-pool", "account-fixtures", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "career"} {
		for _, value := range []string{"true", "false"} {
			root := filepath.Join(t.TempDir(), "unused")
			args := []string{"--candidate-owned-gym", "--" + mode + "=" + value, "--data-dir", root, "--node-binary", "/not-a-node"}
			e := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
			if e == nil || !strings.Contains(e.Error(), "separate explicit true") {
				t.Fatal("conflicting selector did not refuse before dispatch", mode, value, e)
			}
			if _, e = os.Lstat(root); !os.IsNotExist(e) {
				t.Fatal("conflicting mode touched storage", mode, value, e)
			}
		}
	}
}
func TestOwnedFlagsRequireOwnedLifetimeAndExplicitSelection(t *testing.T) {
	for _, flag := range []string{"--candidate-owned-gym", "--candidate-owned-gym=true", "--candidate-owned-gym=false"} {
		root := filepath.Join(t.TempDir(), "unused")
		e := run(context.Background(), []string{flag, "--data-dir", root, "--node-binary", "/not-a-node"}, strings.NewReader(""), io.Discard, io.Discard)
		want := "owned input must be closeable"
		if strings.HasSuffix(flag, "=false") {
			want = "separate explicit true"
		}
		if e == nil || !strings.Contains(e.Error(), want) {
			t.Fatal("selector/stream lifetime refusal", flag, e)
		}
		if _, e = os.Lstat(root); !os.IsNotExist(e) {
			t.Fatal("invalid lifetime touched storage", flag, e)
		}
	}
}
