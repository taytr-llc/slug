// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventFlagRequiresExplicitCandidateStack(t *testing.T) {
	for _, flags := range [][]string{
		{"--candidate-events"},
		{"--candidate-events", "--candidate-manifests"},
		{"--candidate-events", "--candidate-manifests", "--candidate-stable"},
		{"--candidate-events", "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--account-fixtures"},
	} {
		t.Run(strings.Join(flags, "/"), func(t *testing.T) {
			args := append([]string{"--data-dir", filepath.Join(t.TempDir(), "uncreated"), "--node-binary", "/nonexistent-slugd"}, flags...)
			err := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "requires") {
				t.Fatalf("dependent flags were not refused before installation: %v", err)
			}
		})
	}
}
