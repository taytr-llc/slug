package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedPoolFlagsRejectLegacyEvenFalseBeforeEffects(t *testing.T) {
	for _, legacy := range []string{"candidate-pool", "account-fixtures", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "career"} {
		for _, value := range []string{"true", "false"} {
			root := filepath.Join(t.TempDir(), "unused")
			args := []string{"--candidate-hosted-pool", "--" + legacy + "=" + value, "--data-dir", root, "--node-binary", "/not-a-node"}
			if err := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard); err == nil {
				t.Fatal("conflicting explicit mode accepted", legacy, value)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("conflicting selection touched storage", legacy, value)
			}
		}
	}
}
