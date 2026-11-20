package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedRecoveryFlagsRejectOtherModesBeforeEffects(t *testing.T) {
	for _, legacy := range []string{"candidate-hosted-pool", "candidate-pool", "account-fixtures", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "career"} {
		for _, value := range []string{"true", "false"} {
			root := filepath.Join(t.TempDir(), "unused")
			args := []string{"--candidate-hosted-recovery", "--" + legacy + "=" + value, "--data-dir", root, "--node-binary", "/not-a-node"}
			if err := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard); err == nil {
				t.Fatal("conflicting mode accepted", legacy, value)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("rejected selection touched storage", legacy, value)
			}
		}
	}
}
