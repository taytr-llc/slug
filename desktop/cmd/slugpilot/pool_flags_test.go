package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPoolModeRejectsLegacyFlagsBeforeEffects(t *testing.T) {
	for _, old := range []string{"--account-fixtures", "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--candidate-results", "--career"} {
		t.Run(old, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "absent")
			e := run(context.Background(), []string{"--candidate-pool", old, "--data-dir", root, "--node-binary", "/not-a-node"}, strings.NewReader(""), io.Discard, io.Discard)
			if e == nil {
				t.Fatal("mixed mode accepted")
			}
			if _, e = os.Stat(root); !os.IsNotExist(e) {
				t.Fatal("invalid mode changed disk")
			}
		})
	}
}
